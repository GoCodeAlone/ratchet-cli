package daemon

import (
	"context"
	"testing"

	pb "github.com/GoCodeAlone/ratchet-cli/internal/proto"
)

func TestTeam_RealExecution(t *testing.T) {
	engine := newTestEngine(t)
	tm := NewTeamManager(engine, nil)

	teamID, eventCh := tm.StartTeam(context.Background(), &pb.StartTeamReq{
		Task: "real task",
	})
	for range eventCh {
	}

	st, err := tm.GetStatus(teamID)
	if err != nil {
		t.Fatalf("GetStatus: %v", err)
	}
	if st.Status != "completed" {
		t.Errorf("expected completed, got %s", st.Status)
	}
	for _, a := range st.Agents {
		if a.Status != "completed" {
			t.Errorf("agent %s: expected completed, got %s", a.Name, a.Status)
		}
	}
}

func TestTeam_OrchestratorFailure(t *testing.T) {
	// nil engine causes executeAgent to return an error, so team status is "failed".
	tm := NewTeamManager(nil, nil)
	teamID, eventCh := tm.StartTeam(context.Background(), &pb.StartTeamReq{
		Task: "test task",
	})
	for range eventCh {
	}
	st, err := tm.GetStatus(teamID)
	if err != nil {
		t.Fatalf("GetStatus: %v", err)
	}
	if st.Status != "failed" {
		t.Errorf("expected failed, got %s", st.Status)
	}
}

func TestTeam_MessageRouting(t *testing.T) {
	engine := newTestEngine(t) // real engine needed for message routing
	tm := NewTeamManager(engine, nil)

	_, eventCh := tm.StartTeam(context.Background(), &pb.StartTeamReq{
		Task: "message routing task",
	})

	var messages []*pb.AgentMessage
	for ev := range eventCh {
		if msg, ok := ev.Event.(*pb.TeamEvent_AgentMessage); ok {
			messages = append(messages, msg.AgentMessage)
		}
	}

	if len(messages) < 1 {
		t.Error("expected at least one AgentMessage event")
	}
	for _, m := range messages {
		if m.FromAgent == "" {
			t.Error("message FromAgent should not be empty")
		}
		if m.ToAgent == "" {
			t.Error("message ToAgent should not be empty")
		}
		if m.Content == "" {
			t.Error("message Content should not be empty")
		}
	}
}
