package daemon

import (
	"context"
	"crypto/rand"
	"encoding/hex"
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
	cancel      context.CancelFunc // non-nil if agent has its own context
}

type agentMsg struct {
	from    string
	content string
	ts      time.Time
}

// teamObserver tracks an attached client observing a team.
type teamObserver struct {
	id     string
	mode   string // "observe" or "join"
	events chan *pb.TeamActivityEvent
	cancel context.CancelFunc
}

// teamInstance tracks a running team.
type teamInstance struct {
	mu          sync.RWMutex
	id          string
	task        string
	agents      map[string]*teamAgent
	observers   map[string]*teamObserver
	status      string // running, completed, failed
	cancel      context.CancelFunc
	eventCh     chan *pb.TeamEvent
	completedAt time.Time
}

// TeamManager manages team instances.
type TeamManager struct {
	mu     sync.RWMutex
	teams  map[string]*teamInstance
	names  map[string]string // user-assigned name → team ID
	engine *EngineContext
	hooks  *hooks.HookConfig
	stop   chan struct{}
	mesh   *mesh.AgentMesh
}

// generateTeamShortID generates a short human-readable team ID like "t-a3f2".
func generateTeamShortID() string {
	b := make([]byte, 2)
	if _, err := rand.Read(b); err != nil {
		return "t-" + uuid.NewString()[:4]
	}
	return "t-" + hex.EncodeToString(b)
}

// NewTeamManager returns an initialized TeamManager.
func NewTeamManager(engine *EngineContext, hks *hooks.HookConfig) *TeamManager {
	tm := &TeamManager{
		teams:  make(map[string]*teamInstance),
		names:  make(map[string]string),
		engine: engine,
		hooks:  hks,
		stop:   make(chan struct{}),
		mesh:   mesh.NewAgentMesh(),
	}
	go tm.cleanupLoop()
	return tm
}

// resolveTeamID maps a name-or-ID to a canonical team ID.
// Must be called with at least a read lock held on tm.mu.
func (tm *TeamManager) resolveTeamID(idOrName string) string {
	if _, ok := tm.teams[idOrName]; ok {
		return idOrName
	}
	if id, ok := tm.names[idOrName]; ok {
		return id
	}
	return idOrName
}

// Rename assigns a user-friendly name to a team.
func (tm *TeamManager) Rename(teamID, newName string) error {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	if _, ok := tm.teams[teamID]; !ok {
		return fmt.Errorf("team %q not found", teamID)
	}
	if existing, ok := tm.names[newName]; ok && existing != teamID {
		return fmt.Errorf("name %q already assigned to team %s", newName, existing)
	}
	// Remove old name mapping if one exists.
	for name, id := range tm.names {
		if id == teamID {
			delete(tm.names, name)
			break
		}
	}
	tm.names[newName] = teamID
	return nil
}

// ListTeams returns status for all teams, optionally filtered by project (unused in Phase 2).
func (tm *TeamManager) ListTeams(projectID string) []*pb.TeamStatus {
	tm.mu.RLock()
	defer tm.mu.RUnlock()
	var out []*pb.TeamStatus
	for _, ti := range tm.teams {
		ti.mu.RLock()
		var agents []*pb.Agent
		for _, ag := range ti.agents {
			ag.mu.RLock()
			agents = append(agents, &pb.Agent{
				Id:     ag.id,
				Name:   ag.name,
				Role:   ag.role,
				Status: ag.status,
			})
			ag.mu.RUnlock()
		}
		out = append(out, &pb.TeamStatus{
			TeamId: ti.id,
			Task:   ti.task,
			Agents: agents,
			Status: ti.status,
		})
		ti.mu.RUnlock()
	}
	return out
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
			for name, nameID := range tm.names {
				if nameID == id {
					delete(tm.names, name)
				}
			}
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

	teamID := generateTeamShortID()
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
// It first tries to load as a TeamConfig; if that fails it falls back to
// ProjectConfig (which supports workdir, paths, and per-team blackboard mode).
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
		loaded, loadErr := mesh.LoadTeamConfig(req.TeamConfigName)
		if loadErr != nil {
			// Fallback: try loading as a ProjectConfig (has project:, teams:, workdir:, paths:).
			pc, pcErr := mesh.LoadProjectConfig(req.TeamConfigName)
			if pcErr != nil {
				return errCh(fmt.Sprintf("load team config %q: %v", req.TeamConfigName, loadErr))
			}
			if len(pc.Teams) == 0 {
				return errCh(fmt.Sprintf("project config %q has no teams", req.TeamConfigName))
			}
			// Use the first team; future work can select by name via req.
			ptc := &pc.Teams[0]
			tc = ptc.ToTeamConfig()
			// Resolve WorkDir: team-level overrides project-level, fall back to cwd.
			if tc.WorkDir == "" {
				tc.WorkDir = pc.WorkDir
			}
			if tc.WorkDir == "" {
				tc.WorkDir = pc.Cwd
			}
			// Propagate project-level paths when team has none.
			if len(tc.AllowedPaths) == 0 {
				tc.AllowedPaths = pc.Paths
			}
		} else {
			tc = loaded
		}
	}

	configs := mesh.ToNodeConfigs(tc)
	providerFactory := tm.newProviderFactory(ctx)

	return tm.StartMeshTeam(ctx, req.Task, configs, providerFactory)
}

