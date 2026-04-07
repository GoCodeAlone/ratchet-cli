package mesh

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadTeamConfig_Valid(t *testing.T) {
	yaml := `
name: dev-team
timeout: "10m"
max_review_rounds: 3
agents:
  - name: planner
    role: planning
    provider: openai
    model: gpt-4
    max_iterations: 5
    system_prompt: "You plan tasks."
    tools:
      - blackboard_read
      - blackboard_write
  - name: coder
    role: implementation
    provider: anthropic
    model: sonnet
    tools:
      - blackboard_read
      - blackboard_write
      - send_message
`
	path := filepath.Join(t.TempDir(), "team.yaml")
	if err := os.WriteFile(path, []byte(yaml), 0o644); err != nil {
		t.Fatal(err)
	}

	tc, err := LoadTeamConfig(path)
	if err != nil {
		t.Fatalf("LoadTeamConfig: %v", err)
	}
	if tc.Name != "dev-team" {
		t.Fatalf("expected name dev-team, got %s", tc.Name)
	}
	if tc.Timeout != "10m" {
		t.Fatalf("expected timeout 10m, got %s", tc.Timeout)
	}
	if tc.MaxReviewRounds != 3 {
		t.Fatalf("expected max_review_rounds 3, got %d", tc.MaxReviewRounds)
	}
	if len(tc.Agents) != 2 {
		t.Fatalf("expected 2 agents, got %d", len(tc.Agents))
	}
	if tc.Agents[0].Name != "planner" {
		t.Fatalf("expected first agent planner, got %s", tc.Agents[0].Name)
	}
	if tc.Agents[0].MaxIterations != 5 {
		t.Fatalf("expected max_iterations 5, got %d", tc.Agents[0].MaxIterations)
	}
	if len(tc.Agents[1].Tools) != 3 {
		t.Fatalf("expected 3 tools for coder, got %d", len(tc.Agents[1].Tools))
	}
}

func TestValidateTeamConfig_MissingName(t *testing.T) {
	tc := &TeamConfig{
		Agents: []AgentConfig{{Name: "a"}},
	}
	if err := ValidateTeamConfig(tc); err == nil {
		t.Fatal("expected error for missing team name")
	}
}

func TestValidateTeamConfig_NoAgents(t *testing.T) {
	tc := &TeamConfig{Name: "empty"}
	if err := ValidateTeamConfig(tc); err == nil {
		t.Fatal("expected error for no agents")
	}
}

func TestValidateTeamConfig_AgentMissingName(t *testing.T) {
	tc := &TeamConfig{
		Name:   "team",
		Agents: []AgentConfig{{Role: "worker"}},
	}
	if err := ValidateTeamConfig(tc); err == nil {
		t.Fatal("expected error for agent missing name")
	}
}

func TestValidateTeamConfig_UnknownTool(t *testing.T) {
	tc := &TeamConfig{
		Name: "team",
		Agents: []AgentConfig{{
			Name:  "agent",
			Tools: []string{"blackboard_read", "unknown_tool"},
		}},
	}
	if err := ValidateTeamConfig(tc); err == nil {
		t.Fatal("expected error for unknown tool")
	}
}

func TestValidateTeamConfig_DuplicateName(t *testing.T) {
	tc := &TeamConfig{
		Name: "team",
		Agents: []AgentConfig{
			{Name: "worker"},
			{Name: "worker"}, // duplicate
		},
	}
	if err := ValidateTeamConfig(tc); err == nil {
		t.Fatal("expected error for duplicate agent name")
	}
}

func TestValidateTeamConfig_InvalidTimeout(t *testing.T) {
	tc := &TeamConfig{
		Name:    "team",
		Timeout: "not-a-duration",
		Agents:  []AgentConfig{{Name: "a"}},
	}
	if err := ValidateTeamConfig(tc); err == nil {
		t.Fatal("expected error for invalid timeout")
	}
}

func TestToNodeConfigs(t *testing.T) {
	tc := &TeamConfig{
		Name: "team",
		Agents: []AgentConfig{
			{
				Name:          "planner",
				Role:          "planning",
				Provider:      "openai",
				Model:         "gpt-4",
				MaxIterations: 5,
				SystemPrompt:  "Plan things.",
				Tools:         []string{"blackboard_read"},
			},
			{
				Name:     "coder",
				Role:     "coding",
				Provider: "anthropic",
				Model:    "sonnet",
			},
		},
	}

	configs := ToNodeConfigs(tc)
	if len(configs) != 2 {
		t.Fatalf("expected 2 configs, got %d", len(configs))
	}
	if configs[0].Name != "planner" || configs[0].Location != "local" {
		t.Fatalf("unexpected config[0]: %+v", configs[0])
	}
	if configs[0].MaxIterations != 5 {
		t.Fatalf("expected max_iterations 5, got %d", configs[0].MaxIterations)
	}
	if configs[1].Name != "coder" || configs[1].Provider != "anthropic" {
		t.Fatalf("unexpected config[1]: %+v", configs[1])
	}
}

func TestLoadTeamConfigs_Directory(t *testing.T) {
	dir := t.TempDir()

	yaml1 := `
name: team-a
agents:
  - name: agent1
    role: worker
`
	yaml2 := `
name: team-b
agents:
  - name: agent2
    role: planner
    tools:
      - blackboard_read
`
	if err := os.WriteFile(filepath.Join(dir, "team-a.yaml"), []byte(yaml1), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "team-b.yml"), []byte(yaml2), 0o644); err != nil {
		t.Fatal(err)
	}
	// Write a non-YAML file that should be ignored
	if err := os.WriteFile(filepath.Join(dir, "readme.md"), []byte("# hi"), 0o644); err != nil {
		t.Fatal(err)
	}

	configs, err := LoadTeamConfigs(dir)
	if err != nil {
		t.Fatalf("LoadTeamConfigs: %v", err)
	}
	if len(configs) != 2 {
		t.Fatalf("expected 2 configs, got %d", len(configs))
	}

	names := map[string]bool{}
	for _, c := range configs {
		names[c.Name] = true
	}
	if !names["team-a"] || !names["team-b"] {
		t.Fatalf("expected team-a and team-b, got %v", names)
	}
}

func TestLoadTeamConfig_FileNotFound(t *testing.T) {
	_, err := LoadTeamConfig("/nonexistent/team.yaml")
	if err == nil {
		t.Fatal("expected error for nonexistent file")
	}
}

func TestLoadTeamConfig_InvalidYAML(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bad.yaml")
	if err := os.WriteFile(path, []byte("{{invalid yaml"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := LoadTeamConfig(path)
	if err == nil {
		t.Fatal("expected error for invalid YAML")
	}
}

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

func TestParseAgentFlag(t *testing.T) {
	tests := []struct {
		input     string
		wantName  string
		wantProv  string
		wantModel string
		wantErr   bool
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
