package daemon

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/GoCodeAlone/ratchet-cli/internal/mesh"
	"github.com/GoCodeAlone/workflow-plugin-agent/executor"
	gkprov "github.com/GoCodeAlone/workflow-plugin-agent/genkit"
	"github.com/GoCodeAlone/workflow-plugin-agent/provider"

	"github.com/GoCodeAlone/ratchet-cli/internal/hooks"
	pb "github.com/GoCodeAlone/ratchet-cli/internal/proto"
)

// teamAgent is an in-memory agent within a team.
type teamAgent struct {
	mu          sync.RWMutex
	id          string
	name        string
	role        string
	model       string
	provider    string
	status      string // running, completed, failed
	currentTask string
	messages    []agentMsg
}

type agentMsg struct {
	from    string
	content string
	ts      time.Time
}

// teamInstance tracks a running team.
type teamInstance struct {
	mu          sync.RWMutex
	id          string
	task        string
	agents      map[string]*teamAgent
	status      string // running, completed, failed
	cancel      context.CancelFunc
	eventCh     chan *pb.TeamEvent
	completedAt time.Time
}

// TeamManager manages team instances.
type TeamManager struct {
	mu    sync.RWMutex
	teams map[string]*teamInstance
	engine *EngineContext
	hooks  *hooks.HookConfig
	stop  chan struct{}
	mesh  *mesh.AgentMesh
}

// NewTeamManager returns an initialized TeamManager.
func NewTeamManager(engine *EngineContext, hks *hooks.HookConfig) *TeamManager {
	tm := &TeamManager{
		teams:  make(map[string]*teamInstance),
		engine: engine,
		hooks:  hks,
		stop:   make(chan struct{}),
		mesh:  mesh.NewAgentMesh(),
	}
	go tm.cleanupLoop()
	return tm
}

// cleanupLoop periodically removes teams that completed more than 10 minutes ago.
func (tm *TeamManager) cleanupLoop() {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("teamManager cleanupLoop: panic: %v", r)
		}
	}()
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-tm.stop:
			return
		case <-ticker.C:
			tm.purgeCompleted(10 * time.Minute)
		}
	}
}

// purgeCompleted removes teams that completed more than ttl ago.
func (tm *TeamManager) purgeCompleted(ttl time.Duration) {
	now := time.Now()
	tm.mu.Lock()
	defer tm.mu.Unlock()
	for id, ti := range tm.teams {
		ti.mu.RLock()
		completed := ti.completedAt
		ti.mu.RUnlock()
		if !completed.IsZero() && now.Sub(completed) > ttl {
			delete(tm.teams, id)
		}
	}
}

// StartTeam creates a team, spawns default agents, and returns the team ID.
// Events are sent on the returned channel; it is closed when the team finishes.
// If req.TeamConfigName is set, the team is launched via the mesh orchestrator.
func (tm *TeamManager) StartTeam(ctx context.Context, req *pb.StartTeamReq) (string, <-chan *pb.TeamEvent) {
	if req.TeamConfigName != "" {
		return tm.startMeshTeamFromConfig(ctx, req)
	}

	teamID := uuid.New().String()
	runCtx, cancel := context.WithCancel(ctx)

	ti := &teamInstance{
		id:      teamID,
		task:    req.Task,
		agents:  make(map[string]*teamAgent),
		status:  "running",
		cancel:  cancel,
		eventCh: make(chan *pb.TeamEvent, 64),
	}

	tm.mu.Lock()
	tm.teams[teamID] = ti
	tm.mu.Unlock()

	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("team %s: panic: %v", teamID, r)
			}
		}()
		tm.run(runCtx, ti, req)
	}()
	return teamID, ti.eventCh
}

