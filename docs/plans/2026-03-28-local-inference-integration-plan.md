# Local Inference Integration Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Wire Ollama and llama.cpp providers into the orchestrator, add thinking trace streaming to ratchet-cli's proto/daemon/TUI, and auto-detect local providers in the CLI.

**Architecture:** Two repos: workflow-plugin-agent (orchestrator factory + manifest wiring) and ratchet-cli (proto thinking event, daemon routing, TUI collapsible panel, CLI provider-add auto-detect). ratchet-cli uses a local replace directive to workflow-plugin-agent so no version tagging is needed during development.

**Tech Stack:** Go 1.26, protobuf/gRPC, Bubbletea v2/Lipgloss v2 (TUI), workflow-plugin-agent provider SDK.

---

### Task 1: Orchestrator — Add Ollama and llama.cpp Provider Factories

**Files:**
- Modify: `/Users/jon/workspace/workflow-plugin-agent/orchestrator/provider_registry.go:58-72`

**Step 1: Write the failing test**

Add to `/Users/jon/workspace/workflow-plugin-agent/orchestrator/provider_registry_test.go` (create if needed):

```go
package orchestrator

import (
	"testing"
)

func TestProviderRegistry_HasOllamaFactory(t *testing.T) {
	r := NewProviderRegistry(nil, nil)
	if _, ok := r.factories["ollama"]; !ok {
		t.Error("expected 'ollama' factory to be registered")
	}
}

func TestProviderRegistry_HasLlamaCppFactory(t *testing.T) {
	r := NewProviderRegistry(nil, nil)
	if _, ok := r.factories["llama_cpp"]; !ok {
		t.Error("expected 'llama_cpp' factory to be registered")
	}
}
```

**Step 2: Run test to verify it fails**

Run: `cd /Users/jon/workspace/workflow-plugin-agent && go test ./orchestrator/ -run "TestProviderRegistry_Has" -v`
Expected: FAIL — factories not registered

**Step 3: Add factories**

In `orchestrator/provider_registry.go`, after line 70 (`r.factories["gemini"] = geminiProviderFactory`), add:

```go
	r.factories["ollama"] = ollamaProviderFactory
	r.factories["llama_cpp"] = llamaCppProviderFactory
```

Then add the factory functions at the end of the file (before the closing `}`):

```go
func ollamaProviderFactory(_ string, cfg LLMProviderConfig) (provider.Provider, error) {
	return provider.NewOllamaProvider(provider.OllamaConfig{
		Model:     cfg.Model,
		BaseURL:   cfg.BaseURL,
		MaxTokens: cfg.MaxTokens,
	}), nil
}

func llamaCppProviderFactory(_ string, cfg LLMProviderConfig) (provider.Provider, error) {
	return provider.NewLlamaCppProvider(provider.LlamaCppConfig{
		BaseURL:   cfg.BaseURL,
		ModelPath: cfg.Model,
		ModelName: cfg.Model,
		MaxTokens: cfg.MaxTokens,
	}), nil
}
```

**Step 4: Run test to verify it passes**

Run: `cd /Users/jon/workspace/workflow-plugin-agent && go test ./orchestrator/ -run "TestProviderRegistry_Has" -v`
Expected: PASS

**Step 5: Commit**

```bash
cd /Users/jon/workspace/workflow-plugin-agent
git add orchestrator/provider_registry.go orchestrator/provider_registry_test.go
git commit -m "feat: add ollama and llama_cpp factories to orchestrator provider registry"
```

---

### Task 2: Orchestrator — Register step.model_pull in Plugin Manifest

**Files:**
- Modify: `/Users/jon/workspace/workflow-plugin-agent/orchestrator/plugin.go:44,80`

**Step 1: Add step.model_pull to manifest StepTypes**

In `orchestrator/plugin.go` line 44, add `"step.model_pull"` to the `StepTypes` slice.

**Step 2: Add factory to StepFactories()**

In `orchestrator/plugin.go`, after line 81 (`"step.provider_models": agentplugin.NewProviderModelsFactory(),`), add:

```go
		"step.model_pull":            agentplugin.NewModelPullStepFactory(),
```

**Step 3: Verify build**

