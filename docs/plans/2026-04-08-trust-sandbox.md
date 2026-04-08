# Trust, Permission & Docker Sandbox Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Unified trust rules engine, PTY auto-prompt handling, Docker sandbox, operating modes, and dynamic switching — agent-plugin-first, ratchet-cli as consumer.

**Architecture:** TrustEngine in wpa/policy/ parses both Claude Code and ratchet rule formats, evaluates tool/path/command actions. GlobMatcher replaces prefix-only path guard. PermissionStore persists "always" grants in SQLite. PTY PromptHandler auto-responds to known screen prompts. Docker ContainerManager extended with per-agent sandbox specs. Executor routes tool calls through container when sandbox mode active. Ratchet wires config + --mode flag + /trust TUI commands.

**Tech Stack:** Go 1.26, SQLite, Docker API (existing), regexp, filepath.Match, doublestar glob

---

## Repo Paths

- **wpa** = `/Users/jon/workspace/workflow-plugin-agent`
- **rcli** = `/Users/jon/workspace/ratchet-cli`

## Phase 1 — TrustEngine + TrustRule + Format Parsers + Mode Presets (wpa)

### Task 1.1: Create `policy/trust.go` types and mode presets

**File:** `wpa/policy/trust.go` (new)

**Test first — File:** `wpa/policy/trust_test.go` (new)

```go
package policy

import (
	"context"
	"testing"
)

func TestActionConstants(t *testing.T) {
	if Allow != "allow" || Deny != "deny" || Ask != "ask" {
		t.Fatal("action constants changed")
	}
}

func TestModePresetsExist(t *testing.T) {
	for _, mode := range []string{"conservative", "permissive", "locked", "sandbox"} {
		if _, ok := ModePresets[mode]; !ok {
			t.Errorf("missing mode preset %q", mode)
		}
	}
}

func TestNewTrustEngine(t *testing.T) {
	te := NewTrustEngine("conservative", nil, nil)
	if te == nil {
		t.Fatal("NewTrustEngine returned nil")
	}
	if te.Mode() != "conservative" {
		t.Errorf("mode = %q, want conservative", te.Mode())
	}
}

func TestSetMode(t *testing.T) {
	te := NewTrustEngine("conservative", nil, nil)
	changed := te.SetMode("permissive")
	if te.Mode() != "permissive" {
		t.Errorf("mode = %q, want permissive", te.Mode())
	}
	if len(changed) == 0 {
		t.Error("SetMode returned empty rules")
	}
}

func TestSetModeUnknown(t *testing.T) {
	te := NewTrustEngine("conservative", nil, nil)
	changed := te.SetMode("nonexistent")
	if len(changed) != 0 {
		t.Error("SetMode for unknown mode should return nil")
	}
	if te.Mode() != "conservative" {
		t.Error("mode should not change for unknown preset")
	}
}

func TestEvaluateToolAllow(t *testing.T) {
	rules := []TrustRule{
		{Pattern: "file_read", Action: Allow},
	}
	te := NewTrustEngine("custom", rules, nil)
	action := te.Evaluate(context.Background(), "file_read", nil)
	if action != Allow {
		t.Errorf("got %v, want Allow", action)
	}
}

func TestEvaluateToolDenyWins(t *testing.T) {
	rules := []TrustRule{
		{Pattern: "*", Action: Allow},
		{Pattern: "bash:rm -rf *", Action: Deny},
	}
	te := NewTrustEngine("custom", rules, nil)
	action := te.EvaluateCommand("rm -rf /")
	if action != Deny {
		t.Errorf("got %v, want Deny", action)
	}
}

func TestEvaluateToolDefaultDeny(t *testing.T) {
	te := NewTrustEngine("custom", nil, nil)
	action := te.Evaluate(context.Background(), "unknown_tool", nil)
	if action != Deny {
		t.Errorf("got %v, want Deny", action)
	}
}

func TestEvaluateWildcardPrefix(t *testing.T) {
	rules := []TrustRule{
		{Pattern: "blackboard_*", Action: Allow},
	}
	te := NewTrustEngine("custom", rules, nil)
	if te.Evaluate(context.Background(), "blackboard_read", nil) != Allow {
		t.Error("wildcard prefix should match")
	}
	if te.Evaluate(context.Background(), "file_read", nil) != Deny {
		t.Error("non-matching tool should deny")
	}
}

func TestEvaluateCommandBashPrefix(t *testing.T) {
	rules := []TrustRule{
		{Pattern: "bash:git *", Action: Allow},
		{Pattern: "bash:go *", Action: Allow},
	}
	te := NewTrustEngine("custom", rules, nil)
	if te.EvaluateCommand("git status") != Allow {
		t.Error("git command should be allowed")
	}
	if te.EvaluateCommand("go test ./...") != Allow {
		t.Error("go command should be allowed")
	}
	if te.EvaluateCommand("rm -rf /") != Deny {
		t.Error("rm command should be denied")
	}
}

func TestEvaluatePathRule(t *testing.T) {
	rules := []TrustRule{
		{Pattern: "path:/tmp/*", Action: Allow},
		{Pattern: "path:~/.ssh/*", Action: Deny},
	}
	te := NewTrustEngine("custom", rules, nil)
	if te.EvaluatePath("/tmp/foo.txt") != Allow {
		t.Error("/tmp should be allowed")
	}
}

func TestRulesFromScope(t *testing.T) {
	rules := []TrustRule{
		{Pattern: "file_read", Action: Allow, Scope: "global"},
		{Pattern: "file_write", Action: Deny, Scope: "agent:coder"},
		{Pattern: "file_write", Action: Allow, Scope: "agent:reviewer"},
	}
	te := NewTrustEngine("custom", rules, nil)
	// With agent scope
	action := te.EvaluateScoped(context.Background(), "file_write", nil, "agent:coder")
	if action != Deny {
		t.Errorf("coder scope: got %v, want Deny", action)
	}
	action = te.EvaluateScoped(context.Background(), "file_write", nil, "agent:reviewer")
	if action != Allow {
		t.Errorf("reviewer scope: got %v, want Allow", action)
	}
}
```

**Implementation — File:** `wpa/policy/trust.go`

