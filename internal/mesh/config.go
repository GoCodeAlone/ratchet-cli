package mesh

import (
	_ "embed"
	"encoding/json"
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
	Name          string   `yaml:"name" json:"name"`
	Role          string   `yaml:"role" json:"role"`
	Provider      string   `yaml:"provider" json:"provider"`
	Model         string   `yaml:"model" json:"model"`
	MaxIterations int      `yaml:"max_iterations" json:"max_iterations,omitempty"`
	SystemPrompt  string   `yaml:"system_prompt" json:"system_prompt,omitempty"`
	Tools         []string `yaml:"tools" json:"tools,omitempty"`
}

// TeamConfig describes a complete agent team loaded from YAML.
type TeamConfig struct {
	Name            string        `yaml:"name" json:"name"`
	Agents          []AgentConfig `yaml:"agents" json:"agents"`
	Timeout         string        `yaml:"timeout" json:"timeout,omitempty"` // duration string like "10m"
	MaxReviewRounds int           `yaml:"max_review_rounds" json:"max_review_rounds,omitempty"`
}

// knownTools is the set of tool names available to mesh agents.
var knownTools = map[string]bool{
	"blackboard_read":  true,
	"blackboard_write": true,
	"blackboard_list":  true,
	"send_message":     true,
}

// LoadTeamConfig reads a team config file (YAML or JSON) and validates it.
func LoadTeamConfig(path string) (*TeamConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading team config: %w", err)
	}

	var tc TeamConfig
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".json":
		if err := json.Unmarshal(data, &tc); err != nil {
			return nil, fmt.Errorf("parsing team config JSON: %w", err)
		}
	default:
		if err := yaml.Unmarshal(data, &tc); err != nil {
			return nil, fmt.Errorf("parsing team config: %w", err)
		}
	}

	if err := ValidateTeamConfig(&tc); err != nil {
		return nil, fmt.Errorf("validating %s: %w", path, err)
	}
	return &tc, nil
}

