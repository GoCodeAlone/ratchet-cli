# ratchet-cli Workflow Messaging Export And Profile CI Design

**Status:** Approved for autonomous execution
**Date:** 2026-07-05

## Goal

Advance the ratchet-cli harness backlog with two small, reviewable features:
Workflow-ready blackboard messaging export envelopes, and credential-free ACP
launch profile verification that can later run credentialed third-party agent
CI without changing the command surface.

## Global Design Guidance

Source: workspace `AGENTS.md`; ratchet-cli repo README, `docs/policy-matrix.md`,
`docs/harness-emulation.md`, and `docs/competitor-parity.md`.

| guidance | design response |
|---|---|
| Reuse existing plugins and avoid duplicated plumbing. | Blackboard export emits Workflow messaging envelopes for `workflow-plugin-messaging-core` consumers; ratchet-cli still does not post to Slack, Discord, Teams, webhooks, or provider SDKs. |
| Keep Go code minimal and follow repo command/test patterns. | Extend existing `cmd_blackboard.go`, `cmd_acp_client.go`, and `internal/acpclient` profile/runner helpers. |
| Build for Windows. | Add parser/unit tests and binary/package-compatible command paths; no POSIX shell or platform-specific command wrapper is introduced. |
| Policy matrix treats blackboard exports, ACP profiles, and archives as sensitive local metadata. | New outputs avoid credentials, secret values, and automatic delivery; verification reports env key names only and never prints env values. |
| Competitor parity identifies credentialed third-party agent CI and local-first gateway/channels as remaining work. | This slice ships a credential-free profile verification primitive and docs the future credentialed matrix; gateway/channel remains out of scope. |

## Current Baseline

- `ratchet blackboard export [section] [--json|--jsonl]` already emits local
  records with `messaging.text`.
- `workflow-plugin-messaging-core` already defines `step.messaging_send` with
  `channel` and `text` config fields for Slack/Discord/Teams-style plugins.
- ACP launch profiles already store command, args, cwd, env key names, hash,
  and trust state; trusted profiles resolve through `--agent`.
- `internal/acpclient/testdata/fixture-agent` provides a credential-free ACP
  agent binary for local and CI tests.
- `docs/competitor-parity.md` was refreshed after the 2026-07-01 comparison
  window and pins reviewed source revisions for Zed, ACP, Pi, Codex, Claude
  Code, Hermes, OpenClaw, and ACPX.

## Approaches Considered

| option | trade-off | verdict |
|---|---|---|
| A. Direct `ratchet blackboard send --slack/--discord` | Convenient, but duplicates existing Workflow messaging plugins and introduces credentials/channel secrets into ratchet-cli. | Reject. |
| B. Export Workflow messaging envelopes only | Keeps ratchet-cli local-only while giving Workflow pipelines a stable bridge artifact. | Choose. |
| C. Add CI jobs that call real Codex/Claude/Gemini now | Useful but would require secrets, provider accounts, redaction, and flaky external service boundaries. | Reject for this slice. |
| D. Add `profiles verify` with fixture proof now | Gives a reusable local/CI contract check and a future place for credentialed matrices. | Choose. |

## Design

### Workflow Messaging Export Envelope

Extend `ratchet blackboard export` with `--workflow-messaging`:

- Output is JSON by default and JSONL when combined with `--jsonl`.
- Each exported record retains the existing blackboard fields plus a
  `workflow` projection:
  - `step_type: "step.messaging_send"`
  - `plugin_family: "workflow-plugin-messaging-core"`
  - `input.text` from the existing `messaging.text`
  - `required_config: ["channel"]`
- No channel, token, webhook, provider, route, or credential flag is added.
  Routing remains an explicit downstream Workflow concern.
- The existing `messaging.text` projection remains backward-compatible.

Failure behavior:

- `--workflow-messaging` cannot be combined with provider credential flags
  because those flags remain unknown and rejected.
- Empty blackboard sections export an empty array or no JSONL lines.
- Malformed arguments keep the existing usage path.

### ACP Profile Verification

Add `ratchet acp client profiles verify <name> [--prompt text] [--timeout d] [--json]`:

- Resolves the profile through the existing default registry, so untrusted
  profiles fail the same way `--agent` execution fails.