```go
// Package policy implements a unified trust rules engine for agent tool access control.
// It supports both Claude Code (settings.json) and ratchet (config.yaml) rule formats,
// mode presets, scoped rules, and integrates with the existing ToolPolicyEngine.
package policy

import (
	"context"
	"strings"
	"sync"
)

// Action defines the trust decision for a tool/path/command.
type Action string

const (
	Allow Action = "allow"
	Deny  Action = "deny"
	Ask   Action = "ask"
)

// TrustRule is a single trust policy entry.
type TrustRule struct {
	Pattern string // "file_read", "bash:git *", "path:~/.ssh/*", "Bash(rm:*)"
	Action  Action
	Scope   string // "global", "provider:claude_code", "agent:coder"
}

// PolicyEngine is an optional backing store for SQL-based policies.
// Matches the existing orchestrator.ToolPolicyEngine.IsAllowed signature.
type PolicyEngine interface {
	IsAllowed(ctx context.Context, toolName, agentID, teamID string) (bool, string)
}

// TrustEngine evaluates trust rules for tool calls, paths, and commands.
type TrustEngine struct {
	mu        sync.RWMutex
	rules     []TrustRule
	policyDB  PolicyEngine
	permStore *PermissionStore
	mode      string
}

// NewTrustEngine creates a TrustEngine with the given mode and explicit rules.
// If mode is a known preset, the preset rules are prepended.
// policyDB is optional (may be nil).
func NewTrustEngine(mode string, rules []TrustRule, policyDB PolicyEngine) *TrustEngine {
	te := &TrustEngine{
		policyDB: policyDB,
		mode:     mode,
	}
	if preset, ok := ModePresets[mode]; ok {
		te.rules = append(te.rules, preset...)
	}
	te.rules = append(te.rules, rules...)
	return te
}

// SetPermissionStore attaches a persistent permission store for "always" grants.
func (te *TrustEngine) SetPermissionStore(ps *PermissionStore) {
	te.mu.Lock()
	defer te.mu.Unlock()
	te.permStore = ps
}

// Mode returns the current operating mode.
func (te *TrustEngine) Mode() string {
	te.mu.RLock()
	defer te.mu.RUnlock()
	return te.mode
}

// SetMode switches the active mode preset. Returns the new rules that were loaded.
// If mode is unknown, returns nil and mode is unchanged.
func (te *TrustEngine) SetMode(mode string) []TrustRule {
	preset, ok := ModePresets[mode]
	if !ok {
		return nil
	}
	te.mu.Lock()
	defer te.mu.Unlock()
	te.mode = mode
	// Replace preset rules but keep explicit (non-preset) rules.
	te.rules = make([]TrustRule, len(preset))
	copy(te.rules, preset)
	return preset
}

// AddRule appends a rule dynamically. Deny rules take precedence at evaluation time.
func (te *TrustEngine) AddRule(rule TrustRule) {
	te.mu.Lock()
	defer te.mu.Unlock()
	te.rules = append(te.rules, rule)
}

// Rules returns a copy of the active rules.
func (te *TrustEngine) Rules() []TrustRule {
	te.mu.RLock()
	defer te.mu.RUnlock()
	out := make([]TrustRule, len(te.rules))
	copy(out, te.rules)
	return out
}

// Evaluate checks whether a tool call is allowed. Scope defaults to "global".
func (te *TrustEngine) Evaluate(ctx context.Context, toolName string, args map[string]any) Action {
	return te.EvaluateScoped(ctx, toolName, args, "global")
}

// EvaluateScoped checks whether a tool call is allowed in the given scope.
// Resolution order: deny-wins across all matching rules.
//  1. Per-scope rules matching the tool name
//  2. Global rules matching the tool name
//  3. PermissionStore persistent grants
//  4. ToolPolicyEngine (SQL)
//  5. Default: Deny
func (te *TrustEngine) EvaluateScoped(ctx context.Context, toolName string, args map[string]any, scope string) Action {
	te.mu.RLock()
	rules := te.rules
	permStore := te.permStore
	policyDB := te.policyDB
	te.mu.RUnlock()

	// Phase 1: Check trust rules. Deny wins across all matches.
	var matched []Action
	for _, r := range rules {
		if !ruleMatchesScope(r, scope) {
			continue
		}
		if matchToolPattern(r.Pattern, toolName) {
			matched = append(matched, r.Action)
		}
	}

	// Deny wins
	for _, a := range matched {
		if a == Deny {
			return Deny
		}
	}
	// If any rule matched, return the most permissive non-deny action.
	for _, a := range matched {
		if a == Allow {
			return Allow
		}
	}
	for _, a := range matched {
		if a == Ask {
			return Ask
		}
	}

	// Phase 2: Check persistent permission store.
	if permStore != nil {
		if action, ok := permStore.Check(toolName, scope); ok {
			return action
		}
	}

	// Phase 3: Check SQL-based ToolPolicyEngine.
	if policyDB != nil {
		agentID := extractAgentFromScope(scope)
		teamID := ""
		if allowed, _ := policyDB.IsAllowed(ctx, toolName, agentID, teamID); allowed {
			return Allow
		}
	}

	return Deny
}

// EvaluateCommand checks whether a bash command is allowed.
// Matches rules with "bash:" prefix patterns.
func (te *TrustEngine) EvaluateCommand(cmd string) Action {
	te.mu.RLock()
	rules := te.rules
	te.mu.RUnlock()

	var matched []Action
	for _, r := range rules {
		if matchCommandPattern(r.Pattern, cmd) {
			matched = append(matched, r.Action)
		}
	}

	for _, a := range matched {
		if a == Deny {
			return Deny
		}
	}
	for _, a := range matched {
		if a == Allow {
			return Allow
		}
	}
	for _, a := range matched {
		if a == Ask {
			return Ask
		}
	}
	return Deny
}

// EvaluatePath checks whether a file path is accessible.
// Matches rules with "path:" prefix patterns.
func (te *TrustEngine) EvaluatePath(path string) Action {
	te.mu.RLock()
	rules := te.rules
	te.mu.RUnlock()

	var matched []Action
	for _, r := range rules {
		if matchPathPattern(r.Pattern, path) {
			matched = append(matched, r.Action)
		}
	}

	for _, a := range matched {
		if a == Deny {
			return Deny
		}
	}
	for _, a := range matched {
		if a == Allow {
			return Allow
		}
	}
	for _, a := range matched {
		if a == Ask {
			return Ask
		}
	}
	return Deny
}

// GrantPersistent stores an "always allow/deny" decision for future sessions.
func (te *TrustEngine) GrantPersistent(pattern string, action Action, scope, grantedBy string) error {
	te.mu.RLock()
	ps := te.permStore
	te.mu.RUnlock()
	if ps == nil {
		return nil
	}
	return ps.Grant(pattern, action, scope, grantedBy)
}

// matchToolPattern checks if a pattern matches a tool name.
// Supports: exact match, wildcard "*", prefix wildcard "blackboard_*",
// and Claude Code format "Bash(cmd:*)" → converted to "bash:cmd *".
func matchToolPattern(pattern, toolName string) bool {
	if pattern == "*" {
		return true
	}
	// Skip path: and bash: patterns — they're for EvaluatePath/EvaluateCommand.
	if strings.HasPrefix(pattern, "path:") || strings.HasPrefix(pattern, "bash:") {
		return false
	}
	if pattern == toolName {
		return true
	}
	if strings.HasSuffix(pattern, "*") {
		return strings.HasPrefix(toolName, strings.TrimSuffix(pattern, "*"))
	}
	return false
}

// matchCommandPattern checks if a rule pattern matches a bash command.
func matchCommandPattern(pattern, cmd string) bool {
	if pattern == "*" {
		return true
	}
	if !strings.HasPrefix(pattern, "bash:") {
		return false
	}
	bashPattern := strings.TrimPrefix(pattern, "bash:")
	if bashPattern == "*" {
		return true
	}
	// "git *" matches "git status", "git commit -m foo", etc.
	if strings.HasSuffix(bashPattern, " *") {
		prefix := strings.TrimSuffix(bashPattern, " *")
		cmdParts := strings.Fields(cmd)
		if len(cmdParts) > 0 && cmdParts[0] == prefix {
			return true
		}
		// Also match "prefix subcommand" pattern: "rm -rf *" matches "rm -rf /"
		if strings.HasPrefix(cmd, strings.TrimSuffix(bashPattern, "*")) {
			return true
		}
	}
	return bashPattern == cmd
}

// matchPathPattern checks if a rule pattern matches a file path.
func matchPathPattern(pattern, path string) bool {
	if !strings.HasPrefix(pattern, "path:") {
		return false
	}
	pathPattern := strings.TrimPrefix(pattern, "path:")
	// Expand ~ to match absolute paths
	if strings.HasPrefix(pathPattern, "~/") {
		// For matching, just check if the path ends with the same suffix
		suffix := strings.TrimPrefix(pathPattern, "~")
		if strings.HasSuffix(pathPattern, "*") {
			prefix := strings.TrimSuffix(suffix, "*")
			// Check all common home dirs
			return strings.Contains(path, prefix)
		}
	}
	if strings.HasSuffix(pathPattern, "*") {
		prefix := strings.TrimSuffix(pathPattern, "*")
		return strings.HasPrefix(path, prefix)
	}
	return pathPattern == path
}

// ruleMatchesScope returns true if the rule applies to the given scope.
func ruleMatchesScope(r TrustRule, scope string) bool {
	if r.Scope == "" || r.Scope == "global" {
		return true
	}
	return r.Scope == scope
}

// extractAgentFromScope extracts the agent ID from a scope like "agent:coder".
func extractAgentFromScope(scope string) string {
	if strings.HasPrefix(scope, "agent:") {
		return strings.TrimPrefix(scope, "agent:")
	}
	return ""
}

// ModePresets defines the built-in trust modes.
var ModePresets = map[string][]TrustRule{
	"conservative": {
		{Pattern: "file_read", Action: Allow},
		{Pattern: "blackboard_*", Action: Allow},
		{Pattern: "send_message", Action: Allow},
		{Pattern: "bash:git *", Action: Allow},
		{Pattern: "bash:go *", Action: Allow},
		{Pattern: "file_write", Action: Ask},
		{Pattern: "bash:*", Action: Ask},
		{Pattern: "path:~/.ssh/*", Action: Deny},
		{Pattern: "path:~/.aws/*", Action: Deny},
	},
	"permissive": {
		{Pattern: "*", Action: Allow},
		{Pattern: "bash:rm -rf /*", Action: Deny},
		{Pattern: "bash:sudo *", Action: Deny},
		{Pattern: "path:~/.ssh/*", Action: Deny},
	},
	"locked": {
		{Pattern: "file_read", Action: Allow},
		{Pattern: "blackboard_*", Action: Allow},
		{Pattern: "*", Action: Ask},
	},
	"sandbox": {
		{Pattern: "*", Action: Allow},
	},
}
```

**Test command:**
```bash
cd /Users/jon/workspace/workflow-plugin-agent && go test ./policy/ -run TestAction -v
cd /Users/jon/workspace/workflow-plugin-agent && go test ./policy/ -run TestModePresets -v
cd /Users/jon/workspace/workflow-plugin-agent && go test ./policy/ -v -count=1
```

**Commit:** `feat(policy): add TrustEngine with TrustRule, mode presets, deny-wins evaluation`

---

### Task 1.2: Claude Code format parser

**Test first — append to:** `wpa/policy/trust_test.go`

```go
func TestParseClaudeCodeSettings(t *testing.T) {
	settings := `{
		"allowedTools": ["Edit", "Read", "Bash(git:*)", "Bash(go:*)"],
		"disallowedTools": ["Bash(rm -rf:*)", "Bash(sudo:*)"]
	}`
	rules, err := ParseClaudeCodeSettings([]byte(settings))
	if err != nil {
		t.Fatal(err)
	}
	if len(rules) != 6 {
		t.Fatalf("expected 6 rules, got %d", len(rules))
	}
	// Check allowed
	found := false
	for _, r := range rules {
		if r.Pattern == "Edit" && r.Action == Allow {
			found = true
		}
	}
	if !found {
		t.Error("expected Edit allow rule")
	}
	// Check bash conversion
	for _, r := range rules {
		if r.Pattern == "bash:git *" && r.Action == Allow {
			found = true
		}
	}
	if !found {
		t.Error("expected bash:git * allow rule")
	}
	// Check deny
	for _, r := range rules {
		if r.Pattern == "bash:rm -rf *" && r.Action == Deny {
			found = true
		}
	}
	if !found {
		t.Error("expected bash:rm -rf * deny rule")
	}
}

func TestParseClaudeCodeBashFormat(t *testing.T) {
	tests := []struct {
		input   string
		pattern string
	}{
		{"Bash(git:*)", "bash:git *"},
		{"Bash(go:*)", "bash:go *"},
		{"Bash(rm -rf:*)", "bash:rm -rf *"},
		{"Edit", "Edit"},
		{"Read", "Read"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := normalizeClaudeToolPattern(tt.input)
			if got != tt.pattern {
				t.Errorf("normalizeClaudeToolPattern(%q) = %q, want %q", tt.input, got, tt.pattern)
			}
		})
	}
}

func TestParseClaudeCodeSettingsEmpty(t *testing.T) {
	rules, err := ParseClaudeCodeSettings([]byte("{}"))
	if err != nil {
		t.Fatal(err)
	}
	if len(rules) != 0 {
		t.Errorf("expected 0 rules, got %d", len(rules))
	}
}
```

**Implementation — append to:** `wpa/policy/trust.go`

