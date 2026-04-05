# Seamless Local Model Setup Design

**Date:** 2026-04-05
**Repo:** ratchet-cli
**Goal:** Wire up automatic local model setup so first-run users can go from zero to a working local LLM with minimal friction.

## First-Run Auto-Wizard

On TUI startup, if `ListProviders()` returns empty, automatically navigate to the onboarding wizard. The empty providers table is the signal — no flag file needed. Implemented in `app.go` after the initial page transition.

## Enhanced CLI: `ratchet provider setup ollama`

Current behavior: prints `ollama pull <model>` as a manual step.

New flow:

1. **Check for `ollama` binary** — `exec.LookPath("ollama")`
   - If missing, prompt: "Ollama not found. Install it? [Y/n]"
   - If yes: `brew install ollama` (macOS via `runtime.GOOS == "darwin"`) or `curl -fsSL https://ollama.com/install.sh | sh` (Linux)
   - If no: print manual install URL and exit
2. **Check if server running** — `OllamaClient.Health(ctx)`
   - If not running, start it: `exec.Command("ollama", "serve")` in background
   - Wait up to 10s for health check to pass
3. **List installed models** — `OllamaClient.ListModels(ctx)`
   - If models exist, show them and ask which to use (or skip)
   - If none installed, show curated recommendations:
     - `qwen3:8b` (8GB, fast, good tool use)
     - `llama3.3:8b` (8GB, general purpose)
     - `gemma3:4b` (4GB, lightweight)
     - Custom (user types model name)
4. **Pull selected model** — `OllamaClient.Pull(ctx, model, progressFn)` with progress bar
5. **Auto-register provider** — ensure daemon running, call `AddProvider` RPC with alias "ollama", type "ollama", model, baseURL `http://localhost:11434`, isDefault true
6. **Test connection** — call `TestProvider` RPC, show result

## TUI Onboarding: Ollama Model Pull

When user selects "Local models (Ollama)" in the onboarding wizard:

1. Health check Ollama at configured base URL
2. If not running → show error with install/start instructions (no TUI auto-install — keep TUI simple, CLI handles heavy lifting)
3. Fetch models via `OllamaClient.ListModels()`
4. If no models installed → show "No models found" with recommended models list and "Pull now?" option
5. On model selection → `OllamaClient.Pull()` with TUI progress spinner
6. Proceed to test connection as normal

## New CLI: `ratchet model`

```
ratchet model list                                    — list installed Ollama models
ratchet model pull <name>                             — pull model via Ollama
ratchet model pull --from huggingface <repo> <file>   — download GGUF from HuggingFace
```

Thin wrappers around `OllamaClient` and `DownloadHuggingFaceFile()` from workflow-plugin-agent.

## Files Changed

| File | Change |
|---|---|
| `cmd/ratchet/cmd_provider.go` | Rewrite `handleOllamaSetup` — auto-install, pull, register |
| `cmd/ratchet/cmd_model.go` | NEW — model list/pull commands |
| `cmd/ratchet/main.go` | Register `model` subcommand |
| `internal/tui/app.go` | First-run: if no providers, auto-navigate to onboarding |
| `internal/tui/pages/onboarding.go` | Add model pull flow for Ollama when no models installed |

## Not In Scope (Future)

- HuggingFace authentication (gated/private models)
- Model configuration and management UI in TUI
- Advanced model selection (size filtering, capability matching)
- Background auto-detection at daemon startup
- llama.cpp binary management (Ollama subsumes this)
