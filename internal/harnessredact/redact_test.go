package harnessredact

import (
	"strings"
	"testing"
)

func TestRedact(t *testing.T) {
	r := New(
		"/home/dev",
		"/home/dev/work/ratchet-cli",
		"/tmp/ratchet-smoke-123",
		"/tmp/ratchet-smoke-123/ratchet.sock",
		"/tmp/ratchet-smoke-123/ratchet-tui-smoke",
		"/tmp/ratchet-smoke-123/dist/ratchet_linux_amd64.tar.gz",
		"prompt body secret",
		"smoke:persist-allow",
	)
	in := strings.Join([]string{
		"/home/dev/work/ratchet-cli",
		"/home/dev",
		"/tmp/ratchet-smoke-123",
		"/tmp/ratchet-smoke-123/ratchet.sock",
		"/tmp/ratchet-smoke-123/ratchet-tui-smoke",
		"/tmp/ratchet-smoke-123/dist/ratchet_linux_amd64.tar.gz",
		"prompt body secret",
		"smoke:persist-allow",
	}, "\n")

	got := r.String(in)
	for _, raw := range []string{
		"/home/dev",
		"/home/dev/work/ratchet-cli",
		"/tmp/ratchet-smoke-123",
		"prompt body secret",
		"smoke:persist-allow",
	} {
		if strings.Contains(got, raw) {
			t.Fatalf("redacted output still contains %q:\n%s", raw, got)
		}
	}
	for _, marker := range []string{"<home>", "<workspace>", "<temp>", "<prompt>", "<trust-body>"} {
		if !strings.Contains(got, marker) {
			t.Fatalf("redacted output missing marker %q:\n%s", marker, got)
		}
	}
}

func TestRedactDocsAndCommandPayloads(t *testing.T) {
	r := New(
		"/home/dev",
		"/home/dev/work/ratchet-cli",
		"/tmp/ratchet-smoke-123",
		"/tmp/ratchet-smoke-123/ratchet.sock",
		"/tmp/ratchet-smoke-123/ratchet",
		"/tmp/ratchet-smoke-123/dist/ratchet_windows_amd64.zip",
		"doc prompt secret",
		"trust secret body",
	)
	payloads := []string{
		"docs guard failure in /home/dev/work/ratchet-cli/docs/harness-emulation.md with doc prompt secret",
		"home path: /home/dev",
		"temp path: /tmp/ratchet-smoke-123",
		"command failed: /tmp/ratchet-smoke-123/ratchet help wrote trust secret body",
		"artifact manifest: /tmp/ratchet-smoke-123/dist/ratchet_windows_amd64.zip",
	}
	got := r.String(strings.Join(payloads, "\n"))
	for _, raw := range []string{
		"/home/dev",
		"/home/dev/work/ratchet-cli",
		"/tmp/ratchet-smoke-123",
		"doc prompt secret",
		"trust secret body",
	} {
		if strings.Contains(got, raw) {
			t.Fatalf("redacted docs/command payload still contains %q:\n%s", raw, got)
		}
	}
	for _, marker := range []string{"<home>", "<workspace>", "<temp>", "<executable>", "<artifact>", "<prompt>", "<trust-body>"} {
		if !strings.Contains(got, marker) {
			t.Fatalf("redacted docs/command payload missing marker %q:\n%s", marker, got)
		}
	}
}