// executeAgent runs a single team agent via the real executor loop.
// orchestratorContext is the output from a prior orchestrator run; empty for the orchestrator itself.
func (tm *TeamManager) executeAgent(ctx context.Context, ag *teamAgent, task, orchestratorContext string) (string, error) {
	if tm.engine == nil || tm.engine.ProviderRegistry == nil {
		ag.mu.RLock()
		name := ag.name
		ag.mu.RUnlock()
		return "", fmt.Errorf("no engine configured: cannot execute agent %s", name)
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

	pbEv := &pb.TeamEvent{
		Event: &pb.TeamEvent_AgentMessage{
			AgentMessage: &pb.AgentMessage{
				FromAgent: from,
				ToAgent:   to,
				Content:   content,
			},
		},
	}
	ti.eventCh <- pbEv
	tm.broadcastToObservers(ti, &pb.TeamActivityEvent{
		Event: &pb.TeamActivityEvent_AgentMessage{
			AgentMessage: &pb.AgentMessage{
				FromAgent: from,
				ToAgent:   to,
				Content:   content,
			},
		},
	})
}

func (tm *TeamManager) markDone(ti *teamInstance, s string) {
	ti.mu.Lock()
	ti.status = s
	ti.completedAt = time.Now()
	ti.mu.Unlock()
}

// GetStatus returns the current TeamStatus for a given team ID or name.
func (tm *TeamManager) GetStatus(teamID string) (*pb.TeamStatus, error) {
	tm.mu.RLock()
	resolved := tm.resolveTeamID(teamID)
	ti, ok := tm.teams[resolved]
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

// ListAllAgents aggregates agents from all active team instances.
func (tm *TeamManager) ListAllAgents() []*pb.Agent {
	tm.mu.RLock()
	defer tm.mu.RUnlock()
	var agents []*pb.Agent
	for _, ti := range tm.teams {
		ti.mu.RLock()
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
				SessionId:   ti.id,
			})
			ag.mu.RUnlock()
		}
		ti.mu.RUnlock()
	}
	return agents
}

// FindAgent looks up an agent by ID across all teams.
func (tm *TeamManager) FindAgent(agentID string) *pb.Agent {
	tm.mu.RLock()
	defer tm.mu.RUnlock()
	for _, ti := range tm.teams {
		ti.mu.RLock()
		ag, ok := ti.agents[agentID]
		ti.mu.RUnlock()
		if ok {
			ag.mu.RLock()
			result := &pb.Agent{
				Id:          ag.id,
				Name:        ag.name,
				Role:        ag.role,
				Model:       ag.model,
				Provider:    ag.provider,
				Status:      ag.status,
				CurrentTask: ag.currentTask,
				SessionId:   ti.id,
			}
			ag.mu.RUnlock()
			return result
		}
	}
	return nil
}

// KillAgent cancels the team that owns the given agent (team-level cancel).
func (tm *TeamManager) KillAgent(teamID string) error {
	tm.mu.RLock()
	resolved := tm.resolveTeamID(teamID)
	ti, ok := tm.teams[resolved]
	tm.mu.RUnlock()
	if !ok {
		return fmt.Errorf("team %s not found", teamID)
	}
	ti.cancel()
	return nil
}

// AttachTeam registers an observer for a team and returns the event channel.
func (tm *TeamManager) AttachTeam(teamID, mode string) (string, <-chan *pb.TeamActivityEvent, error) {
	tm.mu.RLock()
	resolved := tm.resolveTeamID(teamID)
	ti, ok := tm.teams[resolved]
	tm.mu.RUnlock()
	if !ok {
		return "", nil, fmt.Errorf("team %q not found", teamID)
	}

	obsID := "obs-" + uuid.NewString()[:8]
	obsCtx, cancel := context.WithCancel(context.Background())
	ch := make(chan *pb.TeamActivityEvent, 64)

	obs := &teamObserver{
		id:     obsID,
		mode:   mode,
		events: ch,
		cancel: cancel,
	}

	ti.mu.Lock()
	if ti.observers == nil {
		ti.observers = make(map[string]*teamObserver)
	}
	ti.observers[obsID] = obs
	ti.mu.Unlock()

	// Cleanup when context is done.
	go func() {
		<-obsCtx.Done()
		ti.mu.Lock()
		delete(ti.observers, obsID)
		ti.mu.Unlock()
		close(ch)
	}()

	return obsID, ch, nil
}

