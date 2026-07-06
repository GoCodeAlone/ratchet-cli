package doctor

import (
	"strings"
	"testing"
)

func TestInstallSummaryDetectsHomebrewFormulaAndCask(t *testing.T) {
	tests := []struct {
		name string
		path string
		want string
	}{
		{
			name: "formula",
			path: "/opt/homebrew/Cellar/ratchet-cli/0.30.8/bin/ratchet",
			want: "homebrew formula",
		},
		{
			name: "cask",
			path: "/Applications/ratchet.app/Contents/MacOS/ratchet",
			want: "homebrew cask",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := InstallSummary(tt.path)
			if !strings.Contains(strings.ToLower(got), tt.want) {
				t.Fatalf("InstallSummary(%q) = %q, want %q", tt.path, got, tt.want)
			}
		})
	}
}

func TestReportIncludesCredentialFreeHealthFields(t *testing.T) {
	report := Report{
		Version:      "0.30.8",
		Commit:       "abc123",
		Date:         "2026-07-06T00:00:00Z",
		Executable:   "/tmp/ratchet",
		HomeDir:      "/tmp/home",
		WorkingDir:   "/tmp/work",
		ConfigPath:   "/tmp/home/.ratchet/config.yaml",
		DataDir:      "/tmp/home/.ratchet",
		StateDir:     "/tmp/home/.local/state/ratchet",
		DaemonStatus: "daemon is not running",
	}

	lines := report.TextLines()
	joined := strings.Join(lines, "\n")

	for _, want := range []string{"version: 0.30.8", "daemon: daemon is not running", "config:", "state:"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("doctor report missing %q:\n%s", want, joined)
		}
	}
	if strings.Contains(joined, "provider") || strings.Contains(joined, "token") {
		t.Fatalf("doctor report should stay credential-free:\n%s", joined)
	}
}
