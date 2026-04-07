# Team Lifecycle & Multi-Team Project Orchestration Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Dynamic team composition from CLI, multi-team projects, cross-team Blackboard, human-in-the-loop, project/task tracker, and full session lifecycle.

**Architecture:** Extends existing mesh (Blackboard + Router + LocalNode) with project registry, dynamic node add/remove, configurable BB modes (shared/isolated/orchestrator/bridge), human gate with autoresponder, SQLite task tracker with BB notifications, and OS-native idle notifications.

**Tech Stack:** Go 1.26, SQLite, protobuf/gRPC, Bubbletea v2, osascript/notify-send

---

## Phase 1: Foundation

### Task 1.1: Config Convention — JSON Support + Search Paths

**Files:**
- `internal/mesh/config.go` (modify)
- `internal/mesh/config_test.go` (new)

**Test first** (`internal/mesh/config_test.go`):
```go
package mesh

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadTeamConfigJSON(t *testing.T) {
	dir := t.TempDir()
	jsonFile := filepath.Join(dir, "test-team.json")
	if err := os.WriteFile(jsonFile, []byte(`{
		"name": "json-team",
		"timeout": "5m",
		"agents": [
			{"name": "lead", "role": "orchestrator", "provider": "ollama", "model": "qwen3:8b", "tools": ["blackboard_read", "send_message"]}
		]
	}`), 0644); err != nil {
		t.Fatal(err)
	}

	tc, err := LoadTeamConfig(jsonFile)
	if err != nil {
		t.Fatalf("LoadTeamConfig JSON: %v", err)
	}
	if tc.Name != "json-team" {
		t.Errorf("got name %q, want %q", tc.Name, "json-team")
	}
	if len(tc.Agents) != 1 {
		t.Fatalf("got %d agents, want 1", len(tc.Agents))
	}
	if tc.Agents[0].Provider != "ollama" {
		t.Errorf("got provider %q, want %q", tc.Agents[0].Provider, "ollama")
	}
}

func TestLoadTeamConfigsDir(t *testing.T) {
	dir := t.TempDir()
	yamlContent := `name: y-team
agents:
  - name: worker
    role: worker
    provider: ollama`
	if err := os.WriteFile(filepath.Join(dir, "team1.yaml"), []byte(yamlContent), 0644); err != nil {
		t.Fatal(err)
	}
	jsonContent := `{"name": "j-team", "agents": [{"name": "worker", "role": "worker", "provider": "ollama"}]}`
	if err := os.WriteFile(filepath.Join(dir, "team2.json"), []byte(jsonContent), 0644); err != nil {
		t.Fatal(err)
	}

	configs, err := LoadTeamConfigs(dir)
	if err != nil {
		t.Fatalf("LoadTeamConfigs: %v", err)
	}
	if len(configs) != 2 {
		t.Fatalf("got %d configs, want 2", len(configs))
	}
}

func TestSearchTeamConfig(t *testing.T) {
	dir := t.TempDir()
	teamsDir := filepath.Join(dir, ".ratchet", "teams")
	if err := os.MkdirAll(teamsDir, 0755); err != nil {
		t.Fatal(err)
	}
	yamlContent := `name: my-team
agents:
  - name: lead
    role: orchestrator
    provider: ollama`
	if err := os.WriteFile(filepath.Join(teamsDir, "my-team.yaml"), []byte(yamlContent), 0644); err != nil {
		t.Fatal(err)
	}

	tc, err := SearchTeamConfig("my-team", dir, "")
	if err != nil {
		t.Fatalf("SearchTeamConfig: %v", err)
	}
	if tc.Name != "my-team" {
		t.Errorf("got name %q, want %q", tc.Name, "my-team")
	}
}
```

**Implementation** — modify `internal/mesh/config.go`:

Add JSON loading support to `LoadTeamConfig` and add `SearchTeamConfig`:

```go
import (
	"encoding/json"
	// ... existing imports
)

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
//   1. .ratchet/teams/ in projectDir (if non-empty)
//   2. ~/.ratchet/teams/ in homeDir (if non-empty)
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
```

**Run:**
```bash
cd /Users/jon/workspace/ratchet-cli && go test ./internal/mesh/ -run TestLoadTeamConfigJSON -v
cd /Users/jon/workspace/ratchet-cli && go test ./internal/mesh/ -run TestLoadTeamConfigsDir -v
cd /Users/jon/workspace/ratchet-cli && go test ./internal/mesh/ -run TestSearchTeamConfig -v
```

**Commit:** `feat: JSON team config support + SearchTeamConfig with standard paths`

---

### Task 1.2: Multi-Team Project Config Parsing

**Files:**
- `internal/mesh/config.go` (modify — add `ProjectConfig` type)
- `internal/mesh/config_test.go` (modify — add project config tests)

**Test first** — add to `internal/mesh/config_test.go`:
```go
func TestParseProjectConfig(t *testing.T) {
	yamlContent := `project: email-service
teams:
  - name: design
    agents:
      - name: architect
        provider: ollama
        model: qwen3:8b
        role: orchestrator
      - name: researcher
        provider: claude_code
    blackboard: shared
  - name: dev
    agents:
      - name: lead
        provider: ollama
        role: orchestrator
      - name: coder
        provider: claude_code
    blackboard: shared`

	pc, err := ParseProjectConfig([]byte(yamlContent))
	if err != nil {
		t.Fatalf("ParseProjectConfig: %v", err)
	}
	if pc.Project != "email-service" {
		t.Errorf("got project %q, want %q", pc.Project, "email-service")
	}
	if len(pc.Teams) != 2 {
		t.Fatalf("got %d teams, want 2", len(pc.Teams))
	}
	if pc.Teams[0].Blackboard != "shared" {
		t.Errorf("got bb mode %q, want %q", pc.Teams[0].Blackboard, "shared")
	}
}

func TestParseProjectConfigJSON(t *testing.T) {
	jsonContent := `{
		"project": "api-service",
		"teams": [
			{
				"name": "backend",
				"agents": [{"name": "dev", "role": "worker", "provider": "ollama"}],
				"blackboard": "isolated"
			}
		]
	}`
	pc, err := ParseProjectConfigJSON([]byte(jsonContent))
	if err != nil {
		t.Fatalf("ParseProjectConfigJSON: %v", err)
	}
	if pc.Project != "api-service" {
		t.Errorf("got project %q, want %q", pc.Project, "api-service")
	}
}

func TestValidateProjectConfig(t *testing.T) {
	pc := &ProjectConfig{
		Project: "",
		Teams:   []ProjectTeamConfig{},
	}
	if err := ValidateProjectConfig(pc); err == nil {
		t.Error("expected error for empty project name")
	}
}
```

**Implementation** — add to `internal/mesh/config.go`:
```go
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
```

**Run:**
```bash
cd /Users/jon/workspace/ratchet-cli && go test ./internal/mesh/ -run TestParseProjectConfig -v
cd /Users/jon/workspace/ratchet-cli && go test ./internal/mesh/ -run TestValidateProjectConfig -v
```

**Commit:** `feat: multi-team ProjectConfig parsing with YAML/JSON support`

---

### Task 1.3: --agent/--agents/--name/--bb CLI Flags

**Files:**
- `cmd/ratchet/cmd_team.go` (modify)
- `internal/mesh/config.go` (modify — add `ParseAgentFlag`)

**Test first** — add to `internal/mesh/config_test.go`:
```go
func TestParseAgentFlag(t *testing.T) {
	tests := []struct {
		input    string
		wantName string
		wantProv string
		wantModel string
		wantErr  bool
	}{
		{"lead:ollama:qwen3:8b", "lead", "ollama", "qwen3:8b", false},
		{"coder:claude_code", "coder", "claude_code", "", false},
		{"worker", "worker", "", "", false},
		{"", "", "", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			ac, err := ParseAgentFlag(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if ac.Name != tt.wantName {
				t.Errorf("name: got %q, want %q", ac.Name, tt.wantName)
			}
			if ac.Provider != tt.wantProv {
				t.Errorf("provider: got %q, want %q", ac.Provider, tt.wantProv)
			}
			if ac.Model != tt.wantModel {
				t.Errorf("model: got %q, want %q", ac.Model, tt.wantModel)
			}
		})
	}
}
```

**Implementation** — add to `internal/mesh/config.go`:
```go
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
```

**Modify** `cmd/ratchet/cmd_team.go` — replace `handleTeamStart`:
```go
func handleTeamStart(args []string) {
	var (
		agentFlags   []string
		teamName     string
		bbMode       string
		orchestrator string
		configName   string
		task         string
	)

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--agent":
			if i+1 < len(args) {
				agentFlags = append(agentFlags, args[i+1])
				i++
			}
		case "--agents":
			if i+1 < len(args) {
				for _, a := range strings.Split(args[i+1], ",") {
					agentFlags = append(agentFlags, strings.TrimSpace(a))
				}
				i++
			}
		case "--name":
			if i+1 < len(args) {
				teamName = args[i+1]
				i++
			}
		case "--bb":
			if i+1 < len(args) {
				bbMode = args[i+1]
				i++
			}
		case "--orchestrator":
			if i+1 < len(args) {
				orchestrator = args[i+1]
				i++
			}
		case "--config":
			if i+1 < len(args) {
				configName = args[i+1]
				i++
			}
		case "--task":
			if i+1 < len(args) {
				task = args[i+1]
				i++
			}
		default:
			// Positional: could be config name or task.
			if configName == "" && task == "" && isTeamConfig(args[i]) {
				configName = args[i]
			} else if task == "" {
				task = args[i]
			}
		}
	}

	// Build team config from --agent flags if no --config.
	if configName == "" && len(agentFlags) > 0 {
		tc, err := mesh.BuildTeamConfigFromFlags(teamName, agentFlags, orchestrator, bbMode)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Team: %s (%d agents, bb=%s)\n", tc.Name, len(tc.Agents), bbMode)
		for _, a := range tc.Agents {
			prov := a.Provider
			if prov == "" {
				prov = "(default)"
			}
			fmt.Printf("  • %s — %s", a.Name, prov)
			if a.Model != "" {
				fmt.Printf("/%s", a.Model)
			}
			if a.Role == "orchestrator" {
				fmt.Print(" [orchestrator]")
			}
			fmt.Println()
		}
		fmt.Println()

		// For now, serialize to a temp YAML and pass as config name.
		// TODO: extend StartTeamReq to accept inline agent configs.
		configName = writeTemporaryTeamConfig(tc)
	}

	if task == "" {
		fmt.Println("Usage: ratchet team start [--agent name:provider[:model]]... [--config name] \"task\"")
		return
	}

	c, err := client.EnsureDaemon()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	defer c.Close()

	if configName != "" {
		tc, err := resolveTeamConfig(configName)
		if err == nil {
			fmt.Printf("Using team config: %s (%d agents)\n", tc.Name, len(tc.Agents))
			for _, a := range tc.Agents {
				fmt.Printf("  • %s (%s) — %s/%s\n", a.Name, a.Role, a.Provider, a.Model)
			}
			fmt.Println()
		}
	}

	stream, err := c.StartTeam(context.Background(), &pb.StartTeamReq{
		Task:           task,
		TeamConfigName: configName,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	for event := range stream {
		switch e := event.Event.(type) {
		case *pb.TeamEvent_AgentSpawned:
			if e.AgentSpawned.AgentName == "__team__" {
				fmt.Printf("Team ID: %s\n", e.AgentSpawned.AgentId)
				fmt.Printf("(Use 'ratchet team status %s' to check status)\n\n", e.AgentSpawned.AgentId)
			} else {
				fmt.Printf("[spawned] %s (%s)\n", e.AgentSpawned.AgentName, e.AgentSpawned.Role)
			}
		case *pb.TeamEvent_Token:
			fmt.Print(e.Token.Content)
		case *pb.TeamEvent_AgentMessage:
			fmt.Printf("[%s → %s] %s\n", e.AgentMessage.FromAgent, e.AgentMessage.ToAgent, e.AgentMessage.Content)
		case *pb.TeamEvent_Complete:
			fmt.Printf("\nTeam complete: %s\n", e.Complete.Summary)
		case *pb.TeamEvent_Error:
			fmt.Fprintf(os.Stderr, "error: %s\n", e.Error.Message)
		}
	}
}

func writeTemporaryTeamConfig(tc *mesh.TeamConfig) string {
	data, err := yaml.Marshal(tc)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error marshaling team config: %v\n", err)
		os.Exit(1)
	}
	tmpFile, err := os.CreateTemp("", "ratchet-team-*.yaml")
	if err != nil {
		fmt.Fprintf(os.Stderr, "error creating temp file: %v\n", err)
		os.Exit(1)
	}
	if _, err := tmpFile.Write(data); err != nil {
		fmt.Fprintf(os.Stderr, "error writing temp file: %v\n", err)
		os.Exit(1)
	}
	tmpFile.Close()
	return tmpFile.Name()
}
```

Add `gopkg.in/yaml.v3` import to `cmd/ratchet/cmd_team.go`.

