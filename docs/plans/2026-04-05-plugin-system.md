# Plugin System — Implementation Plan

**Goal:** Implement an extensible plugin system for ratchet-cli that supports skills, agents, commands, hooks, tools (exec + daemon protocols), and MCP servers — compatible with Claude plugin format.

**Architecture:** Plugins are directories with a `plugin.json` manifest declaring capabilities. `plugins.Loader` scans `~/.ratchet/plugins/*/`, parses manifests, and registers each capability type with the appropriate subsystem. Tool binaries use either exec-per-call or long-running JSON-RPC daemon protocol. Install supports GitHub releases and local directories.

**Tech Stack:** Go 1.26, `gh` CLI for GitHub release downloads, JSON-RPC 2.0 for daemon tools

---

## Task 1: Implement plugin manifest parsing

**Files:**
- Create: `internal/plugins/manifest.go`
- Create: `internal/plugins/manifest_test.go`

Manifest struct matching the design:
```go
type Manifest struct {
    Name         string       `json:"name"`
    Version      string       `json:"version"`
    Description  string       `json:"description"`
    Author       Author       `json:"author"`
    Capabilities Capabilities `json:"capabilities"`
}
type Author struct {
    Name  string `json:"name"`
    Email string `json:"email,omitempty"`
}
type Capabilities struct {
    Skills   string `json:"skills,omitempty"`   // relative dir path
    Agents   string `json:"agents,omitempty"`
    Commands string `json:"commands,omitempty"`
    Tools    string `json:"tools,omitempty"`
    Hooks    string `json:"hooks,omitempty"`    // relative file path
    MCP      string `json:"mcp,omitempty"`      // relative file path
}
```

`LoadManifest(pluginDir string) (*Manifest, error)` — looks for `.ratchet-plugin/plugin.json`, falls back to `.claude-plugin/plugin.json`.

Tests: valid manifest, missing manifest, claude-plugin fallback, partial capabilities.

Commit: `feat: add plugin manifest parsing with Claude fallback`

---

## Task 2: Implement plugin registry

**Files:**
- Create: `internal/plugins/registry.go`
- Create: `internal/plugins/registry_test.go`

Registry tracks installed plugins in `~/.ratchet/plugins/registry.json`:
```go
type Registry struct {
    Plugins map[string]RegistryEntry `json:"plugins"`
}
type RegistryEntry struct {
    Source      string    `json:"source"`      // "github:org/repo" or "local:/path"
    Version     string    `json:"version"`
    InstalledAt time.Time `json:"installed_at"`
    Path        string    `json:"path"`
}
```

Methods: `Load() (*Registry, error)`, `Save() error`, `Add(name string, entry RegistryEntry) error`, `Remove(name string) error`, `Get(name string) (RegistryEntry, bool)`.

Tests: load/save round-trip, add/remove, get missing.

Commit: `feat: add plugin registry for tracking installed plugins`

---

## Task 3: Implement GitHub + local install

**Files:**
- Create: `internal/plugins/installer.go`
- Create: `internal/plugins/installer_test.go`

`InstallFromGitHub(ctx context.Context, repo string) error`:
1. Parse `repo` as `owner/name`
2. Use `exec.Command("gh", "release", "download", "--repo", repo, "--pattern", "*.tar.gz", "--dir", tmpDir)` to download latest release
3. Extract tarball to `~/.ratchet/plugins/<name>/`
4. Verify manifest exists
5. Add to registry with `source: "github:<repo>"`

`InstallFromLocal(src string) error`:
1. Verify manifest exists at `src`
2. Copy directory to `~/.ratchet/plugins/<name>/` (or symlink for development)
3. Add to registry with `source: "local:<src>"`

`Uninstall(name string) error`:
1. Remove directory `~/.ratchet/plugins/<name>/`
2. Remove from registry

Tests: local install with temp dir + manifest, uninstall, invalid manifest errors.

Commit: `feat: implement plugin install from GitHub releases and local directories`

---

## Task 4: Implement exec tool protocol

**Files:**
- Create: `internal/plugins/tool_exec.go`
- Create: `internal/plugins/tool_exec_test.go`

Tool definition file (`tools/<name>/tool.json`):
```go
type ToolDef struct {
    Name        string         `json:"name"`
    Description string         `json:"description"`
    Parameters  map[string]any `json:"parameters"`
    Protocol    string         `json:"protocol"` // "exec" or "daemon"
}
```