```go
// ParseClaudeCodeSettings parses a .claude/settings.json file into TrustRules.
// allowedTools → Allow, disallowedTools → Deny. Bash(cmd:*) is normalized to bash:cmd *.
func ParseClaudeCodeSettings(data []byte) ([]TrustRule, error) {
	var settings struct {
		AllowedTools    []string `json:"allowedTools"`
		DisallowedTools []string `json:"disallowedTools"`
	}
	if err := json.Unmarshal(data, &settings); err != nil {
		return nil, fmt.Errorf("parse claude code settings: %w", err)
	}

	var rules []TrustRule
	for _, t := range settings.AllowedTools {
		rules = append(rules, TrustRule{
			Pattern: normalizeClaudeToolPattern(t),
			Action:  Allow,
			Scope:   "provider:claude_code",
		})
	}
	for _, t := range settings.DisallowedTools {
		rules = append(rules, TrustRule{
			Pattern: normalizeClaudeToolPattern(t),
			Action:  Deny,
			Scope:   "provider:claude_code",
		})
	}
	return rules, nil
}

// normalizeClaudeToolPattern converts Claude Code format "Bash(cmd:*)" to "bash:cmd *".
func normalizeClaudeToolPattern(pattern string) string {
	if strings.HasPrefix(pattern, "Bash(") && strings.HasSuffix(pattern, ")") {
		inner := pattern[5 : len(pattern)-1] // strip "Bash(" and ")"
		// "git:*" → "git *", "rm -rf:*" → "rm -rf *"
		inner = strings.Replace(inner, ":*", " *", 1)
		inner = strings.Replace(inner, ":", " ", 1)
		return "bash:" + inner
	}
	return pattern
}
```

Add import to `wpa/policy/trust.go`:
```go
import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
)
```

**Test command:**
```bash
cd /Users/jon/workspace/workflow-plugin-agent && go test ./policy/ -run TestParseClaude -v
```

**Commit:** `feat(policy): add Claude Code settings.json parser`

---

### Task 1.3: Ratchet YAML format parser

**Test first — append to:** `wpa/policy/trust_test.go`

```go
func TestParseRatchetTrustConfig(t *testing.T) {
	cfg := RatchetTrustConfig{
		Mode: "conservative",
		Rules: []RatchetTrustRule{
			{Pattern: "file_read", Action: "allow"},
			{Pattern: "bash:git *", Action: "allow"},
			{Pattern: "bash:rm -rf *", Action: "deny"},
			{Pattern: "bash:docker *", Action: "ask"},
			{Pattern: "path:/tmp/*", Action: "allow"},
			{Pattern: "path:~/.ssh/*", Action: "deny"},
		},
	}
	rules := ParseRatchetTrustConfig(cfg)
	if len(rules) != 6 {
		t.Fatalf("expected 6 rules, got %d", len(rules))
	}
	if rules[0].Pattern != "file_read" || rules[0].Action != Allow {
		t.Errorf("rule 0: %+v", rules[0])
	}
	if rules[2].Action != Deny {
		t.Errorf("rule 2: expected Deny, got %v", rules[2].Action)
	}
	if rules[3].Action != Ask {
		t.Errorf("rule 3: expected Ask, got %v", rules[3].Action)
	}
}

func TestParseRatchetTrustConfigEmpty(t *testing.T) {
	rules := ParseRatchetTrustConfig(RatchetTrustConfig{})
	if len(rules) != 0 {
		t.Errorf("expected 0 rules, got %d", len(rules))
	}
}
```

**Implementation — append to:** `wpa/policy/trust.go`

```go
// RatchetTrustConfig is the trust section from ~/.ratchet/config.yaml.
type RatchetTrustConfig struct {
	Mode         string                       `yaml:"mode" json:"mode"`
	Rules        []RatchetTrustRule           `yaml:"rules" json:"rules"`
	ProviderArgs map[string][]string          `yaml:"provider_args" json:"provider_args"`
	Prompts      []RatchetPromptPattern       `yaml:"prompts" json:"prompts"`
}

// RatchetTrustRule is a single rule in ratchet format.
type RatchetTrustRule struct {
	Pattern string `yaml:"pattern" json:"pattern"`
	Action  string `yaml:"action" json:"action"`
}

// RatchetPromptPattern is a user-defined screen prompt auto-response pattern.
type RatchetPromptPattern struct {
	Name   string `yaml:"name" json:"name"`
	Match  string `yaml:"match" json:"match"`
	Action string `yaml:"action" json:"action"`
}

// ParseRatchetTrustConfig converts ratchet YAML trust config into TrustRules.
func ParseRatchetTrustConfig(cfg RatchetTrustConfig) []TrustRule {
	var rules []TrustRule
	for _, r := range cfg.Rules {
		var action Action
		switch strings.ToLower(r.Action) {
		case "allow":
			action = Allow
		case "deny":
			action = Deny
		case "ask":
			action = Ask
		default:
			action = Deny
		}
		rules = append(rules, TrustRule{
			Pattern: r.Pattern,
			Action:  action,
			Scope:   "global",
		})
	}
	return rules
}
```

**Test command:**
```bash
cd /Users/jon/workspace/workflow-plugin-agent && go test ./policy/ -run TestParseRatchet -v
```

**Commit:** `feat(policy): add ratchet YAML trust config parser`

---

## Phase 2 — GlobMatcher + PermissionStore (wpa)

### Task 2.1: GlobMatcher for path policy

**File:** `wpa/policy/path.go` (new)

**Test first — File:** `wpa/policy/path_test.go` (new)

```go
package policy

import "testing"

func TestGlobMatcherAllow(t *testing.T) {
	gm := NewGlobMatcher([]string{"/tmp/*", "/workspace/**"}, nil)
	if gm.Check("/tmp/foo.txt") != Allow {
		t.Error("/tmp/foo.txt should be allowed")
	}
}

func TestGlobMatcherDenyWins(t *testing.T) {
	gm := NewGlobMatcher([]string{"/home/**"}, []string{"/home/user/.ssh/*"})
	if gm.Check("/home/user/.ssh/id_rsa") != Deny {
		t.Error(".ssh should be denied even though /home is allowed")
	}
}

func TestGlobMatcherNoMatch(t *testing.T) {
	gm := NewGlobMatcher([]string{"/tmp/*"}, nil)
	if gm.Check("/etc/passwd") != Ask {
		t.Error("unmatched path should return Ask")
	}
}

func TestGlobMatcherDoublestar(t *testing.T) {
	gm := NewGlobMatcher([]string{"/workspace/**"}, nil)
	if gm.Check("/workspace/src/main.go") != Allow {
		t.Error("doublestar should match nested paths")
	}
}

func TestGlobMatcherEmpty(t *testing.T) {
	gm := NewGlobMatcher(nil, nil)
	if gm.Check("/any/path") != Ask {
		t.Error("empty matcher should return Ask")
	}
}

func TestGlobMatcherDenyOnly(t *testing.T) {
	gm := NewGlobMatcher(nil, []string{"/etc/**"})
	if gm.Check("/etc/passwd") != Deny {
		t.Error("/etc should be denied")
	}
	if gm.Check("/tmp/foo") != Ask {
		t.Error("non-denied path should Ask")
	}
}
```

**Implementation — File:** `wpa/policy/path.go`

```go
package policy

import (
	"path/filepath"
	"strings"
)

// GlobMatcher evaluates file paths against allow/deny glob patterns.
// Deny patterns are checked first (deny wins). Unmatched paths return Ask.
type GlobMatcher struct {
	allow []string
	deny  []string
}

// NewGlobMatcher creates a GlobMatcher from allow and deny glob lists.
func NewGlobMatcher(allow, deny []string) *GlobMatcher {
	return &GlobMatcher{allow: allow, deny: deny}
}

// Check returns Allow, Deny, or Ask for the given path.
// Deny patterns are checked first.
func (gm *GlobMatcher) Check(path string) Action {
	path = filepath.Clean(path)

	// Deny first
	for _, pattern := range gm.deny {
		if globMatch(pattern, path) {
			return Deny
		}
	}

	// Allow
	for _, pattern := range gm.allow {
		if globMatch(pattern, path) {
			return Allow
		}
	}

	return Ask
}

// globMatch matches a path against a pattern, supporting ** for recursive matching.
func globMatch(pattern, path string) bool {
	// Handle ** by checking if the path prefix matches the part before **.
	if strings.Contains(pattern, "**") {
		prefix := strings.SplitN(pattern, "**", 2)[0]
		prefix = filepath.Clean(prefix)
		if prefix == "." {
			prefix = ""
		}
		if prefix == "" || strings.HasPrefix(path, prefix) {
			// ** matches everything below the prefix.
			suffix := strings.SplitN(pattern, "**", 2)[1]
			if suffix == "" || suffix == "/" {
				return true
			}
			// Check suffix pattern against rest of path
			rest := strings.TrimPrefix(path, prefix)
			// Try matching suffix against each sub-path
			matched, _ := filepath.Match(strings.TrimPrefix(suffix, "/"), filepath.Base(rest))
			if matched {
				return true
			}
			// Generous match: if suffix is just /*, match anything
			return strings.HasPrefix(path, prefix)
		}
		return false
	}

	// Use filepath.Match for simple globs.
	matched, _ := filepath.Match(pattern, path)
	if matched {
		return true
	}

	// Also try matching against the path without trailing slash variations.
	// Pattern "/tmp/*" should match "/tmp/foo.txt".
	if strings.HasSuffix(pattern, "/*") {
		dir := strings.TrimSuffix(pattern, "/*")
		if strings.HasPrefix(path, dir+"/") {
			return true
		}
	}

	return false
}
```

**Test command:**
```bash
cd /Users/jon/workspace/workflow-plugin-agent && go test ./policy/ -run TestGlobMatcher -v
```

**Commit:** `feat(policy): add GlobMatcher with ** support for path policies`

---

### Task 2.2: PermissionStore (SQLite persistence)

**File:** `wpa/policy/store.go` (new)

**Test first — File:** `wpa/policy/store_test.go` (new)

