# Ratchet CLI Provider, Drain, and Managed Hooks Implementation Plan

> **For the implementing agent:** REQUIRED SUB-SKILL: Use autodev:executing-plans to implement this plan task-by-task.

**Goal:** Unify provider setup across CLI/TUI, add explicitly authorized daemon ACP background drains, and enforce administrator-managed hooks with durable metadata audit.

**Architecture:** `workflow-plugin-agent` first exposes its registered provider type names; ratchet then owns one presentation/setup catalog consumed by both CLI and TUI. Later PRs add a daemon-owned ACP drain manager over existing queue/profile primitives and a managed-hook policy applied after all hook sources merge. Existing provider SDKs, daemon secrets provider, `secrets.Redactor`, ACP claim/cancel logic, and Go release matrix remain authoritative.

**Tech Stack:** Go 1.26, Bubble Tea v2/Lip Gloss, gRPC/protobuf, `workflow-plugin-agent`, `workflow/secrets`, YAML v3, `x/sys/unix`, `x/sys/windows`, GoReleaser, GitHub Actions.

**Base branch:** `master` in both repositories

---

## Scope Manifest

**PR Count:** 4
**Tasks:** 11
**Estimated Lines of Change:** ~2,300 (informational; not enforced)

**Out of scope:**
- Remote policy/fleet management, audit upload, or a management SDK.
- Arbitrary background commands, schedule syntax, detached shells, or platform schedulers.
- New provider SDK/model API implementations, secret stores, or redaction types.
- Persisting provider secrets in settings JSON, background state, logs, snapshots, or hook audit.
- Credentialed live third-party provider CI.
- VS Code-style harness optimization/self-improvement experiments; that is the next design cluster.

**PR Grouping:**

| PR # | Title | Tasks | Branch |
|------|-------|-------|--------|
| 1 | `feat(provider): expose registered types` (`workflow-plugin-agent`) | Task 1 | `feat/provider-types-contract` |
| 2 | `feat(provider): unify CLI and TUI setup` (`ratchet-cli`) | Task 2, Task 3, Task 4, Task 5 | `feat/provider-setup-unification` |
| 3 | `feat(acp): supervise background queue drains` (`ratchet-cli`) | Task 6, Task 7, Task 8 | `feat/daemon-acp-background-drain` |
| 4 | `feat(hooks): enforce managed policy` (`ratchet-cli`) | Task 9, Task 10, Task 11 | `feat/managed-hook-policy` |

**Status:** Locked 2026-07-10T11:14:35Z

## Global Execution Rules

1. Use an isolated worktree per PR. Do not disturb unrelated untracked
   `.claude/autodev-state/` content in `workflow-plugin-agent`.
2. Follow TDD in every task: add the named failing test, run it and capture the
   expected failure, implement minimally, rerun focused tests, then broaden.
3. Before every PR: `gofmt`, `git diff --check`, focused tests, `go test ./...`,
   `go vet ./...`, `golangci-lint run --new-from-rev=origin/master`, and the
   named runtime/Windows proof.
4. Before and after `gh pr create`, prove `gh --version` is at least 2.88. Add
   `copilot-pull-request-reviewer`, invoke `autodev:pr-monitoring`, resolve all
   actionable threads, and admin-merge only after local proof and green CI.
5. After each merge, tag the next unused patch version on the merge commit,
   wait for release workflow completion, and verify `gh release view <tag>`
   lists checksums and platform archives. For ratchet releases, also verify the
   Homebrew/tap update, install the released binary in an isolated prefix, and
   require `ratchet --version` to exit within five seconds with that tag.
6. Rebase each later PR on the released predecessor. Never tag a branch commit.
7. After the final PR, write a post-merge retro, update policy/parity docs and
   run `wfctl portfolio scan`; commit/push generated workspace state separately.

## Integration Matrix

| integration | status | execution proof |
|---|---|---|
| Upstream orchestrator provider registry | runtime-integrated | Task 1 real registry API test; Task 2 catalog coverage against it. |
| Upstream provider model listers | runtime-integrated | Task 3 CLI and Task 4 TUI pass typed settings into the real lister boundary with local HTTP fixtures. |
| Daemon provider RPC + existing secret provider/Redactor/registry | runtime-integrated | Tasks 4-5 prove durable operation replay/query, versioned-secret commit/rollback/cleanup, registry refresh, redactor registration, and secret absence from output/state. |
| Bubble Tea provider/model wizard | runtime-integrated | Task 4 state tests; Task 5 fresh PTY render/navigation proof. |
| Daemon gRPC background API | runtime-integrated | Task 7 started service and real client; Task 8 built binary/fixture-agent proof. |
| ACP profile/queue claim/cancel/recovery | runtime-integrated | Task 6 real stores and Task 8 restart/drift/stop proof. |
| User/project/plugin/managed hooks | runtime-integrated | Task 11 final event-time composition and side-effect assertions. |
| Unix ownership and Windows DACL policy checks | runtime-integrated | Task 9 platform helper tests and cross-build; Windows CI executes native tests. |
| Remote policy service/SDK | deferred | Explicit non-goal; local administrator files satisfy current intent. |
| Credentialed provider discovery | deferred | Requires protected external credentials; local fixture verifies request/settings wiring. |
| Harness optimization experiment loop | deferred | Tracked as the immediately following design cluster. |

### Task 1: Export the Upstream Runtime Provider-Type Contract

**Repository:** `GoCodeAlone/workflow-plugin-agent`

**Files:**
- Modify: `orchestrator/provider_registry.go`
- Modify: `orchestrator/provider_registry_test.go`

**Step 1: Write the failing contract test**

Add `TestProviderRegistryProviderTypesSortedDefensiveCopy`. Instantiate
`NewProviderRegistry(nil, nil)`, call `ProviderTypes()`, and assert:

- the exact current set contains `mock`, all API/cloud/local providers, and all
  five CLI-backed providers;
- output is lexicographically sorted;
- mutating the returned slice does not change the next result;
- adding a test factory inside the package appears on the next result.

**Step 2: Prove red**

Run: `go test ./orchestrator -run ProviderTypes -count=1`

Expected: FAIL with `reg.ProviderTypes undefined`.

**Step 3: Implement the minimal read-only API**

Add:

```go
func (r *ProviderRegistry) ProviderTypes() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	types := make([]string, 0, len(r.factories))
	for providerType := range r.factories {
		types = append(types, providerType)
	}
	slices.Sort(types)
	return types
}
```

Import `slices`. Do not expose factories or add mutation methods.

**Step 4: Verify package and repository**

```bash
gofmt -w orchestrator/provider_registry.go orchestrator/provider_registry_test.go
go test ./orchestrator -run 'ProviderTypes|ProviderRegistry' -count=1
go test ./...
go vet ./...
golangci-lint run --new-from-rev=origin/master
```

Expected: all exit 0; the focused test reports PASS and the returned type set
includes `openai_chatgpt`, `anthropic_bedrock`, and `cursor_cli`.

**Step 5: Commit, PR, merge, and release**

```bash
git add orchestrator/provider_registry.go orchestrator/provider_registry_test.go
git commit -m "feat(provider): expose registered types"
```

Follow Global Execution Rules 4-5. Record the released tag for Task 2.

Rollback: revert the merge and publish the next patch without the additive
method; ratchet remains pinned to the prior plugin until Task 2.

### Task 2: Add the Ratchet Provider Setup Catalog and Upstream Pin

**Repository:** `GoCodeAlone/ratchet-cli`

**Files:**
- Create: `internal/provider/catalog.go`
- Create: `internal/provider/catalog_test.go`
- Modify: `go.mod`
- Modify: `go.sum`

**Step 1: Write catalog validation tests**

