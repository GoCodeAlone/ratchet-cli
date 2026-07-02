# ratchet-cli Extension Hooks And ACP Launch Profiles Implementation Plan

> **For the implementing agent:** REQUIRED SUB-SKILL: Use autodev:executing-plans to implement this plan task-by-task.

**Goal:** Add reviewable lifecycle hook execution and reusable ACP launch profiles for explicit foreground ACP client commands.

**Architecture:** Extend existing `internal/hooks`, `internal/plugins`, and `internal/acpclient.AgentSpec` surfaces. Hook trust is local hash-based state; ACP launch profiles are local reviewed specs that existing `--agent` resolution can use.

**Tech Stack:** Go 1.26, stdlib JSON/YAML, existing ratchet-cli daemon/CLI/plugin packages, existing ACP fixture binary smoke.

**Base branch:** master

---

## Scope Manifest

**PR Count:** 4
**Tasks:** 8
**Estimated Lines of Change:** ~1200

**Out of scope:**
- TypeScript extension SDK, hot reload, or LLM-callable custom tool registration.
- Daemon background/scheduled drain.
- Raw ACPX event-log import/export, replay UI, or TypeScript flow runtime compatibility.
- Local-first gateway/channels or per-channel routing.
- Enterprise managed hook distribution.
- Storing secret values in profiles.

**PR Grouping:**

| PR # | Title | Tasks | Branch |
|------|-------|-------|--------|
| 1 | `feat: add hook trust controls` | Task 1, Task 2, Task 3 | `feat/ratchet-cli-hook-trust-controls` |
| 2 | `feat: add ACP launch profiles` | Task 4, Task 5, Task 6 | `feat/ratchet-cli-acp-launch-profiles` |
| 3 | `docs: document hooks profiles release state` | Task 7 | `docs/ratchet-cli-hooks-profiles-release` |
| 4 | `docs: close hook profile plan` | Task 8 | `docs/ratchet-cli-hooks-profiles-closeout` |

**Status:** Draft

## Requirements Trace

| design requirement | task(s) |
|---|---|
| Hash-review project/plugin hooks; user hooks compatibility exception documented. | Task 1, Task 2, Task 7 |
| Reject plugin hook/profile path escapes. | Task 1, Task 5 |
| Workdir-scoped project hook loading for daemon events. | Task 2 |
| `ratchet hooks` review/trust/disable CLI. | Task 3 |
| ACP launch profile store and profile-aware `--agent` resolution. | Task 4, Task 6 |
| Plugin-distributed ACP profile templates. | Task 5 |
| Profile execution proof through ACP fixture and watch/drain. | Task 6 |
| Docs/policy/parity/release/Windows coverage. | Task 7, Task 8 |

### Task 1: Hook Metadata, Trust Store, And Plugin Path Containment

**Files:**
- Modify: `internal/hooks/hooks.go`
- Create: `internal/hooks/trust.go`
- Modify: `internal/hooks/hooks_test.go`
- Modify: `internal/plugins/loader.go`
- Modify: `internal/plugins/manifest.go`
- Modify: `internal/plugins/loader_test.go`
- Modify: `internal/plugins/manifest_test.go`

**Step 1: Write failing hook trust tests**

Add tests:
- `TestHookDescriptorHashStableWithoutAbsoluteHomePath`
- `TestHookTrustStoreTrustDisableUntrust`
- `TestLoadProjectHookStartsUntrusted`
- `TestPluginHookPathEscapeRejected`
- `TestHookWindowsCommandSelection`

Run: `go test ./internal/hooks ./internal/plugins -run 'Hook|Plugin.*Hook|Manifest' -count=1`
Expected: FAIL on missing trust store/path validation.

**Step 2: Implement metadata and trust store**

Add:
- `Hook.CommandWindows string yaml:"command_windows,omitempty"`
- non-serialized metadata: source kind/id/path/plugin, hash, trust status, disabled, unsupported platform.
- `TrustStore` JSON under caller-provided path: trusted hashes and disabled hashes.
- stable hash excludes machine-home absolute prefixes.
- platform command selector with injectable GOOS helper for tests.