Run: `cd /Users/jon/workspace/workflow-plugin-agent && go build ./... && go test ./orchestrator/ -v`
Expected: PASS

**Step 4: Commit**

```bash
cd /Users/jon/workspace/workflow-plugin-agent
git add orchestrator/plugin.go
git commit -m "feat: register step.model_pull in orchestrator plugin manifest"
```

---

### Task 3: Proto — Add ThinkingBlock Event

**Files:**
- Modify: `/Users/jon/workspace/ratchet-cli/internal/proto/ratchet.proto:53-70`

**Step 1: Add ThinkingBlock message and event**

In `ratchet.proto`, add a new message type after `AuthError` (after line 76):

```protobuf
message ThinkingBlock {
  string content = 1;
}
```

In the `ChatEvent` oneof (line 68, after `auth_error`), add:

```protobuf
    ThinkingBlock thinking = 15;
```

**Step 2: Regenerate proto**

Run: `cd /Users/jon/workspace/ratchet-cli && protoc --go_out=. --go_opt=paths=source_relative --go-grpc_out=. --go-grpc_opt=paths=source_relative internal/proto/ratchet.proto`

If `protoc` is not available, use:
Run: `cd /Users/jon/workspace/ratchet-cli && go generate ./internal/proto/...`

**Step 3: Verify build**

Run: `cd /Users/jon/workspace/ratchet-cli && go build ./...`
Expected: PASS (new proto types are generated but not yet used)

**Step 4: Commit**

```bash
cd /Users/jon/workspace/ratchet-cli
git add internal/proto/ratchet.proto internal/proto/ratchet.pb.go internal/proto/ratchet_grpc.pb.go
git commit -m "proto: add ThinkingBlock event for reasoning trace streaming"
```

---

### Task 4: Daemon — Route Thinking Events

**Files:**
- Modify: `/Users/jon/workspace/ratchet-cli/internal/daemon/chat.go:140-151`

**Step 1: Add thinking case to event switch**

In `internal/daemon/chat.go`, after the `case "text":` block (after line 150), add:

```go
		case "thinking":
			if err := stream.Send(&pb.ChatEvent{
				Event: &pb.ChatEvent_Thinking{
					Thinking: &pb.ThinkingBlock{Content: event.Thinking},
				},
			}); err != nil {
				return err
			}
```

**Step 2: Verify build**

Run: `cd /Users/jon/workspace/ratchet-cli && go build ./...`
Expected: PASS

**Step 3: Commit**

```bash
cd /Users/jon/workspace/ratchet-cli
git add internal/daemon/chat.go
git commit -m "feat: route thinking stream events to gRPC clients"
```

---

### Task 5: TUI — Collapsible Thinking Panel Component

**Files:**
- Create: `/Users/jon/workspace/ratchet-cli/internal/tui/components/thinking.go`
- Create: `/Users/jon/workspace/ratchet-cli/internal/tui/components/thinking_test.go`

**Step 1: Write the failing tests**

Create `/Users/jon/workspace/ratchet-cli/internal/tui/components/thinking_test.go`:

```go
package components

import (
	"strings"
	"testing"
)

func TestThinkingPanel_AppendContent(t *testing.T) {
	p := NewThinkingPanel(80)
	p = p.AppendContent("First chunk. ")
	p = p.AppendContent("Second chunk.")

	if p.content != "First chunk. Second chunk." {
		t.Errorf("content: want %q, got %q", "First chunk. Second chunk.", p.content)
	}
}

func TestThinkingPanel_CollapsedView(t *testing.T) {
	p := NewThinkingPanel(80)
	p = p.AppendContent("line1\nline2\nline3")
	p = p.SetCollapsed(true)

	view := p.View()
	if !strings.Contains(view, "3 lines") {
		t.Errorf("collapsed view should show line count, got: %q", view)
	}
	if !strings.Contains(view, "▶") {
		t.Errorf("collapsed view should show ▶, got: %q", view)
	}
}

func TestThinkingPanel_ExpandedView(t *testing.T) {
	p := NewThinkingPanel(80)
	p = p.AppendContent("reasoning here")
	p = p.SetCollapsed(false)

	view := p.View()
	if !strings.Contains(view, "reasoning here") {
		t.Errorf("expanded view should show content, got: %q", view)
	}
	if !strings.Contains(view, "▼") {
		t.Errorf("expanded view should show ▼, got: %q", view)
	}
}

func TestThinkingPanel_Reset(t *testing.T) {
	p := NewThinkingPanel(80)
	p = p.AppendContent("old content")
	p = p.Reset()

	if p.content != "" {
		t.Errorf("after reset, content should be empty, got %q", p.content)
	}
	if p.collapsed {
		t.Error("after reset, should not be collapsed")
	}
}

func TestThinkingPanel_EmptyView(t *testing.T) {
	p := NewThinkingPanel(80)
	view := p.View()
	if view != "" {
		t.Errorf("empty panel should render nothing, got %q", view)
	}
}
```

