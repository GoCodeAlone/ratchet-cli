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
//
// Done is closed when the team finishes. Call Result() after Done closes to
// retrieve the final outcome. Using a close-only signal allows multiple
// independent consumers (e.g., a status tracker and an event bridge) to wait
// without racing for a single value.
type TeamHandle struct {
	ID     string
	Done   <-chan struct{}
	Events <-chan Event
	Cancel func()

	mu     sync.RWMutex
	result TeamResult
}

// Result returns the final outcome recorded for the team.
// Callers should wait for Done to be closed before calling Result.
func (h *TeamHandle) Result() TeamResult {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.result
}

func (h *TeamHandle) setResult(result TeamResult) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.result = result
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
	for _, cfg := range configs {
		if (cfg.Location == "" || cfg.Location == "local") && providerFactory == nil {
			return nil, fmt.Errorf("providerFactory is required for local node configs")
		}
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
	doneCh := make(chan struct{})

	teamCtx, cancelTeam := context.WithCancel(ctx)

	// Create the handle early so the watcher goroutine can call setResult.
	handle := &TeamHandle{
		ID:     teamID,
		Done:   doneCh,
		Events: eventCh,
		Cancel: cancelTeam,
	}

	m.mu.Lock()
	m.teams[teamID] = handle
	m.mu.Unlock()

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
			if prov == nil {
				return nil, fmt.Errorf("no provider available for agent %q (provider=%q model=%q) — check your provider configuration", cfg.Name, cfg.Provider, cfg.Model)
			}
			node = NewLocalNode(cfg, prov, nil)
			// Set the event forwarder now that we have the real node ID.
			node.(*LocalNode).onEvent = makeEventForwarder(eventCh, node.ID())
		}
		nodes = append(nodes, node)
	}

	// mergeInboxes fans in multiple inbound channels into a single buffered
	// channel. The merged channel is buffered (inboxBuffer capacity) to avoid
	// blocking forwarder goroutines when the consumer is slow. Each forwarder
	// also listens on teamCtx.Done() so it exits immediately when the team is
	// cancelled, preventing goroutine leaks.
	mergeInboxes := func(chans ...<-chan Message) <-chan Message {
		switch len(chans) {
		case 0:
			return nil
		case 1:
			return chans[0]
		}
		merged := make(chan Message, inboxBuffer)
		var wg sync.WaitGroup
		wg.Add(len(chans))
		for _, ch := range chans {
			go func(c <-chan Message) {
				defer wg.Done()
				for {
					select {
					case <-teamCtx.Done():
						return
					case msg, ok := <-c:
						if !ok {
							return
						}
						select {
						case merged <- msg:
						case <-teamCtx.Done():
							return
						}
					}
				}
			}(ch)
		}
		go func() {
			wg.Wait()
			close(merged)
		}()
		return merged
	}

	// 4. Register each node with the router and set up outbox wiring.
	type nodeWiring struct {
		node  Node
		inbox <-chan Message
	}
	wiring := make([]nodeWiring, 0, len(nodes))

	for _, node := range nodes {
		inboxes := make([]<-chan Message, 0, 2)

		idInbox, err := router.Register(node.ID())
		if err != nil {
			cancelTeam()
			return nil, fmt.Errorf("registering node %s: %w", node.ID(), err)
		}
		inboxes = append(inboxes, idInbox)

		name := node.Info().Name
		if name != "" && name != node.ID() {
			nameInbox, err := router.Register(name)
			if err != nil {
				cancelTeam()
				return nil, fmt.Errorf("registering node alias %s for %s: %w", name, node.ID(), err)
			}
			inboxes = append(inboxes, nameInbox)
		}

		wiring = append(wiring, nodeWiring{node: node, inbox: mergeInboxes(inboxes...)})

		m.mu.Lock()
		m.nodes[node.ID()] = node
		if name != "" {
			m.nodes[name] = node
		}
		m.mu.Unlock()

		// Emit agent_spawned event with name/role in Data so downstream
		// consumers can populate agent metadata without parsing Content.
		eventCh <- Event{
			Type:    "agent_spawned",
			AgentID: node.ID(),
			Content: fmt.Sprintf("node %s (%s) spawned", node.Info().Name, node.Info().Role),
			Data: map[string]any{
				"name": node.Info().Name,
				"role": node.Info().Role,
			},
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

	// 6. Watcher goroutine: wait for all node goroutines to exit.
	// Nodes decide when to stop via their own ShouldStop checks
	// (blackboard status write) or context cancellation.
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

		handle.setResult(TeamResult{
			Status:    status,
			Artifacts: artifacts,
			Errors:    errs,
		})
		close(doneCh)
		close(eventCh)

		// Clean up router registrations (both ID and name aliases).
		for _, nd := range nodes {
			router.Unregister(nd.ID())
			if name := nd.Info().Name; name != "" && name != nd.ID() {
				router.Unregister(name)
			}
		}

		// Remove completed team and its node references from the mesh
		// registry to prevent unbounded growth across multiple SpawnTeam calls.
		m.mu.Lock()
		delete(m.teams, teamID)
		for _, nd := range nodes {
			delete(m.nodes, nd.ID())
			if name := nd.Info().Name; name != "" {
				delete(m.nodes, name)
			}
		}
		m.mu.Unlock()
	}()

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
		case executor.EventThinking:
			evType = "thinking"
		case executor.EventCompleted:
			evType = "complete"
		case executor.EventFailed:
			evType = "error"
		default:
			evType = string(e.Type)
		}

		content := e.Content
		if content == "" && e.Error != "" {
			content = e.Error
		}

		select {
		case ch <- Event{
			Type:    evType,
			AgentID: agentID,
			Content: content,
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
