# Local Inference Integration: Ratchet + Ratchet-CLI

**Date:** 2026-03-28
**Status:** Approved
**Depends on:** workflow-plugin-agent v0.5.0 (PR #2)

## Goal

Enable ratchet users to run local AI models (via Ollama or llama.cpp) with thinking trace visibility in the TUI — zero API keys, full privacy.

## Release Chain

```mermaid
graph LR
    A["workflow-plugin-agent<br/>PR #2 merge → tag v0.5.0"] --> B["ratchet<br/>bump agent dep + factories → tag v0.1.17"]
    B --> C["ratchet-cli<br/>bump ratchet dep + TUI + proto + CLI"]
```

## 1. Ratchet Server (`ratchet` repo)

### Provider Factory Registration

**File:** `ratchetplugin/provider_registry.go`

Add `"ollama"` and `"llama_cpp"` factories to `NewProviderRegistry()`:

```go
r.factories["ollama"] = func(_ string, cfg LLMProviderConfig) (provider.Provider, error) {
    return provider.NewOllamaProvider(provider.OllamaConfig{
        Model: cfg.Model, BaseURL: cfg.BaseURL, MaxTokens: cfg.MaxTokens,
    }), nil
}
r.factories["llama_cpp"] = func(_ string, cfg LLMProviderConfig) (provider.Provider, error) {
    return provider.NewLlamaCppProvider(provider.LlamaCppConfig{
        BaseURL: cfg.BaseURL, ModelPath: cfg.Model, MaxTokens: cfg.MaxTokens,
    }), nil
}
```

### Step Factory Registration

**File:** `ratchetplugin/plugin.go`

Add to `StepFactories()`:
```go
"step.model_pull": agentplugin.NewModelPullStepFactory(),
```

### Dependency Bump

**File:** `go.mod`

Bump `workflow-plugin-agent` from `v0.4.1` to `v0.5.0`.

**Total: 3 files changed in ratchet.**

## 2. Ratchet-CLI Proto Changes

### New Thinking Event

**File:** `internal/proto/ratchet.proto`

```protobuf
message ChatEvent {
  oneof event {
    // ... existing ...
    ThinkingBlock thinking = 15;
  }
}

message ThinkingBlock {
  string content = 1;
}
```

Regenerate proto with `protoc`.

### Daemon Streaming

**File:** `internal/daemon/chat.go`

Add case in the event routing switch:
```go
case "thinking":
    stream.Send(&pb.ChatEvent{Event: &pb.ChatEvent_Thinking{
        Thinking: &pb.ThinkingBlock{Content: evt.Thinking},
    }})
```

## 3. TUI Collapsible Thinking Panel

### Component

**File:** `internal/tui/components/thinking.go`

```go
type ThinkingPanel struct {
    content   string
    collapsed bool
}
```

- Styled box with "Thinking..." header
- Toggle with `Ctrl+T` keybinding
- Collapsed: "Thinking (N lines) ▶"
- Expanded: full text in dimmed/italic style
- Auto-starts expanded, auto-collapses when first text token arrives

### Chat Page Integration

**File:** `internal/tui/pages/chat.go`

- `ChatEvent_Thinking` → feed content to ThinkingPanel
- Panel renders above the streaming response
- Auto-collapse on first `ChatEvent_Token`

## 4. CLI Provider Auto-Detect

**File:** `cmd/ratchet/cmd_provider.go`

When provider type is `"ollama"` or `"llama_cpp"`, skip API key prompt and offer base URL with sensible default:

```
$ ratchet provider add ollama
Alias [ollama]: my-local
Model [llama3]: qwen3.5:27b-q4_K_M
Base URL [http://localhost:11434]:
✓ Provider "my-local" added (ollama)
```

For `llama_cpp`, default base URL: `http://localhost:8081/v1`.

## 5. Testing

- **ratchet**: Unit test for ollama + llama_cpp factory creation in provider_registry_test.go
- **ratchet-cli**: Bubbletea test for ThinkingPanel component, test provider add with local types (no API key prompt)
- All tests use mocks — no live models

## 6. File Summary

### ratchet repo
```
ratchetplugin/provider_registry.go   MODIFY  add ollama + llama_cpp factories
ratchetplugin/plugin.go              MODIFY  register step.model_pull
go.mod                               MODIFY  bump workflow-plugin-agent to v0.5.0
```

### ratchet-cli repo
```
internal/proto/ratchet.proto         MODIFY  add ThinkingBlock event
internal/proto/*.pb.go               REGEN   protoc output
internal/daemon/chat.go              MODIFY  route thinking events
internal/tui/components/thinking.go  NEW     collapsible thinking panel
internal/tui/pages/chat.go           MODIFY  integrate thinking panel
cmd/ratchet/cmd_provider.go          MODIFY  auto-detect local providers
go.mod                               MODIFY  bump ratchet dep
```

## 7. Dependencies

No new dependencies beyond what workflow-plugin-agent v0.5.0 brings (ollama/ollama/api).
