package main

import (
	"encoding/json"
	"strings"
	"testing"

	pb "github.com/GoCodeAlone/ratchet-cli/internal/proto"
)

func TestHarnessSmokeBlackboardCLI(t *testing.T) {
	if raceEnabled {
		t.Skip("binary-build smoke is covered by normal tests; skip expensive subprocess build under -race")
	}
	bin := buildRatchetSmokeBinary(t)
	home := t.TempDir()
	t.Cleanup(func() {
		_, _ = runRatchetSmoke(t, bin, home, "daemon", "stop")
	})

	out, err := runRatchetSmoke(t, bin, home, "blackboard", "write", "coordination", "status", "ready", "--author", "test-agent")
	if err != nil {
		t.Fatalf("blackboard write: %v\n%s", err, out)
	}
	if !strings.Contains(out, "coordination/status") || !strings.Contains(out, "rev=") {
		t.Fatalf("write output = %q", out)
	}

	out, err = runRatchetSmoke(t, bin, home, "blackboard", "read", "coordination", "status")
	if err != nil {
		t.Fatalf("blackboard read: %v\n%s", err, out)
	}
	if strings.TrimSpace(out) != "ready" {
		t.Fatalf("read output = %q, want ready", out)
	}

	out, err = runRatchetSmoke(t, bin, home, "blackboard", "list", "coordination", "--json")
	if err != nil {
		t.Fatalf("blackboard list json: %v\n%s", err, out)
	}
	var payload pb.BlackboardListResp
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		t.Fatalf("decode list json %q: %v", out, err)
	}
	if len(payload.Entries) != 1 || payload.Entries[0].GetKey() != "status" || payload.Entries[0].GetValue() != "ready" {
		t.Fatalf("list payload = %#v", &payload)
	}
}