**Run:**
```bash
cd /Users/jon/workspace/ratchet-cli && go test ./internal/mesh/ -run TestParseAgentFlag -v
cd /Users/jon/workspace/ratchet-cli && go build ./cmd/ratchet/
```

**Commit:** `feat: --agent/--agents/--name/--bb/--orchestrator CLI flags for team start`

---

### Task 1.4: Remove `orchestrate` Builtin Config

**Files:**
- `internal/mesh/config.go` (modify)
- `internal/mesh/teams/orchestrate.yaml` (delete)

**Implementation** — modify `internal/mesh/config.go`:

Remove:
```go
//go:embed teams/orchestrate.yaml
var defaultOrchestrateTeam []byte
```

Remove `DefaultOrchestrateTeamConfig()`.

Update `BuiltinTeamConfigs()`:
```go
func BuiltinTeamConfigs() (map[string]*TeamConfig, error) {
	tc, err := DefaultCodeGenTeamConfig()
	if err != nil {
		return nil, err
	}
	return map[string]*TeamConfig{
		"code-gen": tc,
	}, nil
}
```

Delete `internal/mesh/teams/orchestrate.yaml`.

Update `cmd/ratchet/cmd_team.go` — the `isTeamConfig` and `resolveTeamConfig` functions automatically adjust since they iterate `BuiltinTeamConfigs()`.

**Run:**
```bash
cd /Users/jon/workspace/ratchet-cli && go build ./cmd/ratchet/
cd /Users/jon/workspace/ratchet-cli && go test ./internal/mesh/ -v
```

**Commit:** `refactor: remove orchestrate builtin config (users compose via flags/saved configs)`

---

### Task 1.5: Project Registry + `cmd_project.go`

**Files:**
- `internal/daemon/projects.go` (new)
- `internal/daemon/projects_test.go` (new)
- `cmd/ratchet/cmd_project.go` (new)
- `cmd/ratchet/main.go` (modify — add `project` case)

**Test first** (`internal/daemon/projects_test.go`):
```go
package daemon

import (
	"testing"
)

func TestProjectRegistry(t *testing.T) {
	pr := NewProjectRegistry()

	// Register a project.
	p, err := pr.Register("email-service", "/path/to/config.yaml")
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	if p.Name != "email-service" {
		t.Errorf("got name %q, want %q", p.Name, "email-service")
	}
	if p.Status != "active" {
		t.Errorf("got status %q, want %q", p.Status, "active")
	}

	// Duplicate name.
	if _, err := pr.Register("email-service", ""); err == nil {
		t.Error("expected error on duplicate project name")
	}

	// List.
	projects := pr.List()
	if len(projects) != 1 {
		t.Fatalf("got %d projects, want 1", len(projects))
	}

	// Get by ID.
	got, err := pr.Get(p.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Name != "email-service" {
		t.Errorf("got name %q, want %q", got.Name, "email-service")
	}

	// Get by name.
	got, err = pr.GetByName("email-service")
	if err != nil {
		t.Fatalf("GetByName: %v", err)
	}
	if got.ID != p.ID {
		t.Errorf("got ID %q, want %q", got.ID, p.ID)
	}

	// Pause.
	if err := pr.SetStatus(p.ID, "paused"); err != nil {
		t.Fatalf("SetStatus pause: %v", err)
	}
	got, _ = pr.Get(p.ID)
	if got.Status != "paused" {
		t.Errorf("got status %q, want %q", got.Status, "paused")
	}

	// AddTeam.
	pr.AddTeam(p.ID, "team-abc")
	got, _ = pr.Get(p.ID)
	if len(got.TeamIDs) != 1 || got.TeamIDs[0] != "team-abc" {
		t.Errorf("TeamIDs: got %v, want [team-abc]", got.TeamIDs)
	}

	// Kill.
	if err := pr.SetStatus(p.ID, "killed"); err != nil {
		t.Fatalf("SetStatus kill: %v", err)
	}
}
```

**Implementation** (`internal/daemon/projects.go`):
```go
package daemon

import (
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
)

// Project is a registered multi-team project.
type Project struct {
	ID         string
	Name       string
	ConfigPath string
	Status     string // active, paused, killed, completed
	TeamIDs    []string
	CreatedAt  time.Time
}

// ProjectRegistry manages active projects in memory.
type ProjectRegistry struct {
	mu       sync.RWMutex
	projects map[string]*Project // keyed by ID
	byName   map[string]string   // name → ID
}

// NewProjectRegistry returns an initialized ProjectRegistry.
func NewProjectRegistry() *ProjectRegistry {
	return &ProjectRegistry{
		projects: make(map[string]*Project),
		byName:   make(map[string]string),
	}
}

// Register creates a new project entry.
func (pr *ProjectRegistry) Register(name, configPath string) (*Project, error) {
	pr.mu.Lock()
	defer pr.mu.Unlock()

	if _, exists := pr.byName[name]; exists {
		return nil, fmt.Errorf("project %q already exists", name)
	}

	p := &Project{
		ID:         "proj-" + uuid.NewString()[:8],
		Name:       name,
		ConfigPath: configPath,
		Status:     "active",
		CreatedAt:  time.Now(),
	}
	pr.projects[p.ID] = p
	pr.byName[name] = p.ID
	return p, nil
}

// Get returns a project by ID.
func (pr *ProjectRegistry) Get(id string) (*Project, error) {
	pr.mu.RLock()
	defer pr.mu.RUnlock()
	p, ok := pr.projects[id]
	if !ok {
		return nil, fmt.Errorf("project %q not found", id)
	}
	cp := *p
	cp.TeamIDs = make([]string, len(p.TeamIDs))
	copy(cp.TeamIDs, p.TeamIDs)
	return &cp, nil
}

// GetByName returns a project by name.
func (pr *ProjectRegistry) GetByName(name string) (*Project, error) {
	pr.mu.RLock()
	id, ok := pr.byName[name]
	pr.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("project %q not found", name)
	}
	return pr.Get(id)
}

// List returns all projects.
func (pr *ProjectRegistry) List() []Project {
	pr.mu.RLock()
	defer pr.mu.RUnlock()
	out := make([]Project, 0, len(pr.projects))
	for _, p := range pr.projects {
		cp := *p
		cp.TeamIDs = make([]string, len(p.TeamIDs))
		copy(cp.TeamIDs, p.TeamIDs)
		out = append(out, cp)
	}
	return out
}

// SetStatus updates the project status.
func (pr *ProjectRegistry) SetStatus(id, status string) error {
	pr.mu.Lock()
	defer pr.mu.Unlock()
	p, ok := pr.projects[id]
	if !ok {
		return fmt.Errorf("project %q not found", id)
	}
	p.Status = status
	return nil
}

// AddTeam associates a team ID with the project.
func (pr *ProjectRegistry) AddTeam(projectID, teamID string) {
	pr.mu.Lock()
	defer pr.mu.Unlock()
	if p, ok := pr.projects[projectID]; ok {
		p.TeamIDs = append(p.TeamIDs, teamID)
	}
}

// RemoveTeam disassociates a team from the project.
func (pr *ProjectRegistry) RemoveTeam(projectID, teamID string) {
	pr.mu.Lock()
	defer pr.mu.Unlock()
	p, ok := pr.projects[projectID]
	if !ok {
		return
	}
	for i, id := range p.TeamIDs {
		if id == teamID {
			p.TeamIDs = append(p.TeamIDs[:i], p.TeamIDs[i+1:]...)
			return
		}
	}
}
```

**CLI** (`cmd/ratchet/cmd_project.go`):
```go
package main

import (
	"fmt"
	"os"
)

func handleProject(args []string) {
	if len(args) == 0 {
		fmt.Println("Usage: ratchet project <start|list|pause|resume|kill> [args...]")
		return
	}

	switch args[0] {
	case "start":
		handleProjectStart(args[1:])
	case "list":
		handleProjectList()
	case "pause":
		handleProjectPauseResume(args[1:], "pause")
	case "resume":
		handleProjectPauseResume(args[1:], "resume")
	case "kill":
		handleProjectKill(args[1:])
	default:
		fmt.Printf("unknown project command: %s\n", args[0])
	}
}

func handleProjectStart(args []string) {
	// Phase 1 stub — will be wired to daemon RPC in Phase 4.
	fmt.Println("project start: not yet implemented (Phase 4)")
	_ = args
}

func handleProjectList() {
	fmt.Println("project list: not yet implemented (Phase 4)")
}

func handleProjectPauseResume(args []string, action string) {
	if len(args) < 1 {
		fmt.Fprintf(os.Stderr, "Usage: ratchet project %s <project-id|name>\n", action)
		return
	}
	fmt.Printf("project %s %s: not yet implemented (Phase 4)\n", action, args[0])
}

func handleProjectKill(args []string) {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "Usage: ratchet project kill <project-id|name>")
		return
	}
	fmt.Printf("project kill %s: not yet implemented (Phase 4)\n", args[0])
}
```

**Modify** `cmd/ratchet/main.go` — add `"project"` case in the switch:
```go
	case "project":
		handleProject(filteredArgs[1:])
```

**Run:**
```bash
cd /Users/jon/workspace/ratchet-cli && go test ./internal/daemon/ -run TestProjectRegistry -v
cd /Users/jon/workspace/ratchet-cli && go build ./cmd/ratchet/
```

**Commit:** `feat: project registry + ratchet project CLI stub`

---

### Task 1.6: Team Save/Load Commands

**Files:**
- `cmd/ratchet/cmd_team.go` (modify — add `save` subcommand)

**Implementation** — add to `cmd/ratchet/cmd_team.go`:
```go
func handleTeam(args []string) {
	if len(args) == 0 {
		fmt.Println("Usage: ratchet team <start|status|list|save|kill> [args...]")
		return
	}

	switch args[0] {
	case "start":
		handleTeamStart(args[1:])
	case "status":
		handleTeamStatus(args[1:])
	case "list":
		handleTeamList()
	case "save":
		handleTeamSave(args[1:])
	case "kill":
		handleTeamKill(args[1:])
	default:
		fmt.Printf("unknown team command: %s\n", args[0])
	}
}

func handleTeamSave(args []string) {
	var (
		name       string
		agentFlags []string
		outputPath string
	)

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--agent":
			if i+1 < len(args) {
				agentFlags = append(agentFlags, args[i+1])
				i++
			}
		case "--output":
			if i+1 < len(args) {
				outputPath = args[i+1]
				i++
			}
		default:
			if name == "" {
				name = args[i]
			}
		}
	}

	if name == "" || len(agentFlags) == 0 {
		fmt.Println("Usage: ratchet team save <name> --agent name:provider[:model] [--output path]")
		return
	}

	tc, err := mesh.BuildTeamConfigFromFlags(name, agentFlags, "", "")
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	if outputPath == "" {
		dir := filepath.Join(".", ".ratchet", "teams")
		if err := os.MkdirAll(dir, 0755); err != nil {
			fmt.Fprintf(os.Stderr, "error creating dir: %v\n", err)
			os.Exit(1)
		}
		outputPath = filepath.Join(dir, name+".yaml")
	}

	data, err := yaml.Marshal(tc)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	if err := os.WriteFile(outputPath, data, 0644); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Saved team config to %s\n", outputPath)
}

func handleTeamKill(args []string) {
	if len(args) < 1 {
		fmt.Println("Usage: ratchet team kill <team-id>")
		return
	}
	// TODO: Wire to KillTeam RPC once it exists.
	fmt.Printf("team kill %s: not yet implemented\n", args[0])
}
```

Add `"path/filepath"` to imports in `cmd/ratchet/cmd_team.go`.

**Run:**
```bash
cd /Users/jon/workspace/ratchet-cli && go build ./cmd/ratchet/
```

**Commit:** `feat: ratchet team save/kill commands`

---

## Phase 2: Lifecycle

### Task 2.1: Proto Changes — All New RPCs (Batched)

**File:** `internal/proto/ratchet.proto` (modify)

Add all new messages and RPCs needed for Phase 2-5:

```protobuf
// === New messages for team lifecycle ===

message ListTeamsReq {
  string project_id = 1;  // optional filter by project
}

message TeamList {
  repeated TeamStatus teams = 1;
}

message TeamAddAgentReq {
  string team_id = 1;
  string agent_spec = 2;  // "name:provider[:model]"
}

message TeamRemoveAgentReq {
  string team_id = 1;
  string agent_name = 2;
}

message TeamRenameReq {
  string team_id = 1;
  string new_name = 2;
}

message KillTeamReq {
  string team_id = 1;
}

// Attach/detach
message AttachTeamReq {
  string team_id = 1;
  string mode = 2;  // "observe" or "join"
}

message TeamActivityEvent {
  oneof event {
    AgentMessage agent_message = 1;
    TokenDelta token = 2;
    ErrorEvent error = 3;
    SessionComplete complete = 4;
    HumanRequest human_request = 5;
  }
}

// Human-in-the-loop
message HumanRequest {
  string request_id = 1;
  string team_id = 2;
  string from_agent = 3;
  string question = 4;
  string timestamp = 5;
}

message HumanResponse {
  string request_id = 1;
  string team_id = 2;
  string content = 3;
}

message PendingHumanReq {
  string team_id = 1;  // optional filter
}

message PendingHumanList {
  repeated HumanRequest requests = 1;
}

message SteerTeamReq {
  string team_id = 1;
  string directive = 2;
}

message DirectMessageReq {
  string team_id = 1;
  string to_agent = 2;
  string content = 3;
}

// Project management
message StartProjectReq {
  string name = 1;
  string config_path = 2;
}

message ProjectStatus {
  string id = 1;
  string name = 2;
  string status = 3;
  repeated string team_ids = 4;
  string created_at = 5;
}

message ProjectList {
  repeated ProjectStatus projects = 1;
}

message ProjectReq {
  string project_id = 1;
}

// Task tracker
message TaskCreateReq {
  string title = 1;
  string project_id = 2;
  string assigned_team = 3;
  int32 priority = 4;
  string description = 5;
}

message TaskUpdateReq {
  string task_id = 1;
  string status = 2;
  string notes = 3;
}

message TaskClaimReq {
  string task_id = 1;
  string agent_name = 2;
}

message TaskListReq {
  string project_id = 1;
  string team = 2;
  string status = 3;
  int32 limit = 4;
}

message TaskInfo {
  string id = 1;
  string title = 2;
  string status = 3;
  string assigned_team = 4;
  string claimed_by = 5;
  int32 priority = 6;
  string description = 7;
  string project_id = 8;
  string created_at = 9;
  string updated_at = 10;
}

message TaskList {
  repeated TaskInfo tasks = 1;
}

message TaskReq {
  string task_id = 1;
}
```

Add new RPCs to the service:
```protobuf
service RatchetDaemon {
  // ... existing RPCs ...

  // Teams (extended)
  rpc ListTeams(ListTeamsReq) returns (TeamList);
  rpc KillTeam(KillTeamReq) returns (Empty);
  rpc RenameTeam(TeamRenameReq) returns (Empty);
  rpc TeamAddAgent(TeamAddAgentReq) returns (Empty);
  rpc TeamRemoveAgent(TeamRemoveAgentReq) returns (Empty);
  rpc AttachTeam(AttachTeamReq) returns (stream TeamActivityEvent);
  rpc SteerTeam(SteerTeamReq) returns (Empty);
  rpc DirectMessage(DirectMessageReq) returns (Empty);

  // Human-in-the-loop
  rpc RespondToHuman(HumanResponse) returns (Empty);
  rpc ListPendingHuman(PendingHumanReq) returns (PendingHumanList);

  // Projects
  rpc StartProject(StartProjectReq) returns (ProjectStatus);
  rpc ListProjects(Empty) returns (ProjectList);
  rpc PauseProject(ProjectReq) returns (Empty);
  rpc ResumeProject(ProjectReq) returns (Empty);
  rpc KillProject(ProjectReq) returns (Empty);
  rpc GetProjectStatus(ProjectReq) returns (ProjectStatus);

  // Task tracker
  rpc CreateTask(TaskCreateReq) returns (TaskInfo);
  rpc ClaimTask(TaskClaimReq) returns (TaskInfo);
  rpc UpdateTask(TaskUpdateReq) returns (TaskInfo);
  rpc ListTasks(TaskListReq) returns (TaskList);
  rpc GetTask(TaskReq) returns (TaskInfo);
}
```

**Run:**
```bash
cd /Users/jon/workspace/ratchet-cli && protoc --go_out=. --go-grpc_out=. --go_opt=paths=source_relative --go-grpc_opt=paths=source_relative internal/proto/ratchet.proto
cd /Users/jon/workspace/ratchet-cli && go build ./...
```

**Commit:** `feat(proto): add RPCs for team lifecycle, projects, human gate, task tracker`

---

### Task 2.2: Team ID/Naming — Short IDs + Rename

**Files:**
- `internal/mesh/mesh.go` (modify — short IDs)
- `internal/daemon/teams.go` (modify — rename, ListTeams)
- `internal/daemon/teams_test.go` (new)

**Test first** (`internal/daemon/teams_test.go`):
```go
package daemon

import (
	"testing"
)

func TestTeamShortID(t *testing.T) {
	// Verify that short team IDs follow "t-XXXX" pattern.
	id := generateTeamShortID()
	if len(id) < 6 || id[:2] != "t-" {
		t.Errorf("got ID %q, want t-XXXX pattern", id)
	}
}

func TestTeamRename(t *testing.T) {
	tm := NewTeamManager(nil, nil)

	// Simulate a team instance.
	ti := &teamInstance{
		id:     "t-abcd",
		task:   "test",
		agents: make(map[string]*teamAgent),
		status: "running",
	}
	tm.mu.Lock()
	tm.teams["t-abcd"] = ti
	tm.mu.Unlock()

	// Rename.
	if err := tm.Rename("t-abcd", "email-dev"); err != nil {
		t.Fatalf("Rename: %v", err)
	}

	// Lookup by new name.
	if _, err := tm.GetStatus("email-dev"); err != nil {
		t.Fatalf("GetStatus by name: %v", err)
	}

	// Lookup by old ID still works.
	if _, err := tm.GetStatus("t-abcd"); err != nil {
		t.Fatalf("GetStatus by ID: %v", err)
	}
}
```

**Implementation** — add to `internal/daemon/teams.go`:
```go
import (
	"crypto/rand"
	"encoding/hex"
)

func generateTeamShortID() string {
	b := make([]byte, 2)
	if _, err := rand.Read(b); err != nil {
		return "t-" + uuid.NewString()[:4]
	}
	return "t-" + hex.EncodeToString(b)
}
```

Modify `StartTeam` and `StartMeshTeam` to use `generateTeamShortID()` instead of `uuid.New().String()`.

Add name→ID mapping:
```go
type TeamManager struct {
	mu      sync.RWMutex
	teams   map[string]*teamInstance
	names   map[string]string // user-assigned name → team ID
	engine  *EngineContext
	hooks   *hooks.HookConfig
	stop    chan struct{}
	mesh    *mesh.AgentMesh
}
```

Initialize `names` in `NewTeamManager`:
```go
func NewTeamManager(engine *EngineContext, hks *hooks.HookConfig) *TeamManager {
	tm := &TeamManager{
		teams:  make(map[string]*teamInstance),
		names:  make(map[string]string),
		engine: engine,
		hooks:  hks,
		stop:   make(chan struct{}),
		mesh:   mesh.NewAgentMesh(),
	}
	go tm.cleanupLoop()
	return tm
}
```

Add `Rename` and `ListTeams`:
```go
// Rename assigns a user-friendly name to a team.
func (tm *TeamManager) Rename(teamID, newName string) error {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	if _, ok := tm.teams[teamID]; !ok {
		return fmt.Errorf("team %q not found", teamID)
	}
	if existing, ok := tm.names[newName]; ok && existing != teamID {
		return fmt.Errorf("name %q already assigned to team %s", newName, existing)
	}
	// Remove old name if one exists.
	for name, id := range tm.names {
		if id == teamID {
			delete(tm.names, name)
			break
		}
	}
	tm.names[newName] = teamID
	return nil
}

// resolveTeamID maps a name-or-ID to a canonical team ID.
func (tm *TeamManager) resolveTeamID(idOrName string) string {
	if _, ok := tm.teams[idOrName]; ok {
		return idOrName
	}
	if id, ok := tm.names[idOrName]; ok {
		return id
	}
	return idOrName
}

// ListTeams returns status for all teams, optionally filtered by project.
func (tm *TeamManager) ListTeams(projectID string) []*pb.TeamStatus {
	tm.mu.RLock()
	defer tm.mu.RUnlock()
	var out []*pb.TeamStatus
	for _, ti := range tm.teams {
		ti.mu.RLock()
		var agents []*pb.Agent
		for _, ag := range ti.agents {
			ag.mu.RLock()
			agents = append(agents, &pb.Agent{
				Id:     ag.id,
				Name:   ag.name,
				Role:   ag.role,
				Status: ag.status,
			})
			ag.mu.RUnlock()
		}
		out = append(out, &pb.TeamStatus{
			TeamId: ti.id,
			Task:   ti.task,
			Agents: agents,
			Status: ti.status,
		})
		ti.mu.RUnlock()
	}
	return out
}
```

Update `GetStatus` to use `resolveTeamID`:
```go
func (tm *TeamManager) GetStatus(teamID string) (*pb.TeamStatus, error) {
	tm.mu.RLock()
	resolved := tm.resolveTeamID(teamID)
	ti, ok := tm.teams[resolved]
	tm.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("team %s not found", teamID)
	}
	// ... rest same as before
}
```

**Run:**
```bash
cd /Users/jon/workspace/ratchet-cli && go test ./internal/daemon/ -run TestTeamShortID -v
cd /Users/jon/workspace/ratchet-cli && go test ./internal/daemon/ -run TestTeamRename -v
```

**Commit:** `feat: short team IDs (t-XXXX), rename, name→ID mapping`

---

### Task 2.3: Dynamic Add/Remove Agents

**Files:**
- `internal/mesh/mesh.go` (modify — add `AddNode`, `RemoveNode`)
- `internal/mesh/mesh_test.go` (new tests)
- `internal/daemon/teams.go` (modify — add/remove handlers)

**Test first** — add to `internal/mesh/mesh_test.go`:
```go
package mesh

import (
	"context"
	"testing"
	"time"

	"github.com/GoCodeAlone/workflow-plugin-agent/provider"
)

func TestAddRemoveNode(t *testing.T) {
	m := NewAgentMesh()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Create a minimal team first.
	noopFactory := func(cfg NodeConfig) provider.Provider { return nil }

	// We can't easily spawn a real team in unit tests, so test the
	// AddNode/RemoveNode plumbing on AgentMesh directly.
	bb := NewBlackboard()
	router := NewRouter()

	// Add a node.
	cfg := NodeConfig{Name: "debugger", Role: "worker", Provider: "ollama", Location: "local"}
	err := m.AddNodeToTeam(ctx, "team-1", cfg, bb, router, nil)
	if err != nil {
		t.Fatalf("AddNodeToTeam: %v", err)
	}

	// Verify BB was updated.
	members := bb.List("team/members")
	if members == nil {
		t.Fatal("expected team/members section")
	}

	// Remove the node.
	if err := m.RemoveNodeFromTeam("team-1", "debugger", router); err != nil {
		t.Fatalf("RemoveNodeFromTeam: %v", err)
	}
}
```

**Implementation** — add to `internal/mesh/mesh.go`:
```go
// AddNodeToTeam dynamically adds a new node to a running team.
// The node is registered with the provided router and its presence is
// recorded in the BB team/members section.
func (m *AgentMesh) AddNodeToTeam(
	ctx context.Context,
	teamID string,
	cfg NodeConfig,
	bb *Blackboard,
	router *Router,
	providerFactory func(NodeConfig) provider.Provider,
) error {
	node := NewLocalNode(cfg, nil, nil) // provider set below
	if providerFactory != nil {
		prov := providerFactory(cfg)
		if prov != nil {
			node = NewLocalNode(cfg, prov, nil)
		}
	}

	// Register with router.
	inbox, err := router.Register(cfg.Name)
	if err != nil {
		return fmt.Errorf("register node %s: %w", cfg.Name, err)
	}

	// Record in mesh registry.
	m.mu.Lock()
	m.nodes[cfg.Name] = node
	m.mu.Unlock()

	// Update BB roster.
	bb.Write("team/members", cfg.Name, map[string]string{
		"role":     cfg.Role,
		"provider": cfg.Provider,
		"status":   "active",
	}, "mesh")

	// Notify orchestrator by writing to BB.
	bb.Write("team/events", fmt.Sprintf("add-%s-%d", cfg.Name, time.Now().UnixMilli()),
		fmt.Sprintf("agent %q (%s) joined the team", cfg.Name, cfg.Role), "mesh")

	// Start the node in a goroutine.
	outbox := make(chan Message, 64)
	go func() {
		defer router.Unregister(cfg.Name)
		go func() {
			for msg := range outbox {
				_ = router.Send(msg)
			}
		}()
		_ = node.Run(ctx, "", bb, inbox, outbox)
		close(outbox)
	}()

	return nil
}

// RemoveNodeFromTeam removes a node from a running team.
func (m *AgentMesh) RemoveNodeFromTeam(teamID, nodeName string, router *Router) error {
	m.mu.Lock()
	node, ok := m.nodes[nodeName]
	if !ok {
		m.mu.Unlock()
		return fmt.Errorf("node %q not found", nodeName)
	}
	delete(m.nodes, nodeName)
	m.mu.Unlock()

	// Unregister from router (closes inbox, which causes node.Run to exit).
	router.Unregister(nodeName)
	if info := node.Info(); info.Name != "" && info.Name != nodeName {
		router.Unregister(info.Name)
	}

	return nil
}
```

