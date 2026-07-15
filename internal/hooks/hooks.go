package hooks

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"text/template"
	"time"

	"gopkg.in/yaml.v3"
)

var (
	managedHookNow   = time.Now
	managedHookSince = time.Since
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

	SessionStart       Event = "session-start"
	SessionEnd         Event = "session-end"
	UserPromptSubmit   Event = "user-prompt-submit"
	Stop               Event = "stop"
	StopFailure        Event = "stop-failure"
	PreToolUse         Event = "pre-tool-use"
	PostToolUse        Event = "post-tool-use"
	PostToolUseFailure Event = "post-tool-use-failure"
	PermissionRequest  Event = "permission-request"
	PermissionDenied   Event = "permission-denied"
	PreCompact         Event = "pre-compact"
	PostCompact        Event = "post-compact"
	SubagentStart      Event = "subagent-start"
	SubagentStop       Event = "subagent-stop"
	WorkflowStart      Event = "workflow-start"
	WorkflowStop       Event = "workflow-stop"
	WorkflowFailure    Event = "workflow-failure"
	Notification       Event = "notification"
	ConfigChange       Event = "config-change"
	FileChanged        Event = "file-changed"

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
	SessionStart, SessionEnd,
	UserPromptSubmit, Stop, StopFailure,
	PreToolUse, PostToolUse, PostToolUseFailure,
	PermissionRequest, PermissionDenied,
	PreCompact, PostCompact,
	SubagentStart, SubagentStop,
	WorkflowStart, WorkflowStop, WorkflowFailure,
	Notification, ConfigChange, FileChanged,
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
	SourceManaged SourceKind = "managed"
)

// Hook defines a single hook command with an optional glob pattern.
type Hook struct {
	Command        string `yaml:"command"`
	CommandWindows string `yaml:"command_windows,omitempty"`
	Glob           string `yaml:"glob,omitempty"`

	Event               Event      `yaml:"-"`
	SourceKind          SourceKind `yaml:"-"`
	SourceID            string     `yaml:"-"`
	SourcePath          string     `yaml:"-"`
	PluginName          string     `yaml:"-"`
	PluginVersion       string     `yaml:"-"`
	Hash                string     `yaml:"-"`
	Trusted             bool       `yaml:"-"`
	Disabled            bool       `yaml:"-"`
	Suppressed          bool       `yaml:"-"`
	UnsupportedPlatform bool       `yaml:"-"`
}

// HookConfig holds the full hooks configuration.
type HookConfig struct {
	Hooks map[Event][]Hook `yaml:"hooks"`
}

// LoadOptions controls hook loading and trust annotation.
type LoadOptions struct {
	WorkingDir  string
	TrustStore  *TrustStore
	SkipUser    bool
	SkipProject bool
	ManagedPath string

	// ManagedReadFile is a test seam. Production callers must leave it nil so
	// administrator policy always passes through the platform secure reader.
	ManagedReadFile func(string) ([]byte, error)
}

