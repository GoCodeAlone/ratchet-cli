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

func TestServicePersistentTrustGrantLifecycle(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	if err := os.MkdirAll(filepath.Join(home, ".ratchet"), 0700); err != nil {
		t.Fatalf("mkdir config dir: %v", err)
	}

	svc, err := NewService(context.Background())
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	state, err := svc.AddTrustGrant(context.Background(), &pb.AddTrustRuleReq{
		Pattern: "bash:go test *",
		Action:  "allow",
		Scope:   "repo",
	})
	if err != nil {
		t.Fatalf("AddTrustGrant: %v", err)
	}
	if !hasTrustGrant(state.Grants, "bash:go test *", "allow", "repo", "operator") {
		t.Fatalf("state grants missing persisted allow: %+v", state.Grants)
	}
	if len(state.Grants) != 1 {
		t.Fatalf("grant count = %d, want 1", len(state.Grants))
	}
	if state.Grants[0].CreatedAt == nil {
		t.Fatalf("created_at is nil for grant: %+v", state.Grants[0])
	}
	if svc.engine != nil {
		svc.engine.Close()
	}

	reloaded, err := NewService(context.Background())
	if err != nil {
		t.Fatalf("reload NewService: %v", err)
	}
	t.Cleanup(func() {
		if reloaded.engine != nil {
			reloaded.engine.Close()
		}
	})
	state, err = reloaded.GetTrustState(context.Background(), &pb.Empty{})
	if err != nil {
		t.Fatalf("GetTrustState after reload: %v", err)
	}
	if !hasTrustGrant(state.Grants, "bash:go test *", "allow", "repo", "operator") {
		t.Fatalf("reloaded state missing persisted allow: %+v", state.Grants)
	}

	state, err = reloaded.RevokeTrustGrant(context.Background(), &pb.RevokeTrustGrantReq{
		Pattern: "bash:go test *",
		Scope:   "repo",
	})
	if err != nil {
		t.Fatalf("RevokeTrustGrant: %v", err)
	}
	if hasTrustGrant(state.Grants, "bash:go test *", "allow", "repo", "operator") {
		t.Fatalf("revoked grant still present: %+v", state.Grants)
	}
	state, err = reloaded.RevokeTrustGrant(context.Background(), &pb.RevokeTrustGrantReq{
		Pattern: "bash:go test *",
		Scope:   "repo",
	})
	if err != nil {
		t.Fatalf("idempotent RevokeTrustGrant: %v", err)
	}
	if len(state.Grants) != 0 {
		t.Fatalf("missing revoke changed grants unexpectedly: %+v", state.Grants)
	}
}

func TestServicePersistentTrustGrantValidation(t *testing.T) {
	svc := newTrustTestService(t, "")

	invalidAdds := []*pb.AddTrustRuleReq{
		{Pattern: "", Action: "allow", Scope: "repo"},
		{Pattern: "bash:*", Action: "", Scope: "repo"},
		{Pattern: "bash:*", Action: "ask", Scope: "repo"},
		{Pattern: "bash:*", Action: "maybe", Scope: "repo"},
	}
	for _, req := range invalidAdds {
		_, err := svc.AddTrustGrant(context.Background(), req)
		if status.Code(err) != codes.InvalidArgument {
			t.Fatalf("AddTrustGrant(%+v) code = %v, want InvalidArgument (err=%v)", req, status.Code(err), err)
		}
	}

	_, err := svc.RevokeTrustGrant(context.Background(), &pb.RevokeTrustGrantReq{})
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("RevokeTrustGrant(empty) code = %v, want InvalidArgument (err=%v)", status.Code(err), err)
	}
}

func TestServicePersistentTrustGrantStoreRequired(t *testing.T) {
	svc := newTrustTestService(t, "")
	svc.trustStore = nil

	_, err := svc.AddTrustGrant(context.Background(), &pb.AddTrustRuleReq{
		Pattern: "bash:go test *",
		Action:  "allow",
		Scope:   "repo",
	})
	if status.Code(err) != codes.FailedPrecondition {
		t.Fatalf("AddTrustGrant without store code = %v, want FailedPrecondition (err=%v)", status.Code(err), err)
	}

	_, err = svc.RevokeTrustGrant(context.Background(), &pb.RevokeTrustGrantReq{
		Pattern: "bash:go test *",
		Scope:   "repo",
	})
	if status.Code(err) != codes.FailedPrecondition {
		t.Fatalf("RevokeTrustGrant without store code = %v, want FailedPrecondition (err=%v)", status.Code(err), err)
	}
}

func TestServiceTrustStateReportsGrantListErrors(t *testing.T) {
	svc := newTrustTestService(t, "")
	if err := svc.engine.DB.Close(); err != nil {
		t.Fatalf("close DB: %v", err)
	}

	_, err := svc.GetTrustState(context.Background(), &pb.Empty{})
	if status.Code(err) != codes.Internal {
		t.Fatalf("GetTrustState with closed DB code = %v, want Internal (err=%v)", status.Code(err), err)
	}
}

func newTrustTestService(t *testing.T, configYAML string) *Service {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	dir := filepath.Join(home, ".ratchet")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatalf("mkdir config dir: %v", err)
	}
	if configYAML != "" {
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

func hasTrustGrant(grants []*pb.TrustGrant, pattern, action, scope, grantedBy string) bool {
	for _, grant := range grants {
		if grant.Pattern == pattern && grant.Action == action && grant.Scope == scope && grant.GrantedBy == grantedBy {
			return true
		}
	}
	return false
}
