# Seamless Local Model Setup — Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Wire up automatic local model setup so first-run users can go from zero to a working local LLM with `ratchet provider setup ollama` or through the TUI onboarding wizard.

**Architecture:** Rewrite `handleOllamaSetup` to auto-install Ollama (with user confirmation), auto-pull a model via `OllamaClient.Pull()`, and auto-register the provider. Enhance TUI onboarding to offer model pulling when Ollama has no models. Add `ratchet model` CLI for ad-hoc model management.

**Tech Stack:** Go 1.26, workflow-plugin-agent v0.6.0 (`OllamaClient`, `DownloadHuggingFaceFile`), Bubbletea v2

---

## Task 1: Rewrite `ratchet provider setup ollama` — auto-install, pull, register

**Files:**
- Modify: `cmd/ratchet/cmd_provider.go`

**Step 1:** Replace `handleOllamaSetup` with a complete setup flow that:

1. Parses `--model` flag (default: `qwen3:8b`)
2. Checks for `ollama` binary via `exec.LookPath`
   - If missing: prompt "Ollama not found. Install it? [Y/n]" via `fmt.Scanln`
   - If yes + macOS (`runtime.GOOS == "darwin"`): run `brew install ollama`
   - If yes + Linux: run `sh -c "curl -fsSL https://ollama.com/install.sh | sh"`
   - If no: print install URL and return
3. Check server health via `provider.NewOllamaClient("").Health(ctx)`
   - If not running: start `ollama serve` in background via `exec.Command`, wait up to 15s polling Health
4. List installed models via `OllamaClient.ListModels(ctx)`
   - If models exist: print them, ask if user wants to use one or pull a new one
5. If no models (or user wants new): show recommended list, prompt selection, pull via `OllamaClient.Pull(ctx, model, progressFn)` with a terminal progress bar
6. Ensure daemon running via `client.EnsureDaemon()`
7. Register provider: `c.AddProvider(ctx, &pb.AddProviderReq{Alias: "ollama", Type: "ollama", Model: model, BaseUrl: "http://localhost:11434", IsDefault: true})`
8. Test connection: `c.TestProvider(ctx, "ollama")`
9. Print success summary

**Step 2:** Add helper functions in the same file:
- `promptYesNo(question string) bool` — reads Y/n from stdin
- `installOllama() error` — OS-detect + exec brew/curl
- `startOllamaServer() error` — background exec + health poll
- `pullModelWithProgress(ctx, client, model string)` — calls `OllamaClient.Pull` with terminal progress output
- `promptModelSelection(models []provider.ModelInfo) string` — show numbered list, read selection

**Step 3:** Add imports: `runtime`, `bufio`, `strings`, `provider` from workflow-plugin-agent

**Step 4:** Build and verify: `go build ./cmd/ratchet/`

**Step 5:** Commit:
```bash
git add cmd/ratchet/cmd_provider.go
git commit -m "feat: rewrite ollama setup with auto-install, pull, and register"
```

---

## Task 2: Add `ratchet model` CLI command

**Files:**
- Create: `cmd/ratchet/cmd_model.go`
- Modify: `cmd/ratchet/main.go`

**Step 1:** Create `cmd/ratchet/cmd_model.go` with:

```
ratchet model list                                  — list installed Ollama models
ratchet model pull <name>                           — pull model via Ollama
ratchet model pull --from huggingface <repo> <file> — download GGUF from HuggingFace
```

`handleModel(args []string)`:
- `list`: call `OllamaClient.ListModels(ctx)`, print table (NAME, SIZE, MODIFIED)
- `pull`: if `--from huggingface`, call `provider.DownloadHuggingFaceFile(ctx, repo, file, "", progressFn)`; else call `OllamaClient.Pull(ctx, name, progressFn)`
- Default baseURL from config or `http://localhost:11434`

**Step 2:** Register in `main.go`:
```go
case "model":
    handleModel(filteredArgs[1:])
```

