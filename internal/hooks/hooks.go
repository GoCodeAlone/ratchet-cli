package hooks

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"text/template"

	"gopkg.in/yaml.v3"
)

// Event is a lifecycle event that can trigger hooks.
type Event string

const (
	PreEdit             Event = "pre-edit"
	PostEdit            Event = "post-edit"
	PreCommand          Event = "pre-command"
	PostCommand         Event = "post-command"
	PreSession          Event = "pre-session"
	PostSession         Event = "post-session"
	PreCommit           Event = "pre-commit"
	PostCommit          Event = "post-commit"
	OnError             Event = "on-error"
	OnToolCall          Event = "on-tool-call"
	OnPermissionRequest Event = "on-permission-request"

	// Plan lifecycle
	PrePlan  Event = "pre-plan"
	PostPlan Event = "post-plan"

	// Fleet lifecycle
	PreFleet  Event = "pre-fleet"
	PostFleet Event = "post-fleet"

	// Agent lifecycle
	OnAgentSpawn    Event = "on-agent-spawn"
	OnAgentComplete Event = "on-agent-complete"

	// Token and cron events
	OnTokenLimit Event = "on-token-limit"
	OnCronTick   Event = "on-cron-tick"
)

// AllEvents lists every valid lifecycle event for documentation and validation.
var AllEvents = []Event{
	PreEdit, PostEdit,
	PreCommand, PostCommand,
	PreSession, PostSession,
	PreCommit, PostCommit,
	OnError, OnToolCall, OnPermissionRequest,
	PrePlan, PostPlan,
	PreFleet, PostFleet,
	OnAgentSpawn, OnAgentComplete,
	OnTokenLimit, OnCronTick,
}

// SourceKind identifies where a hook declaration came from.
type SourceKind string

const (
	SourceUser    SourceKind = "user"
	SourceProject SourceKind = "project"
	SourcePlugin  SourceKind = "plugin"
)

// Hook defines a single hook command with an optional glob pattern.
type Hook struct {
	Command        string `yaml:"command"`
	CommandWindows string `yaml:"command_windows,omitempty"`
	Glob           string `yaml:"glob,omitempty"`

	SourceKind          SourceKind `yaml:"-"`
	SourceID            string     `yaml:"-"`
	SourcePath          string     `yaml:"-"`
	PluginName          string     `yaml:"-"`
	PluginVersion       string     `yaml:"-"`
	Hash                string     `yaml:"-"`
	Trusted             bool       `yaml:"-"`
	Disabled            bool       `yaml:"-"`
	UnsupportedPlatform bool       `yaml:"-"`
}

// HookConfig holds the full hooks configuration.
type HookConfig struct {
	Hooks map[Event][]Hook `yaml:"hooks"`
}

// LoadOptions controls hook loading and trust annotation.
type LoadOptions struct {
	WorkingDir string
	TrustStore *TrustStore
}

// Load reads hook configs from ~/.ratchet/hooks.yaml and .ratchet/hooks.yaml.
// Project-level hooks (.ratchet/hooks.yaml) override global ones.
func Load(workingDir string) (*HookConfig, error) {
	return LoadWithOptions(LoadOptions{WorkingDir: workingDir})
}

// LoadWithOptions reads hook configs and annotates each hook with source and
// trust metadata. User hooks remain trusted by default for compatibility.
func LoadWithOptions(opts LoadOptions) (*HookConfig, error) {
	cfg := &HookConfig{
		Hooks: make(map[Event][]Hook),
	}

	home, _ := os.UserHomeDir()
	sources := []struct {
		kind SourceKind
		id   string
		path string
	}{
		{
			kind: SourceUser,
			id:   "user:hooks.yaml",
			path: filepath.Join(home, ".ratchet", "hooks.yaml"),
		},
		{
			kind: SourceProject,
			id:   "project:.ratchet/hooks.yaml",
			path: filepath.Join(opts.WorkingDir, ".ratchet", "hooks.yaml"),
		},
	}

	for _, src := range sources {
		data, err := os.ReadFile(src.path)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", src.path, err)
		}
		var fileCfg HookConfig
		if err := yaml.Unmarshal(data, &fileCfg); err != nil {
			return nil, fmt.Errorf("parse %s: %w", src.path, err)
		}
		fileCfg.AnnotateSource(SourceMetadata{
			Kind:           src.kind,
			ID:             src.id,
			Path:           src.path,
			TrustByDefault: src.kind == SourceUser,
			TrustStore:     opts.TrustStore,
		})
		// Merge: later paths append to existing hooks
		for event, hooks := range fileCfg.Hooks {
			cfg.Hooks[event] = append(cfg.Hooks[event], hooks...)
		}
	}
	return cfg, nil
}

