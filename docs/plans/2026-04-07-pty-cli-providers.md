# PTY CLI Providers — Implementation Plan

**Goal:** Implement provider.Provider backends that drive AI CLI tools (Claude Code, Copilot, Codex, Gemini, Cursor) via pseudo-terminal, enabling ratchet to orchestrate across providers using existing subscriptions.

**Architecture:** A `ptyProvider` struct implements `provider.Provider` by running CLI tools with their `-p` flag (non-interactive) for Chat() and via PTY for Stream(). Per-tool `CLIAdapter` implementations handle differences in prompt format, flags, and response parsing. The ProviderRegistry gets new factory functions for each CLI type.

**Tech Stack:** Go 1.26, `creack/pty`, workflow-plugin-agent genkit package

---

## Task 1: Core PTY provider + CLIAdapter interface

**Files:**
- Create: `workflow-plugin-agent/genkit/pty_provider.go`
- Create: `workflow-plugin-agent/genkit/pty_provider_test.go`

Implement:

```go
// CLIAdapter defines per-tool behavior for driving a CLI via PTY.
type CLIAdapter interface {
    Name() string                            // provider type name
    Binary() string                          // binary name (e.g. "claude")
    NonInteractiveArgs(msg string) []string  // args for single-shot mode
    HealthCheckArgs() []string               // args for a quick health check
    DetectPrompt(output string) bool         // is the CLI ready for input?
    DetectResponseEnd(output string) bool    // has the response finished?
    ParseResponse(raw string) string         // clean raw output into response text
}

// ptyProvider implements provider.Provider by driving a CLI tool.
type ptyProvider struct {
    adapter   CLIAdapter
    binPath   string
    workDir   string
    authInfo  provider.AuthModeInfo
    timeout   time.Duration

    // PTY session state (for interactive/streaming mode)
    mu       sync.Mutex
    ptmx     *os.File     // PTY master — nil when no active session
    cmd      *exec.Cmd    // running CLI process
    output   bytes.Buffer // accumulated output
}
```

**Chat()**: Run `exec.CommandContext(ctx, binPath, adapter.NonInteractiveArgs(msg)...)`, capture stdout, return `provider.Response{Content: adapter.ParseResponse(stdout)}`. Stateless, no PTY.

**Stream()**: Full PTY interaction:
1. If no active PTY session, start one: `pty.StartWithSize(exec.Command(binPath), &pty.Winsize{Rows:40, Cols:120})`
2. Wait for `adapter.DetectPrompt(output)` to return true (CLI is ready)
3. Write `message + "\n"` to PTY stdin
4. Read output in a goroutine, emit `StreamEvent{Type: "text", Text: chunk}` per read
5. When `adapter.DetectResponseEnd(output)` returns true, emit `StreamEvent{Type: "done"}`
6. Keep the PTY session alive for multi-turn (reuse on next Stream() call)
7. Tool approval prompts from the CLI are passed through as text events (visible to user)

**Close()**: Kill PTY process and clean up on provider removal or daemon shutdown.

**Test**: Create a mock CLI binary (Go test binary that simulates prompt → response → prompt) and test Chat()/Stream()/multi-turn round-trip.

Commit: `feat: add PTY CLI provider core + CLIAdapter interface`

---

## Task 2: CLI adapters for all five tools

**Files:**
- Create: `workflow-plugin-agent/genkit/pty_adapters.go`
- Create: `workflow-plugin-agent/genkit/pty_adapters_test.go`

Implement CLIAdapter for each tool:

**ClaudeCodeAdapter:**
- Binary: `claude`
- NonInteractiveArgs: `["-p", msg, "--output-format", "text"]`
- HealthCheckArgs: `["-p", "say ok", "--output-format", "text"]`
- DetectPrompt: look for `❯` or `>` at line start
- DetectResponseEnd: look for prompt reappearing after response content

**CopilotCLIAdapter:**
- Binary: `copilot`
- NonInteractiveArgs: `["-p", msg]`
- HealthCheckArgs: `["-p", "say ok"]`
- DetectPrompt: look for `>` at line start
- DetectResponseEnd: prompt reappears

**CodexCLIAdapter:**
- Binary: `codex`
- NonInteractiveArgs: `["exec", msg]`
- HealthCheckArgs: `["exec", "say ok"]`
- DetectPrompt: look for composer input area
- DetectResponseEnd: prompt reappears

**GeminiCLIAdapter:**
- Binary: `gemini`
- NonInteractiveArgs: `["-p", msg]`
- HealthCheckArgs: `["-p", "say ok"]`
- DetectPrompt: look for `❯` or `>` at line start
- DetectResponseEnd: prompt reappears

