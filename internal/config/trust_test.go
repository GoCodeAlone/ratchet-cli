package config

import (
	"testing"

	"github.com/GoCodeAlone/workflow-plugin-agent/policy"
)

func TestTrustConfigDefaults(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.Trust.Mode != "conservative" {
		t.Errorf("default mode = %q, want conservative", cfg.Trust.Mode)
	}
}

func TestTrustConfigToRules(t *testing.T) {
	tc := TrustConfig{
		Mode: "custom",
		Rules: []TrustRuleConfig{
			{Pattern: "file_read", Action: "allow"},
			{Pattern: "bash:rm *", Action: "deny"},
		},
	}
	rules := tc.ToTrustRules()
	if len(rules) != 2 {
		t.Fatalf("expected 2 rules, got %d", len(rules))
	}
	if rules[0].Action != policy.Allow {
		t.Error("first rule should be Allow")
	}
	if rules[1].Action != policy.Deny {
		t.Error("second rule should be Deny")
	}
}

func TestTrustConfigProviderArgs(t *testing.T) {
	tc := TrustConfig{
		ProviderArgs: map[string][]string{
			"claude_code": {"--permission-mode", "acceptEdits"},
			"copilot_cli": {"--allow-tool=Edit"},
		},
	}
	args := tc.ProviderArgsFor("claude_code")
	if len(args) != 2 {
		t.Fatalf("expected 2 args, got %d", len(args))
	}
	if args[0] != "--permission-mode" {
		t.Errorf("first arg = %q", args[0])
	}
}

func TestTrustConfigProviderArgsEmpty(t *testing.T) {
	tc := TrustConfig{}
	args := tc.ProviderArgsFor("claude_code")
	if len(args) != 0 {
		t.Errorf("expected 0 args, got %d", len(args))
	}
}
