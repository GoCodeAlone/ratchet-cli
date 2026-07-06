# Retro Loop

The retro loop is optional and disabled by default. It analyzes session evidence
that ratchet-cli already records or receives from command/test outcomes,
permission denials, and runtime errors. Findings are redacted with workflow's
shared `secrets.Redactor` before reports or routing instructions are emitted.

When enabled, daemon lifecycle evidence is appended locally as JSONL at:

```text
~/.ratchet/retro/evidence.jsonl
```

The store tolerates malformed lines on reload, so a partially written or manually
edited line does not prevent later evidence from being analyzed. Known secret
values loaded into workflow's `secrets.Redactor` are redacted before evidence is
written.

## Configuration

```yaml
retro:
  enabled: false
  local_changes: false
  upstream_instructions: true
```

`retro.enabled` gates all routing. `retro.local_changes` allows local action
suggestions. `retro.upstream_instructions` allows PR instructions when a finding
appears to require ratchet-cli or external harness changes.

The daemon does not write retro evidence unless `retro.enabled` is true. Local
mutation remains opt-in: with `retro.local_changes: false`, local configuration
findings are emitted as instructions instead of edits.

Current settings are visible with:

```sh
ratchet config show
```

## Analyze Evidence

Use `ratchet retro analyze` to inspect an explicit evidence file without
starting the daemon or mutating local files:

```sh
ratchet retro analyze --evidence ~/.ratchet/retro/evidence.jsonl --session SESSION_ID
ratchet retro analyze --evidence ~/.ratchet/retro/evidence.jsonl --json
```

The text output groups findings, local actions, and upstream instructions. JSON
output emits the same sections for scripts. `retro.enabled` still gates routing:
with the default disabled config, findings are shown but local actions and
upstream instructions are empty.

Use `ratchet retro instructions` when an agent needs to pass a PR-ready handoff
to a human or another harness without opening the PR itself:

```sh
ratchet retro instructions --evidence ~/.ratchet/retro/evidence.jsonl --session SESSION_ID
ratchet retro instructions --evidence ~/.ratchet/retro/evidence.jsonl --session SESSION_ID --output instructions.md
```

The Markdown output contains findings, upstream instructions, and local actions.
It is still reporting-only and should be reviewed before any upstream issue or
PR is filed.

Use `ratchet retro bundle` when the handoff should be a portable local
directory:

```sh
ratchet retro bundle --evidence ~/.ratchet/retro/evidence.jsonl --session SESSION_ID --output retro-bundle
```

The bundle contains derived `analysis.json`, `instructions.md`, and
`manifest.json` files. It does not copy raw evidence JSONL, mutate local config,
contact GitHub, or open a PR. Treat the bundle as sensitive local context until
its summarized findings have been reviewed.

## Local Improvement Example

Evidence:

```text
permission denied: bash command blocked by local policy
```

Finding:

```text
Pattern: permission denial
Local action: Review local trust or permission configuration for this command class.
```

With `retro.enabled: true` and `retro.local_changes: true`, ratchet-cli may emit
that local action as an instruction. It does not edit config automatically.

## Upstream Improvement Example

Evidence:

```text
go test ./internal/mcp failed because a required harness command is unsupported
```

Finding:

```text
Pattern: test failure
Upstream action: submit a PR with a regression test and the local failure evidence.
```

If the issue cannot be fixed through local configuration, the agent should pass
along a branch name, likely files, tests to run, and rationale for a ratchet-cli
PR. For third-party harness gaps, it should emit instructions only.

## Persisted Evidence Flow

1. Daemon lifecycle events are recorded only when `retro.enabled` is true.
2. Evidence is appended to the local JSONL store after secret redaction.
3. The analyzer loads the events and produces compact findings.
4. Routing separates local config actions, ratchet-cli PR instructions, and
   third-party-only instructions.
5. Optional Markdown instructions can be written for review or PR handoff.