Define tests for `Catalog`, `LookupSetup`, and `ValidateCatalog`. The runtime
coverage test must instantiate the released
`orchestrator.NewProviderRegistry(nil, nil)`, remove only `mock` and ephemeral
`test`, and require every remaining type to resolve to one catalog entry.

Also assert:

- canonical visible entries are unique and aliases do not collide;
- `bedrock` is visible while `anthropic_bedrock` resolves as an accepted alias;
- secret fields never use `SettingField` and no settings field is marked secret;
- required base URLs/settings and manual-model fallback are explicit;
- `Catalog()` returns a defensive copy.

**Step 2: Prove red**

Run: `go test ./internal/provider -run 'Catalog|LookupSetup' -count=1`

Expected: FAIL with `undefined: Catalog`.

**Step 3: Introduce the catalog types**

Implement immutable descriptors equivalent to:

```go
type Category string
type AuthStrategy string
type SetupStrategy string
type ModelStrategy string

type SettingField struct {
	Key, Label, Placeholder, Default string
	Required                         bool
	Choices                          []string
}

type SetupEntry struct {
	Type, DisplayName, Description string
	Aliases                         []string
	Category                        Category
	Auth                            AuthStrategy
	Setup                           SetupStrategy
	Model                           ModelStrategy
	APIKeyEnv, DefaultBaseURL       string
	BaseURLRequired                 bool
	Settings                        []SettingField
	InstallHint, AuthHint           string
	ModelBehavior                   string
	CredentialBoundary              string
}
```

Strategies must cover API key, Anthropic auth choice, GitHub/Copilot device
auth, OpenAI ChatGPT device auth, no-auth local endpoint, CLI-native auth,
dynamic model list, manual model, Ollama pull, and CLI-owned model.

Catalog entries cover:

| group | canonical types |
|---|---|
| API | `anthropic`, `openai`, `openrouter`, `cohere`, `copilot_models`, `gemini` |
| Compatible | `openai_compatible`, `anthropic_compatible`, `custom` |
| Subscription/device | `openai_chatgpt`, `copilot` |
| Cloud | `openai_azure`, `anthropic_foundry`, `anthropic_vertex`, `bedrock` |
| Local | `ollama`, `llama_cpp` |
| CLI-backed | `claude_code`, `copilot_cli`, `codex_cli`, `gemini_cli`, `cursor_cli` |

Cloud settings are non-secret identifiers only: Azure
`resource/deployment_name/api_version`, Foundry `resource`, Vertex
`project_id/region`, and Bedrock `access_key_id/region`. Their single secret
credential uses the existing API-key field; session/Entra tokens are not stored
in settings JSON.

**Step 4: Pin the released plugin and verify skew**

Run: `go get github.com/GoCodeAlone/workflow-plugin-agent@<task-1-tag>` followed
by `go mod tidy`.

Expected: `go.mod` pins the released version directly and no replace directive
or unrelated dependency upgrade appears.

Run: `go list -m github.com/GoCodeAlone/workflow-plugin-agent`.

Expected: exactly the Task 1 tag.

**Step 5: Verify and commit**

```bash
gofmt -w internal/provider/catalog.go internal/provider/catalog_test.go
go test ./internal/provider -run 'Catalog|LookupSetup' -count=1
go test ./internal/provider ./internal/daemon -run 'Catalog|ProviderRegistry' -count=1
git add internal/provider/catalog.go internal/provider/catalog_test.go go.mod go.sum
git commit -m "feat(provider): centralize setup catalog"
```

Expected: all tests PASS and `git diff --check` is empty.

Rollback: pin the prior plugin release, remove catalog files, run `go mod tidy`,
and relaunch the existing provider CLI/TUI paths.

### Task 3: Make Provider CLI Consume the Catalog

**Files:**
- Modify: `cmd/ratchet/cmd_provider.go`
- Modify: `cmd/ratchet/cmd_provider_test.go`

**Step 1: Add failing CLI/catalog conformance tests**

Add tests that:

- setup list/guide output contains every visible catalog entry and accepted
  alias, with stable human and JSON fields;
- `provider add` input dispatch uses catalog base-URL/settings/model strategy;
- Bedrock, Azure, Foundry, Vertex, compatible/custom, ChatGPT, and CLI-backed
  providers select the expected path;
- settings-aware discovery receives exact non-secret settings;
- failed/empty discovery permits manual input only when declared;
- API key/token values never appear in output or serialized settings.

Extract an input-driven helper rather than testing `os.Exit` branches.

**Step 2: Prove red against the duplicate table**

Run: `go test ./cmd/ratchet -run 'Provider.*Catalog|Provider.*Strategy|ProviderSetupGuide' -count=1`

Expected: FAIL because current setup guides contain only seven entries and add
dispatch is hardcoded.

**Step 3: Replace CLI-owned provider metadata**

Delete `providerSetupGuides`. Adapt guide list/guide rendering from
`provider.Catalog()`. Introduce a small CLI adapter that accepts scanner,
secret prompter, model lister, and writer dependencies, then returns:

```go
type providerSetupInput struct {
	APIKey, BaseURL, Model string
	Settings               map[string]string
}
```

Keep provider network/auth calls in existing packages. Preserve existing
`provider setup <alias>` compatibility while allowing every catalog alias in
guide/add flows. Reject any attempt to serialize a secret as settings.

**Step 4: Verify the real CLI surface**

```bash
gofmt -w cmd/ratchet/cmd_provider.go cmd/ratchet/cmd_provider_test.go
go test ./cmd/ratchet -run 'Provider.*Catalog|Provider.*Strategy|ProviderSetupGuide|ProviderModelSelection' -count=1
go run ./cmd/ratchet provider setup list --json
go run ./cmd/ratchet provider setup guide bedrock --json
```

Expected: tests PASS; JSON list includes `bedrock`, `openai_azure`,
`openai_chatgpt`, and `cursor_cli`; guide exits 0 and contains no credential
value.

**Step 5: Commit**

```bash
git add cmd/ratchet/cmd_provider.go cmd/ratchet/cmd_provider_test.go
git commit -m "refactor(provider): drive CLI from catalog"
```

### Task 4: Rebuild the TUI Wizard Around the Catalog

**Files:**
- Modify: `internal/proto/ratchet.proto`
- Modify generated: `internal/proto/ratchet.pb.go`
- Modify generated: `internal/proto/ratchet_grpc.pb.go`
- Modify: `.github/workflows/ci.yml`
- Modify: `go.mod`
- Modify: `internal/releaseguard/workflow_test.go`
- Modify: `internal/daemon/engine.go`
- Modify: `internal/daemon/daemon.go`
- Create: `internal/daemon/provider_operations.go`
- Create: `internal/daemon/provider_operations_test.go`
- Create: `internal/daemon/provider_cleanup.go`
- Create: `internal/daemon/provider_cleanup_test.go`
- Create: `internal/daemon/lock_unix.go`
- Create: `internal/daemon/lock_windows.go`
- Create: `internal/daemon/lock_unix_test.go`
- Create: `internal/daemon/lock_windows_test.go`
- Modify: `internal/daemon/service.go`
- Modify: `internal/daemon/integration_test.go`
- Modify: `internal/daemon/testharness_test.go`
- Modify: `internal/daemon/secret_redactor_test.go`
- Modify: `internal/client/client.go`
- Create: `internal/client/provider_save_test.go`
- Modify: `cmd/ratchet/cmd_provider.go`
- Modify: `cmd/ratchet/cmd_provider_test.go`
- Modify: `internal/tui/app.go`
- Modify: `internal/tui/app_session_tree_test.go`
- Modify: `internal/tui/pages/onboarding.go`
- Create: `internal/tui/pages/onboarding_test.go`