// startMeshTeamFromConfig loads a team config by name and launches a mesh team.
func (tm *TeamManager) startMeshTeamFromConfig(ctx context.Context, req *pb.StartTeamReq) (string, <-chan *pb.TeamEvent) {
	errCh := func(msg string) (string, <-chan *pb.TeamEvent) {
		ch := make(chan *pb.TeamEvent, 1)
		ch <- &pb.TeamEvent{Event: &pb.TeamEvent_Error{Error: &pb.ErrorEvent{Message: msg}}}
		close(ch)
		return "", ch
	}

	// Resolve the team config.
	var tc *mesh.TeamConfig
	builtins, err := mesh.BuiltinTeamConfigs()
	if err == nil {
		if bc, ok := builtins[req.TeamConfigName]; ok {
			tc = bc
		}
	}
	if tc == nil {
		loaded, err := mesh.LoadTeamConfig(req.TeamConfigName)
		if err != nil {
			return errCh(fmt.Sprintf("load team config %q: %v", req.TeamConfigName, err))
		}
		tc = loaded
	}

	configs := mesh.ToNodeConfigs(tc)

	// Build a provider factory that resolves providers via the daemon's
	// ProviderRegistry when available, honouring the per-agent provider/model
	// settings from the team YAML. Falls back to an Ollama provider using the
	// agent's model when no registry is configured (e.g., in tests).
	providerFactory := func(cfg mesh.NodeConfig) provider.Provider {
		if tm.engine != nil && tm.engine.ProviderRegistry != nil {
			var prov provider.Provider
			var provErr error
			if cfg.Provider != "" {
				prov, provErr = tm.engine.ProviderRegistry.GetByAlias(ctx, cfg.Provider)
			}
			if prov == nil || provErr != nil {
				prov, provErr = tm.engine.ProviderRegistry.GetDefault(ctx)
			}
			if prov != nil && provErr == nil {
				return prov
			}
		}
		// Fallback: create a local Ollama provider using the agent's model.
		prov, err := gkprov.NewOllamaProvider(ctx, cfg.Model, "", 0)
		if err != nil {
			log.Printf("team: fallback ollama provider: %v", err)
			return nil
		}
		return prov
	}

	return tm.StartMeshTeam(ctx, req.Task, configs, providerFactory)
}

// executeAgent runs a single team agent via the real executor loop.
// orchestratorContext is the output from a prior orchestrator run; empty for the orchestrator itself.
func (tm *TeamManager) executeAgent(ctx context.Context, ag *teamAgent, task, orchestratorContext string) (string, error) {
	if tm.engine == nil || tm.engine.ProviderRegistry == nil {
		// No engine configured; return a stub so team lifecycle proceeds normally.
		ag.mu.RLock()
		name := ag.name
		ag.mu.RUnlock()
		return fmt.Sprintf("%s completed: %s", name, task), nil
	}

	ag.mu.RLock()
	provAlias := ag.provider
	agName := ag.name
	role := ag.role
	ag.mu.RUnlock()

	var prov provider.Provider
	var err error
	if provAlias != "" {
		prov, err = tm.engine.ProviderRegistry.GetByAlias(ctx, provAlias)
	} else {
		prov, err = tm.engine.ProviderRegistry.GetDefault(ctx)
	}
	if err != nil {
		return "", fmt.Errorf("resolve provider: %w", err)
	}

	var systemPrompt string
	switch role {
	case "orchestrator":
		systemPrompt = fmt.Sprintf("You are %s, the orchestrator agent. Decompose the task into subtasks and delegate to workers with clear instructions.", agName)
	default:
		systemPrompt = fmt.Sprintf("You are %s, a worker agent. Complete the assigned subtask thoroughly and report results.", agName)
	}

	userMsg := task
	if orchestratorContext != "" {
		userMsg = task + "\n\nContext from orchestrator:\n" + orchestratorContext
	}

	ag.mu.Lock()
	ag.currentTask = task
	ag.mu.Unlock()

	result, err := executor.Execute(ctx, executor.Config{
		Provider:      prov,
		MaxIterations: 15,
	}, systemPrompt, userMsg, ag.id)
	if err != nil {
		return "", err
	}
	if result.Status == "failed" {
		return "", fmt.Errorf("agent %s failed: %s", agName, result.Error)
	}
	return result.Content, nil
}

