package mesh

import (
	"context"
	"fmt"
	"log"
	"sync"

	"github.com/GoCodeAlone/workflow-plugin-agent/executor"
	"github.com/GoCodeAlone/workflow-plugin-agent/provider"
)

// TeamHandle is returned by SpawnTeam and allows callers to wait for
// completion, observe events, or cancel the team.
type TeamHandle struct {
	ID     string
	Done   <-chan TeamResult
	Events <-chan Event
	Cancel func()
}

// TeamResult is the final outcome of a team execution.
type TeamResult struct {
	Status    string
	Artifacts map[string]Entry
	Errors    []error
}

// Event is a real-time observation emitted during team execution.
type Event struct {
	Type    string // "agent_spawned", "agent_message", "tool_call", "text", "complete", "error"
	AgentID string
	Content string
	Data    map[string]any
}

// AgentMesh is the top-level orchestrator. It manages a blackboard, a router,
// a node registry, and team configuration.
type AgentMesh struct {
	mu     sync.RWMutex
	nodes  map[string]Node
	teams  map[string]*TeamHandle
	nextID int
}

// NewAgentMesh creates a new AgentMesh ready to spawn teams.
func NewAgentMesh() *AgentMesh {
	return &AgentMesh{
		nodes: make(map[string]Node),
		teams: make(map[string]*TeamHandle),
	}
}

// SpawnTeam initialises a team of agents from the given configs and starts
// them working on the supplied task. It returns a TeamHandle that the caller
// can use to observe progress and collect the final result.
func (m *AgentMesh) SpawnTeam(
	ctx context.Context,
	task string,
	configs []NodeConfig,
	providerFactory func(NodeConfig) provider.Provider,
) (*TeamHandle, error) {
	if len(configs) == 0 {
		return nil, fmt.Errorf("at least one node config is required")
	}

	// 1. Generate a team ID.
	m.mu.Lock()
	m.nextID++
	teamID := fmt.Sprintf("team-%d", m.nextID)
	m.mu.Unlock()

	// 2. Initialise shared state.
	bb := NewBlackboard()
	for _, section := range []string{"plan", "code", "reviews", "status", "artifacts"} {
		bb.Write(section, "_init", "empty", "mesh")
	}
	router := NewRouter()

	// 3. Create nodes.
	nodes := make([]Node, 0, len(configs))
	eventCh := make(chan Event, 256)
	doneCh := make(chan TeamResult, 1)

	teamCtx, cancelTeam := context.WithCancel(ctx)

	for _, cfg := range configs {
		var node Node
		if cfg.Location != "" && cfg.Location != "local" {
			node = NewRemoteNode(cfg.Name, cfg.Location, NodeInfo{
				Name:     cfg.Name,
				Role:     cfg.Role,
				Model:    cfg.Model,
				Provider: cfg.Provider,
			})
		} else {
			prov := providerFactory(cfg)
			node = NewLocalNode(cfg, prov, nil)
			// Set the event forwarder now that we have the real node ID.
			node.(*LocalNode).onEvent = makeEventForwarder(eventCh, node.ID())
		}
		nodes = append(nodes, node)
	}

	// 4. Register each node with the router and set up outbox wiring.
	type nodeWiring struct {
		node  Node
		inbox <-chan Message
	}
	wiring := make([]nodeWiring, 0, len(nodes))

	for _, node := range nodes {
		inbox, err := router.Register(node.ID())
		if err != nil {
			cancelTeam()
			return nil, fmt.Errorf("registering node %s: %w", node.ID(), err)
		}
		wiring = append(wiring, nodeWiring{node: node, inbox: inbox})

		m.mu.Lock()
		m.nodes[node.ID()] = node
		m.mu.Unlock()

		// Emit agent_spawned event.
		eventCh <- Event{
			Type:    "agent_spawned",
			AgentID: node.ID(),
			Content: fmt.Sprintf("node %s (%s) spawned", node.Info().Name, node.Info().Role),
		}
	}

	// 5. Start each node in its own goroutine.
	var wg sync.WaitGroup
	errMu := sync.Mutex{}
	var runErrors []error

	for _, w := range wiring {
		wg.Add(1)
		go func(nd Node, inbox <-chan Message) {
			defer wg.Done()

			outbox := make(chan Message, 64)

			// Wire outbox → router. Wait for this goroutine to finish
			// before signalling wg.Done so the watcher does not close
			// eventCh while messages are still being forwarded.
			var outboxWg sync.WaitGroup
			outboxWg.Add(1)
			go func() {
				defer outboxWg.Done()
				for msg := range outbox {
					if err := router.Send(msg); err != nil {
						log.Printf("mesh: router send error for %s: %v", nd.ID(), err)
					}
					select {
					case eventCh <- Event{
						Type:    "agent_message",
						AgentID: nd.ID(),
						Content: msg.Content,
						Data:    map[string]any{"to": msg.To, "type": msg.Type},
					}:
					default:
					}
				}
			}()

			err := nd.Run(teamCtx, task, bb, inbox, outbox)
			close(outbox)
			outboxWg.Wait()

			if err != nil {
				errMu.Lock()
				runErrors = append(runErrors, fmt.Errorf("node %s: %w", nd.ID(), err))
				errMu.Unlock()

				select {
				case eventCh <- Event{
					Type:    "error",
					AgentID: nd.ID(),
					Content: err.Error(),
				}:
				default:
				}
			} else {
				select {
				case eventCh <- Event{
					Type:    "complete",
					AgentID: nd.ID(),
					Content: "node finished",
				}:
				default:
				}
			}
		}(w.node, w.inbox)
	}

	// 6. Watcher goroutine: monitor the "status" section for all-done.
	go func() {
		wg.Wait()

		// Collect artifacts.
		artifacts := bb.List("artifacts")
		if artifacts == nil {
			artifacts = make(map[string]Entry)
		}

		status := "completed"
		errMu.Lock()
		errs := make([]error, len(runErrors))
		copy(errs, runErrors)
		errMu.Unlock()

		if len(errs) > 0 {
			status = "completed_with_errors"
		}

		doneCh <- TeamResult{
			Status:    status,
			Artifacts: artifacts,
			Errors:    errs,
		}
		close(doneCh)
		close(eventCh)

		// Clean up router registrations.
		for _, nd := range nodes {
			router.Unregister(nd.ID())
		}
	}()

	handle := &TeamHandle{
		ID:     teamID,
		Done:   doneCh,
		Events: eventCh,
		Cancel: cancelTeam,
	}

	m.mu.Lock()
	m.teams[teamID] = handle
	m.mu.Unlock()

	return handle, nil
}

// makeEventForwarder returns an executor.Event callback that converts events
// into mesh Events and sends them on the channel.
func makeEventForwarder(ch chan<- Event, agentID string) func(executor.Event) {
	return func(e executor.Event) {
		var evType string
		switch e.Type {
		case executor.EventToolCallStart:
			evType = "tool_call"
		case executor.EventText:
			evType = "text"
		case executor.EventCompleted:
			evType = "complete"
		case executor.EventFailed:
			evType = "error"
		default:
			evType = string(e.Type)
		}

		select {
		case ch <- Event{
			Type:    evType,
			AgentID: agentID,
			Content: e.Content,
			Data: map[string]any{
				"tool_name": e.ToolName,
				"iteration": e.Iteration,
			},
		}:
		default:
			// Event channel full — drop to avoid blocking the executor.
		}
	}
}