**Step 1: Task 4 acceptance checklist**

Use this list as completion criteria, not an instruction to create every test
up front. Each checkpoint below adds only its own tests so Go package-wide test
compilation is valid. The completed checkpoints must prove:

- provider selection is catalog-derived, category-labeled, scrollable, and
  filterable without changing the frame width;
- `esc` clears filter first, then returns to the previous step;
- settings and base URL survive a recoverable discovery error;
- dynamic discovery receives settings; empty/error results show manual entry;
- ChatGPT device auth and CLI-native providers reach review without API-key
  prompts;
- every visible catalog entry can be selected and reaches its declared first
  setup step, and a source guard rejects a second TUI-owned provider table;
- review omits secret values and masks credential presence;
- submit sends alias/type/model/base URL/settings once;
- every CLI/TUI save calls the dedicated durable RPC with a canonical UUID,
  uses a signal-aware 30-second call, and reconciles separately for 10 seconds;
- operation replay is unconditional first-write-wins for the same alias, another
  alias conflicts, and operation RPCs omit credentials/settings/base URLs/errors;
- SQL rollback preserves the active secret; commit updates registry/redactor
  before success; old-secret cleanup is durable and retryable;
- pending rows reserve secret keys; daemon-owned per-alias workers serialize
  save/remove through terminal state while cleanup workers recheck references;
- one ID is admitted per alias; same-ID calls attach, another receives
  `AliasBusy` without credential retention, and ownership retires at terminal;
- a blocking fake secret provider proves client timeout leaves a live pending
  worker/reservation, same-ID attachment, other-ID busy rejection, unrelated
  aliases proceeding, and deterministic restart recovery;
- cleanup rows deduplicate by secret name and a two-worker pool proves bounded
  concurrency/backoff and idle retirement;
- worker panic becomes classified durable failure and releases alias/cleanup
  ownership; poison cleanup rows cannot starve later due rows;
- a short provider-row critical section orders save apply/finalize with remove,
  default, and model updates without enclosing secret-provider calls;
- an OS-level lock is acquired before PID/socket cleanup, migration, or secret
  reconciliation and retained for daemon lifetime on Unix and Windows;
- startup finalizes applied operations, fails inherited pending, journals only
  unreferenced reserved-prefix secrets before RPC acceptance, then asynchronously
  deletes through the bounded pool; terminal operations prune after 24 hours;
- ambiguous save responses poll pending/not-found operation state with bounded
  backoff; commit resolves, failed reports a class, and unresolved pauses exit;
- nil daemon responses and whitespace credentials/base URLs fail safely;
- native Windows CI executes lock contention, graceful release, and process-exit
  release tests; cross-build remains additional evidence;
- views at 80x24 and 120x40 contain no overflow and always show actionable
  navigation/help text.

**Checkpoint 4A: Generate the dedicated RPC contract**

Add failing `TestProviderOperationRPCContract` and client compile-contract tests
for `CommitProviderSave`, `GetProviderOperation`, operation states, timestamps,
classified failure, and the narrow result (`alias/type/model/is_default`, no
base URL/settings/credential/error). Run:

```bash
go test ./internal/daemon ./internal/client -run 'ProviderOperationRPCContract|ProviderSaveClient' -count=1
```

Expected red: compile failure with undefined operation messages/RPCs. Add the
protobuf messages and RPCs, run `make proto`, implement client adapters, rerun
the command to PASS, then commit `feat(provider): add durable save contract`.

```bash
gofmt -w internal/client/client.go internal/client/provider_save_test.go
git add internal/proto/ratchet.proto internal/proto/ratchet.pb.go internal/proto/ratchet_grpc.pb.go internal/client/client.go internal/client/provider_save_test.go internal/daemon/integration_test.go
git commit -m "feat(provider): add durable save contract"
```

**Checkpoint 4B: Implement schema, operation execution, and cleanup**

Write these tests before implementation:

- `TestProviderOperationSchemaUpgradesLegacyDatabase` creates the previous
  `llm_providers` schema, runs initialization twice, and asserts both new tables;
- `TestProviderOperationSchemaFailureStopsStartup` creates a conflicting view
  and requires initialization failure before service construction;
- `TestCommitProviderSaveRollbackPreservesActiveSecret` forces SQL apply failure
  after a new secret write and proves the prior pointer/value remains active;
- `TestCommitProviderSaveReplayAliasBusyAndConflict` covers same-ID attachment,
  different-ID same-alias `AliasBusy` without a row/credential, terminal replay,
  and cross-alias UUID conflict;
- `TestProviderOperationBlockingSecretAdmissionAndRestart` uses a provider whose
  `Set` ignores context: caller times out, reservation stays pending, unrelated
  alias succeeds, replacement ID is busy, and restart classifies recovery;
- `TestProviderOperationWorkerPanicReleasesOwnership` proves classified failure
  without raw panic text and accepts a later operation;
- `TestProviderCleanupDispatcherFairness` proves unique rows, at most two active
  deletes, persisted `next_attempt_at`, poison-row slot release, and idle wakeup;
- `TestProviderMutationOrdering` overlaps save apply/finalize with remove,
  default, and model update and asserts linearized final state;
- `TestProviderOperationPayloadContainsNoSensitiveFields` checks SQL rows,
  protobuf JSON, status, and logs against credential/base-URL/settings sentinels.

Run red:

```bash
go test ./internal/daemon -run 'ProviderOperationSchema|CommitProviderSave|ProviderOperationBlocking|ProviderOperationWorkerPanic|ProviderCleanupDispatcher|ProviderMutationOrdering|ProviderOperationPayload' -count=1
```

Expected: named tests FAIL because the required tables/executor/dispatcher do
not exist and legacy `AddProvider` mutates alias-stable secrets before commit.

Implement ADR 0006 in `provider_operations.go` and `provider_cleanup.go`.
Required migrations fail startup. `CommitProviderSave` requires a canonical
UUID; legacy `AddProvider` delegates with a server UUID. Journal pending with a
server-only `provider-v2-<unix>-<uuid>` reservation before `Set`. Admit one ID
per alias; same-ID callers attach and another ID fails busy without retention.
Per-alias daemon workers continue non-cancellable calls after caller timeout.
SQL atomically switches the pointer, queues the old key, and stores `applied`;
a daemon-context/query-assisted finalizer registers Redactor, invalidates cache,
and marks committed. A short row mutex orders apply/finalize/remove/default/model
without enclosing secret calls. Panic guards classify failure and retire owners.

Startup `List` is fail-stop, resolves inherited pending/applied rows, journals
unreferenced reserved keys, and then starts one due-row dispatcher feeding two
short cleanup workers. Cleanup rows key by secret name, persist
`next_attempt_at`, recheck provider/nonterminal references, and never sleep while
holding a worker slot. Terminal operations retain 24 hours. Rerun the red
command to PASS and commit `feat(provider): make saves durable`.

```bash
gofmt -w internal/daemon/engine.go internal/daemon/service.go internal/daemon/integration_test.go internal/daemon/testharness_test.go internal/daemon/secret_redactor_test.go internal/daemon/provider_operations.go internal/daemon/provider_operations_test.go internal/daemon/provider_cleanup.go internal/daemon/provider_cleanup_test.go
git add internal/daemon/engine.go internal/daemon/service.go internal/daemon/integration_test.go internal/daemon/testharness_test.go internal/daemon/secret_redactor_test.go internal/daemon/provider_operations.go internal/daemon/provider_operations_test.go internal/daemon/provider_cleanup.go internal/daemon/provider_cleanup_test.go
git commit -m "feat(provider): make saves durable"
```

**Checkpoint 4C: Enforce single-daemon ownership on Unix and Windows**

