package daemon

import (
	"context"
	"strings"
	"testing"
	"time"

	pb "github.com/GoCodeAlone/ratchet-cli/internal/proto"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
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

func TestTeamManager_DirectMessageFromOperator(t *testing.T) {
	tm := NewTeamManager(nil, nil)
	ti := newRunningTeamForDirectMessage()
	tm.mu.Lock()
	tm.teams[ti.id] = ti
	tm.names["friendly"] = ti.id
	tm.mu.Unlock()

	if err := tm.DirectMessage("friendly", "worker", "hello worker"); err != nil {
		t.Fatalf("DirectMessage: %v", err)
	}

	worker := ti.agents["worker-id"]
	worker.mu.RLock()
	if len(worker.messages) != 1 {
		t.Fatalf("worker messages = %d, want 1", len(worker.messages))
	}
	msg := worker.messages[0]
	worker.mu.RUnlock()
	if msg.from != "operator" || msg.content != "hello worker" {
		t.Fatalf("worker message = %+v", msg)
	}

	select {
	case ev := <-ti.eventCh:
		got := ev.GetAgentMessage()
		if got == nil || got.FromAgent != "operator" || got.ToAgent != "worker" || got.Content != "hello worker" {
			t.Fatalf("event = %+v", ev)
		}
	default:
		t.Fatal("expected team event")
	}
}

func TestTeamManager_DirectMessageResolvesRecipientID(t *testing.T) {
	tm := NewTeamManager(nil, nil)
	ti := newRunningTeamForDirectMessage()
	tm.mu.Lock()
	tm.teams[ti.id] = ti
	tm.mu.Unlock()

	if err := tm.DirectMessage(ti.id, "worker-id", "by id"); err != nil {
		t.Fatalf("DirectMessage by id: %v", err)
	}
	worker := ti.agents["worker-id"]
	worker.mu.RLock()
	defer worker.mu.RUnlock()
	if len(worker.messages) != 1 || worker.messages[0].content != "by id" {
		t.Fatalf("worker messages = %+v", worker.messages)
	}
}

func TestTeamManager_DirectMessagePreservesContentWhitespace(t *testing.T) {
	tm := NewTeamManager(nil, nil)
	ti := newRunningTeamForDirectMessage()
	tm.mu.Lock()
	tm.teams[ti.id] = ti
	tm.mu.Unlock()

	content := "  hello worker\n"
	if err := tm.DirectMessage(ti.id, "worker", content); err != nil {
		t.Fatalf("DirectMessage: %v", err)
	}
	worker := ti.agents["worker-id"]
	worker.mu.RLock()
	defer worker.mu.RUnlock()
	if len(worker.messages) != 1 || worker.messages[0].content != content {
		t.Fatalf("worker messages = %+v, want content %q", worker.messages, content)
	}
}

func TestTeamManager_DirectMessageReportsFullEventChannel(t *testing.T) {
	tm := NewTeamManager(nil, nil)
	ti := newRunningTeamForDirectMessage()
	ti.eventCh = make(chan *pb.TeamEvent, 1)
	ti.eventCh <- &pb.TeamEvent{Event: &pb.TeamEvent_Token{Token: &pb.TokenDelta{Content: "full"}}}
	tm.mu.Lock()
	tm.teams[ti.id] = ti
	tm.mu.Unlock()

	if err := tm.DirectMessage(ti.id, "worker", "hello"); err == nil || !strings.Contains(err.Error(), "event channel") {
		t.Fatalf("full event channel error = %v", err)
	}
	worker := ti.agents["worker-id"]
	worker.mu.RLock()
	defer worker.mu.RUnlock()
	if len(worker.messages) != 0 {
		t.Fatalf("message appended despite full event channel: %+v", worker.messages)
	}
}

func TestTeamManager_DirectMessageErrors(t *testing.T) {
	tm := NewTeamManager(nil, nil)
	ti := newRunningTeamForDirectMessage()
	tm.mu.Lock()
	tm.teams[ti.id] = ti
	tm.mu.Unlock()

	if err := tm.DirectMessage("missing", "worker", "hello"); err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("missing team error = %v", err)
	}
	if err := tm.DirectMessage(ti.id, "missing", "hello"); err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("missing agent error = %v", err)
	}
	if err := tm.DirectMessage(ti.id, "worker", ""); err == nil || !strings.Contains(err.Error(), "content") {
		t.Fatalf("empty content error = %v", err)
	}

	ti.mu.Lock()
	ti.status = "completed"
	ti.mu.Unlock()
	if err := tm.DirectMessage(ti.id, "worker", "hello"); err == nil || !strings.Contains(err.Error(), "not running") {
		t.Fatalf("completed team error = %v", err)
	}
}

func TestServiceDirectMessageMapsErrors(t *testing.T) {
	svc := &Service{teams: NewTeamManager(nil, nil)}
	ti := newRunningTeamForDirectMessage()
	svc.teams.mu.Lock()
	svc.teams.teams[ti.id] = ti
	svc.teams.mu.Unlock()

	if _, err := svc.DirectMessage(context.Background(), &pb.DirectMessageReq{
		TeamId:  ti.id,
		ToAgent: "worker",
		Content: "hello",
	}); err != nil {
		t.Fatalf("DirectMessage: %v", err)
	}

	cases := []struct {
		name string
		req  *pb.DirectMessageReq
		code codes.Code
	}{
		{name: "missing team", req: &pb.DirectMessageReq{TeamId: "missing", ToAgent: "worker", Content: "hello"}, code: codes.NotFound},
		{name: "missing agent", req: &pb.DirectMessageReq{TeamId: ti.id, ToAgent: "missing", Content: "hello"}, code: codes.NotFound},
		{name: "empty content", req: &pb.DirectMessageReq{TeamId: ti.id, ToAgent: "worker"}, code: codes.InvalidArgument},
	}
	for _, tc := range cases {
		_, err := svc.DirectMessage(context.Background(), tc.req)
		if status.Code(err) != tc.code {
			t.Fatalf("%s code = %v, want %v (err=%v)", tc.name, status.Code(err), tc.code, err)
		}
	}

	ti.mu.Lock()
	ti.status = "completed"
	ti.mu.Unlock()
	_, err := svc.DirectMessage(context.Background(), &pb.DirectMessageReq{TeamId: ti.id, ToAgent: "worker", Content: "hello"})
	if status.Code(err) != codes.FailedPrecondition {
		t.Fatalf("completed team code = %v, want FailedPrecondition (err=%v)", status.Code(err), err)
	}

	ti.mu.Lock()
	ti.status = "running"
	ti.eventCh = make(chan *pb.TeamEvent, 1)
	ti.eventCh <- &pb.TeamEvent{Event: &pb.TeamEvent_Token{Token: &pb.TokenDelta{Content: "full"}}}
	ti.mu.Unlock()
	_, err = svc.DirectMessage(context.Background(), &pb.DirectMessageReq{TeamId: ti.id, ToAgent: "worker", Content: "hello"})
	if status.Code(err) != codes.ResourceExhausted {
		t.Fatalf("full event channel code = %v, want ResourceExhausted (err=%v)", status.Code(err), err)
	}
}

func newRunningTeamForDirectMessage() *teamInstance {
	return &teamInstance{
		id:        "t-direct",
		task:      "direct message",
		status:    "running",
		agents:    map[string]*teamAgent{"worker-id": {id: "worker-id", name: "worker", role: "worker", status: "running"}},
		observers: make(map[string]*teamObserver),
		eventCh:   make(chan *pb.TeamEvent, 8),
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