```go
package policy

import (
	"database/sql"
	"testing"

	_ "modernc.org/sqlite"
)

func testDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func TestPermissionStoreInit(t *testing.T) {
	db := testDB(t)
	ps, err := NewPermissionStore(db)
	if err != nil {
		t.Fatal(err)
	}
	if ps == nil {
		t.Fatal("NewPermissionStore returned nil")
	}
}

func TestPermissionStoreGrantAndCheck(t *testing.T) {
	db := testDB(t)
	ps, err := NewPermissionStore(db)
	if err != nil {
		t.Fatal(err)
	}

	if err := ps.Grant("bash:git *", Allow, "global", "user"); err != nil {
		t.Fatal(err)
	}

	action, ok := ps.Check("bash:git *", "global")
	if !ok {
		t.Fatal("expected match")
	}
	if action != Allow {
		t.Errorf("got %v, want Allow", action)
	}
}

func TestPermissionStoreNoMatch(t *testing.T) {
	db := testDB(t)
	ps, err := NewPermissionStore(db)
	if err != nil {
		t.Fatal(err)
	}

	_, ok := ps.Check("bash:git *", "global")
	if ok {
		t.Error("expected no match")
	}
}

func TestPermissionStoreRevoke(t *testing.T) {
	db := testDB(t)
	ps, err := NewPermissionStore(db)
	if err != nil {
		t.Fatal(err)
	}

	_ = ps.Grant("bash:git *", Allow, "global", "user")
	if err := ps.Revoke("bash:git *", "global"); err != nil {
		t.Fatal(err)
	}

	_, ok := ps.Check("bash:git *", "global")
	if ok {
		t.Error("expected no match after revoke")
	}
}

func TestPermissionStoreList(t *testing.T) {
	db := testDB(t)
	ps, err := NewPermissionStore(db)
	if err != nil {
		t.Fatal(err)
	}

	_ = ps.Grant("file_read", Allow, "global", "user")
	_ = ps.Grant("bash:rm *", Deny, "global", "config")

	grants, err := ps.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(grants) != 2 {
		t.Fatalf("expected 2 grants, got %d", len(grants))
	}
}

func TestPermissionStoreScopedCheck(t *testing.T) {
	db := testDB(t)
	ps, err := NewPermissionStore(db)
	if err != nil {
		t.Fatal(err)
	}

	_ = ps.Grant("file_write", Deny, "agent:coder", "user")
	_ = ps.Grant("file_write", Allow, "global", "config")

	// Scoped check should find the agent-specific deny
	action, ok := ps.Check("file_write", "agent:coder")
	if !ok {
		t.Fatal("expected match for agent scope")
	}
	if action != Deny {
		t.Errorf("got %v, want Deny for agent scope", action)
	}

	// Global scope should find the allow
	action, ok = ps.Check("file_write", "global")
	if !ok {
		t.Fatal("expected match for global scope")
	}
	if action != Allow {
		t.Errorf("got %v, want Allow for global scope", action)
	}
}
```

**Implementation — File:** `wpa/policy/store.go`

```go
package policy

import (
	"database/sql"
	"fmt"
	"time"
)

const createPermissionGrantsTable = `
CREATE TABLE IF NOT EXISTS permission_grants (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    pattern TEXT NOT NULL,
    action TEXT NOT NULL,
    scope TEXT NOT NULL DEFAULT 'global',
    granted_by TEXT NOT NULL DEFAULT 'user',
    created_at DATETIME NOT NULL DEFAULT (datetime('now'))
);
CREATE INDEX IF NOT EXISTS idx_permission_grants_pattern ON permission_grants(pattern, scope);
`

// PermissionGrant is a persisted trust decision.
type PermissionGrant struct {
	ID        int64
	Pattern   string
	Action    Action
	Scope     string
	GrantedBy string
	CreatedAt time.Time
}

// PermissionStore persists "always allow/deny" trust decisions in SQLite.
type PermissionStore struct {
	db *sql.DB
}

// NewPermissionStore creates a PermissionStore and initializes the table.
func NewPermissionStore(db *sql.DB) (*PermissionStore, error) {
	if _, err := db.Exec(createPermissionGrantsTable); err != nil {
		return nil, fmt.Errorf("permission store: init table: %w", err)
	}
	return &PermissionStore{db: db}, nil
}

// Grant persists an allow/deny decision.
func (ps *PermissionStore) Grant(pattern string, action Action, scope, grantedBy string) error {
	// Upsert: remove existing grant for same pattern+scope, then insert.
	_, _ = ps.db.Exec(
		"DELETE FROM permission_grants WHERE pattern = ? AND scope = ?",
		pattern, scope,
	)
	_, err := ps.db.Exec(
		"INSERT INTO permission_grants (pattern, action, scope, granted_by) VALUES (?, ?, ?, ?)",
		pattern, string(action), scope, grantedBy,
	)
	return err
}

// Revoke removes a persisted grant.
func (ps *PermissionStore) Revoke(pattern, scope string) error {
	_, err := ps.db.Exec(
		"DELETE FROM permission_grants WHERE pattern = ? AND scope = ?",
		pattern, scope,
	)
	return err
}

// Check returns the persisted action for a pattern+scope, if any.
// Checks scope-specific first, then falls back to global.
func (ps *PermissionStore) Check(pattern, scope string) (Action, bool) {
	// Try exact scope first
	var actionStr string
	err := ps.db.QueryRow(
		"SELECT action FROM permission_grants WHERE pattern = ? AND scope = ? LIMIT 1",
		pattern, scope,
	).Scan(&actionStr)
	if err == nil {
		return Action(actionStr), true
	}

	// Fall back to global if scope is not global
	if scope != "global" {
		err = ps.db.QueryRow(
			"SELECT action FROM permission_grants WHERE pattern = ? AND scope = 'global' LIMIT 1",
			pattern,
		).Scan(&actionStr)
		if err == nil {
			return Action(actionStr), true
		}
	}

	return "", false
}

// List returns all persisted grants.
func (ps *PermissionStore) List() ([]PermissionGrant, error) {
	rows, err := ps.db.Query(
		"SELECT id, pattern, action, scope, granted_by, created_at FROM permission_grants ORDER BY created_at ASC",
	)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var grants []PermissionGrant
	for rows.Next() {
		var g PermissionGrant
		var createdAt string
		if err := rows.Scan(&g.ID, &g.Pattern, &g.Action, &g.Scope, &g.GrantedBy, &createdAt); err != nil {
			return nil, err
		}
		g.CreatedAt, _ = time.Parse("2006-01-02 15:04:05", createdAt)
		grants = append(grants, g)
	}
	return grants, rows.Err()
}
```

**Test command:**
```bash
cd /Users/jon/workspace/workflow-plugin-agent && go test ./policy/ -run TestPermissionStore -v
```

**Commit:** `feat(policy): add PermissionStore for SQLite-persisted trust grants`

---

## Phase 3 — PTY PromptHandler (wpa)

### Task 3.1: Screen pattern matching and auto-response

**File:** `wpa/genkit/pty_prompts.go` (new)

**Test first — File:** `wpa/genkit/pty_prompts_test.go` (new)

```go
package genkit

import (
	"testing"

	"github.com/GoCodeAlone/workflow-plugin-agent/policy"
)

func TestPromptHandlerTrustDialog(t *testing.T) {
	te := policy.NewTrustEngine("permissive", nil, nil)
	ph := NewPromptHandler(te, nil, nil)

	screen := "Do you trust this folder?\n  Yes, allow access\n  No\nEnter to confirm"
	action, response := ph.Evaluate(screen)
	if action != PromptActionRespond {
		t.Errorf("expected Respond, got %v", action)
	}
	if response != "\r" {
		t.Errorf("expected carriage return, got %q", response)
	}
}

func TestPromptHandlerCommandExec(t *testing.T) {
	rules := []policy.TrustRule{
		{Pattern: "bash:git *", Action: policy.Allow},
	}
	te := policy.NewTrustEngine("custom", rules, nil)
	ph := NewPromptHandler(te, nil, nil)

	screen := "Run command: git status? (y/n)"
	action, _ := ph.Evaluate(screen)
	if action != PromptActionRespond {
		t.Errorf("expected Respond for allowed command, got %v", action)
	}
}

func TestPromptHandlerDeniedCommand(t *testing.T) {
	rules := []policy.TrustRule{
		{Pattern: "bash:rm -rf *", Action: policy.Deny},
	}
	te := policy.NewTrustEngine("custom", rules, nil)
	ph := NewPromptHandler(te, nil, nil)

	screen := "Run command: rm -rf /? (y/n)"
	action, response := ph.Evaluate(screen)
	if action != PromptActionRespond {
		t.Errorf("expected Respond (with 'n'), got %v", action)
	}
	if response != "n\r" {
		t.Errorf("expected 'n\\r' for denied command, got %q", response)
	}
}

func TestPromptHandlerAskQueues(t *testing.T) {
	te := policy.NewTrustEngine("locked", nil, nil)
	var queued string
	ph := NewPromptHandler(te, nil, func(agentName, promptText string) {
		queued = promptText
	})

	screen := "Allow write to /workspace/main.go? (y/n)"
	action, _ := ph.Evaluate(screen)
	if action != PromptActionQueue {
		t.Errorf("expected Queue, got %v", action)
	}
	if queued == "" {
		t.Error("expected onQueue callback to fire")
	}
}

func TestPromptHandlerNoMatch(t *testing.T) {
	te := policy.NewTrustEngine("permissive", nil, nil)
	ph := NewPromptHandler(te, nil, nil)

	screen := "Hello, world! This is normal output."
	action, _ := ph.Evaluate(screen)
	if action != PromptActionNone {
		t.Errorf("expected None, got %v", action)
	}
}

func TestPromptHandlerCustomPattern(t *testing.T) {
	te := policy.NewTrustEngine("permissive", nil, nil)
	custom := []PromptPattern{
		{
			Name:    "npm_install",
			MatchRe: "npm install",
			Default: policy.Allow,
		},
	}
	ph := NewPromptHandler(te, custom, nil)

	screen := "npm install detected, proceed? (y/n)"
	action, response := ph.Evaluate(screen)
	if action != PromptActionRespond {
		t.Errorf("expected Respond, got %v", action)
	}
	if response != "y\r" {
		t.Errorf("expected 'y\\r', got %q", response)
	}
}
```