Expected behavior:
- user source hooks are trusted by compatibility default;
- project/plugin hooks require trust;
- disabled wins;
- unsupported Windows command skips rather than invoking `sh`.

**Step 3: Reject escaped plugin hook paths**

Add helper in `internal/plugins` to resolve relative capability paths inside plugin root. Use it for hooks now.

Run: `go test ./internal/hooks ./internal/plugins -run 'Hook|Plugin.*Hook|Manifest' -count=1`
Expected: PASS.

**Step 4: Broaden focused tests**

Run: `go test ./internal/hooks ./internal/plugins -count=1`
Expected: PASS.

**Step 5: Commit**

```sh
git add internal/hooks internal/plugins
git commit -m "feat: add hook trust metadata"
```

### Task 2: Workdir-Scoped Hook Loading And Runtime Skip Semantics

**Files:**
- Modify: `internal/daemon/engine.go`
- Modify: `internal/daemon/chat.go`
- Modify: `internal/daemon/plans.go`
- Modify: `internal/daemon/teams.go`
- Modify: `internal/daemon/fleet.go`
- Modify: `internal/daemon/service.go`
- Modify: `internal/daemon/hooks_wiring_test.go`
- Modify: `internal/hooks/hooks.go`

**Step 1: Write failing daemon hook tests**

Add tests:
- project hook in session working dir does not fire until trusted;
- trusted project hook fires for session/chat events with known workdir;
- project hook is skipped for no-workdir events;
- plugin hook changed after trust requires re-trust.

Run: `go test ./internal/daemon -run Hooks -count=1`
Expected: FAIL on no workdir-aware hook runner.

**Step 2: Add EngineContext hook runner**

Add an engine method that:
- merges startup user/plugin hooks with workdir project hooks per event;
- resolves session workdir from `session_id` when possible;
- applies trust store status before `Run`;
- logs skipped untrusted/disabled/unsupported hooks without failing the parent operation.

Do not make hooks fatal for chat/team/plan execution unless the hook itself is trusted and returns an execution error.

**Step 3: Wire existing call sites**

Replace direct `Hooks.Run` calls with the engine runner where an engine context exists. Keep `PlanManager` unit constructor support by allowing an injected hook config path for tests.

Run: `go test ./internal/daemon -run Hooks -count=1`
Expected: PASS.

**Step 4: Regression**

Run: `go test ./internal/hooks ./internal/plugins ./internal/daemon -run 'Hook|Plugin.*Hook|Manifest' -count=1`
Expected: PASS.

**Step 5: Commit**

```sh
git add internal/daemon internal/hooks internal/plugins
git commit -m "feat: enforce trusted runtime hooks"
```

### Task 3: `ratchet hooks` CLI

**Files:**
- Modify: `cmd/ratchet/main.go`
- Create: `cmd/ratchet/cmd_hooks.go`
- Create: `cmd/ratchet/cmd_hooks_test.go`
- Modify: `README.md`

**Step 1: Write failing CLI parse/behavior tests**

Tests:
- help includes `hooks`;
- `hooks list --json --cwd <dir>` shows trusted user and untrusted project/plugin hooks;
- `hooks trust <hash>` changes status;
- `hooks disable <hash>` wins over trust;
- command display is truncated/redacted enough to avoid dumping long local commands.

Run: `go test ./cmd/ratchet -run Hooks -count=1`
Expected: FAIL on missing command.

**Step 2: Implement command**

Subcommands:
- `ratchet hooks list [--json] [--cwd <dir>]`
- `ratchet hooks trust <hash>`
- `ratchet hooks untrust <hash>`
- `ratchet hooks disable <hash>`

CLI must use same trust store and hook discovery as runtime. No live hook execution.

**Step 3: Docs**

README: add short hook trust example and user-hook compatibility note (D1). Help/errors: built around exact command names.

