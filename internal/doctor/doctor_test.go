package doctor

import (
	"errors"
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

func TestCollectWarningsWhenLocalPathDiscoveryFails(t *testing.T) {
	report := CollectWithOptions(CollectOptions{
		Version:      "dev",
		Commit:       "unknown",
		Date:         "unknown",
		DaemonStatus: "daemon is not running",
		Executable: func() (string, error) {
			return "", errors.New("executable unavailable")
		},
		WorkingDir: func() (string, error) {
			return "", errors.New("working directory unavailable")
		},
		HomeDir: func() (string, error) {
			return "", errors.New("home unavailable")
		},
	})

	if report.ConfigPath != "" || report.DataDir != "" || report.StateDir != "" {
		t.Fatalf("paths should stay empty without home/state roots: %#v", report)
	}
	joined := strings.Join(report.Warnings, "\n")
	for _, want := range []string{"executable unavailable", "working directory unavailable", "home unavailable"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("warnings missing %q: %#v", want, report.Warnings)
		}
	}
}

func TestCollectClassifiesResolvedHomebrewFormulaSymlink(t *testing.T) {
	report := CollectWithOptions(CollectOptions{
		Version:      "0.30.9",
		Commit:       "abc123",
		Date:         "2026-07-06T00:00:00Z",
		DaemonStatus: "daemon is not running",
		Executable: func() (string, error) {
			return "/opt/homebrew/bin/ratchet", nil
		},
		ResolveExecutable: func(path string) (string, error) {
			if path != "/opt/homebrew/bin/ratchet" {
				t.Fatalf("resolver path = %q", path)
			}
			return "/opt/homebrew/Cellar/ratchet-cli/0.30.9/bin/ratchet", nil
		},
		WorkingDir: func() (string, error) {
			return "/tmp/work", nil
		},
		HomeDir: func() (string, error) {
			return "/tmp/home", nil
		},
	})

	if !strings.Contains(strings.ToLower(report.Install), "homebrew formula") {
		t.Fatalf("install = %q, want Homebrew Formula", report.Install)
	}
	if report.Executable != "/opt/homebrew/Cellar/ratchet-cli/0.30.9/bin/ratchet" {
		t.Fatalf("executable = %q", report.Executable)
	}
}

func TestCollectWarnsWhenExecutableSymlinkResolutionFails(t *testing.T) {
	resolveErr := errors.New("readlink denied")
	report := CollectWithOptions(CollectOptions{
		Version:      "0.30.9",
		Commit:       "abc123",
		Date:         "2026-07-06T00:00:00Z",
		DaemonStatus: "daemon is not running",
		Executable: func() (string, error) {
			return "/opt/homebrew/bin/ratchet", nil
		},
		ResolveExecutable: func(string) (string, error) {
			return "", resolveErr
		},
		WorkingDir: func() (string, error) {
			return "/tmp/work", nil
		},
		HomeDir: func() (string, error) {
			return "/tmp/home", nil
		},
	})

	if report.Executable != "/opt/homebrew/bin/ratchet" {
		t.Fatalf("executable = %q, want original symlink path", report.Executable)
	}
	joined := strings.Join(report.Warnings, "\n")
	if !strings.Contains(joined, "resolve executable symlink") || !strings.Contains(joined, resolveErr.Error()) {
		t.Fatalf("warnings missing resolver failure: %#v", report.Warnings)
	}
}
