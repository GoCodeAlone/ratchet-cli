# Trust, Permission & Docker Sandbox System — Design

**Date:** 2026-04-08
**Repos:** workflow-plugin-agent (primary) + ratchet-cli (consumer)
**Goal:** Unified trust rules engine supporting both Claude Code and ratchet formats, PTY auto-prompt handling, Docker sandbox isolation, operating mode presets, dynamic mode switching, and per-agent overrides. Maximum functionality pushed to the agent plugin.

## Principle: Agent Plugin First

All reusable trust, policy, sandbox, and prompt-handling logic lives in `workflow-plugin-agent`. Ratchet-cli is a thin consumer that wires config and CLI flags to the plugin's APIs. This ensures any Go project using the agent plugin gets the same trust system without depending on ratchet.

## Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│                     workflow-plugin-agent                        │
│                                                                 │
│  ┌──────────────────┐  ┌──────────────────┐  ┌──────────────┐  │
│  │  policy/trust.go  │  │  policy/path.go   │  │ policy/      │  │
│  │  TrustEngine      │  │  GlobMatcher      │  │ store.go     │  │
│  │  - Parse both     │  │  - filepath.Match  │  │ SQLite       │  │
│  │    formats        │  │  - **, negation    │  │ persistence  │  │
│  │  - Evaluate rules │  │  - Replaces prefix │  │ for "always" │  │
│  │  - Mode presets   │  │    guard           │  │ grants       │  │
│  └────────┬─────────┘  └────────┬──────────┘  └──────┬───────┘  │
│           │                      │                     │          │
│  ┌────────▼──────────────────────▼─────────────────────▼───────┐ │
│  │              orchestrator/tool_policy.go                     │ │
│  │              ToolPolicyEngine (existing, extended)           │ │
│  │              - Tool policies (existing)                      │ │
│  │              - Path policies (new, via GlobMatcher)          │ │
│  │              - Command policies (new, bash pattern matching) │ │
│  │              - Resource policies (new, network/fs scope)     │ │
│  └─────────────────────────────────────────────────────────────┘ │
│                                                                  │
│  ┌──────────────────┐  ┌──────────────────────────────────────┐  │
│  │  genkit/          │  │  orchestrator/container_manager.go   │  │
│  │  pty_prompts.go   │  │  (existing Docker sandbox)           │  │
│  │  - Screen pattern │  │  - EnsureContainer                   │  │
│  │    matching       │  │  - ExecInContainer                   │  │
│  │  - Auto-respond   │  │  - Per-agent sandbox config          │  │
│  │  - Queue unknown  │  │  - Network/CPU/memory limits         │  │
│  │  - Notify via CB  │  │  - Graceful fallback when no Docker  │  │
│  └──────────────────┘  └──────────────────────────────────────┘  │
│                                                                  │
│  ┌──────────────────────────────────────────────────────────────┐ │
│  │  executor/executor.go (existing)                             │ │
│  │  - SandboxMode field on Config                               │ │
│  │  - Route tool calls through container when sandbox=true      │ │
│  │  - TrustEngine consulted before every tool execution         │ │
│  │  - Blocked actions → callback (queue for human / notify)     │ │
│  └──────────────────────────────────────────────────────────────┘ │
└──────────────────────────────────────────────────────────────────┘

┌──────────────────────────────────────────────────────────────────┐
│                          ratchet-cli                              │
│                                                                  │
│  - --mode flag (conservative/permissive/locked/sandbox/custom)   │
│  - ~/.ratchet/config.yaml trust section (ratchet format)         │
│  - .claude/settings.json auto-detection (Claude Code format)     │
│  - Per-agent args + trust overrides in team config               │
│  - /mode and /trust TUI commands for dynamic switching           │
│  - Wires config → agent plugin TrustEngine + ContainerManager    │
└──────────────────────────────────────────────────────────────────┘
```

## 1. Trust Rules Engine (`policy/trust.go` in wpa)

### Unified Rule Type

```go
type TrustRule struct {
    Pattern string // "bash:git *", "path:~/.ssh/*", "Edit", "Bash(rm:*)"
    Action  Action // Allow, Deny, Ask
    Scope   string // "global", "provider:claude_code", "agent:coder"
}

