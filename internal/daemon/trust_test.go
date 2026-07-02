package daemon

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	pb "github.com/GoCodeAlone/ratchet-cli/internal/proto"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestServiceTrustStateFromConfig(t *testing.T) {
	svc := newTrustTestService(t, `
trust:
  mode: locked
  rules:
    - pattern: "bash:custom *"
      action: allow
`)

	state, err := svc.GetTrustState(context.Background(), &pb.Empty{})
	if err != nil {
		t.Fatalf("GetTrustState: %v", err)
	}
	if state.Mode != "locked" {
		t.Fatalf("mode = %q, want locked", state.Mode)
	}
	if !hasTrustRule(state.Rules, "bash:custom *", "allow", "global") {
		t.Fatalf("state rules missing config allow: %+v", state.Rules)
	}
}

func TestServiceTrustMutationsAndReset(t *testing.T) {
	svc := newTrustTestService(t, `
trust:
  mode: conservative
  rules:
    - pattern: "bash:custom *"
      action: allow
`)

	state, err := svc.SetTrustMode(context.Background(), &pb.SetTrustModeReq{Mode: "locked"})
	if err != nil {
		t.Fatalf("SetTrustMode: %v", err)
	}
	if state.Mode != "locked" {
		t.Fatalf("mode = %q, want locked", state.Mode)
	}

	state, err = svc.AddTrustRule(context.Background(), &pb.AddTrustRuleReq{
		Pattern: "bash:go test ./...",
		Action:  "deny",
		Scope:   "repo",
	})
	if err != nil {
		t.Fatalf("AddTrustRule: %v", err)
	}
	if !hasTrustRule(state.Rules, "bash:go test ./...", "deny", "repo") {
		t.Fatalf("state rules missing runtime deny: %+v", state.Rules)
	}

	state, err = svc.ResetTrust(context.Background(), &pb.Empty{})
	if err != nil {
		t.Fatalf("ResetTrust: %v", err)
	}
	if state.Mode != "conservative" {
		t.Fatalf("reset mode = %q, want conservative", state.Mode)
	}
	if hasTrustRule(state.Rules, "bash:go test ./...", "deny", "repo") {
		t.Fatalf("reset kept runtime rule: %+v", state.Rules)
	}
	if !hasTrustRule(state.Rules, "bash:custom *", "allow", "global") {
		t.Fatalf("reset lost config rule: %+v", state.Rules)
	}
}

func TestServiceTrustCustomModeAndValidation(t *testing.T) {
	svc := newTrustTestService(t, `
trust:
  mode: conservative
`)

	state, err := svc.SetTrustMode(context.Background(), &pb.SetTrustModeReq{Mode: "custom"})
	if err != nil {
		t.Fatalf("SetTrustMode custom: %v", err)
	}
	if state.Mode != "custom" {
		t.Fatalf("mode = %q, want custom", state.Mode)
	}

	invalidMode := []string{"", "unknown"}
	for _, mode := range invalidMode {
		_, err := svc.SetTrustMode(context.Background(), &pb.SetTrustModeReq{Mode: mode})
		if status.Code(err) != codes.InvalidArgument {
			t.Fatalf("SetTrustMode(%q) code = %v, want InvalidArgument (err=%v)", mode, status.Code(err), err)
		}
	}

	invalidRules := []*pb.AddTrustRuleReq{
		{Pattern: "", Action: "allow", Scope: "repo"},
		{Pattern: "bash:*", Action: "", Scope: "repo"},
		{Pattern: "bash:*", Action: "maybe", Scope: "repo"},
	}
	for _, req := range invalidRules {
		_, err := svc.AddTrustRule(context.Background(), req)
		if status.Code(err) != codes.InvalidArgument {
			t.Fatalf("AddTrustRule(%+v) code = %v, want InvalidArgument (err=%v)", req, status.Code(err), err)
		}
	}
}

func newTrustTestService(t *testing.T, configYAML string) *Service {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	if configYAML != "" {
		dir := filepath.Join(home, ".ratchet")
		if err := os.MkdirAll(dir, 0700); err != nil {
			t.Fatalf("mkdir config dir: %v", err)
		}
		if err := os.WriteFile(filepath.Join(dir, "config.yaml"), []byte(configYAML), 0600); err != nil {
			t.Fatalf("write config: %v", err)
		}
	}
	svc, err := NewService(context.Background())
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	t.Cleanup(func() {
		if svc.engine != nil {
			svc.engine.Close()
		}
	})
	return svc
}

func hasTrustRule(rules []*pb.TrustRule, pattern, action, scope string) bool {
	for _, rule := range rules {
		if rule.Pattern == pattern && rule.Action == action && rule.Scope == scope {
			return true
		}
	}
	return false
}