Add `TestDaemonLockExcludesSecondOwner`,
`TestDaemonLockReleasesAfterClose`, and helper-process
`TestDaemonLockReleasesAfterProcessExit` in both platform test files. Run:

```bash
go test ./internal/daemon -run 'DaemonLock' -count=1
```

Expected red: undefined lock constructor. Implement owner-only lock-file open,
nonblocking Unix `flock`, Windows `LockFileEx`, and release-on-close. Acquire
before `IsRunning`, socket cleanup, PID write, migration, or reconciliation and
hold through daemon lifetime. Add exact native CI steps:

```yaml
- name: Run Windows daemon lock tests
  run: go test ./internal/daemon -run 'DaemonLock' -count=1
- name: Run Windows ConPTY provider save
  run: go test -tags tui_smoke ./internal/tui -run 'WindowsConPTYProviderSave' -count=1 -timeout=10m
```

Add failing `TestCIRequiresWindowsProviderDurability` before editing the workflow:

```bash
go test ./internal/releaseguard -run 'CIRequiresWindowsProviderDurability' -count=1
```

Expected red: the `windows-conpty-smoke` job lacks both exact run steps. After
the YAML edit, the releaseguard test must PASS.

Local Unix tests must PASS; Windows tests are compile-checked locally and must
run natively in PR CI. Commit `fix(daemon): enforce exclusive ownership`.

```bash
gofmt -w internal/daemon/daemon.go internal/daemon/lock_unix.go internal/daemon/lock_windows.go internal/daemon/lock_unix_test.go internal/daemon/lock_windows_test.go internal/releaseguard/workflow_test.go
git add internal/daemon/daemon.go internal/daemon/lock_unix.go internal/daemon/lock_windows.go internal/daemon/lock_unix_test.go internal/daemon/lock_windows_test.go internal/releaseguard/workflow_test.go .github/workflows/ci.yml go.mod
git commit -m "fix(daemon): enforce exclusive ownership"
```

**Checkpoint 4D: Route every CLI save through durable operations**

Add failing `TestProviderDurableSaveDeadlineAndReconciliation`,
`TestProviderDurableSaveInterruptAndForceExit`,
`TestProviderOperationStatusCommand`,
`TestProviderAddJSONIncludesStableOperationID`, and a source guard enumerating
generic, ChatGPT, Ollama, and CLI-native writers. Run:

```bash
go test ./cmd/ratchet -run 'Provider.*Operation|ProviderDurableSave' -count=1
```

Expected red: missing helper/status command and direct `AddProvider` writers.
Implement one helper using a canonical UUID, signal-aware 30-second RPC, and a
separate 10-second operation poll. First interrupt prints reconciliation status;
second exits nonzero and prints the ID. Add
`provider operation <id> [--json]`. Successful generic
`provider add ... --json` emits an object containing stable `operation_id` and
provider fields; human output also prints the ID. Nil success responses fail
cleanly. Rerun to PASS and commit `refactor(provider): use durable saves`.

```bash
gofmt -w cmd/ratchet/cmd_provider.go cmd/ratchet/cmd_provider_test.go
git add cmd/ratchet/cmd_provider.go cmd/ratchet/cmd_provider_test.go
git commit -m "refactor(provider): use durable saves"
```

**Checkpoint 4E: Finish the catalog-driven TUI state machine**

Add `TestOnboardingDurableSubmission`,
`TestOnboardingPendingReconciliation`,
`TestOnboardingUnresolvedExitDeferral`, and
`TestAppOnboardingCommittedProviderRouting`, then run the state tests:

```bash
go test ./internal/tui/pages -run 'Onboarding|ProviderCatalog' -count=1
go test ./internal/tui -run 'Provider|Onboarding|SlashExit' -count=1 -timeout=10m
```

Expected red: missing `CommitProviderSave` routing, operation-state
reconciliation, Ctrl+C wait, and committed app-state propagation. Implement
catalog filter/viewport/settings/manual model,
specialized auth/CLI/Ollama strategies, bounded render/help, and one
`CommitProviderSave` submission. Review confirmation is the commit boundary;
navigation never deletes an upsert. Reconciliation handles pending/applied/
committed/failed/unresolved and preserves committed provider state in the app.
Rerun both commands to PASS and commit
`feat(tui): unify provider setup wizard`.

```bash
gofmt -w internal/tui/app.go internal/tui/app_session_tree_test.go internal/tui/pages/onboarding.go internal/tui/pages/onboarding_test.go
git add internal/tui/app.go internal/tui/app_session_tree_test.go internal/tui/pages/onboarding.go internal/tui/pages/onboarding_test.go
git commit -m "feat(tui): unify provider setup wizard"
```

**Step 4: Verify state and render behavior**

```bash
make proto
gofmt -w internal/daemon/engine.go internal/daemon/daemon.go internal/daemon/provider_operations.go internal/daemon/provider_operations_test.go internal/daemon/provider_cleanup.go internal/daemon/provider_cleanup_test.go internal/daemon/lock_unix.go internal/daemon/lock_windows.go internal/daemon/lock_unix_test.go internal/daemon/lock_windows_test.go internal/daemon/service.go internal/daemon/integration_test.go internal/client/client.go internal/client/provider_save_test.go cmd/ratchet/cmd_provider.go cmd/ratchet/cmd_provider_test.go internal/tui/app.go internal/tui/app_session_tree_test.go internal/tui/pages/onboarding.go internal/tui/pages/onboarding_test.go
go test ./internal/daemon -run 'ProviderOperation|ProviderSecret|ProviderCRUD|DaemonLock' -count=1
go test ./cmd/ratchet -run 'Provider.*Operation|Provider.*Catalog|ProviderSetupGuide|ProviderModelSelection' -count=1
go test ./internal/tui/pages -run 'Onboarding|ProviderCatalog' -count=1
go test ./internal/tui -run 'Provider|Onboarding|SlashExit' -count=1 -timeout=10m
go test ./internal/releaseguard -run 'CIRequiresWindowsProviderDurability' -count=1
```

Expected: PASS; daemon tests prove replay/rollback/restart/cleanup and registry
resolution; all current save callers carry UUIDs; render tests report all lines
within configured widths and secret sentinels absent.

**Step 5: Verify checkpoint commits and rollback**

```bash
git status --short
git log --oneline --grep='provider\|daemon.*ownership\|tui.*provider'
```

Expected: clean worktree and commits for contract, durable engine, daemon lock,
CLI routing, and TUI state. Rollback within PR 2: revert those checkpoint commits
in reverse order; additive tables/lock file remain harmless to the parent binary,
then run the Task 5 downgrade smoke before merge.

### Task 5: Prove, Document, Merge, and Release Unified Provider Setup

**Files:**
- Modify: `.github/workflows/ci.yml`
- Modify: `internal/releaseguard/workflow_test.go`
- Modify: `cmd/ratchet-tui-smoke/main.go`
- Modify: `cmd/ratchet-tui-smoke/main_windows.go`
- Modify: `cmd/ratchet/harness_smoke_unix_test.go`
- Create: `cmd/ratchet/provider_downgrade_smoke_unix_test.go`
- Modify: `internal/daemon/service_tui_smoke.go`
- Modify: `internal/daemon/service_tui_smoke_test.go`
- Modify: `internal/tui/tui_binary_smoke_unix_test.go`
- Modify: `internal/tui/tui_binary_smoke_windows_test.go`
- Modify: `README.md`
- Modify: `docs/harness-emulation.md`
- Modify: `docs/competitor-parity.md`
- Modify: `docs/policy-matrix.md`
- Modify: `cmd/ratchet/harness_docs_test.go`

**Step 1: Add a fresh-process TUI integration proof**