Wire into `TeamManager` in `internal/daemon/teams.go`:
```go
// AddAgent dynamically adds an agent to a running team.
func (tm *TeamManager) AddAgent(teamID, agentSpec string) error {
	tm.mu.RLock()
	resolved := tm.resolveTeamID(teamID)
	ti, ok := tm.teams[resolved]
	tm.mu.RUnlock()
	if !ok {
		return fmt.Errorf("team %q not found", teamID)
	}

	ac, err := mesh.ParseAgentFlag(agentSpec)
	if err != nil {
		return fmt.Errorf("parse agent spec: %w", err)
	}

	cfg := mesh.NodeConfig{
		Name:     ac.Name,
		Role:     ac.Role,
		Provider: ac.Provider,
		Model:    ac.Model,
		Location: "local",
	}

	// TODO: pass real BB/Router from the team's mesh instance.
	_ = ti
	_ = cfg
	return fmt.Errorf("dynamic add not yet wired to team mesh instance")
}

// RemoveAgent dynamically removes an agent from a running team.
func (tm *TeamManager) RemoveAgent(teamID, agentName string) error {
	tm.mu.RLock()
	resolved := tm.resolveTeamID(teamID)
	ti, ok := tm.teams[resolved]
	tm.mu.RUnlock()
	if !ok {
		return fmt.Errorf("team %q not found", teamID)
	}

	ti.mu.Lock()
	for id, ag := range ti.agents {
		if ag.name == agentName {
			delete(ti.agents, id)
			ti.mu.Unlock()
			return nil
		}
	}
	ti.mu.Unlock()
	return fmt.Errorf("agent %q not found in team %s", agentName, teamID)
}
```

**Run:**
```bash
cd /Users/jon/workspace/ratchet-cli && go test ./internal/mesh/ -run TestAddRemoveNode -v
```

**Commit:** `feat: dynamic AddNode/RemoveNode for running teams`

---

### Task 2.4: Attach/Detach with Observe/Join Modes

**Files:**
- `internal/daemon/teams.go` (modify — add attach/detach)
- `internal/daemon/service.go` (modify — implement AttachTeam RPC)
- `cmd/ratchet/cmd_team.go` (modify — add `attach` subcommand)

**Implementation** — add to `internal/daemon/teams.go`:
```go
// teamObserver tracks an attached client.
type teamObserver struct {
	id     string
	mode   string // "observe" or "join"
	events chan *pb.TeamActivityEvent
	cancel context.CancelFunc
}

// Add to teamInstance:
// observers map[string]*teamObserver

// AttachTeam registers an observer for a team and returns the event channel.
func (tm *TeamManager) AttachTeam(teamID, mode string) (string, <-chan *pb.TeamActivityEvent, error) {
	tm.mu.RLock()
	resolved := tm.resolveTeamID(teamID)
	ti, ok := tm.teams[resolved]
	tm.mu.RUnlock()
	if !ok {
		return "", nil, fmt.Errorf("team %q not found", teamID)
	}

	obsID := "obs-" + uuid.NewString()[:8]
	ctx, cancel := context.WithCancel(context.Background())
	ch := make(chan *pb.TeamActivityEvent, 64)

	obs := &teamObserver{
		id:     obsID,
		mode:   mode,
		events: ch,
		cancel: cancel,
	}

	ti.mu.Lock()
	if ti.observers == nil {
		ti.observers = make(map[string]*teamObserver)
	}
	ti.observers[obsID] = obs
	ti.mu.Unlock()

	// Cleanup when context is done.
	go func() {
		<-ctx.Done()
		ti.mu.Lock()
		delete(ti.observers, obsID)
		ti.mu.Unlock()
		close(ch)
	}()

	return obsID, ch, nil
}

// DetachTeam removes an observer.
func (tm *TeamManager) DetachTeam(teamID, observerID string) {
	tm.mu.RLock()
	resolved := tm.resolveTeamID(teamID)
	ti, ok := tm.teams[resolved]
	tm.mu.RUnlock()
	if !ok {
		return
	}
	ti.mu.Lock()
	if obs, ok := ti.observers[observerID]; ok {
		obs.cancel()
	}
	ti.mu.Unlock()
}

// broadcastToObservers sends an event to all attached observers.
func (tm *TeamManager) broadcastToObservers(ti *teamInstance, event *pb.TeamActivityEvent) {
	ti.mu.RLock()
	defer ti.mu.RUnlock()
	for _, obs := range ti.observers {
		select {
		case obs.events <- event:
		default:
			// Drop if full.
		}
	}
}
```

Add `observers` field to `teamInstance`:
```go
type teamInstance struct {
	mu          sync.RWMutex
	id          string
	task        string
	agents      map[string]*teamAgent
	observers   map[string]*teamObserver
	status      string
	cancel      context.CancelFunc
	eventCh     chan *pb.TeamEvent
	completedAt time.Time
}
```

**Implement `AttachTeam` RPC** in `internal/daemon/service.go`:
```go
func (s *Service) AttachTeam(req *pb.AttachTeamReq, stream pb.RatchetDaemon_AttachTeamServer) error {
	mode := req.Mode
	if mode == "" {
		mode = "observe"
	}

	obsID, ch, err := s.teams.AttachTeam(req.TeamId, mode)
	if err != nil {
		return status.Errorf(codes.NotFound, "%v", err)
	}
	defer s.teams.DetachTeam(req.TeamId, obsID)

	for {
		select {
		case ev, ok := <-ch:
			if !ok {
				return nil
			}
			if err := stream.Send(ev); err != nil {
				return err
			}
		case <-stream.Context().Done():
			return nil
		}
	}
}
```

**CLI** — add to `cmd/ratchet/cmd_team.go`:
```go
case "attach":
	handleTeamAttach(args[1:])
case "rename":
	handleTeamRename(args[1:])
case "add":
	handleTeamAddAgent(args[1:])
case "remove":
	handleTeamRemoveAgent(args[1:])
```

```go
func handleTeamAttach(args []string) {
	if len(args) < 1 {
		fmt.Println("Usage: ratchet team attach <team-id> [--join]")
		return
	}
	teamID := args[0]
	mode := "observe"
	for _, a := range args[1:] {
		if a == "--join" {
			mode = "join"
		}
	}

	c, err := client.EnsureDaemon()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	defer c.Close()

	fmt.Printf("Attaching to team %s (mode: %s). Ctrl+D to detach.\n", teamID, mode)
	// TODO: Wire to AttachTeam streaming RPC once client method exists.
	fmt.Println("attach: streaming not yet implemented")
}

func handleTeamRename(args []string) {
	if len(args) < 2 {
		fmt.Println("Usage: ratchet team rename <team-id> <new-name>")
		return
	}
	fmt.Printf("team rename: %s → %s (not yet wired to RPC)\n", args[0], args[1])
}

func handleTeamAddAgent(args []string) {
	if len(args) < 2 {
		fmt.Println("Usage: ratchet team add <team-id> <name:provider[:model]>")
		return
	}
	fmt.Printf("team add: %s to %s (not yet wired to RPC)\n", args[1], args[0])
}

func handleTeamRemoveAgent(args []string) {
	if len(args) < 2 {
		fmt.Println("Usage: ratchet team remove <team-id> <agent-name>")
		return
	}
	fmt.Printf("team remove: %s from %s (not yet wired to RPC)\n", args[1], args[0])
}
```

**Run:**
```bash
cd /Users/jon/workspace/ratchet-cli && go build ./...
```

**Commit:** `feat: attach/detach with observe/join modes, rename, add/remove CLI`

---

### Task 2.5: Wire RPC Handlers for ListTeams, KillTeam, RenameTeam, Add/Remove

**Files:**
- `internal/daemon/service.go` (modify)

**Implementation** — add to `internal/daemon/service.go`:
```go
func (s *Service) ListTeams(ctx context.Context, req *pb.ListTeamsReq) (*pb.TeamList, error) {
	teams := s.teams.ListTeams(req.ProjectId)
	return &pb.TeamList{Teams: teams}, nil
}

func (s *Service) KillTeam(ctx context.Context, req *pb.KillTeamReq) (*pb.Empty, error) {
	if err := s.teams.KillAgent(req.TeamId); err != nil {
		return nil, status.Errorf(codes.NotFound, "%v", err)
	}
	return &pb.Empty{}, nil
}

func (s *Service) RenameTeam(ctx context.Context, req *pb.TeamRenameReq) (*pb.Empty, error) {
	if err := s.teams.Rename(req.TeamId, req.NewName); err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "%v", err)
	}
	return &pb.Empty{}, nil
}

func (s *Service) TeamAddAgent(ctx context.Context, req *pb.TeamAddAgentReq) (*pb.Empty, error) {
	if err := s.teams.AddAgent(req.TeamId, req.AgentSpec); err != nil {
		return nil, status.Errorf(codes.Internal, "%v", err)
	}
	return &pb.Empty{}, nil
}

func (s *Service) TeamRemoveAgent(ctx context.Context, req *pb.TeamRemoveAgentReq) (*pb.Empty, error) {
	if err := s.teams.RemoveAgent(req.TeamId, req.AgentName); err != nil {
		return nil, status.Errorf(codes.NotFound, "%v", err)
	}
	return &pb.Empty{}, nil
}

func (s *Service) SteerTeam(ctx context.Context, req *pb.SteerTeamReq) (*pb.Empty, error) {
	// Steer sends a directive message to the team's orchestrator via the router.
	// TODO: Wire to team's router once BB/Router are accessible per-team.
	return &pb.Empty{}, nil
}

func (s *Service) DirectMessage(ctx context.Context, req *pb.DirectMessageReq) (*pb.Empty, error) {
	// DirectMessage sends to a specific agent in the team via the router.
	// TODO: Wire to team's router.
	return &pb.Empty{}, nil
}
```

**Run:**
```bash
cd /Users/jon/workspace/ratchet-cli && go build ./...
```

**Commit:** `feat: wire ListTeams, KillTeam, RenameTeam, Add/Remove RPC handlers`

---

## Phase 3: Human-in-the-Loop

### Task 3.1: Human Gate — Message Queue + Pause

**Files:**
- `internal/daemon/human_gate.go` (new)
- `internal/daemon/human_gate_test.go` (new)

**Test first** (`internal/daemon/human_gate_test.go`):
```go
package daemon

import (
	"context"
	"testing"
	"time"
)

func TestHumanGate(t *testing.T) {
	hg := NewHumanGate()

	// Queue a request.
	reqID := hg.Request("t-1234", "architect", "REST or gRPC for the API?")

	// Check pending.
	pending := hg.Pending("")
	if len(pending) != 1 {
		t.Fatalf("got %d pending, want 1", len(pending))
	}
	if pending[0].Question != "REST or gRPC for the API?" {
		t.Errorf("got question %q", pending[0].Question)
	}

	// Respond (simulates user).
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	go func() {
		time.Sleep(100 * time.Millisecond)
		hg.Respond(reqID, "Use REST")
	}()

	response, err := hg.Wait(ctx, reqID)
	if err != nil {
		t.Fatalf("Wait: %v", err)
	}
	if response != "Use REST" {
		t.Errorf("got response %q, want %q", response, "Use REST")
	}

	// Pending should be empty now.
	pending = hg.Pending("")
	if len(pending) != 0 {
		t.Errorf("got %d pending, want 0", len(pending))
	}
}

func TestHumanGateFilterByTeam(t *testing.T) {
	hg := NewHumanGate()
	hg.Request("team-a", "agent1", "q1")
	hg.Request("team-b", "agent2", "q2")

	a := hg.Pending("team-a")
	if len(a) != 1 {
		t.Errorf("team-a: got %d pending, want 1", len(a))
	}
	all := hg.Pending("")
	if len(all) != 2 {
		t.Errorf("all: got %d pending, want 2", len(all))
	}
}
```

**Implementation** (`internal/daemon/human_gate.go`):
```go
package daemon

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
)

// HumanRequest represents a pending question from an agent.
type HumanRequestEntry struct {
	ID        string
	TeamID    string
	FromAgent string
	Question  string
	Timestamp time.Time
	response  chan string
}

// HumanGate queues human requests and blocks agents until answered.
type HumanGate struct {
	mu       sync.Mutex
	pending  map[string]*HumanRequestEntry // reqID → entry
}

// NewHumanGate returns an initialized HumanGate.
func NewHumanGate() *HumanGate {
	return &HumanGate{
		pending: make(map[string]*HumanRequestEntry),
	}
}

// Request enqueues a human request and returns its ID.
// The calling agent should then call Wait(ctx, id) to block until responded.
func (hg *HumanGate) Request(teamID, fromAgent, question string) string {
	reqID := "hr-" + uuid.NewString()[:8]
	entry := &HumanRequestEntry{
		ID:        reqID,
		TeamID:    teamID,
		FromAgent: fromAgent,
		Question:  question,
		Timestamp: time.Now(),
		response:  make(chan string, 1),
	}
	hg.mu.Lock()
	hg.pending[reqID] = entry
	hg.mu.Unlock()
	return reqID
}

// Wait blocks until the human responds or the context is cancelled.
func (hg *HumanGate) Wait(ctx context.Context, reqID string) (string, error) {
	hg.mu.Lock()
	entry, ok := hg.pending[reqID]
	hg.mu.Unlock()
	if !ok {
		return "", fmt.Errorf("request %q not found", reqID)
	}

	select {
	case resp := <-entry.response:
		hg.mu.Lock()
		delete(hg.pending, reqID)
		hg.mu.Unlock()
		return resp, nil
	case <-ctx.Done():
		return "", ctx.Err()
	}
}

// Respond provides a human response to a pending request.
func (hg *HumanGate) Respond(reqID, content string) error {
	hg.mu.Lock()
	entry, ok := hg.pending[reqID]
	hg.mu.Unlock()
	if !ok {
		return fmt.Errorf("request %q not found or already responded", reqID)
	}
	entry.response <- content
	return nil
}

// Pending returns all pending human requests, optionally filtered by team.
func (hg *HumanGate) Pending(teamID string) []HumanRequestEntry {
	hg.mu.Lock()
	defer hg.mu.Unlock()
	var out []HumanRequestEntry
	for _, e := range hg.pending {
		if teamID == "" || e.TeamID == teamID {
			out = append(out, HumanRequestEntry{
				ID:        e.ID,
				TeamID:    e.TeamID,
				FromAgent: e.FromAgent,
				Question:  e.Question,
				Timestamp: e.Timestamp,
			})
		}
	}
	return out
}
```

