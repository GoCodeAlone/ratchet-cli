package hooks

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
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
)

// Hook defines a single hook command with an optional glob pattern.
type Hook struct {
	Command string `yaml:"command"`
	Glob    string `yaml:"glob,omitempty"`
}

// HookConfig holds the full hooks configuration.
type HookConfig struct {
	Hooks map[Event][]Hook `yaml:"hooks"`
}

// Load reads hook configs from ~/.ratchet/hooks.yaml and .ratchet/hooks.yaml.
// Project-level hooks (.ratchet/hooks.yaml) override global ones.
func Load(workingDir string) (*HookConfig, error) {
	cfg := &HookConfig{
		Hooks: make(map[Event][]Hook),
	}

	home, _ := os.UserHomeDir()
	paths := []string{
		filepath.Join(home, ".ratchet", "hooks.yaml"),
		filepath.Join(workingDir, ".ratchet", "hooks.yaml"),
	}

	for _, path := range paths {
		data, err := os.ReadFile(path)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", path, err)
		}
		var fileCfg HookConfig
		if err := yaml.Unmarshal(data, &fileCfg); err != nil {
			return nil, fmt.Errorf("parse %s: %w", path, err)
		}
		// Merge: later paths append to existing hooks
		for event, hooks := range fileCfg.Hooks {
			cfg.Hooks[event] = append(cfg.Hooks[event], hooks...)
		}
	}
	return cfg, nil
}

// Run executes all hooks for the given event, expanding templates with data.
// data keys include: "file", "command", "error", "tool", "session_id"
func (hc *HookConfig) Run(event Event, data map[string]string) error {
	hooks := hc.Hooks[event]
	for _, h := range hooks {
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

		// Expand command template
		cmd, err := expandTemplate(h.Command, data)
		if err != nil {
			return fmt.Errorf("expand hook command: %w", err)
		}

		// Execute via shell
		out, err := exec.Command("sh", "-c", cmd).CombinedOutput()
		if err != nil {
			return fmt.Errorf("hook %s failed: %v\noutput: %s", event, err, out)
		}
	}
	return nil
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