First add failing tests named `TestTUIBinarySmokeProviderSave`,
`TestTUIBinaryWindowsConPTYProviderSave`,
`TestTUISmokeProviderSavePersistsSecretBoundary`,
`TestHarnessSmokeDurableProviderSaveRestart`, and
`TestHarnessSmokeDurableProviderDowngrade`. Run red:

```bash
go test -tags tui_smoke ./internal/daemon -run 'TUISmokeProviderSave' -count=1
go test -tags tui_smoke ./internal/tui -run 'TUIBinary.*ProviderSave' -count=1 -timeout=12m
go test ./cmd/ratchet -run 'HarnessSmokeDurableProvider' -count=1 -timeout=12m
RATCHET_DOWNGRADE_BASE_SHA=8cb5602166ffe529a0f05101dff583bad0919415 \
  go test ./cmd/ratchet -run 'HarnessSmokeDurableProviderDowngrade' -count=1 -timeout=12m
```

Expected: FAIL because smoke state is in-memory/discarded, the binaries do not
accept a caller-owned root, the production lifecycle does not expose operation
status/restart proof, and the parent-version harness does not exist.

Add a dedicated PTY/ConPTY scenario that launches the smoke binary, submits
`/provider add`, observes catalog entries beyond the former five (at least
`Amazon Bedrock` and `Custom endpoint`), filters to Bedrock, enters non-secret
settings, and backs out. Then configure Custom endpoint against a local
OpenAI-compatible HTTP fixture, enter a sentinel credential, discover/select a
model, and complete a successful save/test through `CommitProviderSave`.
Run the smoke daemon on a caller-supplied temporary root with persistent SQLite
and the existing file secret provider so the test can inspect the real state.
Exit in the existing fresh shutdown test; do not add shutdown responsibility to
the long all-surfaces test.

**Step 2: Add daemon secret-boundary proof**

Through the smoke daemon/client boundary, submit a provider with a sentinel
credential and settings. Assert the provider row/settings and rendered review
contain no sentinel while the existing secrets provider resolves it and the
existing redactor redacts it.

For the executable TUI save, assert the operation row is committed, provider row
references a `provider-v2-` key, the existing file provider resolves the
sentinel, the redactor suppresses it in a daemon assertion, and no PTY/ConPTY,
operation, provider, database-status, or rendered-review output contains it.

Restart a real test daemon around persisted SQLite/secret state. Prove committed
operation query/replay survives, inherited pending becomes classified failed,
unreferenced `provider-v2-` secrets are swept before RPC acceptance, referenced
versions remain, cleanup failures stay queued and retry, provider removal queues
its current version, and the real registry resolves the new credential after an
upsert. Verify operation rows/RPC/status/log snapshots contain no sentinel.

Extend the production-binary harness with a local OpenAI-compatible fixture.
Build the current production command into `t.TempDir()` with the existing
`buildRatchetSmokeBinary` helper; never read/write a shared `dist` artifact from
the test. Launch that binary under isolated HOME/state, save through the CLI,
parse its operation ID, invoke `provider operation <id> --json`, restart the
production daemon, list/test the provider, and shut down through RPC. The
downgrade harness requires `RATCHET_DOWNGRADE_BASE_SHA` (CI checkout must use
`fetch-depth: 0`). Without the environment variable, ordinary
and post-merge suites call `t.Skip` with an explicit opt-in message. When set,
the test logs base/current SHAs, requires they differ,
and requires `git show <base>:internal/proto/ratchet.proto` not to contain
`CommitProviderSave`; otherwise it fails as vacuous. It adds a detached worktree,
builds both binaries into `t.TempDir()`, stops the current daemon, observes
socket/PID removal and lock release, then starts the parent and proves it resolves
the versioned secret against the same local fixture. The parent then upserts the
same alias with a second sentinel through legacy `AddProvider`; after another
clean stop, the current binary restarts, resolves the parent-written active
secret, journals and retires only the now-unreferenced v2 version, and performs
one more durable save that retires the legacy key without exposing any sentinel.
Assert provider pointers, operation states, active credentials, and cleanup rows
after each transition. Use a named bounded convergence helper, not sleeps, to
poll terminal cleanup rows and `secrets.Provider.List`; on timeout print only
row state, failure class, key category, and counts, never raw names or values.
Seed and snapshot an unrelated secret before the lifecycle. Compare sorted sets
because `List` order is unspecified: classify only the test's exact legacy key
plus reserved `provider-v2-` keys, require unrelated keys unchanged, and reject
unexpected provider keys. After re-upgrade, require the original v2 key absent
and the active legacy key to be the only provider key. After the final durable
save, require the legacy key absent and the final active v2 key to be the only
provider key. Cleanup always removes the temporary worktree.

Update `tui-smoke` checkout to `fetch-depth: 0`. Keep the restart proof
evergreen on every event and use the verified pre-RPC compatibility revision for
the mixed-version proof. Do not derive this boundary from an event predecessor
or PR base: stacked and future PR bases can already contain the RPC.

```yaml
- name: Run production provider durability smoke
  run: go test ./cmd/ratchet -run 'HarnessSmokeDurableProviderSaveRestart' -count=1 -timeout=12m
- name: Run production provider downgrade smoke
  shell: bash
  env:
    RATCHET_DOWNGRADE_BASE_SHA: 8cb5602166ffe529a0f05101dff583bad0919415
  run: |
    log=$(mktemp)
    trap 'rm -f "$log"' EXIT
    if ! go test -json ./cmd/ratchet -run '^TestHarnessSmokeDurableProviderDowngrade$' -count=1 -timeout=12m >"$log"; then
      jq -c 'select(.Action == "fail" or .Action == "skip") | {Action,Package,Elapsed}' "$log" >&2 || true
      exit 1
    fi
    jq -e 'select(.Action == "pass" and .Test == "TestHarnessSmokeDurableProviderDowngrade")' "$log" >/dev/null
```

The `windows-conpty-smoke` job must retain the Task 4 exact `DaemonLock` and
`WindowsConPTYProviderSave` commands. Extend
`TestCIRequiresWindowsProviderDurability` to assert the Linux checkout depth and
both production commands, the exact pinned `RATCHET_DOWNGRADE_BASE_SHA`, and both
Windows commands. It must reject `github.event.before` and
`github.event.pull_request.base.sha` so later events cannot silently move the
compatibility boundary to a revision that already contains `CommitProviderSave`.
It must also require `shell: bash`, an owned `mktemp` transcript with cleanup,
the anchored JSON test selector whose exit is checked before parsing, and an
exact `jq` pass-event assertion so a failed, renamed, or deleted test cannot
satisfy CI. It must reject raw transcript output such as `cat "$log"` and
require the fixed failure projection that excludes `.Output`.

**Step 3: Update human documentation and guards**

Document one setup flow, category/provider matrix, CLI/TUI parity guarantee,
manual-model fallback, compatible/cloud settings, and secret custody. Remove
claims that the TUI supports only five providers. Keep background drain and
managed hooks deferred until their PRs merge.

**Step 4: Run full and runtime verification**

