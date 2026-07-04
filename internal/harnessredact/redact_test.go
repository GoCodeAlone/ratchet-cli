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
	for _, marker := range []string{"<home>", "<workspace>", "<temp>", "<prompt>", "<trust>"} {
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
	for _, marker := range []string{"<home>", "<workspace>", "<temp>", "<executable>", "<artifact>", "<prompt>", "<trust>"} {
		if !strings.Contains(got, marker) {
			t.Fatalf("redacted docs/command payload missing marker %q:\n%s", marker, got)
		}
	}
}

func TestReleaseGuardRedaction(t *testing.T) {
	r := New(
		"/home/dev",
		"/home/dev/work/ratchet-cli",
		"/tmp/ratchet-releaseguard-123",
		"/tmp/ratchet-releaseguard-123/ratchet.sock",
		"/tmp/ratchet-releaseguard-123/ratchet",
		"/tmp/ratchet-releaseguard-123/dist/ratchet_darwin_arm64.tar.gz",
		"release prompt secret",
		"release trust body",
	)
	payloads := []string{
		"goreleaser snapshot failed in /home/dev/work/ratchet-cli with release prompt secret",
		"manifest artifact /tmp/ratchet-releaseguard-123/dist/ratchet_darwin_arm64.tar.gz contains smoke token",
		"draft-assets output from /tmp/ratchet-releaseguard-123/ratchet wrote release trust body",
		"tap-preflight inspected /home/dev and /tmp/ratchet-releaseguard-123/ratchet.sock",
		"workflow-command failed under /tmp/ratchet-releaseguard-123",
	}
	got := r.String(strings.Join(payloads, "\n"))
	for _, raw := range []string{
		"/home/dev",
		"/home/dev/work/ratchet-cli",
		"/tmp/ratchet-releaseguard-123",
		"release prompt secret",
		"release trust body",
	} {
		if strings.Contains(got, raw) {
			t.Fatalf("redacted releaseguard payload still contains %q:\n%s", raw, got)
		}
	}
	for _, marker := range []string{"<home>", "<workspace>", "<temp>", "<socket>", "<executable>", "<artifact>", "<prompt>", "<trust>"} {
		if !strings.Contains(got, marker) {
			t.Fatalf("redacted releaseguard payload missing marker %q:\n%s", marker, got)
		}
	}
}
