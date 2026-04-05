package mesh

import (
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

//go:embed teams/code-gen.yaml
var defaultCodeGenTeam []byte

// DefaultCodeGenTeamConfig returns the built-in code-gen team configuration.
func DefaultCodeGenTeamConfig() (*TeamConfig, error) {
	var tc TeamConfig
	if err := yaml.Unmarshal(defaultCodeGenTeam, &tc); err != nil {
		return nil, err
	}
	return &tc, nil
}

// BuiltinTeamConfigs returns all built-in team configurations keyed by name.
func BuiltinTeamConfigs() (map[string]*TeamConfig, error) {
	tc, err := DefaultCodeGenTeamConfig()
	if err != nil {
		return nil, err
	}
	return map[string]*TeamConfig{
		"code-gen": tc,
	}, nil
}

// AgentConfig describes a single agent within a team.
type AgentConfig struct {
	Name          string   `yaml:"name"`
	Role          string   `yaml:"role"`
	Provider      string   `yaml:"provider"`
	Model         string   `yaml:"model"`
	MaxIterations int      `yaml:"max_iterations"`
	SystemPrompt  string   `yaml:"system_prompt"`
	Tools         []string `yaml:"tools"`
}

// TeamConfig describes a complete agent team loaded from YAML.
type TeamConfig struct {
	Name            string        `yaml:"name"`
	Agents          []AgentConfig `yaml:"agents"`
	Timeout         string        `yaml:"timeout"` // duration string like "10m"
	MaxReviewRounds int           `yaml:"max_review_rounds"`
}

// knownTools is the set of tool names available to mesh agents.
var knownTools = map[string]bool{
	"blackboard_read":  true,
	"blackboard_write": true,
	"blackboard_list":  true,
	"send_message":     true,
}

// LoadTeamConfig reads a single team YAML file and validates it.
func LoadTeamConfig(path string) (*TeamConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading team config: %w", err)
	}

	var tc TeamConfig
	if err := yaml.Unmarshal(data, &tc); err != nil {
		return nil, fmt.Errorf("parsing team config: %w", err)
	}

	if err := ValidateTeamConfig(&tc); err != nil {
		return nil, fmt.Errorf("validating %s: %w", path, err)
	}
	return &tc, nil
}

// LoadTeamConfigs discovers and loads all .yaml and .yml files in dir.
func LoadTeamConfigs(dir string) ([]TeamConfig, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("reading config directory: %w", err)
	}

	var configs []TeamConfig
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		ext := strings.ToLower(filepath.Ext(entry.Name()))
		if ext != ".yaml" && ext != ".yml" {
			continue
		}
		tc, err := LoadTeamConfig(filepath.Join(dir, entry.Name()))
		if err != nil {
			return nil, err
		}
		configs = append(configs, *tc)
	}
	return configs, nil
}

// ValidateTeamConfig checks that the team config is well-formed.
func ValidateTeamConfig(tc *TeamConfig) error {
	if tc.Name == "" {
		return fmt.Errorf("team name is required")
	}
	if len(tc.Agents) == 0 {
		return fmt.Errorf("team %q must have at least one agent", tc.Name)
	}
	if tc.Timeout != "" {
		if _, err := time.ParseDuration(tc.Timeout); err != nil {
			return fmt.Errorf("invalid timeout %q: %w", tc.Timeout, err)
		}
	}
	names := make(map[string]bool, len(tc.Agents))
	for i, a := range tc.Agents {
		if a.Name == "" {
			return fmt.Errorf("agent %d in team %q: name is required", i, tc.Name)
		}
		if names[a.Name] {
			return fmt.Errorf("agent %d in team %q: duplicate agent name %q", i, tc.Name, a.Name)
		}
		names[a.Name] = true
		for _, tool := range a.Tools {
			if !knownTools[tool] {
				return fmt.Errorf("agent %q in team %q: unknown tool %q", a.Name, tc.Name, tool)
			}
		}
	}
	return nil
}

// ToNodeConfigs converts a TeamConfig's agents into NodeConfigs suitable for
// passing to NewLocalNode or SpawnTeam.
func ToNodeConfigs(tc *TeamConfig) []NodeConfig {
	configs := make([]NodeConfig, len(tc.Agents))
	for i, a := range tc.Agents {
		configs[i] = NodeConfig{
			Name:          a.Name,
			Role:          a.Role,
			Model:         a.Model,
			Provider:      a.Provider,
			Location:      "local",
			SystemPrompt:  a.SystemPrompt,
			Tools:         a.Tools,
			MaxIterations: a.MaxIterations,
		}
	}
	return configs
}
