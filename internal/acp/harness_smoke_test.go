package acp

import (
	"context"
	"testing"

	"github.com/GoCodeAlone/ratchet-cli/internal/daemon"
	acpsdk "github.com/coder/acp-go-sdk"
)

func TestHarnessSmokeInitializeNewAndLoadSession(t *testing.T) {
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
	initResp, err := agent.Initialize(context.Background(), acpsdk.InitializeRequest{})
	if err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	if initResp.AgentInfo == nil || initResp.AgentInfo.Name != "ratchet" {
		t.Fatalf("AgentInfo = %#v", initResp.AgentInfo)
	}
	if !initResp.AgentCapabilities.LoadSession {
		t.Fatal("expected LoadSession capability")
	}

	newResp, err := agent.NewSession(context.Background(), acpsdk.NewSessionRequest{Cwd: t.TempDir()})
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	if newResp.SessionId == "" {
		t.Fatal("expected ACP session id")
	}

	if _, err := agent.LoadSession(context.Background(), acpsdk.LoadSessionRequest{SessionId: newResp.SessionId}); err == nil {
		t.Fatal("expected direct LoadSession with ACP id to fail; ratchet session id is intentionally separate")
	}
}
