package daemon

import (
	"context"
	"testing"
	"time"

	pb "github.com/GoCodeAlone/ratchet-cli/internal/proto"
)

func TestTeamManager_Create(t *testing.T) {
	tm := NewTeamManager(newTestEngine(t), nil)
	teamID, eventCh := tm.StartTeam(context.Background(), &pb.StartTeamReq{
		Task: "test task",
	})
	if teamID == "" {
		t.Fatal("expected non-empty team ID")
	}
	if eventCh == nil {
		t.Fatal("expected non-nil event channel")
	}

	// Drain events.
	for range eventCh {
	}

	st, err := tm.GetStatus(teamID)
	if err != nil {
		t.Fatalf("GetStatus: %v", err)
	}
	if st.TeamId != teamID {
		t.Errorf("expected team ID %s, got %s", teamID, st.TeamId)
	}
	if st.Task != "test task" {
		t.Errorf("unexpected task: %s", st.Task)
	}
}

func TestTeamManager_AgentLifecycle(t *testing.T) {
	tm := NewTeamManager(newTestEngine(t), nil)
	teamID, eventCh := tm.StartTeam(context.Background(), &pb.StartTeamReq{
		Task: "build something",
	})

	var spawned []*pb.AgentSpawned
	for ev := range eventCh {
		if ag, ok := ev.Event.(*pb.TeamEvent_AgentSpawned); ok {
			spawned = append(spawned, ag.AgentSpawned)
		}
	}

	if len(spawned) < 2 {
		t.Errorf("expected at least 2 agents spawned, got %d", len(spawned))
	}

	st, err := tm.GetStatus(teamID)
	if err != nil {
		t.Fatalf("GetStatus: %v", err)
	}
	for _, a := range st.Agents {
		if a.Status != "completed" {
			t.Errorf("agent %s: expected completed, got %s", a.Name, a.Status)
		}
	}
	if st.Status != "completed" {
		t.Errorf("team status: expected completed, got %s", st.Status)
	}
}

func TestTeamManager_DirectMessage(t *testing.T) {
	tm := NewTeamManager(newTestEngine(t), nil)
	_, eventCh := tm.StartTeam(context.Background(), &pb.StartTeamReq{
		Task: "exchange messages",
	})

	var messages []*pb.AgentMessage
	for ev := range eventCh {
		if msg, ok := ev.Event.(*pb.TeamEvent_AgentMessage); ok {
			messages = append(messages, msg.AgentMessage)
		}
	}

	if len(messages) < 1 {
		t.Error("expected at least one agent message exchange")
	}
	// Verify message routing fields.
	for _, m := range messages {
		if m.FromAgent == "" {
			t.Error("message FromAgent should not be empty")
		}
		if m.ToAgent == "" {
			t.Error("message ToAgent should not be empty")
		}
	}
}

func TestTeamManager_KillAgent(t *testing.T) {
	tm := NewTeamManager(nil, nil)

	// Use a context to detect cancellation.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	teamID, eventCh := tm.StartTeam(ctx, &pb.StartTeamReq{
		Task: "long task",
	})

	// Give time for team to start.
	time.Sleep(10 * time.Millisecond)

	if err := tm.KillAgent(teamID); err != nil {
		t.Fatalf("KillAgent: %v", err)
	}

	// Drain (may be already closed or will close after cancel).
	done := make(chan struct{})
	go func() {
		for range eventCh {
		}
		close(done)
	}()

	select {
	case <-done:
		// OK
	case <-time.After(2 * time.Second):
		t.Error("timed out waiting for event channel to close after kill")
	}
}

func TestTeamManager_GetStatus_NotFound(t *testing.T) {
	tm := NewTeamManager(nil, nil)
	_, err := tm.GetStatus("nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent team")
	}
}

func TestTeamShortID(t *testing.T) {
	id := generateTeamShortID()
	if len(id) < 6 || id[:2] != "t-" {
		t.Errorf("got ID %q, want t-XXXX pattern", id)
	}
}

func TestTeamRename(t *testing.T) {
	tm := NewTeamManager(nil, nil)

	ti := &teamInstance{
		id:     "t-abcd",
		task:   "test",
		agents: make(map[string]*teamAgent),
		status: "running",
	}
	tm.mu.Lock()
	tm.teams["t-abcd"] = ti
	tm.mu.Unlock()

	if err := tm.Rename("t-abcd", "email-dev"); err != nil {
		t.Fatalf("Rename: %v", err)
	}

	// Lookup by new name.
	if _, err := tm.GetStatus("email-dev"); err != nil {
		t.Fatalf("GetStatus by name: %v", err)
	}

	// Lookup by old ID still works.
	if _, err := tm.GetStatus("t-abcd"); err != nil {
		t.Fatalf("GetStatus by ID: %v", err)
	}
}

func TestListTeams(t *testing.T) {
	tm := NewTeamManager(nil, nil)

	tm.mu.Lock()
	tm.teams["t-0001"] = &teamInstance{id: "t-0001", task: "task-a", agents: make(map[string]*teamAgent), status: "running"}
	tm.teams["t-0002"] = &teamInstance{id: "t-0002", task: "task-b", agents: make(map[string]*teamAgent), status: "completed"}
	tm.mu.Unlock()

	teams := tm.ListTeams("")
	if len(teams) != 2 {
		t.Fatalf("got %d teams, want 2", len(teams))
	}
}
