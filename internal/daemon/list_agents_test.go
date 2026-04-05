package daemon

import (
	"context"
	"testing"

	"github.com/GoCodeAlone/ratchet-cli/internal/config"
	pb "github.com/GoCodeAlone/ratchet-cli/internal/proto"
)

func TestListAgents_AggregatesTeams(t *testing.T) {
	engine := newTestEngine(t)
	svc := &Service{
		broadcaster: NewSessionBroadcaster(),
		teams:       NewTeamManager(engine, nil),
		fleet:       NewFleetManager(config.ModelRouting{}, engine, nil),
	}

	// Start a team and drain events so agents are populated.
	_, eventCh := svc.teams.StartTeam(context.Background(), &pb.StartTeamReq{Task: "test"})
	for range eventCh {
	}

	resp, err := svc.ListAgents(context.Background(), &pb.Empty{})
	if err != nil {
		t.Fatalf("ListAgents: %v", err)
	}
	if len(resp.Agents) == 0 {
		t.Error("expected at least one agent from completed team")
	}
}

func TestGetAgentStatus_NotFound(t *testing.T) {
	svc := &Service{
		broadcaster: NewSessionBroadcaster(),
		teams:       NewTeamManager(nil, nil),
		fleet:       NewFleetManager(config.ModelRouting{}, nil, nil),
	}
	_, err := svc.GetAgentStatus(context.Background(), &pb.AgentStatusReq{AgentId: "nonexistent"})
	if err == nil {
		t.Error("expected error for nonexistent agent")
	}
}

func TestGetAgentStatus_FoundInTeam(t *testing.T) {
	engine := newTestEngine(t)
	svc := &Service{
		broadcaster: NewSessionBroadcaster(),
		teams:       NewTeamManager(engine, nil),
		fleet:       NewFleetManager(config.ModelRouting{}, engine, nil),
	}

	_, eventCh := svc.teams.StartTeam(context.Background(), &pb.StartTeamReq{Task: "test"})
	for range eventCh {
	}

	// Get the agent list to find an existing agent ID.
	list, err := svc.ListAgents(context.Background(), &pb.Empty{})
	if err != nil || len(list.Agents) == 0 {
		t.Skip("no agents to look up")
	}

	ag, err := svc.GetAgentStatus(context.Background(), &pb.AgentStatusReq{AgentId: list.Agents[0].Id})
	if err != nil {
		t.Fatalf("GetAgentStatus: %v", err)
	}
	if ag.Id != list.Agents[0].Id {
		t.Errorf("expected agent ID %s, got %s", list.Agents[0].Id, ag.Id)
	}
}