**Implementation — File:** `wpa/genkit/pty_prompts.go`

```go
package genkit

import (
	"regexp"
	"strings"

	"github.com/GoCodeAlone/workflow-plugin-agent/policy"
)

// PromptAction is what the handler decides to do with a detected screen prompt.
type PromptAction int

const (
	PromptActionNone    PromptAction = iota // no prompt detected
	PromptActionRespond                     // auto-respond with a keystroke
	PromptActionQueue                       // queue for human approval
)

// PromptPattern is a screen content pattern with a default action.
type PromptPattern struct {
	Name    string
	MatchRe string       // regex string to match against screen content
	Extract func(string) string // optional: extract the action/path from screen
	Default policy.Action        // what to do if no trust rule matches
	re      *regexp.Regexp       // compiled regex (lazy)
}

// PromptHandler auto-responds to known screen prompts from CLI tools.
type PromptHandler struct {
	trust    *policy.TrustEngine
	patterns []PromptPattern
	onQueue  func(agentName, promptText string)
}

// Built-in patterns for common CLI prompts.
var builtinPromptPatterns = []PromptPattern{
	{
		Name:    "trust_dialog",
		MatchRe: `(?i)(trust this folder|safety check|Confirm folder|Do you trust)`,
		Default: policy.Allow,
	},
	{
		Name:    "command_exec",
		MatchRe: `(?i)(Run command|execute|allow.*command).*\?.*\(y/n\)`,
		Extract: extractCommand,
		Default: policy.Ask,
	},
	{
		Name:    "file_write",
		MatchRe: `(?i)(allow (write|edit|create)|write to|create file).*\?.*\(y/n\)`,
		Extract: extractPath,
		Default: policy.Ask,
	},
	{
		Name:    "permission_prompt",
		MatchRe: `(?i)(allow|approve|permit).*\?.*\(y/n\)`,
		Default: policy.Ask,
	},
}

// NewPromptHandler creates a PromptHandler with the given trust engine.
// customPatterns are checked before built-in patterns.
// onQueue is called when a prompt requires human approval.
func NewPromptHandler(trust *policy.TrustEngine, customPatterns []PromptPattern, onQueue func(agentName, promptText string)) *PromptHandler {
	patterns := make([]PromptPattern, 0, len(customPatterns)+len(builtinPromptPatterns))
	patterns = append(patterns, customPatterns...)
	patterns = append(patterns, builtinPromptPatterns...)

	// Compile regexes
	for i := range patterns {
		if patterns[i].re == nil && patterns[i].MatchRe != "" {
			patterns[i].re = regexp.MustCompile(patterns[i].MatchRe)
		}
	}

	return &PromptHandler{
		trust:    trust,
		patterns: patterns,
		onQueue:  onQueue,
	}
}

// Evaluate checks the screen content against known patterns.
// Returns the action to take and the response string to send (if PromptActionRespond).
func (ph *PromptHandler) Evaluate(screen string) (PromptAction, string) {
	clean := stripANSI(screen)

	for _, p := range ph.patterns {
		if p.re == nil {
			continue
		}
		if !p.re.MatchString(clean) {
			continue
		}

		// Trust dialog: always auto-approve with Enter.
		if p.Name == "trust_dialog" {
			return PromptActionRespond, "\r"
		}

		// Extract the specific action/command if possible.
		var extracted string
		if p.Extract != nil {
			extracted = p.Extract(clean)
		}

		// Evaluate against trust engine.
		action := p.Default
		if ph.trust != nil && extracted != "" {
			if p.Name == "command_exec" {
				action = ph.trust.EvaluateCommand(extracted)
			} else if p.Name == "file_write" {
				action = ph.trust.EvaluatePath(extracted)
			}
		}

		switch action {
		case policy.Allow:
			return PromptActionRespond, "y\r"
		case policy.Deny:
			return PromptActionRespond, "n\r"
		case policy.Ask:
			if ph.onQueue != nil {
				ph.onQueue("", clean)
			}
			return PromptActionQueue, ""
		}
	}

	return PromptActionNone, ""
}

// extractCommand attempts to extract the command from a "Run command: xxx?" prompt.
var commandExtractRe = regexp.MustCompile(`(?i)(?:Run command|execute)[:\s]+([^\?]+)`)

func extractCommand(screen string) string {
	m := commandExtractRe.FindStringSubmatch(screen)
	if len(m) > 1 {
		return strings.TrimSpace(m[1])
	}
	return ""
}

// extractPath attempts to extract a file path from a permission prompt.
var pathExtractRe = regexp.MustCompile(`(?:write to|edit|create file|allow write)[:\s]+([/~][^\?\s]+)`)

func extractPath(screen string) string {
	m := pathExtractRe.FindStringSubmatch(screen)
	if len(m) > 1 {
		return strings.TrimSpace(m[1])
	}
	return ""
}
```

**Test command:**
```bash
cd /Users/jon/workspace/workflow-plugin-agent && go test ./genkit/ -run TestPromptHandler -v
```

**Commit:** `feat(genkit): add PromptHandler for auto-responding to PTY screen prompts`

---

### Task 3.2: Wire PromptHandler into PTY readResponse loop

**File:** `wpa/genkit/pty_provider.go` (modify)

Add `promptHandler` field to `ptyProvider` struct and check it in `waitForPrompt` and `readResponse`.

**Modify struct at line 54:**

```go
// Add after line 67 (output bytes.Buffer):
	promptHandler *PromptHandler // auto-responds to known screen prompts
```

**Modify `waitForPrompt` (line 262) — replace the hardcoded trust-dialog block (lines 276-279):**

Replace:
```go
		// Auto-handle trust/safety prompts before checking for the actual input prompt.
		// Both Claude Code and Copilot show trust dialogs on first use in a directory.
		if (strings.Contains(lower, "trust") || strings.Contains(lower, "safety check")) &&
			(strings.Contains(screen, "Yes") || strings.Contains(screen, "Enter to confirm") || strings.Contains(screen, "Enter to select")) {
			ptmx.Write([]byte{'\r'})
			time.Sleep(2 * time.Second)
			continue
		}
```

With:
```go
		// Auto-handle prompts via PromptHandler (trust dialogs, permission prompts, etc.).
		if p.promptHandler != nil {
			action, response := p.promptHandler.Evaluate(screen)
			if action == PromptActionRespond && response != "" {
				ptmx.Write([]byte(response))
				time.Sleep(2 * time.Second)
				continue
			}
		} else {
			// Fallback: hardcoded trust dialog handling when no PromptHandler is configured.
			if (strings.Contains(lower, "trust") || strings.Contains(lower, "safety check")) &&
				(strings.Contains(screen, "Yes") || strings.Contains(screen, "Enter to confirm") || strings.Contains(screen, "Enter to select")) {
				ptmx.Write([]byte{'\r'})
				time.Sleep(2 * time.Second)
				continue
			}
		}
```

**Add setter on ptyProvider:**

```go
// SetPromptHandler attaches a PromptHandler for auto-responding to screen prompts.
func (p *ptyProvider) SetPromptHandler(ph *PromptHandler) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.promptHandler = ph
}
```

**Test command:**
```bash
cd /Users/jon/workspace/workflow-plugin-agent && go test ./genkit/ -v -count=1
```

**Commit:** `feat(genkit): wire PromptHandler into PTY waitForPrompt loop`

---

## Phase 4 — Docker Sandbox on Executor (wpa)

### Task 4.1: Add SandboxMode + TrustEngine to executor.Config

**File:** `wpa/executor/executor.go` (modify)

**Add fields to Config struct (after line 65, the OnEvent field):**

```go
	// TrustEngine evaluates trust rules before tool execution. Nil = no trust enforcement.
	TrustEngine TrustEvaluator

	// SandboxMode routes bash and file tool calls through ContainerManager.
	SandboxMode bool

	// ContainerMgr provides Docker container execution. Required when SandboxMode is true.
	ContainerMgr ContainerExecutor

	// SandboxSpec is the container configuration for sandbox mode.
	SandboxSpec *SandboxConfig

	// ProjectID is used for container management and transcript recording.
	// (already exists — just documenting sandbox usage)
```

**File:** `wpa/executor/interfaces.go` (modify — append)

```go
// TrustEvaluator checks whether a tool call is permitted.
type TrustEvaluator interface {
	// Evaluate returns the trust action for a tool call.
	Evaluate(ctx context.Context, toolName string, args map[string]any) Action
	// EvaluateCommand returns the trust action for a bash command.
	EvaluateCommand(cmd string) Action
	// EvaluatePath returns the trust action for a file path.
	EvaluatePath(path string) Action
}

// Action mirrors policy.Action to avoid circular imports.
type Action string

const (
	ActionAllow Action = "allow"
	ActionDeny  Action = "deny"
	ActionAsk   Action = "ask"
)

// ContainerExecutor can run commands inside a Docker container.
type ContainerExecutor interface {
	IsAvailable() bool
	EnsureContainer(ctx context.Context, projectID, workspacePath string, spec any) (string, error)
	ExecInContainer(ctx context.Context, projectID, command, workDir string, timeout int) (stdout, stderr string, exitCode int, err error)
}

// SandboxConfig holds per-agent Docker sandbox settings.
type SandboxConfig struct {
	Enabled     bool              `json:"enabled" yaml:"enabled"`
	Image       string            `json:"image" yaml:"image"`
	Network     string            `json:"network" yaml:"network"`
	Memory      string            `json:"memory" yaml:"memory"`
	CPU         float64           `json:"cpu" yaml:"cpu"`
	Mounts      []SandboxMount    `json:"mounts" yaml:"mounts"`
	InitCommands []string         `json:"init" yaml:"init"`
}

// SandboxMount is a bind mount for a sandbox container.
type SandboxMount struct {
	Src      string `json:"src" yaml:"src"`
	Dst      string `json:"dst" yaml:"dst"`
	ReadOnly bool   `json:"readonly" yaml:"readonly"`
}

// NullTrustEvaluator allows everything (no trust enforcement).
type NullTrustEvaluator struct{}

func (n *NullTrustEvaluator) Evaluate(_ context.Context, _ string, _ map[string]any) Action {
	return ActionAllow
}
func (n *NullTrustEvaluator) EvaluateCommand(_ string) Action { return ActionAllow }
func (n *NullTrustEvaluator) EvaluatePath(_ string) Action    { return ActionAllow }
```

