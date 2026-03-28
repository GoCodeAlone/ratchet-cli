package daemon

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/google/uuid"

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
	mu      sync.RWMutex
	id      string
	task    string
	agents  map[string]*teamAgent
	status  string // running, completed, failed
	cancel  context.CancelFunc
	eventCh chan *pb.TeamEvent
}

// TeamManager manages team instances.
type TeamManager struct {
	mu    sync.RWMutex
	teams map[string]*teamInstance
}

// NewTeamManager returns an initialized TeamManager.
func NewTeamManager() *TeamManager {
	return &TeamManager{teams: make(map[string]*teamInstance)}
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

// run is the main team goroutine. It spawns agents and simulates their execution.
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
	}

	// Simulate orchestrator → worker message exchange.
	time.Sleep(50 * time.Millisecond)

	select {
	case <-ctx.Done():
		tm.markDone(ti, "failed")
		return
	default:
	}

	orch := tm.agentByRole(ti, "orchestrator")
	worker := tm.agentByRole(ti, "worker")
	if orch != nil && worker != nil {
		msg := fmt.Sprintf("Please work on: %s", req.Task)
		tm.routeMessage(ti, orch.name, worker.name, msg)

		time.Sleep(50 * time.Millisecond)

		reply := fmt.Sprintf("Task %q acknowledged, starting...", req.Task)
		tm.routeMessage(ti, worker.name, orch.name, reply)
	}

	// Mark all agents complete.
	ti.mu.RLock()
	for _, ag := range ti.agents {
		ag.mu.Lock()
		ag.status = "completed"
		ag.mu.Unlock()
	}
	ti.mu.RUnlock()

	ti.eventCh <- &pb.TeamEvent{
		Event: &pb.TeamEvent_Complete{
			Complete: &pb.SessionComplete{
				Summary: fmt.Sprintf("Team completed task: %s", req.Task),
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