// DetachTeam removes an observer by cancelling its context.
func (tm *TeamManager) DetachTeam(teamID, observerID string) {
	tm.mu.RLock()
	resolved := tm.resolveTeamID(teamID)
	ti, ok := tm.teams[resolved]
	tm.mu.RUnlock()
	if !ok {
		return
	}
	ti.mu.Lock()
	if obs, exists := ti.observers[observerID]; exists {
		obs.cancel()
	}
	ti.mu.Unlock()
}

// broadcastToObservers sends an event to all attached observers.
func (tm *TeamManager) broadcastToObservers(ti *teamInstance, event *pb.TeamActivityEvent) {
	ti.mu.RLock()
	defer ti.mu.RUnlock()
	for _, obs := range ti.observers {
		select {
		case obs.events <- event:
		default:
			// Drop if observer channel is full.
		}
	}
}

// AddAgent dynamically adds an agent to a running team's mesh.
// This is a stub; full wiring requires access to the team's BB/Router.
func (tm *TeamManager) AddAgent(teamID, agentSpec string) error {
	tm.mu.RLock()
	resolved := tm.resolveTeamID(teamID)
	_, ok := tm.teams[resolved]
	tm.mu.RUnlock()
	if !ok {
		return fmt.Errorf("team %q not found", teamID)
	}

	ac, err := mesh.ParseAgentFlag(agentSpec)
	if err != nil {
		return fmt.Errorf("parse agent spec: %w", err)
	}
	_ = ac
	// TODO: wire to team's live BB/Router in Task 2.5.
	return fmt.Errorf("dynamic add not yet wired to team mesh instance")
}

// RemoveAgent dynamically removes an agent from a running team.
func (tm *TeamManager) RemoveAgent(teamID, agentName string) error {
	tm.mu.RLock()
	resolved := tm.resolveTeamID(teamID)
	ti, ok := tm.teams[resolved]
	tm.mu.RUnlock()
	if !ok {
		return fmt.Errorf("team %q not found", teamID)
	}

	ti.mu.Lock()
	for id, ag := range ti.agents {
		if ag.name == agentName {
			delete(ti.agents, id)
			cancelFn := ag.cancel
			ti.mu.Unlock()
			if cancelFn != nil {
				cancelFn()
			}
			return nil
		}
	}
	ti.mu.Unlock()
	return fmt.Errorf("agent %q not found in team %q", agentName, teamID)
}

// newProviderFactory builds a provider factory using the engine's ProviderRegistry,
// with an Ollama fallback for standalone/test mode. This is the same factory
// used by startMeshTeamFromConfig and should be passed to StartMeshTeam callers.
func (tm *TeamManager) newProviderFactory(ctx context.Context) func(mesh.NodeConfig) provider.Provider {
	return func(cfg mesh.NodeConfig) provider.Provider {
		if tm.engine != nil && tm.engine.ProviderRegistry != nil {
			var prov provider.Provider
			var provErr error
			if cfg.Provider != "" {
				prov, provErr = tm.engine.ProviderRegistry.GetByAlias(ctx, cfg.Provider)
				if provErr != nil {
					log.Printf("team agent %s: provider %q not found (%v), trying default", cfg.Name, cfg.Provider, provErr)
				}
			}
			if prov == nil {
				prov, provErr = tm.engine.ProviderRegistry.GetDefault(ctx)
				if provErr != nil {
					log.Printf("team agent %s: no default provider: %v", cfg.Name, provErr)
				}
			}
			if prov != nil {
				return prov
			}
		}
		model := cfg.Model
		if model == "" {
			model = "qwen3:1.7b"
		}
		prov, err := gkprov.NewOllamaProvider(ctx, model, "", 0)
		if err != nil {
			log.Printf("team agent %s: fallback ollama provider: %v", cfg.Name, err)
			return nil
		}
		return prov
	}
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
			case "thinking":
				// Suppress thinking events from team output — they contain
				// the model's internal reasoning which is noise in team context.
				// If we want to expose this later, TeamEvent needs a Thinking variant.
				continue
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

		// Emit error events for any agent failures before the completion event.
		if len(result.Errors) > 0 {
			for _, err := range result.Errors {
				select {
				case eventCh <- &pb.TeamEvent{
					Event: &pb.TeamEvent_Error{
						Error: &pb.ErrorEvent{Message: err.Error()},
					},
				}:
				case <-ctx.Done():
					return
				}
			}
		}

		status := result.Status
		if status == "" {
			status = "completed"
		}
		summary := fmt.Sprintf("Team %s task: %s", status, task)

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

		tm.markDone(ti, status)
	}()

	return teamID, eventCh
}