```bash
go test ./internal/provider ./cmd/ratchet ./internal/tui/pages -count=1
go test ./internal/daemon -run 'ProviderOperation|ProviderSecret|TUISmoke' -count=1
go test ./internal/tui -run 'ProviderSetup|TUIBinarySmoke' -count=1 -timeout=12m
go test ./cmd/ratchet -run HarnessEmulationDocs -count=1
go test ./cmd/ratchet -run 'HarnessSmokeDurableProvider' -count=1 -timeout=12m
(
  log=$(mktemp)
  trap 'rm -f "$log"' EXIT
  export RATCHET_DOWNGRADE_BASE_SHA=8cb5602166ffe529a0f05101dff583bad0919415
  if ! go test -json ./cmd/ratchet -run '^TestHarnessSmokeDurableProviderDowngrade$' -count=1 -timeout=12m >"$log"; then
    jq -c 'select(.Action == "fail" or .Action == "skip") | {Action,Package,Elapsed}' "$log" >&2 || true
    exit 1
  fi
  jq -e 'select(.Action == "pass" and .Test == "TestHarnessSmokeDurableProviderDowngrade")' "$log" >/dev/null
)
go test ./...
go vet ./...
golangci-lint run --new-from-rev=origin/master
GOOS=windows GOARCH=amd64 go build ./cmd/ratchet
go build -o ./dist/ratchet ./cmd/ratchet
./dist/ratchet --version
go test ./cmd/ratchet -run HarnessSmokeVersionHelpAndDaemonStatus -count=1
```

Expected: all exit 0; TUI output includes Bedrock/custom choices; Windows build
succeeds; version command prints the development version and exits without
starting the daemon. Every new downgrade-harness failure message containing
subprocess state passes paths and all credential sentinels through the existing
`harnessredact` helper; the CI/local wrapper never prints raw JSON `Output`
events.

**Step 5: Commit and complete PR 2**

```bash
git add .github/workflows/ci.yml internal/releaseguard/workflow_test.go cmd/ratchet-tui-smoke/main.go cmd/ratchet-tui-smoke/main_windows.go cmd/ratchet/harness_smoke_unix_test.go cmd/ratchet/provider_downgrade_smoke_unix_test.go internal/daemon/service_tui_smoke.go internal/daemon/service_tui_smoke_test.go internal/tui/tui_binary_smoke_unix_test.go internal/tui/tui_binary_smoke_windows_test.go README.md docs/harness-emulation.md docs/competitor-parity.md docs/policy-matrix.md cmd/ratchet/harness_docs_test.go
git commit -m "docs: explain unified provider setup"
```

Follow Global Execution Rules 4-5 and verify the released Homebrew binary opens
`provider setup list --json` with the full catalog.

Rollback: before reverting the commit that owns the compatibility harness, run
the exact pinned Step 4 downgrade command and require its exact JSON pass event.
Then run
`ratchet daemon stop`, wait for socket/PID removal and lock release,
inspect/allow pending cleanup to settle when available, revert PR 2 and the
plugin pin, and rebuild the parent binary. Additive operation/cleanup tables
remain ignored and schema-compatible; never use a no-match `go test -run` after
the revert as rollback evidence.

### Task 6: Bind ACP Profile Trust and Build the Background Policy Manager

**Files:**
- Modify: `internal/acpclient/profiles.go`
- Modify: `internal/acpclient/profiles_test.go`
- Modify: `internal/acpclient/spec.go`
- Modify: `internal/acpclient/spec_test.go`
- Create: `internal/acpclient/background.go`
- Create: `internal/acpclient/background_test.go`
- Create: `internal/acpclient/background_audit.go`
- Create: `internal/acpclient/background_audit_test.go`

**Step 1: Add failing trust and persistence tests**

Add tests proving:

- `WithProfiles` rejects/skips a profile where `Trusted` is true but
  `Hash != DescriptorHash()`;
- command, args, env-key names, or cwd drift invalidate stored trust;
- background policy persists session ID, profile name, descriptor hash,
  acknowledgement timestamp/version, state, and outcome, but no prompt,
  response, argv, environment value, or credential;
- start is idempotent only for the same active policy;
- profile drift blocks start/resume; worker error moves to `error` without
  retry; stop persists disabled before cancellation;
- state files are atomic and owner-only.
- start, resume, block, error, and stop append owner-only JSONL records with
  session/profile/hash/outcome metadata and no prompt, response, argv,
  environment value, or credential.

Inject clock, watcher, profile resolver, and persistence path.

**Step 2: Prove red**

Run: `go test ./internal/acpclient -run 'Profile.*Trust|Background' -count=1`

Expected: FAIL because descriptor trust is not checked and background types do
not exist.

**Step 3: Harden shared profile resolution**

Add `Profile.TrustValid() bool` returning a constant-time comparison of stored
`Hash` and `DescriptorHash()` when `Trusted` is true. Use it in
`Registry.WithProfiles`. Do not silently rewrite a non-empty stale hash during
load.

**Step 4: Implement the manager**

Create `BackgroundPolicy`, `BackgroundStatus`, `BackgroundStore`,
`BackgroundAudit`, and `BackgroundManager`. The manager owns one context/cancel
pair per ACP client session ID and delegates work to `WatchQueue`. Persist and
append classified audit before launch and before stop cancellation. Resume only
enabled, acknowledged, trust-valid entries with matching pinned hash. Built-ins
pin `AgentSpec.Fingerprint`; stored profiles pin `Profile.DescriptorHash`.

On terminal worker error, persist `error` and return; never hot-loop. Redacted
audit/status carries outcome class only.

**Step 5: Verify and commit**

```bash
gofmt -w internal/acpclient/profiles.go internal/acpclient/profiles_test.go internal/acpclient/spec.go internal/acpclient/spec_test.go internal/acpclient/background.go internal/acpclient/background_test.go internal/acpclient/background_audit.go internal/acpclient/background_audit_test.go
go test ./internal/acpclient -run 'Profile.*Trust|Background|WatchQueue|DrainQueue' -count=1
git add internal/acpclient
git commit -m "feat(acp): manage trusted background drains"
```

Expected: PASS; a watcher counter remains exactly one after an error and zero
after drifted restart.

### Task 7: Wire Background Drains Through Proto, Daemon, and Client

**Files:**
- Modify: `internal/proto/ratchet.proto`
- Regenerate: `internal/proto/ratchet.pb.go`
- Regenerate: `internal/proto/ratchet_grpc.pb.go`
- Modify: `internal/daemon/service.go`
- Modify: `internal/daemon/testharness_test.go`
- Modify: `internal/daemon/service_tui_smoke.go`
- Create: `internal/daemon/background_drain_test.go`
- Modify: `internal/client/client.go`
- Modify: `internal/client/client_test.go`

**Step 1: Write failing API/lifecycle tests**

Specify four RPCs: `StartACPBackgroundDrain`, `StopACPBackgroundDrain`,
`GetACPBackgroundDrain`, and `ListACPBackgroundDrains`. Messages expose session
ID, profile, pinned hash, state, acknowledgement/start/update timestamps, and
last outcome class only.

Tests require:

- real client reaches a started test service and observes `running`;
- missing acknowledgement returns `InvalidArgument`;
- unknown session/profile returns `NotFound`;
- drifted/untrusted profile returns `FailedPrecondition` without a watcher;
- service shutdown cancels workers and waits for completion;
- TUI smoke/test constructors inject a disabled manager and never read default
  host background state.

**Step 2: Prove red**

Run: `go test ./internal/daemon ./internal/client -run 'BackgroundDrain' -count=1`

Expected: FAIL with missing proto/client/service symbols.

**Step 3: Add proto and regenerate**

Edit the service/messages, then run `make proto`.

Expected: generated client/server interfaces contain all four RPCs and no
manual edits exist in generated files.

**Step 4: Wire explicit ownership**

Add a narrow manager interface to `Service`. Production construction creates
and resumes the default manager only after daemon state/profile stores load.
Test and smoke constructors pass a disabled manager. `Shutdown` disables no
policies but cancels/waits active goroutines; restart re-evaluates persisted
enabled policy.

Map domain errors to stable gRPC codes. Client methods only translate protobuf
records; they never launch local workers.

**Step 5: Exercise the real gRPC boundary**

```bash
gofmt -w internal/daemon/service.go internal/daemon/background_drain_test.go internal/client/client.go internal/client/client_test.go
go test ./internal/daemon ./internal/client -run 'BackgroundDrain' -count=1
go test ./internal/daemon -run 'Shutdown|TUISmokeService' -count=1
```