**Run:**
```bash
cd /Users/jon/workspace/ratchet-cli && go test ./internal/daemon/ -run TestHumanGate -v
```

**Commit:** `feat: human gate — message queue with blocking Wait/Respond`

---

### Task 3.2: Autoresponder

**Files:**
- `internal/daemon/autoresponder.go` (new)
- `internal/daemon/autoresponder_test.go` (new)

**Test first** (`internal/daemon/autoresponder_test.go`):
```go
package daemon

import (
	"testing"
)

func TestAutoresponder(t *testing.T) {
	rules := []AutorespondRule{
		{Match: "approval", Action: "approve"},
		{Match: "which.*approach", Action: "reply", Message: "Use the simpler approach."},
		{Match: "*", Action: "queue"},
	}
	ar := NewAutoresponder(rules)

	tests := []struct {
		question   string
		wantAction string
		wantMsg    string
	}{
		{"Can I get approval to deploy?", "approve", "approved"},
		{"Which approach should I take?", "reply", "Use the simpler approach."},
		{"Random question", "queue", ""},
	}

	for _, tt := range tests {
		action, msg := ar.Match(tt.question)
		if action != tt.wantAction {
			t.Errorf("Match(%q): action=%q, want %q", tt.question, action, tt.wantAction)
		}
		if tt.wantMsg != "" && msg != tt.wantMsg {
			t.Errorf("Match(%q): msg=%q, want %q", tt.question, msg, tt.wantMsg)
		}
	}
}
```

**Implementation** (`internal/daemon/autoresponder.go`):
```go
package daemon

import (
	"os"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// AutorespondRule defines a pattern-matching rule.
type AutorespondRule struct {
	Match   string `yaml:"match"`
	Action  string `yaml:"action"`  // "approve", "reply", "queue"
	Message string `yaml:"message"` // used when action is "reply"
}

// AutorespondConfig is the top-level config from .ratchet/autorespond.yaml.
type AutorespondConfig struct {
	Rules []AutorespondRule `yaml:"rules"`
}

// Autoresponder evaluates incoming human requests against rules.
type Autoresponder struct {
	rules []autoresponderCompiledRule
}

type autoresponderCompiledRule struct {
	pattern *regexp.Regexp
	action  string
	message string
	catchAll bool
}

// NewAutoresponder compiles rules into an evaluator.
func NewAutoresponder(rules []AutorespondRule) *Autoresponder {
	compiled := make([]autoresponderCompiledRule, 0, len(rules))
	for _, r := range rules {
		cr := autoresponderCompiledRule{
			action:  r.Action,
			message: r.Message,
		}
		if r.Match == "*" {
			cr.catchAll = true
		} else {
			re, err := regexp.Compile("(?i)" + r.Match)
			if err != nil {
				continue
			}
			cr.pattern = re
		}
		compiled = append(compiled, cr)
	}
	return &Autoresponder{rules: compiled}
}

// LoadAutoresponder reads .ratchet/autorespond.yaml if it exists.
func LoadAutoresponder(projectDir string) *Autoresponder {
	path := projectDir + "/.ratchet/autorespond.yaml"
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var cfg AutorespondConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil
	}
	return NewAutoresponder(cfg.Rules)
}

// Match evaluates the question against rules and returns (action, message).
// Returns ("queue", "") if no rule matches.
func (ar *Autoresponder) Match(question string) (string, string) {
	lower := strings.ToLower(question)
	for _, r := range ar.rules {
		if r.catchAll {
			return r.action, r.message
		}
		if r.pattern != nil && r.pattern.MatchString(lower) {
			switch r.action {
			case "approve":
				return "approve", "approved"
			case "reply":
				return "reply", r.message
			default:
				return r.action, r.message
			}
		}
	}
	return "queue", ""
}
```

**Run:**
```bash
cd /Users/jon/workspace/ratchet-cli && go test ./internal/daemon/ -run TestAutoresponder -v
```

**Commit:** `feat: autoresponder with pattern-matching rules`

---

### Task 3.3: OS Notifications for Idle Teams

**Files:**
- `internal/daemon/notifications.go` (new)
- `internal/daemon/notifications_test.go` (new)

**Test first** (`internal/daemon/notifications_test.go`):
```go
package daemon

import (
	"runtime"
	"testing"
)

func TestNotificationCommand(t *testing.T) {
	cmd := buildNotifyCommand("Team t-3a7f idle", "No activity for 5 minutes")
	if cmd == nil {
		t.Skip("no notification command available on this platform")
	}
	// Just verify the command structure, don't execute.
	switch runtime.GOOS {
	case "darwin":
		if cmd.Path == "" {
			t.Error("expected osascript path")
		}
	case "linux":
		if cmd.Path == "" {
			t.Error("expected notify-send path")
		}
	}
}
```

**Implementation** (`internal/daemon/notifications.go`):
```go
package daemon

import (
	"log"
	"os/exec"
	"runtime"
)

// buildNotifyCommand returns an exec.Cmd for an OS-native notification.
// Returns nil if the platform is unsupported.
func buildNotifyCommand(title, body string) *exec.Cmd {
	switch runtime.GOOS {
	case "darwin":
		script := `display notification "` + body + `" with title "` + title + `"`
		return exec.Command("osascript", "-e", script)
	case "linux":
		path, err := exec.LookPath("notify-send")
		if err != nil {
			return nil
		}
		return exec.Command(path, title, body)
	case "windows":
		return exec.Command("powershell", "-Command",
			`New-BurntToastNotification -Text "`+title+`", "`+body+`"`)
	default:
		return nil
	}
}

// SendNotification sends an OS-native notification. Non-blocking; errors are logged.
func SendNotification(title, body string) {
	cmd := buildNotifyCommand(title, body)
	if cmd == nil {
		return
	}
	go func() {
		if err := cmd.Run(); err != nil {
			log.Printf("notification: %v", err)
		}
	}()
}
```

**Run:**
```bash
cd /Users/jon/workspace/ratchet-cli && go test ./internal/daemon/ -run TestNotificationCommand -v
```

**Commit:** `feat: OS-native notifications (macOS/Linux/Windows)`

---

### Task 3.4: Wire Human Gate + Notifications into Service

**Files:**
- `internal/daemon/service.go` (modify — add humanGate field, wire RPCs)

**Implementation** — add to `Service` struct:
```go
type Service struct {
	// ... existing fields ...
	humanGate    *HumanGate
	autorespond  *Autoresponder
}
```

Initialize in `NewService`:
```go
svc.humanGate = NewHumanGate()
wd, _ := os.Getwd()
svc.autorespond = LoadAutoresponder(wd)
```

Add RPC handlers:
```go
func (s *Service) RespondToHuman(ctx context.Context, req *pb.HumanResponse) (*pb.Empty, error) {
	if err := s.humanGate.Respond(req.RequestId, req.Content); err != nil {
		return nil, status.Errorf(codes.NotFound, "%v", err)
	}
	return &pb.Empty{}, nil
}

func (s *Service) ListPendingHuman(ctx context.Context, req *pb.PendingHumanReq) (*pb.PendingHumanList, error) {
	entries := s.humanGate.Pending(req.TeamId)
	var out []*pb.HumanRequest
	for _, e := range entries {
		out = append(out, &pb.HumanRequest{
			RequestId: e.ID,
			TeamId:    e.TeamID,
			FromAgent: e.FromAgent,
			Question:  e.Question,
			Timestamp: e.Timestamp.Format("15:04:05"),
		})
	}
	return &pb.PendingHumanList{Requests: out}, nil
}
```

**CLI** — add `pending` and `respond` to `cmd/ratchet/cmd_team.go`:
```go
case "pending":
	handleTeamPending(args[1:])
case "respond":
	handleTeamRespond(args[1:])
```

```go
func handleTeamPending(args []string) {
	c, err := client.EnsureDaemon()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	defer c.Close()
	// TODO: Wire to ListPendingHuman RPC.
	fmt.Println("team pending: not yet wired to RPC")
}

func handleTeamRespond(args []string) {
	if len(args) < 1 {
		fmt.Println("Usage: ratchet team respond <team-id>")
		return
	}
	fmt.Printf("team respond %s: not yet wired to RPC\n", args[0])
}
```

**Run:**
```bash
cd /Users/jon/workspace/ratchet-cli && go build ./...
```

**Commit:** `feat: wire human gate + autoresponder + notifications into service`

---

## Phase 4: Cross-Team

### Task 4.1: Blackboard Modes (shared/isolated/orchestrator/bridge)

**Files:**
- `internal/mesh/project_bb.go` (new)
- `internal/mesh/project_bb_test.go` (new)

**Test first** (`internal/mesh/project_bb_test.go`):
```go
package mesh

import (
	"testing"
)

func TestProjectBBSharedMode(t *testing.T) {
	pbb := NewProjectBlackboard()
	teamA := pbb.TeamBB("design", "shared")
	teamB := pbb.TeamBB("dev", "shared")

	// Team A writes to its namespace.
	teamA.Write("design/spec", "api", "REST API spec", "architect")

	// Team B can read team A's writes via project BB.
	if e, ok := pbb.Root().Read("design/spec", "api"); !ok {
		t.Error("team B cannot read team A's write via root")
	} else if e.Author != "architect" {
		t.Errorf("got author %q, want %q", e.Author, "architect")
	}
}

func TestProjectBBIsolatedMode(t *testing.T) {
	pbb := NewProjectBlackboard()
	teamA := pbb.TeamBB("design", "isolated")
	teamB := pbb.TeamBB("dev", "isolated")

	teamA.Write("data", "key1", "val1", "agent1")
	teamB.Write("data", "key1", "val2", "agent2")

	// Each team sees only its own data.
	if e, ok := teamA.Read("data", "key1"); !ok || e.Value != "val1" {
		t.Error("teamA should see its own value")
	}
	if e, ok := teamB.Read("data", "key1"); !ok || e.Value != "val2" {
		t.Error("teamB should see its own value")
	}
}

func TestProjectBBOrchestratorMode(t *testing.T) {
	pbb := NewProjectBlackboard()
	_ = pbb.TeamBB("dev", "shared")
	orchBB := pbb.TeamBB("oversight", "orchestrator")

	// Dev team writes.
	pbb.Root().Write("dev/code", "main.go", "package main", "coder")

	// Orchestrator can read all via root (read-only view).
	if _, ok := pbb.Root().Read("dev/code", "main.go"); !ok {
		t.Error("orchestrator cannot read dev's BB")
	}

	// Orchestrator has its own writable BB too.
	orchBB.Write("directives", "dev", "focus on tests", "director")
	if e, ok := orchBB.Read("directives", "dev"); !ok || e.Value != "focus on tests" {
		t.Error("orchestrator BB write failed")
	}
}
```