// SourceMetadata describes the source applied to a loaded HookConfig.
type SourceMetadata struct {
	Kind           SourceKind
	ID             string
	Path           string
	PluginName     string
	PluginVersion  string
	TrustByDefault bool
	TrustStore     *TrustStore
}

// AnnotateSource applies stable source and trust metadata to every hook.
func (hc *HookConfig) AnnotateSource(meta SourceMetadata) {
	if hc == nil {
		return
	}
	if hc.Hooks == nil {
		hc.Hooks = make(map[Event][]Hook)
	}
	for event, hookList := range hc.Hooks {
		for i := range hookList {
			h := &hookList[i]
			h.SourceKind = meta.Kind
			h.SourceID = meta.ID
			h.SourcePath = meta.Path
			h.PluginName = meta.PluginName
			h.PluginVersion = meta.PluginVersion
			h.Hash = h.DescriptorHash()
			h.Trusted = meta.TrustByDefault
			if meta.TrustStore != nil {
				h.Disabled = meta.TrustStore.IsDisabled(h.Hash)
				h.Trusted = h.Trusted || meta.TrustStore.IsTrusted(h.Hash)
				if h.Disabled {
					h.Trusted = false
				}
			}
			if _, ok := h.commandForGOOS(runtime.GOOS); !ok {
				h.UnsupportedPlatform = true
			}
		}
		hc.Hooks[event] = hookList
	}
}

// DescriptorHash returns a stable hash for trust decisions. It intentionally
// excludes SourcePath so trust can move across checkouts and machines.
func (h Hook) DescriptorHash() string {
	descriptor := struct {
		SourceKind    SourceKind `json:"source_kind"`
		SourceID      string     `json:"source_id"`
		Command       string     `json:"command"`
		CommandWindow string     `json:"command_windows,omitempty"`
		Glob          string     `json:"glob,omitempty"`
	}{
		SourceKind:    h.SourceKind,
		SourceID:      h.SourceID,
		Command:       h.Command,
		CommandWindow: h.CommandWindows,
		Glob:          h.Glob,
	}
	data, _ := json.Marshal(descriptor)
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func (h Hook) commandForGOOS(goos string) (string, bool) {
	if goos == "windows" {
		if h.CommandWindows != "" {
			return h.CommandWindows, true
		}
		if h.SourceKind != "" {
			return "", false
		}
	}
	if h.Command == "" {
		return "", false
	}
	return h.Command, true
}

func (h Hook) runnable() bool {
	if h.Disabled || h.UnsupportedPlatform {
		return false
	}
	return h.SourceKind == "" || h.Trusted
}

// Run executes all hooks for the given event, expanding templates with data.
// data keys include: "file", "command", "error", "tool", "session_id",
// "plan_id", "fleet_id", "agent_name", "agent_role", "cron_id",
// "tokens_used", "tokens_limit"
func (hc *HookConfig) Run(event Event, data map[string]string) error {
	hooks := hc.Hooks[event]
	for _, h := range hooks {
		if !h.runnable() {
			continue
		}

		// Check glob filter if set
		if h.Glob != "" {
			file := data["file"]
			if file != "" {
				matched, err := filepath.Match(h.Glob, filepath.Base(file))
				if err != nil || !matched {
					continue
				}
			}
		}

		command, ok := h.commandForGOOS(runtime.GOOS)
		if !ok {
			continue
		}

		// Expand command template with shell-escaped values to prevent injection.
		cmd, err := expandTemplate(command, shellEscapeData(data))
		if err != nil {
			return fmt.Errorf("expand hook command: %w", err)
		}

		out, err := execHookCommand(cmd).CombinedOutput()
		if err != nil {
			return fmt.Errorf("hook %s failed: %v\noutput: %s", event, err, out)
		}
	}
	return nil
}

func execHookCommand(command string) *exec.Cmd {
	if runtime.GOOS == "windows" {
		return exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-Command", command)
	}
	return exec.Command("sh", "-c", command)
}

// shellEscapeData returns a copy of data with each value single-quoted for
// safe interpolation into sh -c commands, preventing shell injection.
func shellEscapeData(data map[string]string) map[string]string {
	escaped := make(map[string]string, len(data))
	for k, v := range data {
		// Wrap in single quotes; escape embedded single quotes as '\''
		escaped[k] = "'" + strings.ReplaceAll(v, "'", "'\\''") + "'"
	}
	return escaped
}

func expandTemplate(tmpl string, data map[string]string) (string, error) {
	t, err := template.New("hook").Parse(tmpl)
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	if err := t.Execute(&buf, data); err != nil {
		return "", err
	}
	return buf.String(), nil
}
