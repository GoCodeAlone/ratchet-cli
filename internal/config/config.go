package config

import (
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

type Config struct {
	DefaultProvider   string           `yaml:"default_provider"`
	DefaultModel      string           `yaml:"default_model"`
	Theme             string           `yaml:"theme"`
	InstructionCompat []string         `yaml:"instruction_compat"`
	Permissions       PermissionConfig `yaml:"permissions"`
	Daemon            DaemonConfig     `yaml:"daemon"`
	ModelRouting      ModelRouting     `yaml:"model_routing"`
	Context           ContextConfig    `yaml:"context"`
}

// ContextConfig controls automatic context-window compression behaviour.
type ContextConfig struct {
	// CompressionThreshold is the fraction of the model's context limit at which
	// auto-compression triggers (default 0.9).
	CompressionThreshold float64 `yaml:"compression_threshold"`
	// PreserveMessages is how many recent messages to keep verbatim after compression
	// (default 10).
	PreserveMessages int `yaml:"preserve_messages"`
	// CompressionModel overrides the model used for summarisation. Empty means
	// auto-select the cheapest available model.
	CompressionModel string `yaml:"compression_model"`
	// ModelLimits maps model names to their context window size in tokens.
	// Used by the chat handler to determine when to trigger compression.
	// Falls back to 200000 for unknown models.
	ModelLimits map[string]int `yaml:"model_limits"`
}

// ModelRouting controls which model handles which class of task.
type ModelRouting struct {
	// SimpleTaskModel is used for lightweight steps (set, log, validate, etc.).
	SimpleTaskModel string `yaml:"simple_task_model"`
	// ComplexTaskModel is used for heavy steps (http_call, db_query, code execution, etc.).
	ComplexTaskModel string `yaml:"complex_task_model"`
	// ReviewModel is used for code review / plan review tasks.
	ReviewModel string `yaml:"review_model"`
}

type PermissionConfig struct {
	AutoAllow []string `yaml:"auto_allow"`
	AlwaysAsk []string `yaml:"always_ask"`
}

type DaemonConfig struct {
	AutoStart   bool   `yaml:"auto_start"`
	IdleTimeout string `yaml:"idle_timeout"`
}

func DefaultConfig() *Config {
	return &Config{
		DefaultProvider:   "",
		DefaultModel:      "",
		Theme:             "dark",
		InstructionCompat: []string{"claude", "copilot", "cursor", "windsurf"},
		Permissions: PermissionConfig{
			AutoAllow: []string{"file_read", "file_list", "git_status", "git_diff"},
			AlwaysAsk: []string{"file_write", "bash", "git_push"},
		},
		Daemon: DaemonConfig{
			AutoStart:   true,
			IdleTimeout: "30m",
		},
		Context: ContextConfig{
			CompressionThreshold: 0.9,
			PreserveMessages:     10,
			CompressionModel:     "",
			ModelLimits: map[string]int{
				"claude-opus-4-6":   1000000,
				"claude-sonnet-4-6": 200000,
				"claude-haiku-4-5":  200000,
				"gpt-4o":            128000,
				"gpt-4o-mini":       128000,
			},
		},
	}
}

func Load() (*Config, error) {
	home, _ := os.UserHomeDir()
	path := filepath.Join(home, ".ratchet", "config.yaml")

	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return DefaultConfig(), nil
	}
	if err != nil {
		return nil, err
	}

	cfg := DefaultConfig()
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}

func (c *Config) Save() error {
	home, _ := os.UserHomeDir()
	dir := filepath.Join(home, ".ratchet")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	path := filepath.Join(dir, "config.yaml")

	data, err := yaml.Marshal(c)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0600)
}