**Implementation** (`internal/mesh/project_bb.go`):
```go
package mesh

import (
	"strings"
	"sync"
)

// ProjectBlackboard manages per-team Blackboard instances with configurable
// visibility modes.
type ProjectBlackboard struct {
	mu     sync.RWMutex
	root   *Blackboard             // project-level shared BB
	teams  map[string]*Blackboard  // team name → team-private BB
	modes  map[string]string       // team name → mode
}

// NewProjectBlackboard creates a project-level BB coordinator.
func NewProjectBlackboard() *ProjectBlackboard {
	return &ProjectBlackboard{
		root:  NewBlackboard(),
		teams: make(map[string]*Blackboard),
		modes: make(map[string]string),
	}
}

// Root returns the project-level shared Blackboard.
func (pbb *ProjectBlackboard) Root() *Blackboard {
	return pbb.root
}

// TeamBB returns a Blackboard for the team based on its configured mode.
//
// Modes:
//   - "shared" (default): Returns a NamespacedBB that writes to root under <team>/ prefix,
//     reads from entire root.
//   - "isolated": Returns a private Blackboard with no cross-team visibility.
//   - "orchestrator": Returns a private BB. Team also gets read access to root.
//   - "bridge:<t1>,<t2>": Returns a shared BB between named teams.
func (pbb *ProjectBlackboard) TeamBB(teamName, mode string) *Blackboard {
	pbb.mu.Lock()
	defer pbb.mu.Unlock()

	if mode == "" {
		mode = "shared"
	}
	pbb.modes[teamName] = mode

	switch {
	case mode == "shared":
		// Shared mode: teams write to the project root BB under their namespace.
		// We return the root BB directly — agents use "<team>/<section>" naming convention.
		if _, ok := pbb.teams[teamName]; !ok {
			pbb.teams[teamName] = pbb.root
		}
		return pbb.root

	case mode == "isolated":
		if bb, ok := pbb.teams[teamName]; ok {
			return bb
		}
		bb := NewBlackboard()
		pbb.teams[teamName] = bb
		return bb

	case mode == "orchestrator":
		if bb, ok := pbb.teams[teamName]; ok {
			return bb
		}
		bb := NewBlackboard()
		pbb.teams[teamName] = bb
		return bb

	case strings.HasPrefix(mode, "bridge:"):
		// Bridge teams share a BB instance.
		bridgeKey := mode // use the full mode string as the shared key
		if bb, ok := pbb.teams[bridgeKey]; ok {
			pbb.teams[teamName] = bb
			return bb
		}
		bb := NewBlackboard()
		pbb.teams[bridgeKey] = bb
		pbb.teams[teamName] = bb
		return bb

	default:
		// Fall back to shared.
		return pbb.root
	}
}

// Mode returns the configured BB mode for a team.
func (pbb *ProjectBlackboard) Mode(teamName string) string {
	pbb.mu.RLock()
	defer pbb.mu.RUnlock()
	if m, ok := pbb.modes[teamName]; ok {
		return m
	}
	return "shared"
}
```

**Run:**
```bash
cd /Users/jon/workspace/ratchet-cli && go test ./internal/mesh/ -run TestProjectBB -v
```

**Commit:** `feat: ProjectBlackboard with shared/isolated/orchestrator/bridge modes`

---

### Task 4.2: Handoff + Directive Protocols

**Files:**
- `internal/mesh/protocols.go` (new)
- `internal/mesh/protocols_test.go` (new)

**Test first** (`internal/mesh/protocols_test.go`):
```go
package mesh

import (
	"testing"
)

func TestHandoffProtocol(t *testing.T) {
	bb := NewBlackboard()

	// Design team hands off to dev.
	WriteHandoff(bb, "design", "dev", map[string]string{
		"spec": "REST API with /users endpoint",
	})

	// Dev team reads the handoff.
	handoff, ok := ReadHandoff(bb, "design", "dev")
	if !ok {
		t.Fatal("handoff not found")
	}
	if handoff["spec"] != "REST API with /users endpoint" {
		t.Errorf("got spec %q", handoff["spec"])
	}
}

func TestDirectiveProtocol(t *testing.T) {
	bb := NewBlackboard()

	// Oversight writes directive to dev.
	WriteDirective(bb, "dev", "Focus on error handling tests")

	// Dev reads its directive.
	directive, ok := ReadLatestDirective(bb, "dev")
	if !ok {
		t.Fatal("directive not found")
	}
	if directive != "Focus on error handling tests" {
		t.Errorf("got directive %q", directive)
	}
}
```

**Implementation** (`internal/mesh/protocols.go`):
```go
package mesh

import (
	"fmt"
	"time"
)

// WriteHandoff writes a handoff from one team to another.
// Stored in BB section "handoffs/<from>-to-<to>".
func WriteHandoff(bb *Blackboard, fromTeam, toTeam string, data map[string]string) {
	section := fmt.Sprintf("handoffs/%s-to-%s", fromTeam, toTeam)
	for k, v := range data {
		bb.Write(section, k, v, fromTeam)
	}
	bb.Write(section, "_timestamp", time.Now().Format(time.RFC3339), fromTeam)
}

// ReadHandoff reads a handoff from one team to another.
func ReadHandoff(bb *Blackboard, fromTeam, toTeam string) (map[string]string, bool) {
	section := fmt.Sprintf("handoffs/%s-to-%s", fromTeam, toTeam)
	entries := bb.List(section)
	if entries == nil || len(entries) == 0 {
		return nil, false
	}
	result := make(map[string]string, len(entries))
	for k, e := range entries {
		if k == "_timestamp" || k == "_init" {
			continue
		}
		result[k] = fmt.Sprintf("%v", e.Value)
	}
	return result, len(result) > 0
}

// WriteDirective writes a directive to a team's directive section.
func WriteDirective(bb *Blackboard, toTeam, directive string) {
	section := fmt.Sprintf("directives/%s", toTeam)
	key := fmt.Sprintf("d-%d", time.Now().UnixMilli())
	bb.Write(section, key, directive, "oversight")
}

// ReadLatestDirective reads the most recent directive for a team.
func ReadLatestDirective(bb *Blackboard, teamName string) (string, bool) {
	section := fmt.Sprintf("directives/%s", teamName)
	entries := bb.List(section)
	if entries == nil || len(entries) == 0 {
		return "", false
	}
	// Find highest revision entry.
	var latest Entry
	for k, e := range entries {
		if k == "_init" {
			continue
		}
		if e.Revision > latest.Revision {
			latest = e
		}
	}
	if latest.Key == "" {
		return "", false
	}
	return fmt.Sprintf("%v", latest.Value), true
}
```

**Run:**
```bash
cd /Users/jon/workspace/ratchet-cli && go test ./internal/mesh/ -run TestHandoffProtocol -v
cd /Users/jon/workspace/ratchet-cli && go test ./internal/mesh/ -run TestDirectiveProtocol -v
```

**Commit:** `feat: handoff + directive cross-team protocols`

---

### Task 4.3: Wire Project Registry into Daemon

**Files:**
- `internal/daemon/service.go` (modify — add projects field, wire RPCs)

**Implementation** — add to `Service` struct:
```go
type Service struct {
	// ... existing fields ...
	projects *ProjectRegistry
}
```

Initialize in `NewService`:
```go
svc.projects = NewProjectRegistry()
```

Add RPC handlers:
```go
func (s *Service) StartProject(ctx context.Context, req *pb.StartProjectReq) (*pb.ProjectStatus, error) {
	p, err := s.projects.Register(req.Name, req.ConfigPath)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "%v", err)
	}

	// If config path is provided, load and start teams.
	if req.ConfigPath != "" {
		pc, err := mesh.LoadProjectConfig(req.ConfigPath)
		if err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "load project config: %v", err)
		}

		for _, teamCfg := range pc.Teams {
			tc := teamCfg.ToTeamConfig()
			configs := mesh.ToNodeConfigs(tc)
			teamID, _ := s.teams.StartMeshTeam(ctx, "project:"+p.Name, configs, nil)
			if teamID != "" {
				s.projects.AddTeam(p.ID, teamID)
			}
		}
	}

	return &pb.ProjectStatus{
		Id:        p.ID,
		Name:      p.Name,
		Status:    p.Status,
		TeamIds:   p.TeamIDs,
		CreatedAt: p.CreatedAt.Format(time.RFC3339),
	}, nil
}

func (s *Service) ListProjects(ctx context.Context, _ *pb.Empty) (*pb.ProjectList, error) {
	projects := s.projects.List()
	var out []*pb.ProjectStatus
	for _, p := range projects {
		out = append(out, &pb.ProjectStatus{
			Id:        p.ID,
			Name:      p.Name,
			Status:    p.Status,
			TeamIds:   p.TeamIDs,
			CreatedAt: p.CreatedAt.Format(time.RFC3339),
		})
	}
	return &pb.ProjectList{Projects: out}, nil
}

func (s *Service) PauseProject(ctx context.Context, req *pb.ProjectReq) (*pb.Empty, error) {
	if err := s.projects.SetStatus(req.ProjectId, "paused"); err != nil {
		return nil, status.Errorf(codes.NotFound, "%v", err)
	}
	return &pb.Empty{}, nil
}

func (s *Service) ResumeProject(ctx context.Context, req *pb.ProjectReq) (*pb.Empty, error) {
	if err := s.projects.SetStatus(req.ProjectId, "active"); err != nil {
		return nil, status.Errorf(codes.NotFound, "%v", err)
	}
	return &pb.Empty{}, nil
}

func (s *Service) KillProject(ctx context.Context, req *pb.ProjectReq) (*pb.Empty, error) {
	p, err := s.projects.Get(req.ProjectId)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "%v", err)
	}
	// Kill all teams in the project.
	for _, teamID := range p.TeamIDs {
		_ = s.teams.KillAgent(teamID)
	}
	if err := s.projects.SetStatus(req.ProjectId, "killed"); err != nil {
		return nil, status.Errorf(codes.Internal, "%v", err)
	}
	return &pb.Empty{}, nil
}

func (s *Service) GetProjectStatus(ctx context.Context, req *pb.ProjectReq) (*pb.ProjectStatus, error) {
	p, err := s.projects.Get(req.ProjectId)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "%v", err)
	}
	return &pb.ProjectStatus{
		Id:        p.ID,
		Name:      p.Name,
		Status:    p.Status,
		TeamIds:   p.TeamIDs,
		CreatedAt: p.CreatedAt.Format(time.RFC3339),
	}, nil
}
```

**Run:**
```bash
cd /Users/jon/workspace/ratchet-cli && go build ./...
```

**Commit:** `feat: wire project registry RPCs into daemon service`

---

## Phase 5: Tracker

### Task 5.1: SQLite Task Tracker Schema + Core

**Files:**
- `internal/mesh/tracker.go` (new)
- `internal/mesh/tracker_test.go` (new)

**Test first** (`internal/mesh/tracker_test.go`):
```go
package mesh

import (
	"database/sql"
	"testing"

	_ "github.com/mattn/go-sqlite3"
)

func testDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func TestTrackerCreateAndGet(t *testing.T) {
	db := testDB(t)
	tr, err := NewTracker(db)
	if err != nil {
		t.Fatal(err)
	}

	// Create a project.
	projID, err := tr.CreateProject("email-service", "")
	if err != nil {
		t.Fatal(err)
	}

	// Create a task.
	taskID, err := tr.CreateTask(projID, "Implement API", "Build REST endpoints", "dev", 1)
	if err != nil {
		t.Fatal(err)
	}
	if taskID == "" {
		t.Fatal("empty task ID")
	}

	// Get the task.
	task, err := tr.GetTask(taskID)
	if err != nil {
		t.Fatal(err)
	}
	if task.Title != "Implement API" {
		t.Errorf("got title %q, want %q", task.Title, "Implement API")
	}
	if task.Status != "pending" {
		t.Errorf("got status %q, want %q", task.Status, "pending")
	}
}

func TestTrackerClaim(t *testing.T) {
	db := testDB(t)
	tr, err := NewTracker(db)
	if err != nil {
		t.Fatal(err)
	}

	projID, _ := tr.CreateProject("test-proj", "")
	taskID, _ := tr.CreateTask(projID, "Task 1", "", "dev", 0)

	// Claim.
	if err := tr.ClaimTask(taskID, "coder"); err != nil {
		t.Fatal(err)
	}

	task, _ := tr.GetTask(taskID)
	if task.ClaimedBy != "coder" {
		t.Errorf("claimed_by: got %q, want %q", task.ClaimedBy, "coder")
	}

	// Double claim should fail.
	if err := tr.ClaimTask(taskID, "other"); err == nil {
		t.Error("expected error on double claim")
	}
}

func TestTrackerUpdate(t *testing.T) {
	db := testDB(t)
	tr, err := NewTracker(db)
	if err != nil {
		t.Fatal(err)
	}

	projID, _ := tr.CreateProject("test-proj", "")
	taskID, _ := tr.CreateTask(projID, "Task 1", "", "dev", 0)

	if err := tr.UpdateTask(taskID, "in_progress", "started work"); err != nil {
		t.Fatal(err)
	}

	task, _ := tr.GetTask(taskID)
	if task.Status != "in_progress" {
		t.Errorf("status: got %q, want %q", task.Status, "in_progress")
	}
}

func TestTrackerList(t *testing.T) {
	db := testDB(t)
	tr, err := NewTracker(db)
	if err != nil {
		t.Fatal(err)
	}

	projID, _ := tr.CreateProject("test-proj", "")
	tr.CreateTask(projID, "Task 1", "", "dev", 1)
	tr.CreateTask(projID, "Task 2", "", "qa", 0)
	tr.CreateTask(projID, "Task 3", "", "dev", 2)

	// List all.
	tasks, err := tr.ListTasks("", "", "", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks) != 3 {
		t.Errorf("got %d tasks, want 3", len(tasks))
	}

	// Filter by team.
	tasks, err = tr.ListTasks("", "dev", "", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks) != 2 {
		t.Errorf("got %d tasks for dev, want 2", len(tasks))
	}
}

func TestProjectStatus(t *testing.T) {
	db := testDB(t)
	tr, err := NewTracker(db)
	if err != nil {
		t.Fatal(err)
	}

	projID, _ := tr.CreateProject("test-proj", "")
	taskID1, _ := tr.CreateTask(projID, "Task 1", "", "dev", 0)
	tr.CreateTask(projID, "Task 2", "", "dev", 0)
	tr.UpdateTask(taskID1, "completed", "done")

	ps, err := tr.ProjectStatus(projID)
	if err != nil {
		t.Fatal(err)
	}
	if ps.Total != 2 {
		t.Errorf("total: got %d, want 2", ps.Total)
	}
	if ps.Completed != 1 {
		t.Errorf("completed: got %d, want 1", ps.Completed)
	}
}
```