**Step 4: Verify**

Run:
- `go test ./cmd/ratchet -run Hooks -count=1`
- `go test ./internal/hooks ./internal/plugins ./internal/daemon ./cmd/ratchet -run 'Hook|Plugin.*Hook|Manifest' -count=1`

Expected: PASS.

**Step 5: PR1 verification**

Run:
- `go test ./internal/hooks ./internal/plugins ./internal/daemon ./cmd/ratchet -run 'Hook|Plugin.*Hook|Manifest' -count=1`
- `go test ./... -count=1 -p=1 -timeout=20m`
- `go vet ./...`
- `golangci-lint run --new-from-rev=origin/master`
- `GOOS=windows GOARCH=amd64 go build ./cmd/ratchet`
- `GOOS=windows GOARCH=arm64 go build ./cmd/ratchet`
- `git diff --check`

Expected: all pass; remove generated `ratchet.exe`.

**Step 6: Commit**

```sh
git add cmd/ratchet README.md internal/hooks internal/plugins internal/daemon
git commit -m "feat: add hook trust CLI"
```

### Task 4: ACP Launch Profile Store And Registry Resolution

**Files:**
- Create: `internal/acpclient/profiles.go`
- Create: `internal/acpclient/profiles_test.go`
- Modify: `internal/acpclient/spec.go`
- Modify: `internal/acpclient/spec_test.go`

**Step 1: Write failing profile store tests**

Tests:
- add/list/remove profile persists JSON;
- command/args validation reuses `AgentSpec.Validate`;
- profile hash changes when command/args/env keys change;
- untrusted profile resolution fails;
- trusted profile resolution succeeds after built-ins and rejects built-in name shadowing (D2).

Run: `go test ./internal/acpclient -run 'Profile|Registry|Spec' -count=1`
Expected: FAIL on missing profile store.

**Step 2: Implement store**

Add:
- `Profile` with name, `AgentSpec`, cwd, source metadata, hash, trusted, timestamps.
- `ProfileStore` at configurable path plus default under ACP client state dir.
- registry overlay method that checks built-ins first, then trusted profiles.
- no env values stored; env keys only.

**Step 3: Verify**

Run: `go test ./internal/acpclient -run 'Profile|Registry|Spec' -count=1`
Expected: PASS.

**Step 4: Commit**

```sh
git add internal/acpclient
git commit -m "feat: add ACP launch profile store"
```

### Task 5: Plugin-Distributed ACP Profile Templates And CLI Management

**Files:**
- Modify: `internal/plugins/manifest.go`
- Modify: `internal/plugins/loader.go`
- Modify: `internal/plugins/loader_test.go`
- Modify: `cmd/ratchet/cmd_acp_client.go`
- Modify: `cmd/ratchet/cmd_acp_client_test.go`

**Step 1: Write failing plugin/template tests**

Tests:
- manifest `capabilities.acpProfiles` loads YAML/JSON templates;
- path escapes are rejected;
- `ratchet acp client profiles list --json` lists local profiles and plugin templates;
- `profiles add`, `profiles install`, `profiles trust`, `profiles remove` work;
- built-in name shadowing errors are explicit (D2);
- no `managed` config key or support claim is emitted (D3).

Run: `go test ./internal/plugins ./cmd/ratchet -run 'Profile|ACPClient.*Profile|Plugin.*Profile|Manifest' -count=1`
Expected: FAIL on missing template/CLI support.

**Step 2: Implement plugin template loader**

Add `Capabilities.ACPProfiles string json:"acpProfiles,omitempty"`. Load templates into `LoadResult.ACPProfiles` with plugin name/version/source path. Reuse containment helper from Task 1.

**Step 3: Implement profile CLI**

Under `ratchet acp client profiles`:
- `list [--json]`
- `add <name> --command ... [--arg ...] [--env-key ...] [--cwd ...] [--trust]`
- `install <plugin>/<profile> --as <name> [--trust]`
- `trust <name>`
- `remove <name>`