Expected: PASS; start response is `running`, stop response is `disabled`, and
shutdown completes with zero active workers.

**Step 6: Commit**

```bash
git add internal/proto internal/daemon internal/client
git commit -m "feat(daemon): expose ACP background drains"
```

Rollback: revert the RPC/service commit and rebuild; Task 6 state remains inert
because no production manager starts it.

### Task 8: Add Background CLI, Runtime Proof, Docs, Merge, and Release

**Files:**
- Modify: `cmd/ratchet/cmd_acp_client.go`
- Modify: `cmd/ratchet/cmd_acp_client_test.go`
- Modify: `cmd/ratchet/cli_integration_test.go`
- Modify: `README.md`
- Modify: `docs/harness-emulation.md`
- Modify: `docs/competitor-parity.md`
- Modify: `docs/policy-matrix.md`
- Modify: `cmd/ratchet/harness_docs_test.go`

**Step 1: Add failing parser/output tests**

Cover:

```text
ratchet acp client background start <session-id> --agent <profile> --acknowledge-unattended [--json]
ratchet acp client background status [<session-id>] [--json]
ratchet acp client background stop <session-id> [--json]
```

Require explicit acknowledgement, no `--command`/argv options, stable status
tables, no prompt/env/command text, and actionable blocked/error output.

**Step 2: Prove red, implement, and verify CLI help**

Run: `go test ./cmd/ratchet -run 'ACPClientBackground' -count=1`.

Expected: FAIL with unrecognized `background` command.

Implement parser/handlers over client RPC only. Then run:

```bash
go test ./cmd/ratchet -run 'ACPClientBackground' -count=1
go run ./cmd/ratchet acp client background
```

Expected: tests PASS; command prints start/status/stop usage and exits without
starting a worker.

**Step 3: Add release-shaped fixture proof**

Build the real ratchet binary and existing ACP fixture agent. In a temporary
state root: create/trust a fixture profile, create a session with two queued
prompts, start the daemon, enable background drain, observe both prompts
complete, restart daemon with an unchanged profile and observe resume, alter
the profile descriptor and observe `blocked`, then stop. Assert no prompt text
appears in daemon/CLI logs.

**Step 4: Update policy and human docs**

Move background drain from deferred to supported. Document acknowledgement,
trusted descriptor pinning, error/no-retry semantics, status/stop/restart,
local sensitive-state boundary, and Windows parity. Keep arbitrary scheduling
and commands deferred.

**Step 5: Full verification**

```bash
go test ./internal/acpclient ./internal/daemon ./internal/client ./cmd/ratchet -run 'Background|Profile.*Trust|DrainQueue|WatchQueue' -count=1
go test ./...
go vet ./...
golangci-lint run --new-from-rev=origin/master
go test ./cmd/ratchet -run HarnessEmulationDocs -count=1
GOOS=windows GOARCH=amd64 go test -c ./internal/acpclient -o dist/acpclient.test.exe
GOOS=windows GOARCH=amd64 go test -c ./internal/client -o dist/client.test.exe
GOOS=windows GOARCH=amd64 go test -c ./cmd/ratchet -o dist/ratchet.test.exe
GOOS=windows GOARCH=amd64 go build ./cmd/ratchet
```

Expected: all exit 0; fixture integration completes two prompts, drift starts
zero agents, and Windows binaries compile.

**Step 6: Commit and complete PR 3**

```bash
git add cmd/ratchet README.md docs/harness-emulation.md docs/competitor-parity.md docs/policy-matrix.md
git commit -m "docs: explain ACP background drains"
```

Follow Global Execution Rules 4-5. Runtime-launch the released binary against a
temporary daemon and require background `status --json` to return within five
seconds.

Rollback: stop/disable all policies with the old binary before reverting PR 3;
publish the next patch. Persisted metadata contains no content and may remain.

### Task 9: Securely Load and Apply Managed Hook Policy

**Files:**
- Modify: `internal/hooks/hooks.go`
- Modify: `internal/hooks/hooks_test.go`
- Create: `internal/hooks/managed.go`
- Create: `internal/hooks/managed_test.go`
- Create: `internal/hooks/managed_path_unix.go`
- Create: `internal/hooks/managed_path_unix_test.go`
- Create: `internal/hooks/managed_path_windows.go`
- Create: `internal/hooks/managed_path_windows_test.go`
- Modify: `.github/workflows/ci.yml`
- Modify: `go.mod`
- Modify: `go.sum`

**Step 1: Write failing policy and platform-security tests**

Test missing file, additive, managed-only, malformed mode/event/YAML, duplicate
hook hash, immutable managed trust/disable, and suppression status. Test final
policy application after an arbitrary source slice is assembled.

For Unix, test pure ownership/mode validation plus real `O_NOFOLLOW` rejection
of symlinks and non-regular files. Require uid 0 and no group/other write. For
Windows, build DACL fixtures from SDDL and require only Administrators/SYSTEM
write-equivalent ACEs; reject reparse points and any Users/Everyone write ACE.

**Step 2: Prove red**

Run: `go test ./internal/hooks -run 'Managed|SecurePolicy' -count=1`.

Expected: FAIL with missing `ManagedPolicy`/`SourceManaged` symbols.

**Step 3: Implement policy model and final filtering**

Add `SourceManaged`, `Hook.Suppressed`, and:

```go
type ManagedMode string

type ManagedPolicy struct {
	Mode  ManagedMode `yaml:"mode"`
	Hooks HookConfig  `yaml:",inline"`
}
```

`ApplyManagedPolicy` preserves every hook for diagnostics, marks non-managed
hooks suppressed in managed-only mode, and makes `runnable()` reject suppressed
hooks. `ApplyTrust` must never unset managed trust or apply local disable state
to managed hooks.

**Step 4: Implement fixed secure readers**

Default paths follow the design. Tests may inject `LoadOptions.ManagedPath` and
a secure-reader seam; production has no environment override.

Unix uses `x/sys/unix` `O_NOFOLLOW` plus `Fstat` before reading. Windows obtains
ProgramData through `windows.KnownFolderPath(windows.FOLDERID_ProgramData, ...)`
rather than `%ProgramData%`, opens the file without following reparse points,
inspects owner/DACL with `x/sys/windows`, and rejects non-admin modification
rights. Promote `x/sys` to a direct dependency; do not add an ACL library.

Missing file returns no policy. Any present unreadable/insecure/malformed file
returns a typed `ErrManagedPolicy`.

**Step 5: Verify native/current and cross-platform code**

```bash
gofmt -w internal/hooks/managed*.go internal/hooks/hooks.go internal/hooks/*_test.go
go test ./internal/hooks -run 'Managed|SecurePolicy' -count=1
GOOS=windows GOARCH=amd64 go test -c ./internal/hooks -o dist/hooks-windows.test.exe
GOOS=linux GOARCH=amd64 go test -c ./internal/hooks -o dist/hooks-linux.test
```

Expected: current-platform tests PASS and both test binaries compile. Windows
CI later executes native DACL tests.

Add `go test ./internal/hooks -run 'Managed|SecurePolicy' -count=1` to the
existing `windows-2025` CI job. Expected on the PR: a named native Windows
managed-policy check passes, rather than relying only on cross-compilation.

**Step 6: Commit**

```bash
git add internal/hooks go.mod go.sum .github/workflows/ci.yml
git commit -m "feat(hooks): load secure managed policy"
```

Rollback: remove the managed file first, revert the commit, and rerun hook
loading tests; otherwise rollback would silently weaken an installed policy.

### Task 10: Add Managed Hook Audit and Operator CLI