// LoadTeamConfigs discovers and loads all .yaml, .yml, and .json files in dir.
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
		if ext != ".yaml" && ext != ".yml" && ext != ".json" {
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

// SearchTeamConfig searches for a named team config in standard paths:
//  1. .ratchet/teams/ in projectDir (if non-empty)
//  2. ~/.ratchet/teams/ in homeDir (if non-empty)
//
// Returns the first match. Falls back to loading as a file path.
func SearchTeamConfig(name, projectDir, homeDir string) (*TeamConfig, error) {
	searchDirs := make([]string, 0, 2)
	if projectDir != "" {
		searchDirs = append(searchDirs, filepath.Join(projectDir, ".ratchet", "teams"))
	}
	if homeDir != "" {
		searchDirs = append(searchDirs, filepath.Join(homeDir, ".ratchet", "teams"))
	}

	extensions := []string{".yaml", ".yml", ".json"}
	for _, dir := range searchDirs {
		for _, ext := range extensions {
			path := filepath.Join(dir, name+ext)
			if _, err := os.Stat(path); err == nil {
				return LoadTeamConfig(path)
			}
		}
	}

	// Try as a direct file path.
	if _, err := os.Stat(name); err == nil {
		return LoadTeamConfig(name)
	}

	return nil, fmt.Errorf("team config %q not found in search paths", name)
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

// ProjectTeamConfig extends TeamConfig with per-team Blackboard mode.
type ProjectTeamConfig struct {
	Name       string        `yaml:"name" json:"name"`
	Agents     []AgentConfig `yaml:"agents" json:"agents"`
	Timeout    string        `yaml:"timeout,omitempty" json:"timeout,omitempty"`
	Blackboard string        `yaml:"blackboard,omitempty" json:"blackboard,omitempty"` // shared, isolated, orchestrator, bridge:<t1>,<t2>
}

// ProjectConfig defines a multi-team project.
type ProjectConfig struct {
	Project string              `yaml:"project" json:"project"`
	Teams   []ProjectTeamConfig `yaml:"teams" json:"teams"`
}

// ParseProjectConfig parses a YAML project config.
func ParseProjectConfig(data []byte) (*ProjectConfig, error) {
	var pc ProjectConfig
	if err := yaml.Unmarshal(data, &pc); err != nil {
		return nil, fmt.Errorf("parsing project config: %w", err)
	}
	if err := ValidateProjectConfig(&pc); err != nil {
		return nil, err
	}
	return &pc, nil
}

// ParseProjectConfigJSON parses a JSON project config.
func ParseProjectConfigJSON(data []byte) (*ProjectConfig, error) {
	var pc ProjectConfig
	if err := json.Unmarshal(data, &pc); err != nil {
		return nil, fmt.Errorf("parsing project config JSON: %w", err)
	}
	if err := ValidateProjectConfig(&pc); err != nil {
		return nil, err
	}
	return &pc, nil
}

// LoadProjectConfig reads a project config file (YAML or JSON).
func LoadProjectConfig(path string) (*ProjectConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading project config: %w", err)
	}
	ext := strings.ToLower(filepath.Ext(path))
	if ext == ".json" {
		return ParseProjectConfigJSON(data)
	}
	return ParseProjectConfig(data)
}

// ValidateProjectConfig checks that the project config is well-formed.
func ValidateProjectConfig(pc *ProjectConfig) error {
	if pc.Project == "" {
		return fmt.Errorf("project name is required")
	}
	if len(pc.Teams) == 0 {
		return fmt.Errorf("project %q must have at least one team", pc.Project)
	}
	teamNames := make(map[string]bool, len(pc.Teams))
	for i, t := range pc.Teams {
		if t.Name == "" {
			return fmt.Errorf("team %d in project %q: name is required", i, pc.Project)
		}
		if teamNames[t.Name] {
			return fmt.Errorf("team %d in project %q: duplicate team name %q", i, pc.Project, t.Name)
		}
		teamNames[t.Name] = true
		if len(t.Agents) == 0 {
			return fmt.Errorf("team %q in project %q: must have at least one agent", t.Name, pc.Project)
		}
		// Validate BB mode.
		switch {
		case t.Blackboard == "", t.Blackboard == "shared", t.Blackboard == "isolated", t.Blackboard == "orchestrator":
			// valid
		case strings.HasPrefix(t.Blackboard, "bridge:"):
			parts := strings.Split(strings.TrimPrefix(t.Blackboard, "bridge:"), ",")
			if len(parts) < 2 {
				return fmt.Errorf("team %q: bridge mode requires at least 2 team names", t.Name)
			}
		default:
			return fmt.Errorf("team %q: unknown blackboard mode %q", t.Name, t.Blackboard)
		}
	}
	return nil
}

// ToTeamConfig converts a ProjectTeamConfig to a standard TeamConfig.
func (ptc *ProjectTeamConfig) ToTeamConfig() *TeamConfig {
	return &TeamConfig{
		Name:    ptc.Name,
		Agents:  ptc.Agents,
		Timeout: ptc.Timeout,
	}
}

// ParseAgentFlag parses a CLI agent flag in the format "name:provider[:model]".
func ParseAgentFlag(s string) (AgentConfig, error) {
	if s == "" {
		return AgentConfig{}, fmt.Errorf("empty agent flag")
	}
	parts := strings.SplitN(s, ":", 3)
	ac := AgentConfig{Name: parts[0]}
	if len(parts) >= 2 {
		ac.Provider = parts[1]
	}
	if len(parts) >= 3 {
		ac.Model = parts[2]
	}
	return ac, nil
}

// BuildTeamConfigFromFlags constructs a TeamConfig from CLI flags.
func BuildTeamConfigFromFlags(name string, agentFlags []string, orchestrator string, bbMode string) (*TeamConfig, error) {
	if len(agentFlags) == 0 {
		return nil, fmt.Errorf("at least one --agent is required")
	}

	tc := &TeamConfig{Name: name}
	if tc.Name == "" {
		tc.Name = "cli-team"
	}

	for _, flag := range agentFlags {
		ac, err := ParseAgentFlag(flag)
		if err != nil {
			return nil, fmt.Errorf("parsing agent flag %q: %w", flag, err)
		}
		tc.Agents = append(tc.Agents, ac)
	}

	// First agent is orchestrator by default, or use --orchestrator flag.
	orchName := orchestrator
	if orchName == "" && len(tc.Agents) > 0 {
		orchName = tc.Agents[0].Name
	}
	for i := range tc.Agents {
		if tc.Agents[i].Name == orchName {
			tc.Agents[i].Role = "orchestrator"
			// Orchestrators get all tools by default.
			if len(tc.Agents[i].Tools) == 0 {
				tc.Agents[i].Tools = []string{"blackboard_read", "blackboard_write", "blackboard_list", "send_message"}
			}
		}
	}

	return tc, nil
}