**Step 4: Verify**

Run:
- `go test ./internal/acpclient ./internal/plugins ./cmd/ratchet -run 'Profile|ACPClient.*Profile|Plugin.*Profile|Manifest' -count=1`
- `git diff --check`

Expected: PASS.

**Step 5: Commit**

```sh
git add internal/acpclient internal/plugins cmd/ratchet
git commit -m "feat: add ACP profile CLI"
```

### Task 6: Use ACP Launch Profiles In Exec, Drain, Watch, Compare, And Flow

**Files:**
- Modify: `cmd/ratchet/cmd_acp_client.go`
- Modify: `cmd/ratchet/cmd_acp_client_test.go`
- Modify: `cmd/ratchet/acp_client_binary_test.go`
- Modify: `internal/acpclient/flow_runner.go`
- Modify: `internal/acpclient/flow_test.go`

**Step 1: Write failing integration tests**

Tests:
- `exec --agent <trusted-profile>` uses stored command/args;
- untrusted profile fails with actionable error;
- `drain` and `watch` use trusted profile without `--command`;
- `compare --agent <profile>` works for profile plus built-in/custom agents;
- flow default agent and node agent can reference trusted profiles.

Run: `go test ./cmd/ratchet ./internal/acpclient -run 'Profile|Flow|ACPClient.*BinarySmoke' -count=1 -timeout=10m`
Expected: FAIL on commands not resolving profiles.

**Step 2: Implement profile-aware resolution**

Centralize ACP client registry construction so all command paths load local profiles once and pass the overlay registry through exec/drain/watch/compare/flow.

**Step 3: Binary smoke**

Extend ACP fixture smoke:
- add a trusted local profile for fixture process;
- queue prompts;
- `watch --agent <profile> --stop-when-empty`;
- `flow run` with profile default agent.

Run: `go test ./cmd/ratchet -run 'ACPClient.*BinarySmoke|ACPClient.*Profile' -count=1 -timeout=10m`
Expected: PASS.

**Step 4: PR2 verification**

Run:
- `go test ./internal/acpclient ./internal/plugins ./cmd/ratchet -run 'Profile|Flow|ACPClient.*BinarySmoke|Plugin.*Profile|Manifest' -count=1 -timeout=10m`
- `go test ./... -count=1 -p=1 -timeout=20m`
- `go vet ./...`
- `golangci-lint run --new-from-rev=origin/master`
- `GOOS=windows GOARCH=amd64 go build ./cmd/ratchet`
- `GOOS=windows GOARCH=arm64 go build ./cmd/ratchet`
- `git diff --check`

Expected: all pass; remove generated `ratchet.exe`.

**Step 5: Commit**

```sh
git add cmd/ratchet internal/acpclient internal/plugins
git commit -m "feat: use ACP launch profiles"
```

### Task 7: Public Docs, Policy Matrix, And Parity Refresh

**Files:**
- Modify: `README.md`
- Modify: `docs/harness-emulation.md`
- Modify: `docs/competitor-parity.md`
- Modify: `docs/policy-matrix.md`
- Modify: `cmd/ratchet/harness_docs_test.go`
- Modify: `docs/plans/2026-07-02-ratchet-cli-extension-hooks-profiles-design.md`

**Step 1: Write failing docs guard**

Extend `TestHarnessEmulationDocsCoverPolicyMatrixLayers` to require:
- `ratchet hooks list`
- `ratchet hooks trust`
- `ratchet acp client profiles`
- `hook trust`
- `ACP launch profiles`
- `managed hooks remain deferred`
- `TypeScript extension SDK remains deferred`

Run: `go test ./cmd/ratchet -run HarnessEmulationDocs -count=1`
Expected: FAIL before docs updates.

**Step 2: Update docs**