// DefaultManagedPolicyPath returns the fixed platform administrator-policy path.
func DefaultManagedPolicyPath() (string, error) {
	return defaultManagedPolicyPath()
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
	allSources := []struct {
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
	sources := make([]struct {
		kind SourceKind
		id   string
		path string
	}, 0, len(allSources))
	for _, src := range allSources {
		if src.kind == SourceUser && opts.SkipUser {
			continue
		}
		if src.kind == SourceProject && opts.SkipProject {
			continue
		}
		sources = append(sources, src)
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
			h.Event = event
			h.SourceKind = meta.Kind
			h.SourceID = meta.ID
			h.SourcePath = meta.Path
			h.PluginName = meta.PluginName
			h.PluginVersion = meta.PluginVersion
			h.Hash = h.DescriptorHash()
			h.Trusted = meta.TrustByDefault || meta.Kind == SourceManaged
			h.Disabled = false
			if meta.TrustStore != nil && meta.Kind != SourceManaged {
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

// ApplyTrust refreshes hook hashes and trust decisions against the supplied
// store. This lets long-running daemons observe trust changes without restart.
func (hc *HookConfig) ApplyTrust(store *TrustStore) {
	if hc == nil {
		return
	}
	for event, hookList := range hc.Hooks {
		for i := range hookList {
			h := &hookList[i]
			if h.Event == "" {
				h.Event = event
			}
			h.Hash = h.DescriptorHash()
			h.Trusted = h.SourceKind == "" || h.SourceKind == SourceUser || h.SourceKind == SourceManaged
			h.Disabled = false
			if store != nil && h.SourceKind != SourceManaged {
				h.Disabled = store.IsDisabled(h.Hash)
				h.Trusted = h.Trusted || store.IsTrusted(h.Hash)
				if h.Disabled {
					h.Trusted = false
				}
			}
			if _, ok := h.commandForGOOS(runtime.GOOS); !ok {
				h.UnsupportedPlatform = true
			} else {
				h.UnsupportedPlatform = false
			}
		}
		hc.Hooks[event] = hookList
	}
}

// DescriptorHash returns a stable hash for trust decisions. It intentionally
// excludes SourcePath so trust can move across checkouts and machines.
func (h Hook) DescriptorHash() string {
	descriptor := struct {
		Event         Event      `json:"event"`
		SourceKind    SourceKind `json:"source_kind"`
		SourceID      string     `json:"source_id"`
		Command       string     `json:"command"`
		CommandWindow string     `json:"command_windows,omitempty"`
		Glob          string     `json:"glob,omitempty"`
	}{
		Event:         h.Event,
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
	if h.Disabled || h.Suppressed || h.UnsupportedPlatform {
		return false
	}
	return h.SourceKind == "" || h.Trusted
}

// Run executes all hooks for the given event, expanding templates with data.
// data keys include: "file", "command", "error", "tool", "session_id",
// "plan_id", "fleet_id", "agent_name", "agent_role", "cron_id",
// "tokens_used", "tokens_limit"
func (hc *HookConfig) Run(event Event, data map[string]string) error {
	return hc.RunWithOptions(event, data, RunOptions{})
}

// RunOptions supplies execution boundaries for managed hooks.
type RunOptions struct {
	Audit HookAuditWriter
}

// RunWithOptions executes eligible hooks and durably audits managed hooks.
func (hc *HookConfig) RunWithOptions(event Event, data map[string]string, opts RunOptions) error {
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

		if h.SourceKind != SourceManaged {
			// Expand command template with shell-escaped values to prevent injection.
			cmd, err := expandTemplate(command, escapeDataForGOOS(data, runtime.GOOS))
			if err != nil {
				return fmt.Errorf("expand hook command: %w", err)
			}
			out, err := execHookCommand(cmd).CombinedOutput()
			if err != nil {
				return fmt.Errorf("hook %s failed: %v\noutput: %s", event, err, out)
			}
			continue
		}

		if h.Event == "" {
			h.Event = event
		}
		hash := h.DescriptorHash()
		started := managedHookNow()
		startRecord := HookAuditRecord{
			Timestamp: started.UTC(),
			Event:     event,
			Hash:      hash,
			Source:    SourceManaged,
			Result:    HookAuditStarted,
		}
		if opts.Audit == nil || opts.Audit.Append(startRecord) != nil {
			return newManagedHookExecutionError(event, hash, HookAuditDegraded, false, true)
		}

		cmd, commandErr := expandTemplate(command, escapeDataForGOOS(data, runtime.GOOS))
		if commandErr == nil {
			commandErr = runManagedHookCommand(execHookCommand(cmd))
		}
		result := HookAuditSuccess
		if commandErr != nil {
			result = HookAuditCommandFailed
		}
		terminal := HookAuditRecord{
			Timestamp:  managedHookNow().UTC(),
			Event:      event,
			Hash:       hash,
			Source:     SourceManaged,
			Result:     result,
			DurationMS: managedHookSince(started).Milliseconds(),
		}
		auditErr := opts.Audit.Append(terminal)
		if commandErr != nil || auditErr != nil {
			return newManagedHookExecutionError(event, hash, result, commandErr != nil, auditErr != nil)
		}
	}
	return nil
}

func runManagedHookCommand(cmd *exec.Cmd) error {
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	return cmd.Run()
}

type managedHookExecutionError struct {
	event         Event
	hash          string
	result        HookAuditResult
	commandFailed bool
	auditDegraded bool
}

func newManagedHookExecutionError(event Event, hash string, result HookAuditResult, commandFailed, auditDegraded bool) error {
	return &managedHookExecutionError{
		event:         event,
		hash:          hash,
		result:        result,
		commandFailed: commandFailed,
		auditDegraded: auditDegraded,
	}
}

func (e *managedHookExecutionError) Error() string {
	if e.auditDegraded && e.result != HookAuditDegraded {
		return fmt.Sprintf("managed hook event=%s hash=%s result=%s audit=%s", e.event, e.hash, e.result, HookAuditDegraded)
	}
	return fmt.Sprintf("managed hook event=%s hash=%s result=%s", e.event, e.hash, e.result)
}

func (e *managedHookExecutionError) Unwrap() []error {
	errs := make([]error, 0, 2)
	if e.commandFailed {
		errs = append(errs, ErrManagedHookCommandFailed)
	}
	if e.auditDegraded {
		errs = append(errs, ErrHookAuditDegraded)
	}
	return errs
}

func execHookCommand(command string) *exec.Cmd {
	if runtime.GOOS == "windows" {
		return exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-Command", command)
	}
	return exec.Command("sh", "-c", command)
}

func escapeDataForGOOS(data map[string]string, goos string) map[string]string {
	if goos == "windows" {
		return powershellEscapeData(data)
	}
	return shellEscapeData(data)
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

func powershellEscapeData(data map[string]string) map[string]string {
	escaped := make(map[string]string, len(data))
	for k, v := range data {
		escaped[k] = "'" + strings.ReplaceAll(v, "'", "''") + "'"
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
