package agent

import (
	"fmt"
	"sync"
	"time"
)

// Team represents a group of agents working on a task.
type Team struct {
	ID      string
	Task    string
	Agents  []TeamAgent
	Status  string // running, completed, failed
	mu      sync.RWMutex
}

// TeamAgent is a member of a Team.
type TeamAgent struct {
	ID          string
	Name        string
	Role        string
	Model       string
	Provider    string
	Status      string // idle, running, done
	CurrentTask string
}

// Orchestrator manages multi-agent team execution.
type Orchestrator struct {
	mu    sync.RWMutex
	teams map[string]*Team
}

func NewOrchestrator() *Orchestrator {
	return &Orchestrator{
		teams: make(map[string]*Team),
	}
}

// SpawnTeam creates a team of agents for a complex task.
// The team ID is returned immediately; execution is async.
func (o *Orchestrator) SpawnTeam(task string, defs []AgentDefinition) (*Team, error) {
	if len(defs) == 0 {
		return nil, fmt.Errorf("no agent definitions provided")
	}

	team := &Team{
		ID:     fmt.Sprintf("team-%d", time.Now().UnixNano()),
		Task:   task,
		Status: "running",
	}

	for _, def := range defs {
		team.Agents = append(team.Agents, TeamAgent{
			ID:       fmt.Sprintf("%s-%d", def.Name, time.Now().UnixNano()),
			Name:     def.Name,
			Role:     def.Role,
			Model:    def.Model,
			Provider: def.Provider,
			Status:   "idle",
		})
	}

	o.mu.Lock()
	o.teams[team.ID] = team
	o.mu.Unlock()

	return team, nil
}

// GetTeam returns a team by ID.
func (o *Orchestrator) GetTeam(id string) (*Team, bool) {
	o.mu.RLock()
	defer o.mu.RUnlock()
	t, ok := o.teams[id]
	return t, ok
}

// UpdateAgentStatus updates an agent's status within a team.
func (o *Orchestrator) UpdateAgentStatus(teamID, agentID, status, currentTask string) {
	o.mu.RLock()
	team, ok := o.teams[teamID]
	o.mu.RUnlock()
	if !ok {
		return
	}
	team.mu.Lock()
	defer team.mu.Unlock()
	for i := range team.Agents {
		if team.Agents[i].ID == agentID {
			team.Agents[i].Status = status
			team.Agents[i].CurrentTask = currentTask
			return
		}
	}
}
