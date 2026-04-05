package daemon

import (
	"context"
	"testing"

	pb "github.com/GoCodeAlone/ratchet-cli/internal/proto"
)

func TestExecuteAgent_NilEngineReturnsError(t *testing.T) {
	tm := NewTeamManager(nil, nil)

	_, eventCh := tm.StartTeam(context.Background(), &pb.StartTeamReq{Task: "test"})
	for range eventCh {
	}

	// With nil engine, agents cannot execute → team should be "failed".
	// (This was previously a stub that returned fake success.)
	teams := tm.ListAllAgents()
	if len(teams) == 0 {
		t.Fatal("expected at least one agent to have been registered")
	}
	for _, ag := range teams {
		if ag.Status == "completed" {
			t.Errorf("agent %s should not have status 'completed' with nil engine", ag.Name)
		}
	}
}

func TestExecuteAgent_NilEngineTeamStatusFailed(t *testing.T) {
	tm := NewTeamManager(nil, nil)

	teamID, eventCh := tm.StartTeam(context.Background(), &pb.StartTeamReq{Task: "test"})
	for range eventCh {
	}

	st, err := tm.GetStatus(teamID)
	if err != nil {
		t.Fatalf("GetStatus: %v", err)
	}
	if st.Status != "failed" {
		t.Errorf("expected team status 'failed' with nil engine, got %q", st.Status)
	}
}