- Starts the configured ACP process using existing `acpclient.Start` and runs a
  small prompt through `RunPrompt`.
- Uses default prompt `ratchet profile verification`.
- JSON output includes profile name, command fingerprint, status, ACP session
  id, stop reason, and text length. It does not include prompt text, response
  text, env values, or raw event logs.
- Human output says whether the profile verified and prints the session id and
  stop reason only.
- A future credentialed CI matrix can install trusted profiles for third-party
  agents and invoke this command with repository secrets provided by CI, without
  ratchet-cli learning provider-specific credentials.

Failure behavior:

- Unknown or untrusted profiles return a non-zero error.
- Missing command/env prerequisites come from existing profile/spec validation
  and process launch errors.
- Timeout uses the existing context timeout pattern from ACP client exec.

## Security Review

| risk | mitigation |
|---|---|
| Blackboard export leaks secrets to external channels. | ratchet-cli only emits local files/stdout; delivery is delegated to Workflow messaging plugins and requires downstream routing. |
| A channel flag becomes a hidden provider credential path. | No channel/token/provider/webhook flags are added; existing rejection tests are preserved. |
| `profiles verify --json` leaks prompts or responses. | Output includes lengths and status metadata only. |
| Verification starts an unreviewed local command. | The command resolves only through trusted ACP launch profiles. |
| Credentialed CI later logs secret env values. | Profiles store env key names only; docs require CI redaction and this command does not print env values. |

## Infrastructure Impact

No cloud resources, IAM, queues, migrations, or production deployment. Local
runtime impact is limited to launching an operator-trusted ACP profile on
explicit command invocation. Rollback is reverting the ratchet-cli PRs and
publishing a patch release; local profile JSON remains compatible with older
binaries that ignore `verify`.

## Multi-Component Validation

| boundary | proof |
|---|---|
| daemon blackboard -> CLI export -> Workflow messaging shape | Command tests verify `--workflow-messaging` records include `step.messaging_send`, `workflow-plugin-messaging-core`, `input.text`, and no provider credentials. |
| profile store -> trusted registry -> ACP process | Unit tests and binary smoke build the fixture ACP agent, add/trust a profile, and verify it through the built CLI. |
| docs -> public harness claims | Harness docs tests require the new bridge and profile verification claims. |
| Windows packaging | Cross-build `GOOS=windows GOARCH=amd64 go build ./cmd/ratchet` proves no platform-specific compile break. |

## Assumptions

| id | assumption | challenge | fallback |
|---|---|---|---|
| A1 | Workflow messaging consumers can fill `channel` downstream. | Some users may expect ratchet-cli to choose a channel. | Keep routing out of ratchet-cli; add examples showing required downstream config. |
| A2 | Prompt+response verification is a better CI contract than initialize-only. | Some agents may charge per prompt or require auth. | Callers can use fixture profiles in default CI; credentialed matrices remain opt-in. |
| A3 | Redacted verification output is sufficient for CI triage. | Debugging a failing third-party agent may need raw logs. | Users can run normal `exec` locally; CI keeps `verify` redacted. |

## Self-Challenge

1. Laziest solution: document piping `blackboard export --jsonl` into a Workflow
   step. Rejected because the current record lacks explicit Workflow contract
   metadata and would invite ad hoc mapping.
2. Fragile assumption: downstream pipelines can supply `channel`. If false,
   direct provider send would be tempting, but that conflicts with the plugin
   reuse constraint and secret boundary.
3. YAGNI risk: `profiles verify` could duplicate `exec`. Kept because it gives
   CI a redacted, stable command that does not expose prompts/responses.

## Out Of Scope

- Direct Slack, Discord, Teams, webhook, email, or provider delivery.
- New AWS, storage, or messaging SDK implementations in ratchet-cli.
- Secret storage or env value export.
- Managed hooks or TypeScript extension SDK.
- Daemon background scheduling or hidden queue drain.
- Local-first gateway/channel routing.
- Real third-party provider CI secrets in this PR set.
- ACPX TypeScript `.flow.ts` execution.

## Rollback

Revert the ratchet-cli PRs and release a patch. No migrations or persistent
format changes are required. Existing blackboard export and ACP profile stores
remain readable because the new fields/command are additive.