**Step 3:** Build and test: `go build ./cmd/ratchet/ && ./ratchet model --help`

**Step 4:** Commit:
```bash
git add cmd/ratchet/cmd_model.go cmd/ratchet/main.go
git commit -m "feat: add ratchet model list/pull CLI command"
```

---

## Task 3: Enhance TUI onboarding — model pull when Ollama has no models

**Files:**
- Modify: `internal/tui/pages/onboarding.go`

**Step 1:** Add a new onboarding step `stepPullModel` between `stepFetchModels` and `stepSelectModel`:
- Add to the `onboardingStep` enum: `stepPullModel`
- Add fields to `OnboardingModel`: `pullingModel bool`, `pullProgress float64`, `pullModelName string`, `recommendedModels []string`

**Step 2:** In the `modelsListMsg` handler (around line 269), when `len(msg.models) == 0` and provider is `ollama`:
- Instead of going to `stepSelectModel` with empty list, transition to `stepPullModel`
- Set `recommendedModels` to `[]string{"qwen3:8b", "llama3.3:8b", "gemma3:4b"}`

**Step 3:** Implement `updatePullModel(msg tea.Msg)`:
- Render a selection list of recommended models
- On Enter: start pulling via async command that calls `OllamaClient.Pull` with progress callback
- Progress updates via `pullProgressMsg{pct float64}` tea.Msg sent periodically
- On completion: re-fetch models and transition to `stepSelectModel`

**Step 4:** Implement `viewPullModel()`:
- Show "No models installed. Pull one to get started:"
- Numbered list of recommended models
- If pulling: show spinner + progress percentage
- Esc to go back

**Step 5:** Wire into the step routing in `Update()` and `View()`.

**Step 6:** Build and test: `go build ./...`

**Step 7:** Commit:
```bash
git add internal/tui/pages/onboarding.go
git commit -m "feat: add model pull flow to TUI onboarding for Ollama"
```

---

## Task 4: Write tests

**Files:**
- Create: `cmd/ratchet/cmd_provider_test.go`
- Create: `cmd/ratchet/cmd_model_test.go`

**Step 1:** Test helper functions in `cmd_provider.go`:
- `TestPromptYesNo` — mock stdin, verify yes/no parsing
- `TestInstallOllama_Darwin` / `TestInstallOllama_Linux` — verify correct command constructed (don't actually exec)

**Step 2:** Test model command parsing in `cmd_model.go`:
- `TestHandleModel_List_NoServer` — verify graceful error when Ollama not running
- `TestHandleModel_Pull_MissingName` — verify usage message

**Step 3:** Run tests: `go test ./cmd/ratchet/ -v -count=1`

**Step 4:** Run full suite: `go test ./... -count=1`

**Step 5:** Commit:
```bash
git add cmd/ratchet/cmd_provider_test.go cmd/ratchet/cmd_model_test.go
git commit -m "test: add tests for provider setup and model commands"
```

---

## Task 5: Verify first-run auto-wizard still works

**Files:** No changes needed — verify only.

The first-run auto-wizard is already implemented in `internal/tui/app.go:271`:
```go
func (a App) transitionFromSplash() (tea.Model, tea.Cmd) {
    if a.reconfigure || len(a.providers) == 0 {
        // → onboarding wizard
    }
}
```

**Step 1:** Verify by reading the code path: `app.go Init()` → `checkProviders()` → `ProvidersCheckedMsg` → `transitionFromSplash()` → if empty → onboarding.

**Step 2:** Run `go build ./...` to confirm no regressions.

**Step 3:** Run `go test ./... -count=1` for full regression.

**Step 4:** Commit (if any fixes needed).

---

## Execution Order

```
Task 1 (CLI setup rewrite) → Task 2 (model command) → Task 3 (TUI pull flow) → Task 4 (tests) → Task 5 (verify)
```

All tasks are sequential — Task 1 establishes helpers reused by Task 2, Task 3 uses similar patterns, Task 4 tests everything.
