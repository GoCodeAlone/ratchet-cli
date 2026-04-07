package acp

import (
	"testing"

	acpsdk "github.com/coder/acp-go-sdk"
)

// TestRatchetAgentInterfaces verifies compile-time interface satisfaction.
func TestRatchetAgentInterfaces(t *testing.T) {
	var _ acpsdk.Agent = (*RatchetAgent)(nil)
	var _ acpsdk.AgentLoader = (*RatchetAgent)(nil)
	var _ acpsdk.AgentExperimental = (*RatchetAgent)(nil)
}

// TestNewRatchetAgent verifies the constructor.
func TestNewRatchetAgent(t *testing.T) {
	agent := NewRatchetAgent(nil)
	if agent == nil {
		t.Fatal("expected non-nil agent")
	}
	if agent.sessions == nil {
		t.Error("expected sessions map initialized")
	}
	if agent.cancels == nil {
		t.Error("expected cancels map initialized")
	}
}