type Action string
const (
    Allow Action = "allow"
    Deny  Action = "deny"
    Ask   Action = "ask"   // queue for human approval
)
```

### Both Formats Supported

**Claude Code format** (auto-detected from `.claude/settings.json`):
```json
{
  "allowedTools": ["Edit", "Read", "Bash(git:*)", "Bash(go:*)"],
  "disallowedTools": ["Bash(rm -rf:*)", "Bash(sudo:*)"]
}
```

Parsed into TrustRules: `allowedTools` → `Action: Allow`, `disallowedTools` → `Action: Deny`.

**Ratchet format** (`~/.ratchet/config.yaml`):
```yaml
trust:
  mode: conservative
  rules:
    - pattern: "file_read"
      action: allow
    - pattern: "bash:git *"
      action: allow
    - pattern: "bash:rm -rf *"
      action: deny
    - pattern: "bash:docker *"
      action: ask
    - pattern: "path:/tmp/*"
      action: allow
    - pattern: "path:~/.ssh/*"
      action: deny
  provider_args:
    claude_code: ["--permission-mode", "acceptEdits"]
    copilot_cli: ["--allow-tool=Edit", "--allow-tool=Write"]
```

### Mode Presets

```go
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
        // Same as permissive but all execution routed through Docker
        {Pattern: "*", Action: Allow},
    },
}
```

### Resolution Order

```
per-agent rules (team config)
  → per-provider rules (config provider_args)
    → project rules (.claude/settings.json or .ratchet/config.yaml)
      → global rules (~/.ratchet/config.yaml)
        → mode preset
          → ToolPolicyEngine (SQL, existing)
            → deny (default)
```

Deny wins at any level.

### TrustEngine API

```go
type TrustEngine struct {
    rules       []TrustRule
    policyDB    *ToolPolicyEngine  // existing SQL engine
    permStore   *PermissionStore   // persisted "always" grants
    mode        string
}

func NewTrustEngine(mode string, rules []TrustRule, policyDB *ToolPolicyEngine) *TrustEngine

// Evaluate checks whether a tool call is allowed.
// Returns Allow, Deny, or Ask.
func (te *TrustEngine) Evaluate(ctx context.Context, toolName string, args map[string]any) Action

// EvaluatePath checks whether a file path is accessible.
func (te *TrustEngine) EvaluatePath(path string) Action

// EvaluateCommand checks whether a bash command is allowed.
func (te *TrustEngine) EvaluateCommand(cmd string) Action

// GrantPersistent stores an "always allow" decision for future sessions.
func (te *TrustEngine) GrantPersistent(pattern string, action Action)

// SetMode switches the active mode preset. Returns rules that changed.
func (te *TrustEngine) SetMode(mode string) []TrustRule
```

## 2. Path Policy with Glob (`policy/path.go` in wpa)

Replaces the prefix-only `pathGuardTool` in ratchet's mesh:

```go
// GlobMatcher evaluates file paths against allow/deny glob patterns.
type GlobMatcher struct {
    allow []string // e.g., "/Users/*/workspace/**", "/tmp/**"
    deny  []string // e.g., "~/.ssh/**", "~/.aws/**", "/etc/**"
}

func NewGlobMatcher(allow, deny []string) *GlobMatcher

