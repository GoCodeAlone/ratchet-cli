package acp

import (
	"context"
	"testing"

	"github.com/GoCodeAlone/ratchet-cli/internal/daemon"
	acpsdk "github.com/coder/acp-go-sdk"
)

func TestParityNewSessionIDCanBeLoaded(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	if err := daemon.EnsureDataDir(); err != nil {
		t.Fatalf("EnsureDataDir: %v", err)
	}
	svc, err := daemon.NewService(context.Background())
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	t.Cleanup(func() { _, _ = svc.Shutdown(context.Background(), nil) })

	agent := NewRatchetAgent(svc)
	newResp, err := agent.NewSession(context.Background(), acpsdk.NewSessionRequest{Cwd: t.TempDir()})
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	if _, err := agent.LoadSession(context.Background(), acpsdk.LoadSessionRequest{SessionId: newResp.SessionId}); err != nil {
		t.Fatalf("LoadSession with returned id: %v", err)
	}
}

func TestParitySetSessionModelUpdatesSession(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	if err := daemon.EnsureDataDir(); err != nil {
		t.Fatalf("EnsureDataDir: %v", err)
	}
	svc, err := daemon.NewService(context.Background())
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	t.Cleanup(func() { _, _ = svc.Shutdown(context.Background(), nil) })

	agent := NewRatchetAgent(svc)
	newResp, err := agent.NewSession(context.Background(), acpsdk.NewSessionRequest{Cwd: t.TempDir()})
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	if _, err := agent.SetSessionModel(context.Background(), acpsdk.SetSessionModelRequest{
		SessionId: newResp.SessionId,
		ModelId:   "claude-sonnet-4",
	}); err != nil {
		t.Fatalf("SetSessionModel: %v", err)
	}
	sessions, err := svc.ListSessions(context.Background(), nil)
	if err != nil {
		t.Fatalf("ListSessions: %v", err)
	}
	for _, session := range sessions.Sessions {
		if session.Id == string(newResp.SessionId) {
			if session.Model != "claude-sonnet-4" {
				t.Fatalf("session model = %q, want claude-sonnet-4", session.Model)
			}
			return
		}
	}
	t.Fatalf("session %s not found", newResp.SessionId)
}

func TestParitySetSessionModeRequiresKnownSession(t *testing.T) {
	agent := NewRatchetAgent(nil)
	if _, err := agent.SetSessionMode(context.Background(), acpsdk.SetSessionModeRequest{
		SessionId: "missing",
		ModeId:    "plan",
	}); err == nil {
		t.Fatal("expected unknown session error")
	}
}
