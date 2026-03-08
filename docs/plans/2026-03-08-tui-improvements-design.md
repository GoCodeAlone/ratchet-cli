# TUI Improvements Design

## Overview

Five improvements to the ratchet TUI: auto-expanding input, enhanced slash commands with autocompletion, richer status bar, and Copilot token exchange fix.

## 1. Auto-Expanding Input

Replace fixed 3-line textarea with a 1-line input that grows to 6 lines as content wraps or newlines are added (Shift+Enter). Shrinks back to 1 after submit.

**Implementation:**
- Count newlines + wrapped lines in textarea value
- Dynamically set textarea height: `max(1, min(lineCount, 6))`
- Recalculate layout on every keystroke that changes line count
- Viewport absorbs freed/consumed space

**Layout (bottom-up):**
```
┌─────────────────────────────┐
│       Viewport (messages)   │  height = total - input - status - spacing
├─────────────────────────────┤
│  > _                        │  height = 1-6 lines + 2 border
├─────────────────────────────┤
│  status line 1 (info)       │  height = 1
│  status line 2 (keybinds)   │  height = 1
└─────────────────────────────┘
```

## 2. Enhanced Slash Commands

| Command | Description | Implementation |
|---------|-------------|----------------|
| `/help` | List all commands | Exists — update to include new commands |
| `/model [name]` | Show or switch model | Call `ListProviders` for available models; if name given, `SetDefaultProvider` or update session model |
| `/provider list\|add\|remove\|default\|test` | Provider management | Exists |
| `/clear` | Clear conversation history | Reset `m.messages`, refresh viewport |
| `/compact [focus]` | Summarize conversation to save context | Send system prompt asking model to summarize, replace history with summary |
| `/cost` | Show session token usage | Read from status bar's token counters |
| `/agents` | List active agents | Call `ListAgents` RPC, format as table |
| `/sessions` | List sessions | Call `ListSessions` RPC, format as table |
| `/exit` | Quit ratchet | Return `tea.Quit` cmd |

Each command returns `Result{Lines, Cmd}` — extending the existing `Result` struct with an optional `tea.Cmd` for commands that need side effects (quit, navigate).

## 3. Slash Command Autocompletion

When user types `/` as the first character, show a filterable dropdown rendered directly above the input area.

**Behavior:**
- Trigger: input starts with `/`
- Filter: match typed text against command names (fuzzy or prefix)
- Navigation: Up/Down to select, Tab/Enter to complete, Escape to dismiss
- Display: floating list of matching commands with short descriptions
- Position: rendered between viewport and input in the chat View()

**Component:** `AutocompleteModel` in `components/autocomplete.go`
- Holds registered command names + descriptions
- Tracks filter text, selected index, visibility
- `View()` returns styled list or empty string when hidden

**Integration:** Chat model checks if autocomplete is active before forwarding keys to textarea. When active, Up/Down/Tab/Enter/Escape go to autocomplete; other keys update filter.

## 4. Enhanced Status Bar

Two lines below the input. Top line: contextual info. Bottom line: keybind hints.

**Top line (left-aligned, space-separated segments):**
- Project directory (shortened with `~/`): from `session.WorkingDir`
- Current model: from active provider
- Active agents count (shown only if > 0)
- Background tasks count (shown only if > 0)
- Session elapsed time
- Token counts: `↑N ↓N` (input/output)

**Bottom line (right-aligned):**
- `Ctrl+S sidebar  Ctrl+T team  Ctrl+C quit`

**New StatusBar fields:**
```go
type StatusBar struct {
    WorkingDir    string
    Provider      string
    Model         string
    SessionStart  time.Time
    InputTokens   int
    OutputTokens  int
    ActiveAgents  int
    BackgroundTasks int
    Width         int
}
```

## 5. Copilot Token Exchange Fix

**Problem:** Ratchet sends the GitHub OAuth token directly as Bearer to `api.githubcopilot.com`. The API requires a short-lived Copilot bearer token obtained by exchanging the OAuth token.

**Fix in `workflow-plugin-agent/provider/copilot.go`:**

1. Add token exchange: `GET https://api.github.com/copilot_internal/v2/token` with `Authorization: Token <oauth_token>`
2. Cache the returned `token` and `expires_at`
3. Auto-refresh when expired (check before each API call)
4. Add missing headers per OpenAI/Copilot conventions:
   - `Editor-Version: ratchet/0.1.8`
   - `Editor-Plugin-Version: ratchet/0.1.8`
   - `Copilot-Integration-Id: vscode-chat` (must be `vscode-chat`, not `ratchet`)

**Token exchange response:**
```json
{
  "token": "...",
  "expires_at": 1234567890,
  "refresh_in": 1500
}
```

**Config change:** `CopilotConfig.Token` becomes the GitHub OAuth token. The provider internally manages the Copilot bearer token lifecycle.

## File Changes

### ratchet-cli repo
- `internal/tui/components/input.go` — auto-expand logic
- `internal/tui/components/statusbar.go` — two-line layout, new fields
- `internal/tui/components/autocomplete.go` — new file
- `internal/tui/commands/commands.go` — new commands, Result.Cmd field
- `internal/tui/pages/chat.go` — relayout for dynamic input height, autocomplete integration, status bar positioning
- `internal/tui/tui_render_test.go` — update tests for new layout

### workflow-plugin-agent repo
- `provider/copilot.go` — token exchange, header fixes, token refresh
