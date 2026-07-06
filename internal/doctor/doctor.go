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

// CollectOptions allows tests and future callers to inject local discovery.
type CollectOptions struct {
	Version           string
	Commit            string
	Date              string
	DaemonStatus      string
	Executable        func() (string, error)
	ResolveExecutable func(string) (string, error)
	WorkingDir        func() (string, error)
	HomeDir           func() (string, error)
	StateHome         string
}

// Collect builds a Report without starting daemons or reading credentials.
func Collect(version, commit, date, daemonStatus string) Report {
	return CollectWithOptions(CollectOptions{
		Version:           version,
		Commit:            commit,
		Date:              date,
		DaemonStatus:      daemonStatus,
		Executable:        os.Executable,
		ResolveExecutable: filepath.EvalSymlinks,
		WorkingDir:        os.Getwd,
		HomeDir:           os.UserHomeDir,
		StateHome:         os.Getenv("XDG_STATE_HOME"),
	})
}

// CollectWithOptions builds a Report with injectable local discovery functions.
func CollectWithOptions(opts CollectOptions) Report {
	exe, exeErr := callDiscovery(opts.Executable)
	if exe != "" {
		resolved, resolveErr := resolveExecutable(exe, opts.ResolveExecutable)
		if resolveErr != nil {
			exeErr = errorsJoin(exeErr, fmt.Errorf("resolve executable symlink: %w", resolveErr))
		} else if resolved != "" {
			exe = resolved
		}
	}
	wd, wdErr := callDiscovery(opts.WorkingDir)
	home, homeErr := callDiscovery(opts.HomeDir)
	dataDir, configPath := ratchetDataPaths(home)
	stateDir := ratchetStateDir(home, opts.StateHome)
	report := Report{
		Version:      opts.Version,
		Commit:       opts.Commit,
		Date:         opts.Date,
		Executable:   exe,
		Install:      InstallSummary(exe),
		HomeDir:      home,
		WorkingDir:   wd,
		ConfigPath:   configPath,
		DataDir:      dataDir,
		StateDir:     stateDir,
		DaemonStatus: opts.DaemonStatus,
	}
	appendDiscoveryWarning(&report, "executable", exeErr)
	appendDiscoveryWarning(&report, "working directory", wdErr)
	appendDiscoveryWarning(&report, "home directory", homeErr)
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

func ratchetDataPaths(home string) (dataDir, configPath string) {
	if home == "" {
		return "", ""
	}
	dataDir = filepath.Join(home, ".ratchet")
	return dataDir, filepath.Join(dataDir, "config.yaml")
}

func ratchetStateDir(home, stateHome string) string {
	if stateHome != "" {
		return filepath.Join(stateHome, "ratchet")
	}
	if home == "" {
		return ""
	}
	return filepath.Join(home, ".local", "state", "ratchet")
}

func callDiscovery(fn func() (string, error)) (string, error) {
	if fn == nil {
		return "", nil
	}
	return fn()
}

func appendDiscoveryWarning(report *Report, label string, err error) {
	if report == nil || err == nil {
		return
	}
	report.Warnings = append(report.Warnings, fmt.Sprintf("%s discovery failed: %v", label, err))
}

func resolveExecutable(path string, resolver func(string) (string, error)) (string, error) {
	if resolver == nil {
		return path, nil
	}
	return resolver(path)
}

func errorsJoin(existing, next error) error {
	switch {
	case existing == nil:
		return next
	case next == nil:
		return existing
	default:
		return fmt.Errorf("%v; %w", existing, next)
	}
}

func valueOrUnknown(value string) string {
	if strings.TrimSpace(value) == "" {
		return "unknown"
	}
	return value
}