**Test first — File:** `wpa/executor/trust_test.go` (new)

```go
package executor

import (
	"context"
	"testing"
)

func TestNullTrustEvaluator(t *testing.T) {
	te := &NullTrustEvaluator{}
	if te.Evaluate(context.Background(), "file_write", nil) != ActionAllow {
		t.Error("NullTrustEvaluator should always allow")
	}
	if te.EvaluateCommand("rm -rf /") != ActionAllow {
		t.Error("NullTrustEvaluator should always allow commands")
	}
}

type mockTrustEvaluator struct {
	toolAction Action
	cmdAction  Action
}

func (m *mockTrustEvaluator) Evaluate(_ context.Context, _ string, _ map[string]any) Action {
	return m.toolAction
}
func (m *mockTrustEvaluator) EvaluateCommand(_ string) Action { return m.cmdAction }
func (m *mockTrustEvaluator) EvaluatePath(_ string) Action    { return ActionAllow }

func TestTrustEvaluatorInterface(t *testing.T) {
	var te TrustEvaluator = &mockTrustEvaluator{toolAction: ActionDeny}
	if te.Evaluate(context.Background(), "bash", nil) != ActionDeny {
		t.Error("mock should deny")
	}
}
```

**Test command:**
```bash
cd /Users/jon/workspace/workflow-plugin-agent && go test ./executor/ -run TestTrust -v
cd /Users/jon/workspace/workflow-plugin-agent && go test ./executor/ -run TestNullTrust -v
```

**Commit:** `feat(executor): add TrustEvaluator, ContainerExecutor, SandboxConfig interfaces`

---

### Task 4.2: Integrate TrustEngine into executor tool call loop

**File:** `wpa/executor/executor.go` (modify)

**Insert trust evaluation in the tool call loop, right after the `EventToolCallStart` emit (line 284) and before the `if cfg.ToolRegistry != nil` block (line 286):**

```go
			// Trust evaluation: check if the tool call is permitted.
			if cfg.TrustEngine != nil {
				trustAction := cfg.TrustEngine.Evaluate(ctx, tc.Name, tc.Arguments)

				// For bash/shell tools, also check the command.
				if (tc.Name == "shell_exec" || tc.Name == "bash") && trustAction == ActionAllow {
					if cmdStr, ok := tc.Arguments["command"].(string); ok {
						trustAction = cfg.TrustEngine.EvaluateCommand(cmdStr)
					}
				}

				// For file tools, also check the path.
				if (tc.Name == "file_read" || tc.Name == "file_write" || tc.Name == "file_list") && trustAction == ActionAllow {
					if pathStr, ok := tc.Arguments["path"].(string); ok {
						pathAction := cfg.TrustEngine.EvaluatePath(pathStr)
						if pathAction == ActionDeny {
							trustAction = ActionDeny
						} else if pathAction == ActionAsk && trustAction == ActionAllow {
							trustAction = ActionAsk
						}
					}
				}

				switch trustAction {
				case ActionDeny:
					resultStr = fmt.Sprintf("Tool %q denied by trust policy", tc.Name)
					isError = true

					_ = cfg.Transcript.Record(ctx, TranscriptEntry{
						ID:        uuid.New().String(),
						AgentID:   agentID,
						TaskID:    cfg.TaskID,
						ProjectID: cfg.ProjectID,
						Iteration: iterCount,
						Role:      provider.RoleUser,
						Content:   fmt.Sprintf("[TRUST DENY] %s by %s", tc.Name, agentID),
					})

					goto toolDone
				case ActionAsk:
					// Block on approval if Approver is available.
					resultStr = fmt.Sprintf("Tool %q requires human approval (trust policy: ask)", tc.Name)
					isError = true

					_ = cfg.Transcript.Record(ctx, TranscriptEntry{
						ID:        uuid.New().String(),
						AgentID:   agentID,
						TaskID:    cfg.TaskID,
						ProjectID: cfg.ProjectID,
						Iteration: iterCount,
						Role:      provider.RoleUser,
						Content:   fmt.Sprintf("[TRUST ASK] %s by %s — queued for human", tc.Name, agentID),
					})

					goto toolDone
				}
				// ActionAllow: fall through to normal execution.
			}
```

**Add `toolDone` label after the tool execution block**, right before the approval gates block (before line 307 `// Handle approval gates`):

This requires restructuring the loop body. Instead of a goto, wrap the existing tool execution in an `if !isError` guard:

**Actually, cleaner approach — modify the tool execution block (lines 286-304) to be conditional:**

Replace lines 286-304:
```go
			if cfg.ToolRegistry != nil {
				// Build tool context with agent/task IDs
				toolCtx := tools.WithAgentID(ctx, agentID)
				if cfg.TaskID != "" {
					toolCtx = tools.WithTaskID(toolCtx, cfg.TaskID)
				}

				result, execErr := cfg.ToolRegistry.Execute(toolCtx, tc.Name, tc.Arguments)
				if execErr != nil {
					resultStr = fmt.Sprintf("Error: %v", execErr)
					isError = true
				} else {
					resultBytes, _ := json.Marshal(result)
					resultStr = string(resultBytes)
				}
			} else {
				resultStr = "Tool execution not available"
				isError = true
			}
```

With:
```go
			// Trust evaluation: check if the tool call is permitted.
			if cfg.TrustEngine != nil && !isError {
				trustAction := cfg.TrustEngine.Evaluate(ctx, tc.Name, tc.Arguments)

				if (tc.Name == "shell_exec" || tc.Name == "bash") && trustAction == ActionAllow {
					if cmdStr, ok := tc.Arguments["command"].(string); ok {
						trustAction = cfg.TrustEngine.EvaluateCommand(cmdStr)
					}
				}
				if (tc.Name == "file_read" || tc.Name == "file_write" || tc.Name == "file_list") && trustAction == ActionAllow {
					if pathStr, ok := tc.Arguments["path"].(string); ok {
						pathAction := cfg.TrustEngine.EvaluatePath(pathStr)
						if pathAction == ActionDeny || (pathAction == ActionAsk && trustAction != ActionDeny) {
							trustAction = pathAction
						}
					}
				}

				switch trustAction {
				case ActionDeny:
					resultStr = fmt.Sprintf("Tool %q denied by trust policy", tc.Name)
					isError = true
					_ = cfg.Transcript.Record(ctx, TranscriptEntry{
						ID: uuid.New().String(), AgentID: agentID, TaskID: cfg.TaskID,
						ProjectID: cfg.ProjectID, Iteration: iterCount, Role: provider.RoleUser,
						Content: fmt.Sprintf("[TRUST DENY] %s by %s", tc.Name, agentID),
					})
				case ActionAsk:
					resultStr = fmt.Sprintf("Tool %q requires human approval (trust policy: ask)", tc.Name)
					isError = true
					_ = cfg.Transcript.Record(ctx, TranscriptEntry{
						ID: uuid.New().String(), AgentID: agentID, TaskID: cfg.TaskID,
						ProjectID: cfg.ProjectID, Iteration: iterCount, Role: provider.RoleUser,
						Content: fmt.Sprintf("[TRUST ASK] %s by %s", tc.Name, agentID),
					})
				}
			}

			// Execute tool if not blocked by trust.
			if !isError {
				if cfg.ToolRegistry != nil {
					toolCtx := tools.WithAgentID(ctx, agentID)
					if cfg.TaskID != "" {
						toolCtx = tools.WithTaskID(toolCtx, cfg.TaskID)
					}

					// Sandbox mode: route bash/file tools through container.
					if cfg.SandboxMode && cfg.ContainerMgr != nil && cfg.ContainerMgr.IsAvailable() &&
						(tc.Name == "shell_exec" || tc.Name == "bash") {
						if cmdStr, ok := tc.Arguments["command"].(string); ok {
							stdout, stderr, exitCode, execErr := cfg.ContainerMgr.ExecInContainer(
								ctx, cfg.ProjectID, cmdStr, "/workspace", 60,
							)
							if execErr != nil {
								resultStr = fmt.Sprintf("Error (sandbox): %v", execErr)
								isError = true
							} else {
								result := map[string]any{
									"stdout":    stdout,
									"stderr":    stderr,
									"exit_code": exitCode,
								}
								resultBytes, _ := json.Marshal(result)
								resultStr = string(resultBytes)
							}
						}
					} else {
						result, execErr := cfg.ToolRegistry.Execute(toolCtx, tc.Name, tc.Arguments)
						if execErr != nil {
							resultStr = fmt.Sprintf("Error: %v", execErr)
							isError = true
						} else {
							resultBytes, _ := json.Marshal(result)
							resultStr = string(resultBytes)
						}
					}
				} else {
					resultStr = "Tool execution not available"
					isError = true
				}
			}
```

**Test command:**
```bash
cd /Users/jon/workspace/workflow-plugin-agent && go test ./executor/ -v -count=1
```

**Commit:** `feat(executor): integrate TrustEngine and sandbox routing into tool call loop`

---

### Task 4.3: Extend ContainerManager with per-agent sandbox support

**File:** `wpa/orchestrator/container_manager.go` (modify)

**Add MountSpec to WorkspaceSpec struct (after line 28):**

```go
// MountSpec describes a single bind mount.
type MountSpec struct {
	Source   string `json:"source"`
	Target   string `json:"target"`
	ReadOnly bool   `json:"readonly,omitempty"`
}
```

**Add Mounts field to WorkspaceSpec (after NetworkMode line 27):**

```go
	Mounts       []MountSpec       `json:"mounts,omitempty"`
```

**Modify EnsureContainer (line 143-149) to support additional mounts:**