**Step 2: Run tests to verify they fail**

Run: `cd /Users/jon/workspace/ratchet-cli && go test ./internal/tui/components/ -run "TestThinkingPanel" -v`
Expected: FAIL — `ThinkingPanel` doesn't exist

**Step 3: Implement thinking.go**

Create `/Users/jon/workspace/ratchet-cli/internal/tui/components/thinking.go`:

```go
package components

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
)

// ThinkingPanel displays a collapsible panel for model reasoning traces.
// It auto-starts expanded and can be toggled with Ctrl+T.
type ThinkingPanel struct {
	content   string
	collapsed bool
	width     int
}

// NewThinkingPanel creates a new thinking panel.
func NewThinkingPanel(width int) ThinkingPanel {
	return ThinkingPanel{width: width}
}

// AppendContent adds text to the thinking panel.
func (p ThinkingPanel) AppendContent(text string) ThinkingPanel {
	p.content += text
	return p
}

// SetCollapsed sets the collapsed state.
func (p ThinkingPanel) SetCollapsed(collapsed bool) ThinkingPanel {
	p.collapsed = collapsed
	return p
}

// ToggleCollapsed flips the collapsed state.
func (p ThinkingPanel) ToggleCollapsed() ThinkingPanel {
	p.collapsed = !p.collapsed
	return p
}

// Reset clears the content and resets to expanded state.
func (p ThinkingPanel) Reset() ThinkingPanel {
	p.content = ""
	p.collapsed = false
	return p
}

// HasContent returns true if the panel has any content.
func (p ThinkingPanel) HasContent() bool {
	return p.content != ""
}

// SetWidth updates the panel width.
func (p ThinkingPanel) SetWidth(w int) ThinkingPanel {
	p.width = w
	return p
}

// View renders the panel. Returns empty string if no content.
func (p ThinkingPanel) View() string {
	if p.content == "" {
		return ""
	}

	headerStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("243")).
		Bold(true)

	contentStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("243")).
		Italic(true)

	lines := strings.Split(strings.TrimRight(p.content, "\n"), "\n")
	lineCount := len(lines)

	if p.collapsed {
		header := headerStyle.Render(fmt.Sprintf("▶ Thinking (%d lines)", lineCount))
		return header
	}

	header := headerStyle.Render("▼ Thinking")
	body := contentStyle.Render(p.content)
	return header + "\n" + body
}
```

**Step 4: Run tests to verify they pass**

Run: `cd /Users/jon/workspace/ratchet-cli && go test ./internal/tui/components/ -run "TestThinkingPanel" -v`
Expected: ALL PASS

**Step 5: Commit**

```bash
cd /Users/jon/workspace/ratchet-cli
git add internal/tui/components/thinking.go internal/tui/components/thinking_test.go
git commit -m "feat: add collapsible ThinkingPanel TUI component"
```

---

### Task 6: TUI — Integrate Thinking Panel into Chat Page

**Files:**
- Modify: `/Users/jon/workspace/ratchet-cli/internal/tui/pages/chat.go`

**Step 1: Add thinking panel field to ChatModel**

In `pages/chat.go`, add to the `ChatModel` struct (after `planView` on line 44):

```go
	thinkingPanel components.ThinkingPanel
```

In `NewChat()`, initialize it (after line 80):

```go
	thinkingPanel: components.NewThinkingPanel(80),
```

**Step 2: Handle ThinkingBlock events**

Find the `handleChatEvent` method (or the `ChatEventMsg` handling in `Update`). In the event type switch, add a case for `*pb.ChatEvent_Thinking`:

```go
	case *pb.ChatEvent_Thinking:
		m.thinkingPanel = m.thinkingPanel.AppendContent(e.Thinking.Content)
```

For `*pb.ChatEvent_Token`, add auto-collapse on first token if thinking is showing:

```go
	case *pb.ChatEvent_Token:
		if m.thinkingPanel.HasContent() && !m.thinkingPanel.collapsed {
			m.thinkingPanel = m.thinkingPanel.SetCollapsed(true)
		}
		// ... existing token handling ...
```

**Step 3: Add Ctrl+T toggle keybinding**

In the `Update` method's key handling, add:

```go
case key.Matches(msg, key.NewBinding(key.WithKeys("ctrl+t"))):
	m.thinkingPanel = m.thinkingPanel.ToggleCollapsed()
```

**Step 4: Render thinking panel in View()**

In the `View()` method, render the thinking panel above the streaming response:

```go
	if thinkView := m.thinkingPanel.View(); thinkView != "" {
		// Insert thinkView above the message area
	}
```

**Step 5: Reset thinking on new message**

When a new chat turn starts, reset the panel:

```go
	m.thinkingPanel = m.thinkingPanel.Reset()
```

**Step 6: Verify build**

Run: `cd /Users/jon/workspace/ratchet-cli && go build ./...`
Expected: PASS

**Step 7: Commit**

```bash
cd /Users/jon/workspace/ratchet-cli
git add internal/tui/pages/chat.go
git commit -m "feat: integrate thinking panel into chat page with Ctrl+T toggle"
```

---

### Task 7: CLI — Auto-Detect Local Providers in Provider Add

**Files:**
- Modify: `/Users/jon/workspace/ratchet-cli/cmd/ratchet/cmd_provider.go:27-47`

**Step 1: Refactor provider add to skip API key for local providers**

Replace lines 35-47 of `cmd_provider.go` with:

```go
		var apiKey, baseURL string
		switch providerType {
		case "ollama":
			// No API key needed for Ollama
			baseURL, err = providerauth.PromptBaseURL("http://localhost:11434")
			if err != nil {
				fmt.Fprintf(os.Stderr, "error: %v\n", err)
				os.Exit(1)
			}
		case "llama_cpp":
			// No API key needed for llama.cpp
			baseURL, err = providerauth.PromptBaseURL("http://localhost:8081/v1")
			if err != nil {
				fmt.Fprintf(os.Stderr, "error: %v\n", err)
				os.Exit(1)
			}
		default:
			apiKey, err = providerauth.PromptAPIKey(providerType)
			if err != nil {
				fmt.Fprintf(os.Stderr, "error: %v\n", err)
				os.Exit(1)
			}
			if providerType == "custom" || providerType == "openai" {
				baseURL, err = providerauth.PromptBaseURL("")
				if err != nil {
					fmt.Fprintf(os.Stderr, "error: %v\n", err)
					os.Exit(1)
				}
			}
		}
```

**Step 2: Verify build**

Run: `cd /Users/jon/workspace/ratchet-cli && go build ./...`
Expected: PASS

**Step 3: Commit**

```bash
cd /Users/jon/workspace/ratchet-cli
git add cmd/ratchet/cmd_provider.go
git commit -m "feat: auto-detect local providers, skip API key for ollama and llama_cpp"
```

---

### Task 8: Build Verification and Full Test Run

**Step 1: Verify workflow-plugin-agent**

Run: `cd /Users/jon/workspace/workflow-plugin-agent && go build ./... && go test ./... -count=1 && go vet ./...`
Expected: ALL PASS

**Step 2: Verify ratchet-cli**

Run: `cd /Users/jon/workspace/ratchet-cli && go build ./... && go test ./... -count=1 && go vet ./...`
Expected: ALL PASS

**Step 3: Fix any issues and commit**

```bash
# If fixes needed in workflow-plugin-agent:
cd /Users/jon/workspace/workflow-plugin-agent
git add <specific files>
git commit -m "fix: resolve issues found during build verification"

# If fixes needed in ratchet-cli:
cd /Users/jon/workspace/ratchet-cli
git add <specific files>
git commit -m "fix: resolve issues found during build verification"
```