**Implementation** (`internal/mesh/tracker.go`):
```go
package mesh

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// Task represents a tracked task.
type Task struct {
	ID           string
	ProjectID    string
	Title        string
	Description  string
	AssignedTeam string
	ClaimedBy    string
	Status       string
	Priority     int
	Notes        string
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// ProjectStatusSummary is a compact project overview.
type ProjectStatusSummary struct {
	ProjectID string
	Name      string
	Total     int
	Completed int
	InProgress int
	Pending   int
}

// Tracker manages tasks and projects in SQLite.
type Tracker struct {
	db *sql.DB
}

// NewTracker initializes the schema and returns a Tracker.
func NewTracker(db *sql.DB) (*Tracker, error) {
	schema := `
	CREATE TABLE IF NOT EXISTS tracker_projects (
		id TEXT PRIMARY KEY,
		name TEXT UNIQUE NOT NULL,
		config_path TEXT,
		status TEXT DEFAULT 'active',
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS tracker_tasks (
		id TEXT PRIMARY KEY,
		project_id TEXT REFERENCES tracker_projects(id),
		title TEXT NOT NULL,
		description TEXT DEFAULT '',
		assigned_team TEXT DEFAULT '',
		claimed_by TEXT DEFAULT '',
		status TEXT DEFAULT 'pending',
		priority INTEGER DEFAULT 0,
		notes TEXT DEFAULT '',
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
	);`

	if _, err := db.Exec(schema); err != nil {
		return nil, fmt.Errorf("tracker schema: %w", err)
	}
	return &Tracker{db: db}, nil
}

// CreateProject creates a new project entry.
func (tr *Tracker) CreateProject(name, configPath string) (string, error) {
	id := "proj-" + uuid.NewString()[:8]
	_, err := tr.db.Exec(
		`INSERT INTO tracker_projects (id, name, config_path) VALUES (?, ?, ?)`,
		id, name, configPath,
	)
	if err != nil {
		return "", fmt.Errorf("create project: %w", err)
	}
	return id, nil
}

// CreateTask creates a new task.
func (tr *Tracker) CreateTask(projectID, title, description, assignedTeam string, priority int) (string, error) {
	id := "task-" + uuid.NewString()[:8]
	now := time.Now()
	_, err := tr.db.Exec(
		`INSERT INTO tracker_tasks (id, project_id, title, description, assigned_team, priority, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		id, projectID, title, description, assignedTeam, priority, now, now,
	)
	if err != nil {
		return "", fmt.Errorf("create task: %w", err)
	}
	return id, nil
}

// GetTask retrieves a task by ID.
func (tr *Tracker) GetTask(taskID string) (*Task, error) {
	row := tr.db.QueryRow(
		`SELECT id, project_id, title, description, assigned_team, claimed_by, status, priority, notes, created_at, updated_at FROM tracker_tasks WHERE id = ?`,
		taskID,
	)
	var t Task
	if err := row.Scan(&t.ID, &t.ProjectID, &t.Title, &t.Description, &t.AssignedTeam, &t.ClaimedBy, &t.Status, &t.Priority, &t.Notes, &t.CreatedAt, &t.UpdatedAt); err != nil {
		return nil, fmt.Errorf("get task: %w", err)
	}
	return &t, nil
}

// ClaimTask claims a task for an agent. Fails if already claimed.
func (tr *Tracker) ClaimTask(taskID, agentName string) error {
	result, err := tr.db.Exec(
		`UPDATE tracker_tasks SET claimed_by = ?, status = 'in_progress', updated_at = ? WHERE id = ? AND claimed_by = ''`,
		agentName, time.Now(), taskID,
	)
	if err != nil {
		return fmt.Errorf("claim task: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("task %q already claimed or not found", taskID)
	}
	return nil
}

// UpdateTask updates task status and notes.
func (tr *Tracker) UpdateTask(taskID, status, notes string) error {
	_, err := tr.db.Exec(
		`UPDATE tracker_tasks SET status = ?, notes = ?, updated_at = ? WHERE id = ?`,
		status, notes, time.Now(), taskID,
	)
	if err != nil {
		return fmt.Errorf("update task: %w", err)
	}
	return nil
}

// ListTasks returns tasks matching filters.
func (tr *Tracker) ListTasks(projectID, team, status string, limit int) ([]Task, error) {
	query := `SELECT id, project_id, title, description, assigned_team, claimed_by, status, priority, notes, created_at, updated_at FROM tracker_tasks WHERE 1=1`
	var args []any

	if projectID != "" {
		query += ` AND project_id = ?`
		args = append(args, projectID)
	}
	if team != "" {
		query += ` AND assigned_team = ?`
		args = append(args, team)
	}
	if status != "" {
		query += ` AND status = ?`
		args = append(args, status)
	}
	query += ` ORDER BY priority DESC, created_at ASC`
	if limit > 0 {
		query += fmt.Sprintf(` LIMIT %d`, limit)
	}

	rows, err := tr.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("list tasks: %w", err)
	}
	defer rows.Close()

	var tasks []Task
	for rows.Next() {
		var t Task
		if err := rows.Scan(&t.ID, &t.ProjectID, &t.Title, &t.Description, &t.AssignedTeam, &t.ClaimedBy, &t.Status, &t.Priority, &t.Notes, &t.CreatedAt, &t.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan task: %w", err)
		}
		tasks = append(tasks, t)
	}
	return tasks, rows.Err()
}

// ProjectStatus returns a summary of project completion.
func (tr *Tracker) ProjectStatus(projectID string) (*ProjectStatusSummary, error) {
	var name string
	err := tr.db.QueryRow(`SELECT name FROM tracker_projects WHERE id = ?`, projectID).Scan(&name)
	if err != nil {
		return nil, fmt.Errorf("project not found: %w", err)
	}

	rows, err := tr.db.Query(
		`SELECT status, COUNT(*) FROM tracker_tasks WHERE project_id = ? GROUP BY status`,
		projectID,
	)
	if err != nil {
		return nil, fmt.Errorf("project status: %w", err)
	}
	defer rows.Close()

	ps := &ProjectStatusSummary{ProjectID: projectID, Name: name}
	for rows.Next() {
		var s string
		var count int
		if err := rows.Scan(&s, &count); err != nil {
			continue
		}
		ps.Total += count
		switch s {
		case "completed":
			ps.Completed = count
		case "in_progress":
			ps.InProgress = count
		case "pending":
			ps.Pending = count
		}
	}
	return ps, nil
}
```

**Run:**
```bash
cd /Users/jon/workspace/ratchet-cli && go test ./internal/mesh/ -run TestTracker -v
cd /Users/jon/workspace/ratchet-cli && go test ./internal/mesh/ -run TestProjectStatus -v
```

**Commit:** `feat: SQLite task tracker with CRUD, claim, and project status`

---

### Task 5.2: Tracker Mesh Tools

**Files:**
- `internal/mesh/tracker_tools.go` (new)
- `internal/mesh/tracker_tools_test.go` (new)

**Test first** (`internal/mesh/tracker_tools_test.go`):
```go
package mesh

import (
	"context"
	"encoding/json"
	"testing"

	_ "github.com/mattn/go-sqlite3"
)

func TestTaskCreateTool(t *testing.T) {
	db := testDB(t)
	tr, err := NewTracker(db)
	if err != nil {
		t.Fatal(err)
	}

	projID, _ := tr.CreateProject("test", "")
	bb := NewBlackboard()
	tool := &TaskCreateTool{tracker: tr, bb: bb, defaultProject: projID}

	result, err := tool.Execute(context.Background(), json.RawMessage(`{
		"title": "Build API",
		"assigned_team": "dev",
		"priority": 2
	}`))
	if err != nil {
		t.Fatal(err)
	}
	if result == "" {
		t.Error("empty result")
	}
}

func TestTaskListTool(t *testing.T) {
	db := testDB(t)
	tr, err := NewTracker(db)
	if err != nil {
		t.Fatal(err)
	}

	projID, _ := tr.CreateProject("test", "")
	tr.CreateTask(projID, "Task 1", "", "dev", 0)
	tr.CreateTask(projID, "Task 2", "", "dev", 1)

	tool := &TaskListTool{tracker: tr}
	result, err := tool.Execute(context.Background(), json.RawMessage(`{"limit": 10}`))
	if err != nil {
		t.Fatal(err)
	}
	if result == "" {
		t.Error("empty result")
	}
}
```

**Implementation** (`internal/mesh/tracker_tools.go`):
```go
package mesh

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// TaskCreateTool is a mesh tool for creating tasks.
type TaskCreateTool struct {
	tracker        *Tracker
	bb             *Blackboard
	defaultProject string
}

func (t *TaskCreateTool) Name() string        { return "task_create" }
func (t *TaskCreateTool) Description() string { return "Create a new task in the project tracker" }
func (t *TaskCreateTool) Schema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"title": {"type": "string", "description": "Task title"},
			"project": {"type": "string", "description": "Project ID (optional, uses default)"},
			"assigned_team": {"type": "string", "description": "Team to assign the task to"},
			"priority": {"type": "integer", "description": "Priority (0=low, 1=normal, 2=high)"},
			"description": {"type": "string", "description": "Detailed description"}
		},
		"required": ["title"]
	}`)
}

func (t *TaskCreateTool) Execute(ctx context.Context, input json.RawMessage) (string, error) {
	var args struct {
		Title        string `json:"title"`
		Project      string `json:"project"`
		AssignedTeam string `json:"assigned_team"`
		Priority     int    `json:"priority"`
		Description  string `json:"description"`
	}
	if err := json.Unmarshal(input, &args); err != nil {
		return "", fmt.Errorf("parse args: %w", err)
	}

	projID := args.Project
	if projID == "" {
		projID = t.defaultProject
	}

	taskID, err := t.tracker.CreateTask(projID, args.Title, args.Description, args.AssignedTeam, args.Priority)
	if err != nil {
		return "", err
	}

	// Write BB notification.
	if t.bb != nil && args.AssignedTeam != "" {
		writeNotification(t.bb, args.AssignedTeam,
			fmt.Sprintf("new task: %s — %s (priority %d)", taskID, args.Title, args.Priority))
	}

	return fmt.Sprintf("created task %s: %s", taskID, args.Title), nil
}

// TaskClaimTool is a mesh tool for claiming tasks.
type TaskClaimTool struct {
	tracker *Tracker
	bb      *Blackboard
}

func (t *TaskClaimTool) Name() string        { return "task_claim" }
func (t *TaskClaimTool) Description() string { return "Claim a task for yourself" }
func (t *TaskClaimTool) Schema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"task_id": {"type": "string"},
			"agent_name": {"type": "string"}
		},
		"required": ["task_id", "agent_name"]
	}`)
}

func (t *TaskClaimTool) Execute(ctx context.Context, input json.RawMessage) (string, error) {
	var args struct {
		TaskID    string `json:"task_id"`
		AgentName string `json:"agent_name"`
	}
	if err := json.Unmarshal(input, &args); err != nil {
		return "", fmt.Errorf("parse args: %w", err)
	}
	if err := t.tracker.ClaimTask(args.TaskID, args.AgentName); err != nil {
		return "", err
	}
	return fmt.Sprintf("task %s claimed by %s", args.TaskID, args.AgentName), nil
}

// TaskUpdateTool is a mesh tool for updating tasks.
type TaskUpdateTool struct {
	tracker *Tracker
	bb      *Blackboard
}

func (t *TaskUpdateTool) Name() string        { return "task_update" }
func (t *TaskUpdateTool) Description() string { return "Update a task's status and notes" }
func (t *TaskUpdateTool) Schema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"task_id": {"type": "string"},
			"status": {"type": "string", "enum": ["pending", "in_progress", "completed", "failed", "blocked"]},
			"notes": {"type": "string"}
		},
		"required": ["task_id", "status"]
	}`)
}

func (t *TaskUpdateTool) Execute(ctx context.Context, input json.RawMessage) (string, error) {
	var args struct {
		TaskID string `json:"task_id"`
		Status string `json:"status"`
		Notes  string `json:"notes"`
	}
	if err := json.Unmarshal(input, &args); err != nil {
		return "", fmt.Errorf("parse args: %w", err)
	}
	if err := t.tracker.UpdateTask(args.TaskID, args.Status, args.Notes); err != nil {
		return "", err
	}

	// Notify on completion.
	if t.bb != nil && args.Status == "completed" {
		task, _ := t.tracker.GetTask(args.TaskID)
		if task != nil && task.AssignedTeam != "" {
			writeNotification(t.bb, task.AssignedTeam,
				fmt.Sprintf("task completed: %s — %s", args.TaskID, task.Title))
		}
	}

	return fmt.Sprintf("task %s updated to %s", args.TaskID, args.Status), nil
}

// TaskListTool is a mesh tool for listing tasks.
type TaskListTool struct {
	tracker *Tracker
}

func (t *TaskListTool) Name() string        { return "task_list" }
func (t *TaskListTool) Description() string { return "List tasks (compact one-line summaries)" }
func (t *TaskListTool) Schema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"project": {"type": "string"},
			"team": {"type": "string"},
			"status": {"type": "string"},
			"limit": {"type": "integer", "default": 10}
		}
	}`)
}

