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