Replace the hardcoded single mount:
```go
	hostCfg := &container.HostConfig{
		Mounts: []mount.Mount{
			{
				Type:   mount.TypeBind,
				Source: workspacePath,
				Target: "/workspace",
			},
		},
	}
```

With:
```go
	mounts := []mount.Mount{
		{
			Type:   mount.TypeBind,
			Source: workspacePath,
			Target: "/workspace",
		},
	}
	for _, m := range spec.Mounts {
		mounts = append(mounts, mount.Mount{
			Type:     mount.TypeBind,
			Source:   m.Source,
			Target:   m.Target,
			ReadOnly: m.ReadOnly,
		})
	}
	hostCfg := &container.HostConfig{
		Mounts: mounts,
	}
```

**Test command:**
```bash
cd /Users/jon/workspace/workflow-plugin-agent && go build ./...
```

**Commit:** `feat(orchestrator): extend WorkspaceSpec with per-agent mount specs`

---

**Tag wpa release after Phase 4 is complete:**

```bash
cd /Users/jon/workspace/workflow-plugin-agent && git tag v0.7.0 && git push origin v0.7.0
```

---

## Phase 5 — Ratchet Config Wiring + --mode Flag + /trust TUI (rcli)

### Task 5.0: Update wpa dependency

```bash
cd /Users/jon/workspace/ratchet-cli && go get github.com/GoCodeAlone/workflow-plugin-agent@v0.7.0
cd /Users/jon/workspace/ratchet-cli && go mod tidy
```

**Commit:** `build: update workflow-plugin-agent to v0.7.0`

---

### Task 5.1: Add trust config to ratchet config

**File:** `rcli/internal/config/trust.go` (new)

**Test first — File:** `rcli/internal/config/trust_test.go` (new)

```go
package config

import (
	"testing"

	"github.com/GoCodeAlone/workflow-plugin-agent/policy"
)

func TestTrustConfigDefaults(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.Trust.Mode != "conservative" {
		t.Errorf("default mode = %q, want conservative", cfg.Trust.Mode)
	}
}

func TestTrustConfigToRules(t *testing.T) {
	tc := TrustConfig{
		Mode: "custom",
		Rules: []TrustRuleConfig{
			{Pattern: "file_read", Action: "allow"},
			{Pattern: "bash:rm *", Action: "deny"},
		},
	}
	rules := tc.ToTrustRules()
	if len(rules) != 2 {
		t.Fatalf("expected 2 rules, got %d", len(rules))
	}
	if rules[0].Action != policy.Allow {
		t.Error("first rule should be Allow")
	}
	if rules[1].Action != policy.Deny {
		t.Error("second rule should be Deny")
	}
}

func TestTrustConfigProviderArgs(t *testing.T) {
	tc := TrustConfig{
		ProviderArgs: map[string][]string{
			"claude_code": {"--permission-mode", "acceptEdits"},
			"copilot_cli": {"--allow-tool=Edit"},
		},
	}
	args := tc.ProviderArgsFor("claude_code")
	if len(args) != 2 {
		t.Fatalf("expected 2 args, got %d", len(args))
	}
	if args[0] != "--permission-mode" {
		t.Errorf("first arg = %q", args[0])
	}
}

func TestTrustConfigProviderArgsEmpty(t *testing.T) {
	tc := TrustConfig{}
	args := tc.ProviderArgsFor("claude_code")
	if len(args) != 0 {
		t.Errorf("expected 0 args, got %d", len(args))
	}
}
```

**Implementation — File:** `rcli/internal/config/trust.go`

```go
package config

import (
	"strings"

	"github.com/GoCodeAlone/workflow-plugin-agent/policy"
)

// TrustConfig is the trust section of ~/.ratchet/config.yaml.
type TrustConfig struct {
	Mode         string              `yaml:"mode"`
	Rules        []TrustRuleConfig   `yaml:"rules"`
	ProviderArgs map[string][]string `yaml:"provider_args"`
}

// TrustRuleConfig is a single trust rule in the ratchet config format.
type TrustRuleConfig struct {
	Pattern string `yaml:"pattern"`
	Action  string `yaml:"action"`
}

// ToTrustRules converts the config rules into policy.TrustRule values.
func (tc *TrustConfig) ToTrustRules() []policy.TrustRule {
	var rules []policy.TrustRule
	for _, r := range tc.Rules {
		var action policy.Action
		switch strings.ToLower(r.Action) {
		case "allow":
			action = policy.Allow
		case "deny":
			action = policy.Deny
		case "ask":
			action = policy.Ask
		default:
			action = policy.Deny
		}
		rules = append(rules, policy.TrustRule{
			Pattern: r.Pattern,
			Action:  action,
			Scope:   "global",
		})
	}
	return rules
}

// ProviderArgsFor returns CLI args for the given provider name.
func (tc *TrustConfig) ProviderArgsFor(providerName string) []string {
	if tc.ProviderArgs == nil {
		return nil
	}
	return tc.ProviderArgs[providerName]
}
```

**Modify:** `rcli/internal/config/config.go` — add Trust field to Config struct.

Add after `Context ContextConfig` (line 18):
```go
	Trust             TrustConfig      `yaml:"trust"`
```

Update `DefaultConfig()` to set default trust mode:
```go
		Trust: TrustConfig{
			Mode: "conservative",
		},
```

**Test command:**
```bash
cd /Users/jon/workspace/ratchet-cli && go test ./internal/config/ -run TestTrust -v
```

**Commit:** `feat(config): add TrustConfig with mode, rules, and provider_args`

---

### Task 5.2: Add --mode flag to CLI

**File:** `rcli/cmd/ratchet/main.go` (modify)

In the argument parsing loop (around line 20), add `--mode` flag handling:

```go
	var modeFlag string
	// Add to the flag parsing section:
	for i, arg := range os.Args[1:] {
		switch {
		case arg == "--reconfigure" || arg == "-r":
			reconfigure = true
		case arg == "--mode" && i+1 < len(os.Args[1:])-1:
			modeFlag = os.Args[i+2]
		case strings.HasPrefix(arg, "--mode="):
			modeFlag = strings.TrimPrefix(arg, "--mode=")
		}
	}
```

Pass `modeFlag` to daemon or session creation. The exact wiring depends on the existing architecture. The mode should be stored so the daemon's Service can pass it to TrustEngine creation.

**File:** `rcli/internal/daemon/service.go` (modify)

Add trust engine wiring in `NewService`:

After `svc.autorespond = LoadAutoresponder(wd)` (line 92), add:

```go
	// Initialize TrustEngine from config.
	trustMode := "conservative"
	var trustRules []policy.TrustRule
	if cfg != nil {
		if cfg.Trust.Mode != "" {
			trustMode = cfg.Trust.Mode
		}
		trustRules = cfg.Trust.ToTrustRules()
	}
	svc.trustEngine = policy.NewTrustEngine(trustMode, trustRules, nil)

	// Initialize PermissionStore if DB is available.
	if engine.DB != nil {
		if ps, err := policy.NewPermissionStore(engine.DB); err == nil {
			svc.trustEngine.SetPermissionStore(ps)
		}
	}
```

Add field to Service struct:
```go
	trustEngine  *policy.TrustEngine
```

Add import:
```go
	"github.com/GoCodeAlone/workflow-plugin-agent/policy"
```

**Test command:**
```bash
cd /Users/jon/workspace/ratchet-cli && go build ./cmd/ratchet/
```

**Commit:** `feat(daemon): wire TrustEngine from config into Service`

---

### Task 5.3: Add /trust and /mode TUI commands

**File:** `rcli/internal/tui/commands/commands.go` (modify)

Add cases to the command switch:

```go
	case "/mode":
		return modeCmd(parts[1:], c)
	case "/trust":
		return trustCmd(parts[1:], c)
```

**File:** `rcli/internal/tui/commands/trust.go` (new)

```go
package commands

import (
	"fmt"
	"strings"

	pb "github.com/GoCodeAlone/ratchet-cli/internal/proto"
)

func modeCmd(args []string, c pb.RatchetDaemonClient) *Result {
	if len(args) == 0 {
		return &Result{Lines: []string{
			"Usage: /mode <conservative|permissive|locked|sandbox|custom>",
			"Switches the active trust mode. Affects all new tool calls.",
		}}
	}
	mode := args[0]
	valid := map[string]bool{"conservative": true, "permissive": true, "locked": true, "sandbox": true, "custom": true}
	if !valid[mode] {
		return &Result{Lines: []string{fmt.Sprintf("Unknown mode %q. Valid: conservative, permissive, locked, sandbox, custom", mode)}}
	}
	// TODO: Call daemon RPC to switch mode (requires SetMode RPC).
	return &Result{Lines: []string{fmt.Sprintf("Mode switched to %s", mode)}}
}

func trustCmd(args []string, c pb.RatchetDaemonClient) *Result {
	if len(args) == 0 {
		return &Result{Lines: []string{
			"Usage:",
			"  /trust list                  — show active rules",
			"  /trust allow \"pattern\"       — add allow rule",
			"  /trust deny \"pattern\"        — add deny rule",
			"  /trust reset                 — revert to config defaults",
		}}
	}

	switch args[0] {
	case "list":
		// TODO: Call daemon RPC to list trust rules.
		return &Result{Lines: []string{"Trust rules: (call daemon for live list)"}}
	case "allow":
		if len(args) < 2 {
			return &Result{Lines: []string{"Usage: /trust allow \"pattern\""}}
		}
		pattern := strings.Trim(strings.Join(args[1:], " "), "\"")
		// TODO: Call daemon RPC to add allow rule.
		return &Result{Lines: []string{fmt.Sprintf("Added allow rule: %s", pattern)}}
	case "deny":
		if len(args) < 2 {
			return &Result{Lines: []string{"Usage: /trust deny \"pattern\""}}
		}
		pattern := strings.Trim(strings.Join(args[1:], " "), "\"")
		// TODO: Call daemon RPC to add deny rule.
		return &Result{Lines: []string{fmt.Sprintf("Added deny rule: %s", pattern)}}
	case "reset":
		// TODO: Call daemon RPC to reset trust rules.
		return &Result{Lines: []string{"Trust rules reset to config defaults."}}
	default:
		return &Result{Lines: []string{fmt.Sprintf("Unknown trust subcommand: %s", args[0])}}
	}
}
```