func (t *TaskListTool) Execute(ctx context.Context, input json.RawMessage) (string, error) {
	var args struct {
		Project string `json:"project"`
		Team    string `json:"team"`
		Status  string `json:"status"`
		Limit   int    `json:"limit"`
	}
	if err := json.Unmarshal(input, &args); err != nil {
		return "", fmt.Errorf("parse args: %w", err)
	}
	if args.Limit <= 0 {
		args.Limit = 10
	}

	tasks, err := t.tracker.ListTasks(args.Project, args.Team, args.Status, args.Limit)
	if err != nil {
		return "", err
	}

	if len(tasks) == 0 {
		return "no tasks found", nil
	}

	var sb strings.Builder
	for _, task := range tasks {
		assignee := task.ClaimedBy
		if assignee == "" {
			assignee = task.AssignedTeam
		}
		sb.WriteString(fmt.Sprintf("%s | %s | %s | %s | p%d\n",
			task.ID, task.Title, task.Status, assignee, task.Priority))
	}
	return sb.String(), nil
}

// TaskGetTool is a mesh tool for getting full task detail.
type TaskGetTool struct {
	tracker *Tracker
}

func (t *TaskGetTool) Name() string        { return "task_get" }
func (t *TaskGetTool) Description() string { return "Get full detail for a specific task" }
func (t *TaskGetTool) Schema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"task_id": {"type": "string"}
		},
		"required": ["task_id"]
	}`)
}

func (t *TaskGetTool) Execute(ctx context.Context, input json.RawMessage) (string, error) {
	var args struct {
		TaskID string `json:"task_id"`
	}
	if err := json.Unmarshal(input, &args); err != nil {
		return "", fmt.Errorf("parse args: %w", err)
	}

	task, err := t.tracker.GetTask(args.TaskID)
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("ID: %s\nTitle: %s\nStatus: %s\nTeam: %s\nClaimed: %s\nPriority: %d\nDescription: %s\nNotes: %s\nCreated: %s\nUpdated: %s",
		task.ID, task.Title, task.Status, task.AssignedTeam, task.ClaimedBy,
		task.Priority, task.Description, task.Notes,
		task.CreatedAt.Format(time.RFC3339), task.UpdatedAt.Format(time.RFC3339),
	), nil
}

// ProjectStatusTool returns a project completion summary.
type ProjectStatusTool struct {
	tracker *Tracker
}

func (t *ProjectStatusTool) Name() string        { return "project_status" }
func (t *ProjectStatusTool) Description() string { return "Get project completion summary" }
func (t *ProjectStatusTool) Schema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"project": {"type": "string", "description": "Project ID"}
		}
	}`)
}

func (t *ProjectStatusTool) Execute(ctx context.Context, input json.RawMessage) (string, error) {
	var args struct {
		Project string `json:"project"`
	}
	if err := json.Unmarshal(input, &args); err != nil {
		return "", fmt.Errorf("parse args: %w", err)
	}

	ps, err := t.tracker.ProjectStatus(args.Project)
	if err != nil {
		return "", err
	}

	pct := 0
	if ps.Total > 0 {
		pct = (ps.Completed * 100) / ps.Total
	}
	return fmt.Sprintf("Project: %s\nTotal: %d | Completed: %d | In Progress: %d | Pending: %d | %d%% done",
		ps.Name, ps.Total, ps.Completed, ps.InProgress, ps.Pending, pct), nil
}

// writeNotification writes a one-line notification to BB, rotating at 20 entries.
func writeNotification(bb *Blackboard, team, msg string) {
	section := fmt.Sprintf("notifications/%s", team)
	entries := bb.List(section)

	// Rotate: if >= 20 entries, the old ones will naturally be overwritten
	// since we use a rotating key.
	key := fmt.Sprintf("n-%d", len(entries)%20)
	bb.Write(section, key, msg, "tracker")
}
```

Add the missing `time` import at the top of the file (used in `TaskGetTool`).

**Run:**
```bash
cd /Users/jon/workspace/ratchet-cli && go test ./internal/mesh/ -run TestTaskCreateTool -v
cd /Users/jon/workspace/ratchet-cli && go test ./internal/mesh/ -run TestTaskListTool -v
```

**Commit:** `feat: tracker mesh tools (task_create/claim/update/list/get, project_status)`

---

### Task 5.3: Wire Tracker to knownTools + LocalNode

**Files:**
- `internal/mesh/config.go` (modify — add tracker tool names to `knownTools`)
- `internal/mesh/local_node.go` (modify — register tracker tools when available)

**Implementation** — update `knownTools` in `internal/mesh/config.go`:
```go
var knownTools = map[string]bool{
	"blackboard_read":  true,
	"blackboard_write": true,
	"blackboard_list":  true,
	"send_message":     true,
	"task_create":      true,
	"task_claim":       true,
	"task_update":      true,
	"task_list":        true,
	"task_get":         true,
	"project_status":   true,
}
```

**Run:**
```bash
cd /Users/jon/workspace/ratchet-cli && go build ./...
cd /Users/jon/workspace/ratchet-cli && go test ./internal/mesh/ -v
```

**Commit:** `feat: register tracker tools in knownTools + wire to mesh`

---

### Task 5.4: Wire Tracker RPCs in Service

**Files:**
- `internal/daemon/service.go` (modify — add tracker field, wire RPCs)

**Implementation** — add `tracker` field to `Service`:
```go
type Service struct {
	// ... existing fields ...
	tracker *mesh.Tracker
}
```

Initialize in `NewService` (after `engine.DB` is available):
```go
tracker, err := mesh.NewTracker(engine.DB)
if err != nil {
	engine.Close()
	return nil, fmt.Errorf("init tracker: %w", err)
}
svc.tracker = tracker
```

Add RPC handlers:
```go
func (s *Service) CreateTask(ctx context.Context, req *pb.TaskCreateReq) (*pb.TaskInfo, error) {
	taskID, err := s.tracker.CreateTask(req.ProjectId, req.Title, req.Description, req.AssignedTeam, int(req.Priority))
	if err != nil {
		return nil, status.Errorf(codes.Internal, "%v", err)
	}
	task, err := s.tracker.GetTask(taskID)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "%v", err)
	}
	return taskToProto(task), nil
}

func (s *Service) ClaimTask(ctx context.Context, req *pb.TaskClaimReq) (*pb.TaskInfo, error) {
	if err := s.tracker.ClaimTask(req.TaskId, req.AgentName); err != nil {
		return nil, status.Errorf(codes.FailedPrecondition, "%v", err)
	}
	task, err := s.tracker.GetTask(req.TaskId)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "%v", err)
	}
	return taskToProto(task), nil
}

func (s *Service) UpdateTask(ctx context.Context, req *pb.TaskUpdateReq) (*pb.TaskInfo, error) {
	if err := s.tracker.UpdateTask(req.TaskId, req.Status, req.Notes); err != nil {
		return nil, status.Errorf(codes.Internal, "%v", err)
	}
	task, err := s.tracker.GetTask(req.TaskId)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "%v", err)
	}
	return taskToProto(task), nil
}

func (s *Service) ListTasks(ctx context.Context, req *pb.TaskListReq) (*pb.TaskList, error) {
	tasks, err := s.tracker.ListTasks(req.ProjectId, req.Team, req.Status, int(req.Limit))
	if err != nil {
		return nil, status.Errorf(codes.Internal, "%v", err)
	}
	var out []*pb.TaskInfo
	for _, t := range tasks {
		out = append(out, taskToProto(&t))
	}
	return &pb.TaskList{Tasks: out}, nil
}

func (s *Service) GetTask(ctx context.Context, req *pb.TaskReq) (*pb.TaskInfo, error) {
	task, err := s.tracker.GetTask(req.TaskId)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "%v", err)
	}
	return taskToProto(task), nil
}

func taskToProto(t *mesh.Task) *pb.TaskInfo {
	return &pb.TaskInfo{
		Id:           t.ID,
		Title:        t.Title,
		Status:       t.Status,
		AssignedTeam: t.AssignedTeam,
		ClaimedBy:    t.ClaimedBy,
		Priority:     int32(t.Priority),
		Description:  t.Description,
		ProjectId:    t.ProjectID,
		CreatedAt:    t.CreatedAt.Format(time.RFC3339),
		UpdatedAt:    t.UpdatedAt.Format(time.RFC3339),
	}
}
```

**Run:**
```bash
cd /Users/jon/workspace/ratchet-cli && go build ./...
```

**Commit:** `feat: wire tracker RPCs into daemon service`

---

## Summary of Files

| Phase | File | Action |
|---|---|---|
| 1 | `internal/mesh/config.go` | Modify — JSON support, SearchTeamConfig, ParseAgentFlag, BuildTeamConfigFromFlags, ProjectConfig |
| 1 | `internal/mesh/config_test.go` | New — config tests |
| 1 | `internal/mesh/teams/orchestrate.yaml` | Delete |
| 1 | `cmd/ratchet/cmd_team.go` | Modify — --agent/--agents/--name/--bb flags, save/kill |
| 1 | `cmd/ratchet/cmd_project.go` | New — project CLI stubs |
| 1 | `cmd/ratchet/main.go` | Modify — add `project` case |
| 1 | `internal/daemon/projects.go` | New — ProjectRegistry |
| 1 | `internal/daemon/projects_test.go` | New |
| 2 | `internal/proto/ratchet.proto` | Modify — all new messages + RPCs |
| 2 | `internal/daemon/teams.go` | Modify — short IDs, rename, ListTeams, add/remove, attach/detach |
| 2 | `internal/daemon/teams_test.go` | New |
| 2 | `internal/mesh/mesh.go` | Modify — AddNodeToTeam, RemoveNodeFromTeam |
| 2 | `internal/mesh/mesh_test.go` | Modify — add/remove tests |
| 2 | `internal/daemon/service.go` | Modify — wire lifecycle RPCs |
| 3 | `internal/daemon/human_gate.go` | New |
| 3 | `internal/daemon/human_gate_test.go` | New |
| 3 | `internal/daemon/autoresponder.go` | New |
| 3 | `internal/daemon/autoresponder_test.go` | New |
| 3 | `internal/daemon/notifications.go` | New |
| 3 | `internal/daemon/notifications_test.go` | New |
| 4 | `internal/mesh/project_bb.go` | New — ProjectBlackboard with modes |
| 4 | `internal/mesh/project_bb_test.go` | New |
| 4 | `internal/mesh/protocols.go` | New — handoff + directive protocols |
| 4 | `internal/mesh/protocols_test.go` | New |
| 5 | `internal/mesh/tracker.go` | New — SQLite task tracker |
| 5 | `internal/mesh/tracker_test.go` | New |
| 5 | `internal/mesh/tracker_tools.go` | New — mesh tools for tracker |
| 5 | `internal/mesh/tracker_tools_test.go` | New |

## Commit Sequence

1. `feat: JSON team config support + SearchTeamConfig with standard paths`
2. `feat: multi-team ProjectConfig parsing with YAML/JSON support`
3. `feat: --agent/--agents/--name/--bb/--orchestrator CLI flags for team start`
4. `refactor: remove orchestrate builtin config`
5. `feat: project registry + ratchet project CLI stub`
6. `feat: ratchet team save/kill commands`
7. `feat(proto): add RPCs for team lifecycle, projects, human gate, task tracker`
8. `feat: short team IDs (t-XXXX), rename, name→ID mapping`
9. `feat: dynamic AddNode/RemoveNode for running teams`
10. `feat: attach/detach with observe/join modes, rename, add/remove CLI`
11. `feat: wire ListTeams, KillTeam, RenameTeam, Add/Remove RPC handlers`
12. `feat: human gate — message queue with blocking Wait/Respond`
13. `feat: autoresponder with pattern-matching rules`
14. `feat: OS-native notifications (macOS/Linux/Windows)`
15. `feat: wire human gate + autoresponder + notifications into service`
16. `feat: ProjectBlackboard with shared/isolated/orchestrator/bridge modes`
17. `feat: handoff + directive cross-team protocols`
18. `feat: wire project registry RPCs into daemon service`
19. `feat: SQLite task tracker with CRUD, claim, and project status`
20. `feat: tracker mesh tools (task_create/claim/update/list/get, project_status)`
21. `feat: register tracker tools in knownTools + wire to mesh`
22. `feat: wire tracker RPCs into daemon service`