`ExecTool` implements the tool registry's `Tool` interface:
```go
type ExecTool struct {
    def      ToolDef
    binPath  string
}
func (t *ExecTool) Name() string
func (t *ExecTool) Definition() provider.ToolDef
func (t *ExecTool) Execute(ctx context.Context, args map[string]any) (any, error)
```

`Execute`: marshals args to JSON, runs the binary via `exec.CommandContext`, writes JSON to stdin, reads JSON from stdout, returns parsed result.

Tests: mock binary (shell script that echoes JSON), verify round-trip.

Commit: `feat: implement exec-per-call tool protocol`

---

## Task 5: Implement daemon tool protocol

**Files:**
- Create: `internal/plugins/tool_daemon.go`
- Create: `internal/plugins/tool_daemon_test.go`

`DaemonTool` manages a long-running process:
```go
type DaemonTool struct {
    defs    []ToolDef   // tools declared by the daemon
    binPath string
    cmd     *exec.Cmd
    stdin   io.WriteCloser
    stdout  *bufio.Reader
    mu      sync.Mutex
    nextID  int
}
```

`Start(ctx)`: starts the binary, sends `{"jsonrpc":"2.0","id":1,"method":"initialize"}`, reads response to get declared tools.

`Call(ctx, name, args) (any, error)`: sends JSON-RPC call, reads response. Mutex serializes calls.

`Stop()`: sends stdin EOF, waits for process to exit with timeout, then kills.

Each tool declared by the daemon gets a wrapper `DaemonToolRef` that implements the tool interface and delegates to `DaemonTool.Call`.

Tests: mock daemon binary (Go test helper), initialize → call → stop lifecycle.

Commit: `feat: implement daemon (long-running) tool protocol`

---

## Task 6: Rewrite plugin loader to use manifests

**Files:**
- Modify: `internal/plugins/loader.go`

Rewrite `LoadAll()` to:
1. Scan `~/.ratchet/plugins/*/` for directories (not bare executables)
2. For each directory, call `LoadManifest(dir)`
3. Based on capabilities:
   - Skills: read `skills/*/SKILL.md` files, return as `[]skills.Skill`
   - Agents: read `agents/*.yaml` files, return as `[]agent.AgentDefinition`
   - Commands: read `commands/*.md` files, return as `[]Command`
   - Hooks: read hooks JSON, return as `*hooks.HookConfig`
   - Tools: scan `tools/*/tool.json`, create `ExecTool` or `DaemonTool` based on protocol
   - MCP: read `.mcp.json`, return config for MCP server startup

New `LoadResult` struct aggregates all discovered capabilities:
```go
type LoadResult struct {
    Skills   []skills.Skill
    Agents   []agent.AgentDefinition
    Hooks    *hooks.HookConfig
    Tools    []ToolProvider  // interface with Name/Definition/Execute
    MCPConfigs []MCPConfig
}
```

Commit: `feat: rewrite plugin loader to discover all capability types from manifests`

---

## Task 7: Wire plugin loading into daemon and rewrite CLI commands

**Files:**
- Modify: `internal/daemon/engine.go`
- Modify: `cmd/ratchet/cmd_plugin.go`

**engine.go**: After existing plugin loading, call the new `Loader.LoadAll()`. Register discovered tools with `ToolRegistry`. Merge discovered hooks with `engine.Hooks`. Start any daemon tools. Store skills/agents/commands for later query.

**cmd_plugin.go**: Rewrite to use new installer:
- `install`: detect GitHub (`owner/repo` format) vs local (`./path` or `/path`), call appropriate installer
- `list`: load registry, print table (NAME, VERSION, SOURCE)
- `remove`: call `Uninstall`

Commit: `feat: wire plugin loading into daemon startup and rewrite CLI commands`

---

## Task 8: Write integration tests and verify

**Files:**
- Create: `internal/plugins/integration_test.go`

Full integration test:
1. Create a temp plugin directory with manifest + skill + tool.json + mock tool binary
2. Call `LoadAll()`, verify all capabilities discovered
3. Execute a tool, verify result
4. Install from local, verify registry updated
5. Remove, verify cleaned up

Run full suite: `go test -race ./... -count=1`
Run linter: `golangci-lint run`

Commit: `test: add plugin system integration tests`

---

## Execution Order

```
Task 1 (manifest) → Task 2 (registry) → Task 3 (installer) → Task 4 (exec tool)
                                                              → Task 5 (daemon tool)
                  → Task 6 (loader rewrite) → Task 7 (wiring) → Task 8 (tests)
```

Tasks 1-3 are sequential. Tasks 4-5 can parallelize. Task 6 depends on 1, 4, 5. Task 7 depends on 6. Task 8 verifies everything.