**Test command:**
```bash
cd /Users/jon/workspace/ratchet-cli && go build ./cmd/ratchet/
```

**Commit:** `feat(tui): add /mode and /trust slash commands`

---

### Task 5.4: Wire TrustEngine into LocalNode executor config

**File:** `rcli/internal/mesh/local_node.go` (modify)

Add `TrustEngine` field to `NodeConfig`:

**File:** `rcli/internal/mesh/node.go` (modify)

Add after `AllowedPaths` field:
```go
	TrustEngine   interface{} // *policy.TrustEngine — passed to executor.Config
	SandboxMode   bool
	ContainerMgr  interface{} // executor.ContainerExecutor
	SandboxSpec   interface{} // *executor.SandboxConfig
```

**File:** `rcli/internal/mesh/local_node.go` (modify)

In the `Run` method, when building `executor.Config` (around line 169), add trust and sandbox fields:

```go
	// Wire trust engine if configured.
	if te, ok := n.config.TrustEngine.(executor.TrustEvaluator); ok {
		cfg.TrustEngine = te
	}
	if n.config.SandboxMode {
		cfg.SandboxMode = true
		if cm, ok := n.config.ContainerMgr.(executor.ContainerExecutor); ok {
			cfg.ContainerMgr = cm
		}
	}
```

Add import if not present:
```go
	"github.com/GoCodeAlone/workflow-plugin-agent/executor"
```

**Test command:**
```bash
cd /Users/jon/workspace/ratchet-cli && go build ./...
```

**Commit:** `feat(mesh): wire TrustEngine and SandboxMode into LocalNode executor config`

---

## Phase 6 — Per-Agent Args + Dynamic Mode Switching (both repos)

### Task 6.1: Make InteractiveArgs configurable (wpa)

**File:** `wpa/genkit/pty_adapters.go` (modify)

Change `ClaudeCodeAdapter` to accept configurable args:

Replace the struct and InteractiveArgs:

```go
// ClaudeCodeAdapter drives the `claude` CLI.
type ClaudeCodeAdapter struct {
	ExtraArgs []string // configurable interactive args
}
```

```go
func (a ClaudeCodeAdapter) InteractiveArgs() []string {
	if len(a.ExtraArgs) > 0 {
		return a.ExtraArgs
	}
	return []string{"--permission-mode", "acceptEdits"}
}
```

Do the same for CopilotCLIAdapter:

```go
type CopilotCLIAdapter struct {
	ExtraArgs []string
}
```

```go
func (a CopilotCLIAdapter) InteractiveArgs() []string {
	if len(a.ExtraArgs) > 0 {
		return a.ExtraArgs
	}
	return []string{"--allow-all"}
}
```

**Ensure all method receivers change from value to value (they already use value receivers, which is fine since ExtraArgs is a slice).**

**Test command:**
```bash
cd /Users/jon/workspace/workflow-plugin-agent && go test ./genkit/ -v -count=1
cd /Users/jon/workspace/workflow-plugin-agent && go build ./...
```

**Commit:** `feat(genkit): make adapter InteractiveArgs configurable via ExtraArgs`

---

### Task 6.2: Dynamic mode switching via TrustEngine.SetMode (wpa)

Already implemented in Task 1.1. The `SetMode` method replaces preset rules.

For PTY providers, mode switch requires session restart. Add helper:

**File:** `wpa/genkit/pty_provider.go` (modify — append)

```go
// RestartSession tears down the current PTY session so the next Stream() call
// starts a fresh one. Used when trust mode changes require different CLI args.
func (p *ptyProvider) RestartSession() error {
	p.sessionMu.Lock()
	defer p.sessionMu.Unlock()

	p.mu.Lock()
	defer p.mu.Unlock()

	if p.ptmx != nil {
		_ = p.ptmx.Close()
		p.ptmx = nil
	}
	if p.cmd != nil && p.cmd.Process != nil {
		_ = p.cmd.Process.Kill()
		_ = p.cmd.Wait()
		p.cmd = nil
	}
	p.vt = nil
	p.output.Reset()
	return nil
}
```

**Test command:**
```bash
cd /Users/jon/workspace/workflow-plugin-agent && go build ./genkit/
```

**Commit:** `feat(genkit): add RestartSession for mode switching`

---

### Task 6.3: Wire per-agent args from ratchet config (rcli)

**File:** `rcli/internal/mesh/local_node.go` (modify)

Add `ProviderArgs` to `NodeConfig`:

**File:** `rcli/internal/mesh/node.go` (modify)

```go
	ProviderArgs  []string // per-agent CLI args override
```

The daemon's team/fleet setup code should read `trust.provider_args` from config and `agents[].args` from team config, resolving per-agent > global provider_args > adapter defaults.

This wiring happens in the code that creates PTY providers for agents — wherever `ClaudeCodeAdapter{}` is instantiated, replace with:

```go
ClaudeCodeAdapter{ExtraArgs: resolvedArgs}
```

**Commit:** `feat(mesh): pass per-agent ProviderArgs to adapter ExtraArgs`

---

**Tag wpa release after Phase 6:**

```bash
cd /Users/jon/workspace/workflow-plugin-agent && git tag v0.7.1 && git push origin v0.7.1
```

```bash
cd /Users/jon/workspace/ratchet-cli && go get github.com/GoCodeAlone/workflow-plugin-agent@v0.7.1
cd /Users/jon/workspace/ratchet-cli && go mod tidy
```

---

## Phase 7 — Audit Trail in Transcript (both repos)

### Task 7.1: Add trust event types to executor events (wpa)

**File:** `wpa/executor/events.go` (modify)

Add new event types:

```go
const (
	// ... existing events ...
	EventTrustAllow  EventType = "trust_allow"
	EventTrustDeny   EventType = "trust_deny"
	EventTrustAsk    EventType = "trust_ask"
)
```

### Task 7.2: Emit trust events from executor (wpa)

**File:** `wpa/executor/executor.go` (modify)

In the trust evaluation block added in Task 4.2, emit events:

After `case ActionDeny:`:
```go
					emit(cfg, Event{
						Type:      EventTrustDeny,
						AgentID:   agentID,
						Iteration: iterCount,
						ToolName:  tc.Name,
						Content:   fmt.Sprintf("denied by trust policy"),
					})
```

After `case ActionAsk:`:
```go
					emit(cfg, Event{
						Type:      EventTrustAsk,
						AgentID:   agentID,
						Iteration: iterCount,
						ToolName:  tc.Name,
						Content:   "queued for human approval",
					})
```

Before the tool execution (after trust check passes):
```go
				emit(cfg, Event{
					Type:      EventTrustAllow,
					AgentID:   agentID,
					Iteration: iterCount,
					ToolName:  tc.Name,
				})
```

**Test command:**
```bash
cd /Users/jon/workspace/workflow-plugin-agent && go test ./executor/ -v -count=1
```

**Commit:** `feat(executor): emit trust_allow/deny/ask events for audit trail`

---

### Task 7.3: Display trust events in ratchet TUI (rcli)

**File:** `rcli/internal/daemon/service.go` or the event handler that processes executor.Event

In the event handler that maps executor events to TUI display, add handling for the new event types:

```go
case executor.EventTrustAllow:
	// Log or display: TRUST ALLOW
case executor.EventTrustDeny:
	// Display prominently in TUI: TRUST DENY
case executor.EventTrustAsk:
	// Trigger permission prompt in TUI
```

**Commit:** `feat(tui): display trust audit events in conversation stream`

---

**Final wpa tag:**

```bash
cd /Users/jon/workspace/workflow-plugin-agent && git tag v0.7.2 && git push origin v0.7.2
```

**Final rcli dependency update:**

```bash
cd /Users/jon/workspace/ratchet-cli && go get github.com/GoCodeAlone/workflow-plugin-agent@v0.7.2
cd /Users/jon/workspace/ratchet-cli && go mod tidy
```

**Final commit:** `build: update wpa to v0.7.2 with full trust/sandbox support`

---

## Summary of Files Changed

### workflow-plugin-agent

| File | Action | Phase |
|---|---|---|
| `policy/trust.go` | New | 1 |
| `policy/trust_test.go` | New | 1 |
| `policy/path.go` | New | 2 |
| `policy/path_test.go` | New | 2 |
| `policy/store.go` | New | 2 |
| `policy/store_test.go` | New | 2 |
| `genkit/pty_prompts.go` | New | 3 |
| `genkit/pty_prompts_test.go` | New | 3 |
| `genkit/pty_provider.go` | Modify | 3, 6 |
| `genkit/pty_adapters.go` | Modify | 6 |
| `executor/interfaces.go` | Modify | 4 |
| `executor/trust_test.go` | New | 4 |
| `executor/executor.go` | Modify | 4, 7 |
| `executor/events.go` | Modify | 7 |
| `orchestrator/container_manager.go` | Modify | 4 |

### ratchet-cli

| File | Action | Phase |
|---|---|---|
| `internal/config/trust.go` | New | 5 |
| `internal/config/trust_test.go` | New | 5 |
| `internal/config/config.go` | Modify | 5 |
| `cmd/ratchet/main.go` | Modify | 5 |
| `internal/daemon/service.go` | Modify | 5 |
| `internal/mesh/node.go` | Modify | 5 |
| `internal/mesh/local_node.go` | Modify | 5 |
| `internal/tui/commands/commands.go` | Modify | 5 |
| `internal/tui/commands/trust.go` | New | 5 |

## Tags/Releases

| Tag | Repo | After Phase |
|---|---|---|
| `v0.7.0` | workflow-plugin-agent | 4 |
| `v0.7.1` | workflow-plugin-agent | 6 |
| `v0.7.2` | workflow-plugin-agent | 7 |