**CursorCLIAdapter:**
- Binary: `agent`
- NonInteractiveArgs: `["-p", msg]`
- HealthCheckArgs: `["-p", "say ok"]`
- DetectPrompt: look for `>` at line start
- DetectResponseEnd: prompt reappears

**ParseResponse**: Strip ANSI codes, trim whitespace, remove CLI-specific boilerplate (spinner text, status lines).

**NOTE**: DetectPrompt/DetectResponseEnd patterns will need calibration per-tool by actually running each CLI interactively. The patterns above are initial estimates — the integration tests (Task 6) will validate and refine them.

**Test**: Verify each adapter produces correct args, parse strips ANSI, prompt detection works on sample output.

Commit: `feat: add CLI adapters for Claude Code, Copilot, Codex, Gemini, Cursor`

---

## Task 3: Factory functions + ProviderRegistry integration

**Files:**
- Modify: `workflow-plugin-agent/genkit/providers.go`
- Modify: `workflow-plugin-agent/orchestrator/provider_registry.go`
- Modify: `workflow-plugin-agent/provider_registry.go`

Add factory functions:
```go
func NewClaudeCodeProvider(workDir string) (provider.Provider, error)
func NewCopilotCLIProvider(workDir string) (provider.Provider, error)
func NewCodexCLIProvider(workDir string) (provider.Provider, error)
func NewGeminiCLIProvider(workDir string) (provider.Provider, error)
func NewCursorCLIProvider(workDir string) (provider.Provider, error)
```

Each: verify binary exists via `exec.LookPath`, create `ptyProvider` with the appropriate adapter.

Register in both ProviderRegistry files:
```go
r.factories["claude_code"] = func(ctx, _, cfg) { return NewClaudeCodeProvider(cfg.BaseURL) }
r.factories["copilot_cli"] = func(ctx, _, cfg) { return NewCopilotCLIProvider(cfg.BaseURL) }
r.factories["codex_cli"]   = func(ctx, _, cfg) { return NewCodexCLIProvider(cfg.BaseURL) }
r.factories["gemini_cli"]  = func(ctx, _, cfg) { return NewGeminiCLIProvider(cfg.BaseURL) }
r.factories["cursor_cli"]  = func(ctx, _, cfg) { return NewCursorCLIProvider(cfg.BaseURL) }
```

Note: `cfg.BaseURL` is repurposed as `workDir` for PTY providers since they don't have a URL.

Commit: `feat: register PTY CLI providers in ProviderRegistry`

---

## Task 4: ratchet-cli setup commands

**Files:**
- Modify: `cmd/ratchet/cmd_provider.go`

Add setup commands for each CLI tool:
```
ratchet provider setup claude-code
ratchet provider setup copilot-cli
ratchet provider setup codex-cli
ratchet provider setup gemini-cli
ratchet provider setup cursor-cli
```

Each setup flow:
1. Check for binary via `exec.LookPath`
2. If missing, show install instructions (not auto-install — these are subscription services)
3. Run health check: `<binary> -p "say ok"` with 30s timeout
4. If health check passes, register provider via `AddProvider` RPC
5. Optionally set as default

Commit: `feat: add ratchet provider setup for all 5 CLI tools`

---

## Task 5: Bump ratchet-cli dep + build + test

**Files:**
- Modify: `ratchet-cli/go.mod`

Tag workflow-plugin-agent, bump in ratchet-cli, build, run all tests.

Commit: `chore: bump workflow-plugin-agent for PTY CLI providers`

---

## Task 6: Integration tests with real CLI tools

**Files:**
- Create: `ratchet-cli/internal/tui/pty_cli_integration_test.go`

For each CLI tool that's installed on the machine:
1. Run setup (verify binary, health check)
2. Send a non-interactive Chat() message, verify response contains expected text
3. Start a Stream() PTY session, send a message, verify streaming events received
4. Multi-turn: send a second message in the same PTY session, verify context maintained
5. Test provider list shows the new provider
6. Verify tool approval prompts are visible in stream output (where applicable)

Use `exec.LookPath` to skip tests for tools that aren't installed.

Commit: `test: add PTY CLI provider integration tests`

---

## Task 7: End-to-end validation with real CLIs

Manual validation (not automated): for each installed CLI tool:
1. `ratchet provider setup <tool>`
2. `ratchet provider default <tool>`
3. `ratchet -p "What is 2+2?"`
4. Verify correct response
5. `ratchet team start code-gen --task "Write a hello world in Python"`
6. Verify team agents use the CLI provider

Document results.

---

## Execution Order

```
Task 1 (core) → Task 2 (adapters) → Task 3 (registry) → Task 4 (setup commands)
                                                        → Task 5 (dep bump)
                                                        → Task 6 (integration tests)
                                                        → Task 7 (E2E validation)
```

Tasks 1-3 are in workflow-plugin-agent. Tasks 4-7 are in ratchet-cli.
