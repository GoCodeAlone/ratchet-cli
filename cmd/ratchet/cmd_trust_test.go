package main

import (
	"context"
	"strings"
	"testing"

	pb "github.com/GoCodeAlone/ratchet-cli/internal/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func TestHandleTrustListAndGrants(t *testing.T) {
	fake := &fakeTrustCLIClient{state: trustCLIState()}
	withFakeTrustCLIClient(t, fake)

	out := captureStdout(t, func() {
		handleTrust([]string{"list"})
	})
	for _, want := range []string{"Mode: conservative", "Runtime rules:", "bash:go *", "Persistent grants:", "bash:go test *"} {
		if !strings.Contains(out, want) {
			t.Fatalf("trust list output missing %q:\n%s", want, out)
		}
	}

	out = captureStdout(t, func() {
		handleTrust([]string{"grants"})
	})
	for _, want := range []string{"Persistent grants:", "operator", "bash:go test *"} {
		if !strings.Contains(out, want) {
			t.Fatalf("trust grants output missing %q:\n%s", want, out)
		}
	}
}

func TestHandleTrustRuntimeAndPersistentMutations(t *testing.T) {
	fake := &fakeTrustCLIClient{state: trustCLIState()}
	withFakeTrustCLIClient(t, fake)

	out := captureStdout(t, func() {
		handleTrust([]string{"allow", "bash:go vet *", "--scope", "repo"})
	})
	if !strings.Contains(out, "Added runtime allow rule") || fake.runtimePattern != "bash:go vet *" || fake.runtimeScope != "repo" {
		t.Fatalf("runtime allow output/state = %q / %+v", out, fake)
	}

	out = captureStdout(t, func() {
		handleTrust([]string{"persist", "deny", "bash:rm *"})
	})
	if !strings.Contains(out, "Persisted deny grant") || fake.grantPattern != "bash:rm *" || fake.grantScope != "global" {
		t.Fatalf("persist deny output/state = %q / %+v", out, fake)
	}

	out = captureStdout(t, func() {
		handleTrust([]string{"revoke", "bash:rm *"})
	})
	if !strings.Contains(out, "Revoked persistent trust grant") || fake.revokedPattern != "bash:rm *" || fake.revokedScope != "global" {
		t.Fatalf("revoke output/state = %q / %+v", out, fake)
	}
}

func TestHandleTrustUsageValidation(t *testing.T) {
	fake := &fakeTrustCLIClient{state: trustCLIState()}
	withFakeTrustCLIClient(t, fake)

	cases := [][]string{
		{},
		{"allow", "--scope"},
		{"persist"},
		{"persist", "ask", "bash:*"},
		{"persist", "allow", "--scope"},
		{"revoke", "--scope"},
	}
	for _, args := range cases {
		out := captureStdout(t, func() {
			handleTrust(args)
		})
		if !strings.Contains(strings.ToLower(out), "usage:") {
			t.Fatalf("handleTrust(%v) output missing usage:\n%s", args, out)
		}
	}
}

func withFakeTrustCLIClient(t *testing.T, fake *fakeTrustCLIClient) {
	t.Helper()
	old := ensureTrustClient
	ensureTrustClient = func() (trustCLIClient, error) {
		return fake, nil
	}
	t.Cleanup(func() { ensureTrustClient = old })
}

func trustCLIState() *pb.TrustState {
	return &pb.TrustState{
		Mode: "conservative",
		Rules: []*pb.TrustRule{
			{Pattern: "bash:go *", Action: "allow", Scope: "repo"},
		},
		Grants: []*pb.TrustGrant{
			{
				Id:        1,
				Pattern:   "bash:go test *",
				Action:    "allow",
				Scope:     "repo",
				GrantedBy: "operator",
				CreatedAt: timestamppb.Now(),
			},
		},
	}
}

type fakeTrustCLIClient struct {
	state          *pb.TrustState
	runtimePattern string
	runtimeAction  string
	runtimeScope   string
	grantPattern   string
	grantAction    string
	grantScope     string
	revokedPattern string
	revokedScope   string
}

func (f *fakeTrustCLIClient) Close() error {
	return nil
}

func (f *fakeTrustCLIClient) GetTrustState(context.Context) (*pb.TrustState, error) {
	return f.state, nil
}

func (f *fakeTrustCLIClient) AddTrustRule(_ context.Context, pattern, action, scope string) (*pb.TrustState, error) {
	f.runtimePattern = pattern
	f.runtimeAction = action
	f.runtimeScope = scope
	return f.state, nil
}

func (f *fakeTrustCLIClient) ResetTrust(context.Context) (*pb.TrustState, error) {
	return f.state, nil
}

func (f *fakeTrustCLIClient) AddTrustGrant(_ context.Context, pattern, action, scope string) (*pb.TrustState, error) {
	f.grantPattern = pattern
	f.grantAction = action
	f.grantScope = scope
	return f.state, nil
}

func (f *fakeTrustCLIClient) RevokeTrustGrant(_ context.Context, pattern, scope string) (*pb.TrustState, error) {
	f.revokedPattern = pattern
	f.revokedScope = scope
	return f.state, nil
}
