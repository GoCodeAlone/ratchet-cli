package commands

import (
	"context"
	"errors"
	"strings"
	"testing"

	pb "github.com/GoCodeAlone/ratchet-cli/internal/proto"
)

func TestModeCommandUsesDaemonTrustState(t *testing.T) {
	fake := &fakeTrustClient{}
	result := modeCmd([]string{"locked"}, fake)

	if fake.mode != "locked" {
		t.Fatalf("SetTrustMode mode = %q, want locked", fake.mode)
	}
	if !resultContains(result, "Mode switched to locked") {
		t.Fatalf("result lines = %v", result.Lines)
	}
}

func TestTrustListUsesDaemonState(t *testing.T) {
	fake := &fakeTrustClient{
		state: &pb.TrustState{
			Mode:  "conservative",
			Rules: []*pb.TrustRule{{Pattern: "bash:go *", Action: "allow", Scope: "global"}},
		},
	}
	result := trustCmd([]string{"list"}, fake)

	if fake.listCalls != 1 {
		t.Fatalf("GetTrustState calls = %d, want 1", fake.listCalls)
	}
	if !resultContains(result, "Mode: conservative") || !resultContains(result, "bash:go *") {
		t.Fatalf("result lines = %v", result.Lines)
	}
}

func TestTrustAllowDenyAndResetUseDaemon(t *testing.T) {
	fake := &fakeTrustClient{}

	result := trustCmd([]string{"allow", "bash:go test ./...", "--scope", "repo"}, fake)
	if fake.rulePattern != "bash:go test ./..." || fake.ruleAction != "allow" || fake.ruleScope != "repo" {
		t.Fatalf("allow call = pattern=%q action=%q scope=%q", fake.rulePattern, fake.ruleAction, fake.ruleScope)
	}
	if !resultContains(result, "Added allow rule: bash:go test ./...") {
		t.Fatalf("allow result lines = %v", result.Lines)
	}

	result = trustCmd([]string{"deny", "bash:rm -rf *"}, fake)
	if fake.rulePattern != "bash:rm -rf *" || fake.ruleAction != "deny" || fake.ruleScope != "global" {
		t.Fatalf("deny call = pattern=%q action=%q scope=%q", fake.rulePattern, fake.ruleAction, fake.ruleScope)
	}
	if !resultContains(result, "Added deny rule: bash:rm -rf *") {
		t.Fatalf("deny result lines = %v", result.Lines)
	}

	result = trustCmd([]string{"reset"}, fake)
	if fake.resetCalls != 1 {
		t.Fatalf("ResetTrust calls = %d, want 1", fake.resetCalls)
	}
	if !resultContains(result, "Trust rules reset to config defaults") {
		t.Fatalf("reset result lines = %v", result.Lines)
	}
}

func TestTrustRuleScopeRequiresValue(t *testing.T) {
	fake := &fakeTrustClient{}
	for _, result := range []*Result{
		trustCmd([]string{"allow", "--scope"}, fake),
		trustCmd([]string{"deny", "bash:*", "--scope"}, fake),
	} {
		if !resultContains(result, "Usage: /trust") {
			t.Fatalf("result lines = %v", result.Lines)
		}
	}
	if fake.rulePattern != "" || fake.ruleAction != "" || fake.ruleScope != "" {
		t.Fatalf("daemon should not be called, got pattern=%q action=%q scope=%q", fake.rulePattern, fake.ruleAction, fake.ruleScope)
	}
}

func TestTrustCommandsRequireDaemon(t *testing.T) {
	for _, result := range []*Result{
		modeCmd([]string{"locked"}, nil),
		trustCmd([]string{"list"}, nil),
		trustCmd([]string{"allow", "bash:go *"}, nil),
		trustCmd([]string{"deny", "bash:rm *"}, nil),
		trustCmd([]string{"reset"}, nil),
	} {
		if !resultContains(result, "Not connected to daemon") {
			t.Fatalf("result lines = %v", result.Lines)
		}
	}
}

func TestTrustCommandReportsDaemonErrors(t *testing.T) {
	fake := &fakeTrustClient{err: errors.New("daemon unavailable")}
	result := trustCmd([]string{"list"}, fake)
	if !resultContains(result, "Error: daemon unavailable") {
		t.Fatalf("result lines = %v", result.Lines)
	}
}

type fakeTrustClient struct {
	state       *pb.TrustState
	err         error
	mode        string
	listCalls   int
	resetCalls  int
	rulePattern string
	ruleAction  string
	ruleScope   string
}

func (f *fakeTrustClient) GetTrustState(context.Context) (*pb.TrustState, error) {
	f.listCalls++
	if f.err != nil {
		return nil, f.err
	}
	if f.state != nil {
		return f.state, nil
	}
	return &pb.TrustState{Mode: "locked"}, nil
}

func (f *fakeTrustClient) SetTrustMode(_ context.Context, mode string) (*pb.TrustState, error) {
	f.mode = mode
	if f.err != nil {
		return nil, f.err
	}
	return &pb.TrustState{Mode: mode}, nil
}

func (f *fakeTrustClient) AddTrustRule(_ context.Context, pattern, action, scope string) (*pb.TrustState, error) {
	f.rulePattern = pattern
	f.ruleAction = action
	f.ruleScope = scope
	if f.err != nil {
		return nil, f.err
	}
	return &pb.TrustState{Mode: "custom", Rules: []*pb.TrustRule{{Pattern: pattern, Action: action, Scope: scope}}}, nil
}

func (f *fakeTrustClient) ResetTrust(context.Context) (*pb.TrustState, error) {
	f.resetCalls++
	if f.err != nil {
		return nil, f.err
	}
	return &pb.TrustState{Mode: "conservative"}, nil
}

func resultContains(result *Result, needle string) bool {
	if result == nil {
		return false
	}
	return strings.Contains(strings.Join(result.Lines, "\n"), needle)
}
