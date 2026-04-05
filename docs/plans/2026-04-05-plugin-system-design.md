# Ratchet Plugin System — Design

**Date:** 2026-04-05
**Repo:** ratchet-cli
**Goal:** Extensible plugin system that lets users add skills, agents, commands, hooks, tools, and MCP servers to ratchet without forking. Compatible with Claude plugin format.

## Plugin Structure

```
my-plugin/
├── .ratchet-plugin/
│   └── plugin.json           # manifest (falls back to .claude-plugin/)
├── skills/                    # SKILL.md files (AI instructions)
│   └── my-skill/SKILL.md
├── agents/                    # Agent definitions (YAML)
│   └── code-reviewer.yaml
├── commands/                  # Slash commands (.md files)
│   └── deploy.md
├── hooks/hooks.json           # Lifecycle hooks
├── tools/                     # Executable tool binaries
│   └── my-tool               # Binary implementing tool protocol
├── .mcp.json                  # MCP server config (Claude-compatible)
└── README.md
```

## Manifest (`plugin.json`)

Located at `.ratchet-plugin/plugin.json`. Falls back to `.claude-plugin/plugin.json` for Claude plugin compatibility. Ratchet plugins are a superset — Claude plugins work as-is.

```json
{
  "name": "my-plugin",
  "version": "1.0.0",
  "description": "Adds deployment tools and agents",
  "author": {"name": "GoCodeAlone"},
  "capabilities": {
    "skills": "./skills/",
    "agents": "./agents/",
    "commands": "./commands/",
    "tools": "./tools/",
    "hooks": "./hooks/hooks.json",
    "mcp": "./.mcp.json"
  }
}
```

## Tool Binary Protocol

Tools declare their protocol in a `tool.json` alongside the binary:

```json
{
  "name": "my-tool",
  "description": "Does something useful",
  "parameters": {"type": "object", "properties": {}},
  "protocol": "exec"
}
```

### `protocol: "exec"` — One exec per tool call

Ratchet runs the binary with JSON on stdin, reads JSON response from stdout. Stateless.

```
echo '{"name":"my-tool","arguments":{"key":"val"}}' | ./tools/my-tool
→ stdout: {"result": "done", "output": {...}}
```

### `protocol: "daemon"` — Long-running process

Ratchet starts the binary once, sends JSON-RPC 2.0 requests over stdin. The binary stays alive between calls.

```
→ stdin:  {"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}
← stdout: {"jsonrpc":"2.0","id":1,"result":{"protocol":"daemon","tools":[...]}}
→ stdin:  {"jsonrpc":"2.0","id":2,"method":"call","params":{"name":"my-tool","arguments":{...}}}
← stdout: {"jsonrpc":"2.0","id":2,"result":{"output":{...}}}
```

## CLI Commands

```
ratchet plugin install GoCodeAlone/my-plugin          # GitHub release download
ratchet plugin install ./path/to/local/plugin          # Local directory (copy/symlink)
ratchet plugin list                                     # List installed
ratchet plugin remove <name>                           # Uninstall
```

**GitHub install**: Downloads latest release tarball from GitHub repo, extracts to `~/.ratchet/plugins/<name>/`.

**Local install**: Copies or symlinks the directory to `~/.ratchet/plugins/<name>/`.

## Plugin Loading (Daemon Startup)

On startup, `plugins.Loader.LoadAll()` scans `~/.ratchet/plugins/*/`:
1. Read manifest (`plugin.json`)
2. Register skills → skill registry
3. Register agents → agent definitions
4. Register commands → command registry
5. Register hooks → hooks config (merged with user hooks)
6. Register tools → tool registry (exec or daemon protocol)
7. Start MCP servers → tool registry (via existing MCP infrastructure)

## Plugin Registry

`~/.ratchet/plugins/registry.json` tracks installed plugins:
```json
{
  "plugins": {
    "my-plugin": {
      "source": "github:GoCodeAlone/my-plugin",
      "version": "1.0.0",
      "installed_at": "2026-04-05T...",
      "path": "~/.ratchet/plugins/my-plugin"
    }
  }
}
```

## Files Changed

| File | Change |
|---|---|
| `internal/plugins/manifest.go` | NEW — manifest parsing, capability discovery |
| `internal/plugins/installer.go` | NEW — GitHub + local install logic |
| `internal/plugins/registry.go` | NEW — installed plugins tracking |
| `internal/plugins/tool_exec.go` | NEW — exec-per-call tool protocol |
| `internal/plugins/tool_daemon.go` | NEW — long-running daemon tool protocol |
| `internal/plugins/loader.go` | Rewrite — load all capabilities from manifest |
| `cmd/ratchet/cmd_plugin.go` | Rewrite — install/list/remove commands |
| `internal/daemon/engine.go` | Wire plugin loading into daemon startup |

## Future Work (Revisit Later)

These items are explicitly deferred from v1 and should be revisited:

- **Plugin marketplace/discovery** — browse and search available plugins from a registry (similar to Claude's `/plugin > Discover`)
- **Plugin update checking** — notify users when installed plugins have newer versions; `ratchet plugin update <name>`
- **Plugin signing/verification** — cryptographic verification of plugin authenticity to prevent supply chain attacks
- **Provider plugins** — extend ratchet with custom LLM providers via plugin (beyond the built-in Genkit adapters)
- **HuggingFace auth for plugins** — gated model downloads require tokens
- **Plugin dependencies** — plugins that depend on other plugins
- **Plugin configuration** — per-plugin settings in `~/.ratchet/config.yaml`
- **TUI plugin browser** — browse/install/remove plugins from the TUI