// run is the main team goroutine. It executes agents via the real executor loop.
func (tm *TeamManager) run(ctx context.Context, ti *teamInstance, req *pb.StartTeamReq) {
	defer close(ti.eventCh)

	// Default agent roster when none specified: orchestrator + worker.
	specs := []struct{ name, role, model, provider string }{
		{"orchestrator", "orchestrator", "", req.OrchestratorProvider},
		{"worker-1", "worker", "", ""},
	}

	// Spawn agents.
	for _, spec := range specs {
		ag := &teamAgent{
			id:       uuid.New().String(),
			name:     spec.name,
			role:     spec.role,
			model:    spec.model,
			provider: spec.provider,
			status:   "running",
		}
		ti.mu.Lock()
		ti.agents[ag.id] = ag
		ti.mu.Unlock()

		ti.eventCh <- &pb.TeamEvent{
			Event: &pb.TeamEvent_AgentSpawned{
				AgentSpawned: &pb.AgentSpawned{
					AgentId:   ag.id,
					AgentName: ag.name,
					Role:      ag.role,
				},
			},
		}
		if tm.hooks != nil {
			_ = tm.hooks.Run(hooks.OnAgentSpawn, map[string]string{"agent_name": ag.name, "agent_role": ag.role})
		}
	}

	select {
	case <-ctx.Done():
		tm.markDone(ti, "failed")
		return
	default:
	}

	orch := tm.agentByRole(ti, "orchestrator")
	worker := tm.agentByRole(ti, "worker")

	// Execute orchestrator.
	var orchResult string
	if orch != nil {
		result, err := tm.executeAgent(ctx, orch, req.Task, "")
		if err != nil {
			log.Printf("team %s: orchestrator failed: %v", ti.id, err)
			orch.mu.Lock()
			orch.status = "failed"
			orch.mu.Unlock()
		} else {
			orchResult = result
			orch.mu.Lock()
			orch.status = "completed"
			orch.mu.Unlock()
			if tm.hooks != nil {
				_ = tm.hooks.Run(hooks.OnAgentComplete, map[string]string{"agent_name": orch.name})
			}
			if worker != nil {
				tm.routeMessage(ti, orch.name, worker.name, orchResult)
			}
		}
	}

	// Execute worker with orchestrator's output as context.
	var workerResult string
	if worker != nil {
		result, err := tm.executeAgent(ctx, worker, req.Task, orchResult)
		if err != nil {
			log.Printf("team %s: worker failed: %v", ti.id, err)
			worker.mu.Lock()
			worker.status = "failed"
			worker.mu.Unlock()
		} else {
			workerResult = result
			worker.mu.Lock()
			worker.status = "completed"
			worker.mu.Unlock()
			if tm.hooks != nil {
				_ = tm.hooks.Run(hooks.OnAgentComplete, map[string]string{"agent_name": worker.name})
			}
			if orch != nil {
				tm.routeMessage(ti, worker.name, orch.name, workerResult)
			}
		}
	}

	// Determine overall team status based on agent outcomes.
	teamStatus := "completed"
	summary := fmt.Sprintf("Team completed task: %s", req.Task)

	ti.mu.RLock()
	for _, ag := range ti.agents {
		ag.mu.RLock()
		s := ag.status
		ag.mu.RUnlock()
		if s == "failed" {
			teamStatus = "failed"
			break
		}
	}
	ti.mu.RUnlock()

	if teamStatus == "failed" {
		summary = fmt.Sprintf("Team failed task: %s", req.Task)
	} else if workerResult != "" {
		summary = workerResult
	}

	ti.eventCh <- &pb.TeamEvent{
		Event: &pb.TeamEvent_Complete{
			Complete: &pb.SessionComplete{
				Summary: summary,
			},
		},
	}

	tm.markDone(ti, teamStatus)
}

func (tm *TeamManager) agentByRole(ti *teamInstance, role string) *teamAgent {
	ti.mu.RLock()
	defer ti.mu.RUnlock()
	for _, ag := range ti.agents {
		if ag.role == role {
			return ag
		}
	}
	return nil
}

func (tm *TeamManager) routeMessage(ti *teamInstance, from, to, content string) {
	ti.mu.Lock()
	for _, ag := range ti.agents {
		if ag.name == from {
			ag.messages = append(ag.messages, agentMsg{from: from, content: content, ts: time.Now()})
			break
		}
	}
	ti.mu.Unlock()

	ti.eventCh <- &pb.TeamEvent{
		Event: &pb.TeamEvent_AgentMessage{
			AgentMessage: &pb.AgentMessage{
				FromAgent: from,
				ToAgent:   to,
				Content:   content,
			},
		},
	}
}

func (tm *TeamManager) markDone(ti *teamInstance, s string) {
	ti.mu.Lock()
	ti.status = s
	ti.completedAt = time.Now()
	ti.mu.Unlock()
}

// GetStatus returns the current TeamStatus for a given team ID.
func (tm *TeamManager) GetStatus(teamID string) (*pb.TeamStatus, error) {
	tm.mu.RLock()
	ti, ok := tm.teams[teamID]
	tm.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("team %s not found", teamID)
	}

	ti.mu.RLock()
	defer ti.mu.RUnlock()

	var agents []*pb.Agent
	for _, ag := range ti.agents {
		ag.mu.RLock()
		agents = append(agents, &pb.Agent{
			Id:          ag.id,
			Name:        ag.name,
			Role:        ag.role,
			Model:       ag.model,
			Provider:    ag.provider,
			Status:      ag.status,
			CurrentTask: ag.currentTask,
		})
		ag.mu.RUnlock()
	}

	return &pb.TeamStatus{
		TeamId: teamID,
		Task:   ti.task,
		Agents: agents,
		Status: ti.status,
	}, nil
}

