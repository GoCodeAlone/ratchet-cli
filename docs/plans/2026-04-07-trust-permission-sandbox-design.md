# Trust, Permission & Sandbox System — Design (Draft)

**Date:** 2026-04-07
**Repos:** ratchet-cli + workflow-plugin-agent
**Status:** Draft — needs refinement before implementation

## Problem

PTY agents (Claude Code, Copilot) require permission configuration to operate in orchestration mode. Currently, adapter args are hardcoded. Users need control over what agents can do, with sensible defaults and the ability to customize per-provider, per-agent, and per-project.

## Existing Infrastructure (Reusable)

| Component | Location | Status |
|---|---|---|
| ToolPolicyEngine (SQL, deny-wins, groups) | wpa/orchestrator/tool_policy.go | Production |
| Registry.Execute gating | wpa/tools/types.go | Production |
| ApprovalGate (channel blocking) | ratchet/daemon/approval_gate.go | Production |
| Context propagation (AgentID/TeamID) | wpa/tools/context.go | Production |
| Mesh tool allowlist | ratchet/mesh/local_node.go | Beta |
| Path guard (prefix) | ratchet/mesh/local_node.go | Beta |
| Config permissions (auto-allow/always-ask) | ratchet/config/config.go | Defined, not wired |
| Hooks system | ratchet/hooks/hooks.go | Production |

## Design

### 1. Ratchet Operating Mode (CLI flag + config)

```bash
# CLI flag sets the mode for the session
ratchet --mode conservative        # default
ratchet --mode permissive          # auto-approve most actions
ratchet --mode locked              # deny all, queue everything for human
ratchet --mode custom              # load rules from config

# Switch dynamically in TUI
/mode permissive
/mode conservative
```

Mode maps to a set of trust rules. Modes are presets — `custom` loads explicit rules from config.

### 2. Trust Rules Config (`~/.ratchet/config.yaml`)

```yaml
trust:
  mode: conservative    # default mode

  # Rules: pattern → action
  # Patterns: exact, glob, regex (prefixed with ~)
  # Actions: allow, deny, ask (queue for human)
  rules:
    # Tool-level rules
    - pattern: "file_read"
      action: allow
    - pattern: "file_write"
      action: ask
    - pattern: "bash"
      action: ask
    - pattern: "bash:git *"         # glob on command
      action: allow
    - pattern: "bash:rm -rf *"
      action: deny
    - pattern: "bash:docker *"
      action: ask

    # Path-level rules
    - pattern: "path:/tmp/*"
      action: allow
    - pattern: "path:/Users/*/workspace/*"
      action: allow
    - pattern: "path:/etc/*"
      action: deny
    - pattern: "path:~/.ssh/*"
      action: deny

    # Provider-level PTY args
    - pattern: "pty:claude_code"
      args: ["--permission-mode", "acceptEdits"]
    - pattern: "pty:copilot_cli"
      args: ["--allow-tool=Edit", "--allow-tool=Write"]

  # Preset modes (expand to rule sets)
  modes:
    conservative:
      auto_approve: [trust_dialogs, command_execution]
      require_human: [file_write, file_delete, destructive]
      deny: [network_unrestricted]
    permissive:
      auto_approve: [trust_dialogs, command_execution, file_write, file_read]
      require_human: [file_delete, destructive]
      deny: []
    locked:
      auto_approve: [trust_dialogs]
      require_human: [everything_else]
      deny: []
```

### 3. Per-Agent Override in Team Config

```yaml
teams:
  - name: dev
    agents:
      - name: coder
        provider: claude_code
        args: ["--permission-mode", "bypassPermissions"]  # override global
        trust:
          rules:
            - pattern: "bash:*"
              action: allow          # this agent can run any command
      - name: reviewer
        provider: copilot_cli
        # inherits global trust rules + provider args
```

### 4. Resolution Order

```
per-agent trust rules
  → per-agent args
    → global provider_args (pty:provider_name)
      → ratchet trust rules
        → ratchet mode preset
          → ToolPolicyEngine (SQL, existing)
            → deny (default)
```

First match wins. Deny always wins over allow at the same level.

### 5. PTY Auto-Prompt Handler

Screen pattern matching in the PTY session, driven by trust rules:

```go
type PromptPattern struct {
    Pattern string          // regex or substring match on screen content
    Action  string          // "approve", "deny", "queue"
    Keys    string          // keystrokes to send (e.g., "y\r", "\r")
}
```

Built-in patterns:
- Trust dialogs: "Trust this folder" → approve (Enter)
- Command execution: "Run command?" / "Execute?" → check trust rules
- File write: "Allow edit?" / "Write file?" → check trust rules
- Unknown (screen stuck >30s) → queue for human

When queued:
1. Write to HumanGate pending queue
2. Send OS notification
3. Write to BB: `notifications/<team> = "agent X waiting for human approval"`
4. Orchestrator sees the notification, knows to work on other tasks

### 6. Push to Agent Plugin

Move these to workflow-plugin-agent so all consumers benefit:

| Component | Current Location | Target |
|---|---|---|
| Trust rule parsing/matching | New | wpa/policy/trust.go |
| Path policy (glob, not just prefix) | ratchet/mesh | wpa/policy/path.go |
| PTY prompt patterns | New | wpa/genkit/pty_prompts.go |
| Permission persistence | Missing | wpa/policy/store.go (SQLite) |

The agent plugin's ToolPolicyEngine extends to cover:
- Tool policies (existing)
- Path policies (new — glob matching)
- Command policies (new — bash command patterns)
- Resource policies (new — network, filesystem scope)

### 7. Sandbox Mode

```bash
ratchet --sandbox    # or --mode sandbox
```

Sandbox mode:
- All file operations restricted to project workdir
- Network access denied by default
- No access to ~/.ssh, ~/.aws, credentials
- Bash commands run in a restricted shell (no sudo, no rm -rf /)
- Future: WASM isolation for MCP servers (workflow-plugin-sandbox repo)

### 8. Dynamic Mode Switching

```
/mode permissive     # switch mode in TUI
/trust allow bash    # add rule dynamically
/trust deny "rm -rf" # add deny rule
/trust list          # show active rules
```

For PTY providers, mode switch requires session restart:
1. Save conversation state to Blackboard
2. Kill PTY session
3. Restart with new args
4. Restore context via prompt injection

## Implementation Phases

Phase 1: Wire existing config permissions + persist "always" grants
Phase 2: Trust rules config + mode presets + CLI flag
Phase 3: PTY auto-prompt handler with pattern matching
Phase 4: Push trust/path policy to agent plugin
Phase 5: Per-agent overrides + dynamic mode switching
Phase 6: Sandbox mode (restricted shell, path deny)
Phase 7: WASM sandbox plugin (future, separate repo)