**Files:**
- Create: `internal/hooks/audit.go`
- Create: `internal/hooks/audit_test.go`
- Modify: `internal/hooks/hooks.go`
- Modify: `internal/hooks/hooks_test.go`
- Modify: `cmd/ratchet/cmd_hooks.go`
- Modify: `cmd/ratchet/cmd_hooks_test.go`

**Step 1: Write failing audit/privacy tests**

Tests require owner-only JSONL with `started` and terminal records containing
timestamp, event, hash, source, result class, and duration only. Inject a writer
that fails before start and prove the command never runs. Fail terminal append
and prove execution error reports degraded audit without including command,
payload, output, error text, or secret sentinel.

Add CLI tests for `hooks policy [--json]` and `hooks audit [--json]`, plus list
source/suppression fields and rejection of trust/disable operations on managed
hashes.

**Step 2: Prove red**

Run: `go test ./internal/hooks ./cmd/ratchet -run 'Managed.*Audit|HooksPolicy|HooksAudit' -count=1`.

Expected: FAIL with missing audit/options/commands.

**Step 3: Implement audit-aware execution**

Add `RunWithOptions(event, data, RunOptions{Audit: ...})`; retain `Run` as the
compatibility wrapper. Before a managed hook process starts, append and sync a
`started` record. Append terminal success/failure after execution. Store only
the classified result (`success`, `command_failed`, `audit_degraded`) and
elapsed milliseconds. No command/output/data fields exist in the record schema.
Managed command failures return only event/hash/result classification to the
engine, so existing daemon error logging cannot emit raw managed-hook output.
Unmanaged compatibility errors continue through the existing redactor boundary.

If any future text is unavoidable, require the existing `secrets.Redactor`
adapter; do not implement replacement redaction.

**Step 4: Implement operator inspection**

`hooks policy` reports mode, secure source path, and managed hook count. `hooks
audit` reads bounded records newest-first and supports JSON. `hooks list` shows
`managed`, `suppressed`, `untrusted`, `disabled`, or `unsupported` status.
Trust/disable rejects managed hashes with an actionable immutable-policy error.

**Step 5: Verify and commit**

```bash
gofmt -w internal/hooks/audit.go internal/hooks/audit_test.go internal/hooks/hooks.go internal/hooks/hooks_test.go cmd/ratchet/cmd_hooks.go cmd/ratchet/cmd_hooks_test.go
go test ./internal/hooks ./cmd/ratchet -run 'Managed.*Audit|HooksPolicy|HooksAudit|HooksList' -count=1
git add internal/hooks cmd/ratchet/cmd_hooks.go cmd/ratchet/cmd_hooks_test.go
git commit -m "feat(hooks): audit managed execution"
```

Expected: PASS; recursive search of test audit files finds none of the command,
payload, output, error, or secret sentinels.

### Task 11: Enforce Policy at Final Hook Composition, Prove Runtime, Document, Merge, and Release

**Files:**
- Modify: `internal/daemon/engine.go`
- Modify: `internal/daemon/engine_hooks.go`
- Modify: `internal/daemon/hooks_wiring_test.go`
- Modify: `internal/daemon/plugin_reload_test.go`
- Modify: `internal/daemon/service_tui_smoke.go`
- Create: `internal/daemon/managed_hooks_runtime_test.go`
- Modify: `README.md`
- Modify: `docs/harness-emulation.md`
- Modify: `docs/competitor-parity.md`
- Modify: `docs/policy-matrix.md`
- Modify: `cmd/ratchet/harness_docs_test.go`

**Step 1: Add failing all-source enforcement tests**

Build user, plugin, late-loaded project, and managed hooks with distinct local
file side effects. Under additive mode, all eligible hooks run. Under
managed-only, only managed runs while list diagnostics retain suppressed
sources. Reload plugins and repeat to prove ordering cannot bypass policy.

Add startup/reload tests: absent policy is normal; present malformed/insecure
policy returns typed failure and does not publish a partially unmanaged hook
set. TUI smoke injects no managed path and never reads host administrator state.

**Step 2: Prove red**

Run: `go test ./internal/daemon -run 'ManagedHooks|PluginReload.*Managed' -count=1`.

Expected: FAIL because project hooks currently append after reload policy and
hook-load errors are discarded.

**Step 3: Wire final composition and fail-closed errors**

Store effective managed policy and audit writer in `EngineContext`. Load secure
managed source before publishing a plugin reload. Propagate `ErrManagedPolicy`
from initial engine construction and reload; preserve existing nonfatal policy
only for unrelated optional plugin failures.

In `EngineContext.RunHooks`, merge daemon user/managed/plugin hooks with the
event's project hooks, apply trust, then apply managed policy last and call
`RunWithOptions`. Never log hook output or prompt data on audit failure.

**Step 4: Runtime-launch the real hook path**

In `managed_hooks_runtime_test.go`, start the real engine/service with an
injected temp managed path, fire a real session lifecycle event through the
daemon boundary, and assert the managed shell side effect plus JSONL
start/success records. Switch to managed-only with user, plugin, and project
fixtures and assert their side-effect files remain absent. Install malformed
policy and assert daemon/reload fails closed. Separately build the production
binary and run `hooks policy --json` against an unmanaged temp home to prove the
normal absent-policy startup path.

**Step 5: Update docs and transition status**

Document platform paths/ownership, additive vs managed-only, immutable local
commands, policy/audit inspection, audit data exclusions, fail-closed behavior,
and rollback order. Move managed hooks from deferred to supported; remote
management/SDK remains deferred.

**Step 6: Full verification**

```bash
go test ./internal/hooks ./internal/daemon ./cmd/ratchet -run 'Hook|Managed|Audit|PluginReload' -count=1
go test ./...
go vet ./...
golangci-lint run --new-from-rev=origin/master
go test ./cmd/ratchet -run HarnessEmulationDocs -count=1
GOOS=windows GOARCH=amd64 go build ./cmd/ratchet
GOOS=windows GOARCH=amd64 go test -c ./internal/hooks -o dist/hooks.test.exe
GOOS=windows GOARCH=amd64 go test -c ./internal/daemon -o dist/daemon.test.exe
GOOS=windows GOARCH=amd64 go test -c ./cmd/ratchet -o dist/ratchet.test.exe
```

Expected: all exit 0; managed-only runtime creates exactly one managed side
effect and two audit records; malformed policy launches zero hooks; Windows
build/test compilation succeeds.

**Step 7: Commit and complete PR 4**

```bash
git add internal/daemon README.md docs/harness-emulation.md docs/competitor-parity.md docs/policy-matrix.md cmd/ratchet/harness_docs_test.go
git commit -m "docs: explain managed hook policy"
```

Follow Global Execution Rules 4-5. Verify the released Homebrew binary's
`hooks policy --json` reports `mode: none` on an unmanaged host without hanging
or starting the daemon unexpectedly.

Rollback: remove/migrate administrator policy first, stop the daemon, revert PR
4, publish the next patch, and preserve the metadata audit file for operators.

## Final Closeout

After all four PRs are merged, released, and green:

1. Run `autodev:post-merge-retrospective` against this design/plan and include
   PR review/CI/release evidence.
2. Confirm `master` in both repos equals origin and each primary worktree is
   clean; remove only worktrees/branches created by this plan.
3. Reconcile `GoCodeAlone/workspace`: update the ratchet/plugin project states,
   run `wfctl portfolio scan`, ensure the next VS Code-style harness optimization
   cluster is present in generated follow-ups, then PR/admin-merge workspace
   state and reset the local workspace checkout to clean `main`.
4. Invoke scope-lock completion with merged PRs, release tags, full test/lint,
   Windows, runtime, Homebrew, and workspace-state evidence.
