package daemon

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/GoCodeAlone/workflow-plugin-agent/executor"
	"github.com/GoCodeAlone/workflow-plugin-agent/provider"
	"github.com/google/uuid"

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
	mu     sync.RWMutex
	teams  map[string]*teamInstance
	engine *EngineContext
	hooks  *hooks.HookConfig
	stop   chan struct{}
}

// NewTeamManager returns an initialized TeamManager.
func NewTeamManager(engine *EngineContext, hks *hooks.HookConfig) *TeamManager {
	tm := &TeamManager{
		teams:  make(map[string]*teamInstance),
		engine: engine,
		hooks:  hks,
		stop:   make(chan struct{}),
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
func (tm *TeamManager) StartTeam(ctx context.Context, req *pb.StartTeamReq) (string, <-chan *pb.TeamEvent) {
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

	summary := fmt.Sprintf("Team completed task: %s", req.Task)
	if workerResult != "" {
		summary = workerResult
	}

	ti.eventCh <- &pb.TeamEvent{
		Event: &pb.TeamEvent_Complete{
			Complete: &pb.SessionComplete{
				Summary: summary,
			},
		},
	}

	tm.markDone(ti, "completed")
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
