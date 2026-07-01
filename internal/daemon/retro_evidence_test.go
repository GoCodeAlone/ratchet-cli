package daemon

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/GoCodeAlone/ratchet-cli/internal/config"
	pb "github.com/GoCodeAlone/ratchet-cli/internal/proto"
	"github.com/GoCodeAlone/ratchet-cli/internal/retro"
	"github.com/GoCodeAlone/workflow/secrets"
)

func TestNewRetroRecorderHonorsConfig(t *testing.T) {
	if rec := newRetroRecorder(config.RetroConfig{}, t.TempDir(), nil); rec != nil {
		t.Fatalf("disabled recorder = %#v, want nil", rec)
	}
	if rec := newRetroRecorder(config.RetroConfig{Enabled: true}, t.TempDir(), nil); rec == nil {
		t.Fatal("enabled recorder is nil")
	}
}

func TestServiceRetroEvidenceRecordsSessionLifecycle(t *testing.T) {
	engine := newTestEngine(t)
	redactor := secrets.NewRedactor()
	redactor.AddValue("provider", "sk-session-secret")
	path := filepath.Join(t.TempDir(), "evidence.jsonl")
	store := retro.NewEvidenceStore(path, redactor)
	svc := &Service{
		engine:        engine,
		sessions:      NewSessionManager(engine.DB),
		retroRecorder: retro.NewRecorder(store),
	}

	ctx := context.Background()
	session, err := svc.CreateSession(ctx, &pb.CreateSessionReq{
		WorkingDir:    "/tmp/project",
		Provider:      "sk-session-secret",
		InitialPrompt: "retro test",
	})
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	if _, err := svc.KillSession(ctx, &pb.KillReq{SessionId: session.Id}); err != nil {
		t.Fatalf("KillSession: %v", err)
	}

	events, err := store.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("events len = %d, want 2: %#v", len(events), events)
	}
	if events[0].Kind != retro.EventSessionCreated || events[1].Kind != retro.EventSessionCompleted {
		t.Fatalf("event kinds = %s, %s", events[0].Kind, events[1].Kind)
	}
	for _, event := range events {
		if event.SessionID != session.Id {
			t.Fatalf("event session id = %q, want %q", event.SessionID, session.Id)
		}
		if strings.Contains(event.Message, "sk-session-secret") {
			t.Fatalf("event leaked provider secret: %#v", event)
		}
	}
}