// KillAgent cancels the team that owns the given agent (team-level cancel).
func (tm *TeamManager) KillAgent(teamID string) error {
	tm.mu.RLock()
	ti, ok := tm.teams[teamID]
	tm.mu.RUnlock()
	if !ok {
		return fmt.Errorf("team %s not found", teamID)
	}
	ti.cancel()
	return nil
}

// StartMeshTeam creates a team via the mesh orchestrator, converts mesh Events
// to pb.TeamEvents, and returns a channel of events.
func (tm *TeamManager) StartMeshTeam(
	ctx context.Context,
	task string,
	configs []mesh.NodeConfig,
	providerFactory func(mesh.NodeConfig) provider.Provider,
) (string, <-chan *pb.TeamEvent) {
	handle, err := tm.mesh.SpawnTeam(ctx, task, configs, providerFactory)
	if err != nil {
		ch := make(chan *pb.TeamEvent, 1)
		ch <- &pb.TeamEvent{
			Event: &pb.TeamEvent_Error{
				Error: &pb.ErrorEvent{
					Message: fmt.Sprintf("spawn team: %v", err),
				},
			},
		}
		close(ch)
		return "", ch
	}

	teamID := handle.ID
	eventCh := make(chan *pb.TeamEvent, 64)

	// Track as a teamInstance so GetStatus works for mesh teams too.
	ti := &teamInstance{
		id:      teamID,
		task:    task,
		agents:  make(map[string]*teamAgent),
		status:  "running",
		cancel:  handle.Cancel,
		eventCh: eventCh,
	}
	tm.mu.Lock()
	tm.teams[teamID] = ti
	tm.mu.Unlock()

	// Convert mesh events to pb events in a goroutine.
	go func() {
		defer close(eventCh)

		for ev := range handle.Events {
			var pbEv *pb.TeamEvent
			switch ev.Type {
			case "agent_spawned":
				agentID := ev.AgentID
				agentName := ev.AgentID
				role := ""
				if ev.Data != nil {
					if n, ok := ev.Data["name"].(string); ok {
						agentName = n
					}
					if r, ok := ev.Data["role"].(string); ok {
						role = r
					}
				}

				ti.mu.Lock()
				ti.agents[agentID] = &teamAgent{
					id:     agentID,
					name:   agentName,
					role:   role,
					status: "running",
				}
				ti.mu.Unlock()

				pbEv = &pb.TeamEvent{
					Event: &pb.TeamEvent_AgentSpawned{
						AgentSpawned: &pb.AgentSpawned{
							AgentId:   agentID,
							AgentName: agentName,
							Role:      role,
						},
					},
				}
			case "agent_message":
				toAgent := ""
				if ev.Data != nil {
					if t, ok := ev.Data["to"].(string); ok {
						toAgent = t
					}
				}
				pbEv = &pb.TeamEvent{
					Event: &pb.TeamEvent_AgentMessage{
						AgentMessage: &pb.AgentMessage{
							FromAgent: ev.AgentID,
							ToAgent:   toAgent,
							Content:   ev.Content,
						},
					},
				}
			case "text":
				pbEv = &pb.TeamEvent{
					Event: &pb.TeamEvent_Token{
						Token: &pb.TokenDelta{
							Content: ev.Content,
						},
					},
				}
			case "error":
				pbEv = &pb.TeamEvent{
					Event: &pb.TeamEvent_Error{
						Error: &pb.ErrorEvent{
							Message: ev.Content,
						},
					},
				}
			case "complete":
				// Individual agent complete — track status.
				ti.mu.Lock()
				if ag, ok := ti.agents[ev.AgentID]; ok {
					ag.mu.Lock()
					ag.status = "completed"
					ag.mu.Unlock()
				}
				ti.mu.Unlock()
			}

			if pbEv != nil {
				select {
				case eventCh <- pbEv:
				case <-ctx.Done():
					return
				}
			}
		}

		// Retrieve the final result (Done is already closed at this point
		// because handle.Events is closed by the same goroutine that closes
		// doneCh, so Result() is safe to call without waiting again).
		result := handle.Result()

		summary := fmt.Sprintf("Team completed task: %s (status: %s)", task, result.Status)
		if len(result.Errors) > 0 {
			summary += fmt.Sprintf(" [%d errors]", len(result.Errors))
		}

		select {
		case eventCh <- &pb.TeamEvent{
			Event: &pb.TeamEvent_Complete{
				Complete: &pb.SessionComplete{
					Summary: summary,
				},
			},
		}:
		case <-ctx.Done():
		}

		tm.markDone(ti, result.Status)
	}()

	return teamID, eventCh
}