Docs must:
- state user hooks compatibility exception (D1);
- state built-in ACP agents win over profile names and profile names cannot shadow built-ins (D2);
- state managed hooks are reserved/deferred (D3);
- update source snapshot dates/commits for current 2026-07-02 sources;
- avoid claiming daemon background drain or broad SDK support.

**Step 3: Verify docs**

Run:
- `go test ./cmd/ratchet -run HarnessEmulationDocs -count=1`
- `rg -n "$(printf '/%s/|/%s/|/var/%s' Users home folders)" docs/plans/2026-07-02-ratchet-cli-extension-hooks-profiles*.md README.md docs/harness-emulation.md docs/competitor-parity.md docs/policy-matrix.md || true`
- `git diff --check`

Expected: docs test PASS; machine-path scan has no matches.

**Step 4: Commit**

```sh
git add README.md docs/harness-emulation.md docs/competitor-parity.md docs/policy-matrix.md cmd/ratchet/harness_docs_test.go docs/plans/2026-07-02-ratchet-cli-extension-hooks-profiles-design.md
git commit -m "docs: document hook profile policy"
```

### Task 8: Release, Closeout, And Workspace State

**Files:**
- Modify: `docs/plans/2026-07-02-ratchet-cli-extension-hooks-profiles.md`
- Create: `docs/retros/2026-07-02-ratchet-cli-extension-hooks-profiles-retro.md`

**Step 1: Release gate on merged master**

After PR3 and default-branch CI are green, create a fresh release worktree at `origin/master`.

Run:
- `bash <autodev-plugin>/tests/plan-scope-check.sh --verify-lock docs/plans/2026-07-02-ratchet-cli-extension-hooks-profiles.md`
- `go test ./... -count=1 -p=1 -timeout=20m`
- `go vet ./...`
- `golangci-lint run --new-from-rev=origin/master`
- `GOOS=windows GOARCH=amd64 go build ./cmd/ratchet`
- `GOOS=windows GOARCH=arm64 go build ./cmd/ratchet`
- `goreleaser check`
- `git diff --check`

Expected: all pass; remove generated `ratchet.exe`.

**Step 2: Tag release**

- ensure `HEAD == origin/master`;
- ensure remote tag `v0.24.0` does not exist;
- tag and push `v0.24.0`;
- monitor Release workflow.

Expected release assets:
- `checksums.txt`
- `ratchet_darwin_amd64.tar.gz`
- `ratchet_darwin_arm64.tar.gz`
- `ratchet_linux_amd64.tar.gz`
- `ratchet_linux_arm64.tar.gz`
- `ratchet_windows_amd64.zip`
- `ratchet_windows_arm64.zip`
- Homebrew cask updated to 0.24.0.

**Step 3: Create closeout branch**

Create `docs/ratchet-cli-hooks-profiles-closeout` from released `origin/master`.

**Step 4: Close scope and retro**

Run scope-lock completion after v0.24.0 release evidence exists:

```sh
bash <autodev-plugin>/hooks/scope-lock-complete docs/plans/2026-07-02-ratchet-cli-extension-hooks-profiles.md --evidence "<merged PRs and v0.24.0 release evidence>"
```

Write retro with design/plan findings, CI/review misses, and project-guidance result.

**Step 5: Commit closeout artifacts**

```sh
git add docs/plans/2026-07-02-ratchet-cli-extension-hooks-profiles.md docs/retros/2026-07-02-ratchet-cli-extension-hooks-profiles-retro.md
git commit -m "docs: close hook profile plan"
```

## Post-Plan Workspace State

After ratchet-cli PR4 merges and v0.24.0 is verified, update workspace `docs/FOLLOWUPS.md`, `docs/PROJECTS.md`, and `.autodev/state/phase-progress.jsonl`. Open and merge a separate workspace PR with local verification:

- `jq -c . .autodev/state/phase-progress.jsonl >/dev/null`
- `git diff --check`
- `rg -n "v0.24.0|extension hooks|ACP launch profiles" docs/FOLLOWUPS.md docs/PROJECTS.md .autodev/state/phase-progress.jsonl`