// Check returns Allow, Deny, or the default (Ask) for the given path.
// Deny patterns are checked first (deny wins).
func (gm *GlobMatcher) Check(path string) Action
```

Uses `doublestar.Match` for `**` glob support (or `filepath.Match` for simple globs).

## 3. Permission Persistence (`policy/store.go` in wpa)

```sql
CREATE TABLE permission_grants (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    pattern TEXT NOT NULL,       -- "bash:git *", "path:/tmp/*"
    action TEXT NOT NULL,        -- "allow", "deny"
    scope TEXT NOT NULL,         -- "global", "session:xxx", "agent:coder"
    granted_by TEXT NOT NULL,    -- "user", "autoresponder", "config"
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
```

When user selects "always allow" in TUI permission prompt → stored here. Checked before TrustEngine rules (persisted grants override config for the matching scope).

## 4. PTY Auto-Prompt Handler (`genkit/pty_prompts.go` in wpa)

```go
type PromptHandler struct {
    trust     *TrustEngine
    patterns  []PromptPattern
    onQueue   func(agentName, promptText string) // callback when queued for human
}

type PromptPattern struct {
    Name    string         // "trust_dialog", "command_exec", "file_write"
    Match   *regexp.Regexp // screen content match
    Extract func(screen string) string // extract the action/path being requested
    Default Action         // what to do if no trust rule matches
}
```

Built-in patterns:
- Trust dialogs: `(?i)trust this folder|safety check` → auto-approve
- Command execution: `(?i)run command|execute.*\?|y/n` → evaluate via TrustEngine
- File write: `(?i)allow edit|write file|create file` → evaluate via TrustEngine
- Permission prompt: `(?i)allow|deny|approve` → evaluate via TrustEngine

User-definable patterns in config:
```yaml
trust:
  prompts:
    - name: "docker_run"
      match: "docker run"
      action: ask
    - name: "npm_install"
      match: "npm install"
      action: allow
```

When action is `Ask` and no human attached:
1. Queue in HumanGate
2. Send OS notification
3. Write to Blackboard: `notifications/<team> = "agent X waiting for human approval: <prompt>"`
4. Orchestrator sees notification, reassigns work

## 5. Docker Sandbox (`orchestrator/container_manager.go` — extend existing)

Existing ContainerManager already handles:
- Container creation with bind-mounted workspace
- ExecInContainer for running commands
- CPU/memory/network limits
- SQLite persistence

**New: per-agent sandbox config** (in team config or project config):
```yaml
agents:
  - name: coder
    provider: claude_code
    sandbox:
      enabled: true
      image: "golang:1.26-bookworm"
      network: none           # none, bridge, host
      memory: 2g
      cpu: 2.0
      mounts:
        - src: /Users/jon/workspace/ratchet-cli
          dst: /workspace
          readonly: false
      init: ["go mod download"]
```

**New: SandboxMode on executor.Config** (in wpa):
```go
type Config struct {
    // ... existing fields ...
    SandboxMode    bool
    ContainerMgr   *ContainerManager
    SandboxSpec    *WorkspaceSpec
}
```

When `SandboxMode == true`, the executor routes `bash` and file tool calls through `ContainerManager.ExecInContainer` instead of direct OS execution. File reads/writes use the bind-mounted workspace path.

**Fallback**: When Docker is unavailable and sandbox mode is requested, the engine:
1. Logs a warning
2. Falls back to path-guard + trust rules enforcement (no container isolation)
3. Emits event: `"sandbox_fallback": "Docker unavailable, using path-guard mode"`

## 6. Operating Modes + Dynamic Switching

### CLI Flags

```bash
ratchet --mode conservative        # default
ratchet --mode permissive
ratchet --mode locked
ratchet --mode sandbox             # Docker isolation + permissive rules inside container
ratchet --mode custom              # load explicit rules from config
```

### Dynamic Switching in TUI

```
/mode permissive              # switch mode
/trust allow "bash:go test *" # add rule dynamically
/trust deny "bash:rm *"       # add deny rule
/trust list                   # show active rules
/trust reset                  # revert to config defaults
```

For PTY providers, mode switch requires session restart:
1. Save conversation state to Blackboard
2. Kill PTY session
3. Rebuild InteractiveArgs from new mode/rules
4. Restart PTY with new args
5. Restore context via prompt injection from Blackboard

### Per-Agent Args (user-defined, not hardcoded)

```yaml
# ~/.ratchet/config.yaml
trust:
  provider_args:
    claude_code: ["--permission-mode", "acceptEdits"]
    copilot_cli: ["--allow-tool=Edit", "--allow-tool=Write", "--allow-tool=Bash"]

# Per-agent override in team config
agents:
  - name: coder
    provider: claude_code
    args: ["--permission-mode", "bypassPermissions"]  # overrides global
```

Resolution: per-agent args → global provider_args → adapter defaults (empty).

`InteractiveArgs()` on adapters reads from config instead of returning hardcoded values.

## 7. Integration Points

### Executor Integration (wpa)

```go
// In executor.Execute, before each tool call:
action := cfg.TrustEngine.Evaluate(ctx, toolCall.Name, toolCall.Args)
switch action {
case policy.Allow:
    // proceed
case policy.Deny:
    // return error to agent
case policy.Ask:
    // block on cfg.Approver.WaitForResolution()
    // if no approver, return error
}
```

### Mesh Integration (ratchet)

LocalNode passes TrustEngine to executor.Config. Docker sandbox config flows through NodeConfig → executor.Config.

### Audit Trail

Every trust decision logged to transcript:
```
[00:05.2] TRUST ALLOW bash:git status by coder (rule: bash:git *)
[00:08.1] TRUST ASK file_write:/workspace/main.go by coder (rule: file_write → ask)
[00:08.1] HUMAN QUEUE coder: "Allow write to /workspace/main.go?"
[00:12.3] HUMAN APPROVE file_write:/workspace/main.go by user (scope: session)
[00:15.0] TRUST DENY bash:rm -rf / by coder (rule: bash:rm -rf * → deny)
```

## Files — All Changes

### workflow-plugin-agent (primary)

| File | Action | Description |
|---|---|---|
| `policy/trust.go` | **New** | TrustEngine, TrustRule, mode presets, both format parsers |
| `policy/trust_test.go` | **New** | Parsing, matching, mode switching, resolution order |
| `policy/path.go` | **New** | GlobMatcher replacing prefix-only guard |
| `policy/path_test.go` | **New** | Glob matching tests |
| `policy/store.go` | **New** | SQLite permission persistence |
| `policy/store_test.go` | **New** | Persistence tests |
| `genkit/pty_prompts.go` | **New** | Screen pattern matching, auto-response, queue |
| `genkit/pty_prompts_test.go` | **New** | Prompt handler tests |
| `genkit/pty_adapters.go` | **Modify** | InteractiveArgs from config, not hardcoded |
| `genkit/pty_provider.go` | **Modify** | Wire PromptHandler into readResponse loop |
| `orchestrator/tool_policy.go` | **Modify** | Integrate TrustRule, path/command policies |
| `orchestrator/container_manager.go` | **Modify** | Per-agent sandbox spec |
| `executor/executor.go` | **Modify** | SandboxMode, TrustEngine on Config, route through container |
| `executor/interfaces.go` | **Modify** | BlockedAction callback for queue-for-human |

### ratchet-cli (consumer)

| File | Action | Description |
|---|---|---|
| `internal/config/trust.go` | **New** | Parse ~/.ratchet/config.yaml trust section |
| `internal/config/trust_test.go` | **New** | Config parsing tests |
| `cmd/ratchet/main.go` | **Modify** | --mode flag |
| `internal/daemon/service.go` | **Modify** | Wire TrustEngine + ContainerManager |
| `internal/mesh/local_node.go` | **Modify** | Pass TrustEngine + sandbox to executor |
| `internal/tui/` | **Modify** | /mode, /trust commands |

## Implementation Phases

```
Phase 1: TrustEngine + both format parsers + mode presets (wpa)
Phase 2: GlobMatcher path policy + permission persistence (wpa)
Phase 3: PTY auto-prompt handler (wpa)
Phase 4: Docker sandbox integration on executor (wpa)
Phase 5: Ratchet config wiring + --mode flag + /trust commands (ratchet)
Phase 6: Per-agent args from config + dynamic mode switching (both)
Phase 7: Audit trail in transcript (both)
```
