package doctor

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// Report is a credential-free local health snapshot for support and CI smoke.
type Report struct {
	Version      string   `json:"version"`
	Commit       string   `json:"commit"`
	Date         string   `json:"date"`
	Executable   string   `json:"executable"`
	Install      string   `json:"install"`
	HomeDir      string   `json:"home_dir"`
	WorkingDir   string   `json:"working_dir"`
	ConfigPath   string   `json:"config_path"`
	DataDir      string   `json:"data_dir"`
	StateDir     string   `json:"state_dir"`
	DaemonStatus string   `json:"daemon_status"`
	Warnings     []string `json:"warnings,omitempty"`
}

// Collect builds a Report without starting daemons or reading credentials.
func Collect(version, commit, date, daemonStatus string) Report {
	exe, _ := os.Executable()
	wd, _ := os.Getwd()
	home, _ := os.UserHomeDir()
	dataDir := filepath.Join(home, ".ratchet")
	stateDir := ratchetStateDir(home)
	report := Report{
		Version:      version,
		Commit:       commit,
		Date:         date,
		Executable:   exe,
		Install:      InstallSummary(exe),
		HomeDir:      home,
		WorkingDir:   wd,
		ConfigPath:   filepath.Join(dataDir, "config.yaml"),
		DataDir:      dataDir,
		StateDir:     stateDir,
		DaemonStatus: daemonStatus,
	}
	if strings.Contains(strings.ToLower(report.Install), "homebrew formula") {
		report.Warnings = append(report.Warnings, "Homebrew cask is preferred; Formula is kept for compatibility with older installs.")
	}
	return report
}

// InstallSummary classifies common install layouts without invoking package managers.
func InstallSummary(executable string) string {
	lower := strings.ToLower(filepath.ToSlash(executable))
	switch {
	case strings.Contains(lower, "/cellar/ratchet-cli/"):
		return "Homebrew Formula"
	case strings.Contains(lower, "/caskroom/ratchet-cli/"), strings.Contains(lower, "/ratchet.app/contents/macos/"):
		return "Homebrew cask"
	case strings.Contains(lower, "/go/bin/"):
		return "go install"
	case executable == "":
		return "unknown"
	default:
		return fmt.Sprintf("%s binary", runtime.GOOS)
	}
}

// TextLines renders a stable human-readable report.
func (r Report) TextLines() []string {
	lines := []string{
		"Ratchet doctor",
		"",
		fmt.Sprintf("version: %s (%s, %s)", valueOrUnknown(r.Version), valueOrUnknown(r.Commit), valueOrUnknown(r.Date)),
		fmt.Sprintf("install: %s", valueOrUnknown(r.Install)),
		fmt.Sprintf("executable: %s", valueOrUnknown(r.Executable)),
		fmt.Sprintf("daemon: %s", valueOrUnknown(r.DaemonStatus)),
		fmt.Sprintf("home: %s", valueOrUnknown(r.HomeDir)),
		fmt.Sprintf("workdir: %s", valueOrUnknown(r.WorkingDir)),
		fmt.Sprintf("config: %s", valueOrUnknown(r.ConfigPath)),
		fmt.Sprintf("data: %s", valueOrUnknown(r.DataDir)),
		fmt.Sprintf("state: %s", valueOrUnknown(r.StateDir)),
	}
	if len(r.Warnings) > 0 {
		lines = append(lines, "", "Warnings")
		for _, warning := range r.Warnings {
			lines = append(lines, "- "+warning)
		}
	}
	return lines
}

func ratchetStateDir(home string) string {
	if state := os.Getenv("XDG_STATE_HOME"); state != "" {
		return filepath.Join(state, "ratchet")
	}
	return filepath.Join(home, ".local", "state", "ratchet")
}

func valueOrUnknown(value string) string {
	if strings.TrimSpace(value) == "" {
		return "unknown"
	}
	return value
}
