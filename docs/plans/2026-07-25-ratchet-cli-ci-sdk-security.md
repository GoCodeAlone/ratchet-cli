# Ratchet CLI CI And SDK Security Maintenance Implementation Plan

> **For the implementing agent:** REQUIRED SUB-SKILL: Use autodev:executing-plans to implement this plan task-by-task.

**Goal:** Repair release-driven agent registry publication, migrate ratchet-cli and workflow-plugin-agent to current GitHub Action runtimes, remediate Docker/Ollama/gRPC advisories at the owning boundary, consume the released plugin from ratchet-cli, and publish verified cross-platform releases.

**Architecture:** Fix workflow-registry alias resolution before the next plugin tag. Isolate Actions changes from SDK changes; release workflow-plugin-agent before changing ratchet-cli's module graph. Use existing SDK wrappers, releaseguard, registry sync, GoReleaser, native Windows jobs, and Homebrew publication as authorities.

**Tech Stack:** Go 1.26.4, Bash, GitHub Actions, GoReleaser v2/action v7, `wfctl`, workflow-registry, Docker v29.6.2, Ollama v0.32.4, gRPC v1.82.1, Homebrew.

**Base branch:** `master` for ratchet-cli/workflow-plugin-agent; `main` for workflow-registry.

---

## Scope Manifest

**PR Count:** 8
**Tasks:** 8
**Estimated Lines of Change:** ~750

**Out of scope:**
- org-wide GitHub Action upgrades or runner replacement/dogfood changes
- downstream Docker/Ollama overrides, SDK forks, or copied clients
- provider UX, daemon schema, workflow DSL, or public plugin API changes
- nested TUI test-build cleanup, startup-reconciliation cancellation follow-up, and self-improving harness features

**PR Grouping:**

| PR # | Title | Tasks | Branch |
|---|---|---|---|
| 1 | `fix: resolve short plugin release aliases` (`workflow-registry`) | Task 1 | `fix/plugin-release-alias-resolution` |
| 2 | `ci: update ratchet Action runtimes` (`ratchet-cli`) | Task 2 | `chore/ratchet-action-runtimes` |
| 3 | `ci: update agent plugin Action runtimes` (`workflow-plugin-agent`) | Task 3 | `chore/agent-action-runtimes` |
| 4 | `chore: sync workflow-plugin-agent to v0.12.9` (`workflow-registry`, generated) | Task 4 | `chore/sync-workflow-plugin-agent-v0.12.9` |
| 5 | `fix: remediate agent SDK advisories` (`workflow-plugin-agent`) | Task 5 | `fix/agent-sdk-security` |
| 6 | `chore: sync workflow-plugin-agent to v0.12.10` (`workflow-registry`, generated) | Task 6 | `chore/sync-workflow-plugin-agent-v0.12.10` |
| 7 | `fix: consume remediated agent SDKs` (`ratchet-cli`) | Task 7 | `fix/ratchet-agent-sdk-security` |
| 8 | `docs: close CI and SDK security plan` (`ratchet-cli`) | Task 8 | `docs/ci-sdk-security-closeout` |

**Status:** Locked 2026-07-25T19:55:44Z

## Integration Matrix

| Integration | Class | Owning task | Required real proof |
|---|---|---|---|
| plugin release dispatch -> registry resolver | runtime-integrated | Task 1, Task 3 | repository dispatch resolves `agent` to `workflow-plugin-agent` and opens/updates PR |
| registry -> plugin release assets | runtime-integrated | Task 4, Task 6 | main/Pages manifest tag and six hashes match release; bulk index omits private plugin |
| GitHub Action majors | runtime-integrated | Task 2, Task 3 | actual PR and tag workflows use new actions |
| Docker SDK | runtime-integrated | Task 5, Task 7 | owner fake lifecycle plus released ratchet consumer |
| Ollama SDK | runtime-integrated | Task 5, Task 7 | real upstream client against httptest plus ratchet provider path |
| gRPC | runtime-integrated | Task 5, Task 7 | plugin adapter and ratchet daemon/client tests |
| Homebrew tap | runtime-integrated | Task 2, Task 7, Task 8 | hashes, Formula/Cask, installed bounded version |

### Task 1: Repair Workflow-Registry Short-Name Dispatch

**Repository:** `GoCodeAlone/workflow-registry`

**Files:**
- Modify: `tests/test-resolve-sync-plugin-filter.sh`
- Modify: `scripts/resolve-sync-plugin-filter.sh`
- Modify: `.github/workflows/validate.yml`
- Modify: `.github/workflows/sync-registry-manifests.yml`

**Execution backport (2026-07-25):** workflow `main` raised its Go floor to
1.26.5 after plan lock. Task 1's existing green validation/sync gate therefore
updates both registry workflow Go/cache pins from 1.26.4 to 1.26.5. The Scope
Manifest is unchanged; see the design backport for evidence.

**Step 1: Create isolated worktree**

Run:
```bash
git fetch origin main --prune
git check-ignore -q .worktrees
git worktree add .worktrees/plugin-release-alias-resolution \
  -b fix/plugin-release-alias-resolution origin/main
```
Expected: clean worktree at `origin/main`; do not touch the dirty primary checkout.

**Step 2: Add exact failing regression**

Extend the fixture with:
```bash
mkdir -p "${tmp}/plugins/workflow-plugin-agent" \
  "${tmp}/plugins/workflow-plugin-spoof"
cat > "${tmp}/plugins/workflow-plugin-agent/manifest.json" <<'JSON'
{"name":"workflow-plugin-agent","repository":"https://github.com/GoCodeAlone/workflow-plugin-agent"}
JSON
cat > "${tmp}/plugins/workflow-plugin-spoof/manifest.json" <<'JSON'
{"name":"workflow-plugin-spoof","repository":"https://github.com/OtherOrg/workflow-plugin-spoof"}
JSON
```

Add assertions:
```bash
out="$(run_case repository_dispatch plugin-release agent "")"
assert_contains "${out}" "plugin=workflow-plugin-agent"
assert_contains "${out}" "skip=0"

out="$(run_case repository_dispatch plugin-release spoof "")"
assert_contains "${out}" "plugin="
assert_contains "${out}" "skip=1"
```

**Step 3: Prove RED**

Run: `bash tests/test-resolve-sync-plugin-filter.sh`
Expected: FAIL because short `agent` returns `skip=1`.

**Step 4: Implement constrained fallback**

In `scripts/resolve-sync-plugin-filter.sh`:
1. keep name/path validation before filesystem access;
2. inspect `plugins/workflow-plugin-${plugin}/manifest.json` even when the direct path is absent;
3. normalize accepted repository forms to `owner/repo`;
4. select the prefixed path only when identity equals `GoCodeAlone/workflow-plugin-${plugin}`;
5. retain existing core-vs-external selection when both paths exist;
6. emit `skip=1` on absent or mismatched candidates.

Do not generalize to arbitrary owners or prefixes.

**Step 5: Prove GREEN and regression safety**

Run:
```bash
bash tests/test-resolve-sync-plugin-filter.sh
bash tests/test-sync-workflow-token.sh
bash tests/test-validate-manifests-hygiene.sh
bash scripts/validate-manifests.sh
git diff --check
```
Expected: every command exits 0; filter test prints `resolve-sync-plugin-filter tests passed`.

Run: `golangci-lint run --new-from-rev=origin/main`
Expected: exit 0 or documented not-applicable because this PR changes no Go files.

Run the exact Linux core-manifest validation path with Go 1.26.5:
```bash
wfctl plugin registry-sync core \
  --registry-dir . \
  --workflow-repo /path/to/workflow-main
```
Expected: `Core plugin manifests match workflow plugin declarations.`

**Step 6: Commit, review, merge, and deploy**

Commit: `fix: resolve short plugin release aliases`

Create PR #1, add Copilot, invoke PR monitoring, settle all checks/threads, and
admin squash-merge. Verify the main-commit `Validate Registry` and
`Build & Deploy Registry` workflows complete successfully.

Run a non-destructive manual dispatch with plugin `workflow-plugin-agent`:
```bash
gh workflow run sync-registry-manifests.yml \
  --repo GoCodeAlone/workflow-registry \
  -f plugin=agent
```
Expected: resolver emits `plugin=workflow-plugin-agent`, never `skip=1`.

Rollback: revert the merge; fully prefixed dispatch and daily full sync remain
available. Re-run the registry build/deploy after revert.

### Task 2: Migrate Ratchet-CLI Action Runtimes

**Repository:** `GoCodeAlone/ratchet-cli`

**Files:**
- Modify: `internal/releaseguard/workflow_test.go`
- Modify: `.github/workflows/ci.yml`
- Modify: `.github/workflows/release.yml`
- Include committed design/plan/review artifacts under `docs/plans/`

**Step 1: Add current-major contract before YAML changes**

Add a table-driven releaseguard test that scans both workflows:
```go
required := []string{
	"actions/checkout@v7",
	"actions/setup-go@v7",
}
forbidden := []string{
	"actions/checkout@v4",
	"actions/setup-go@v5",
	"actions/upload-artifact@v4",
	"actions/github-script@v7",
	"codecov/codecov-action@v4",
	"golangci/golangci-lint-action@v8",
	"require('@actions/github')",
}
```

Require CI-only majors `upload-artifact@v7`, `codecov-action@v7`,
`golangci-lint-action@v9`; require release `github-script@v9`. Update existing
exact checkout/setup assertions to v7. Retain every Windows/release/tap guard.

**Step 2: Prove RED**

Run:
```bash
go test ./internal/releaseguard \
  -run 'Test.*Workflow|TestCIReleaseCheckJob' -count=1
```
Expected: FAIL listing old v4/v5/v7/v8 pins.

**Step 3: Update only Action majors**

Modify YAML:
- checkout v4 -> v7
- setup-go v5 -> v7
- upload-artifact v4 -> v7
- github-script v7 -> v9
- codecov action v4 -> v7
- golangci-lint action v8 -> v9
- retain GoReleaser v7, inputs, permissions, runners, and scripts

The github-script bodies continue using injected `github`, `context`, `core`,
and Node built-in `fs`; do not import `@actions/github`.

**Step 4: Prove GREEN**

Run:
```bash
go test ./internal/releaseguard -count=1
go test ./... 
go vet ./...
golangci-lint run --new-from-rev=origin/master
goreleaser check
goreleaser release --snapshot --clean --skip=publish
scripts/check-release-artifacts.sh --manifest-only dist
```
Expected: all exit 0; snapshot contains six platform archives plus checksums.

**Step 5: Runtime launch and skew audit**

Run the snapshot's host binary with a 10-second process bound:
```bash
perl -e 'alarm 10; exec @ARGV' <snapshot-ratchet-path> --version
```
Expected: version output and exit 0.

Audit all action pins with `rg -n 'uses:' .github/workflows`; expected only the
designed majors and existing GoReleaser v7.2.x major. Capture the transcript for
the PR body.

**Step 6: Commit, review, merge, release**

Commits:
1. `test: require current Action runtimes`
2. `ci: update ratchet Action runtimes`

Create PR #2 with the eight-row Scope Manifest, add Copilot, invoke monitoring,
and admin squash-merge after all required Linux/native-Windows/release/CodeQL
checks and threads settle.

Tag the merge commit with next patch (initial target v0.30.38), monitor release,
and verify:
- public non-draft release;
- checksums plus Darwin/Linux/Windows amd64+arm64 archives;
- all six checksum validations;
- Formula and Cask version/hashes;
- Homebrew upgrade;
- bounded installed `ratchet --version`;
- provider catalog count remains 22.

Rollback: revert the action commit, retain previous action majors, and publish
the next patch from the revert merge; never move the failed tag.

### Task 3: Migrate Workflow-Plugin-Agent Action Runtimes

**Repository:** `GoCodeAlone/workflow-plugin-agent`

**Files:**
- Create: `workflow_actions_test.go`
- Modify: `.github/workflows/ci.yml`
- Modify: `.github/workflows/release.yml`

**Step 1: Create worktree and preserve primary residue**

Run:
```bash
git fetch origin master --prune
git check-ignore -q .worktrees
git worktree add .worktrees/agent-action-runtimes \
  -b chore/agent-action-runtimes origin/master
```
Expected: clean worktree; primary `.claude/autodev-state/` remains untouched.

**Step 2: Write parsed workflow contracts**

Use `gopkg.in/yaml.v3` to load both workflows. Assert:
- checkout/setup-go v7 in every relevant job;
- github-script v9 in publish-release;
- GoReleaser remains v7;
- Go stays 1.26/go.mod;
- registry-dispatch action/`plugin-release` payload stays present;
- no `require('@actions/github')`;
- `.goreleaser.yml` retains Darwin/Linux/Windows amd64+arm64 builds.

**Step 3: Prove RED**

Run: `go test . -run TestWorkflowActionContracts -count=1`
Expected: FAIL on checkout v4/setup-go v5/github-script v7.

**Step 4: Update only Action majors**

Change checkout v4 -> v7, setup-go v5 -> v7, github-script v7 -> v9. Retain
GoReleaser v7 and SHA-pinned repository-dispatch v4. Do not change runner labels,
permissions, wfctl pins, or release script behavior.

**Step 5: Prove GREEN and launch plugin snapshot**

Run:
```bash
go test . -run TestWorkflowActionContracts -count=1
go test -race ./...
go vet ./...
golangci-lint run --new-from-rev=origin/master
```
Expected: all exit 0.

The plugin GoReleaser before hook rewrites `plugin.json` and runs `go mod tidy`.
Run release validation in a disposable source copy so verification cannot add
generated changes to the feature branch:
```bash
snapshot_dir="$(mktemp -d)"
rsync -a --exclude .git --exclude .worktrees --exclude dist ./ "${snapshot_dir}/"
(cd "${snapshot_dir}" &&
  goreleaser check &&
  goreleaser release --snapshot --clean --skip=publish)
```
Expected: six OS/architecture artifacts; feature worktree remains unchanged.

Build the external plugin in the layout accepted by wfctl and run
`wfctl plugin verify-capabilities` against the real binary/manifest.
Expected: capability verification exits 0.

**Step 6: Commit, review, merge, release**

Commits:
1. `test: guard agent Action runtimes`
2. `ci: update agent Action runtimes`

Create PR #3, add Copilot, monitor, and admin squash-merge after test,
contract, release-check, and security checks settle.

Tag the merge commit with next patch (initial target v0.12.9). Monitor all
release jobs, including `Notify workflow-registry`. Verify six archives and
checksums. Do not continue unless the registry sync run resolves
`agent -> workflow-plugin-agent` without `skip=1` and creates/updates PR #4.

Rollback: revert the Action commit and publish the next plugin patch; registry
version remains on the last verified release until its generated PR merges.

### Task 4: Publish Workflow-Plugin-Agent v0.12.9 In Registry

**Repository:** `GoCodeAlone/workflow-registry`

**Files (generated):**
- Modify: `plugins/workflow-plugin-agent/manifest.json`
- Modify when generated sync requires it: `README.md`

**Step 1: Inspect generated PR**

Expected branch: `chore/sync-workflow-plugin-agent-v0.12.9`. Confirm PR body
cites `repository_dispatch (event_type=plugin-release)`. If a parallel release
advanced the tag after scope lock, stop and use the formal manifest amendment,
alignment, and re-lock path before accepting a differently named branch.

**Step 2: Validate exact release projection**

Run:
```bash
jq -r '.version' plugins/workflow-plugin-agent/manifest.json
bash scripts/validate-manifests.sh
```
Expected: version matches Task 3 tag and validation passes.

Download release checksums and compare every manifest `downloads[].sha256` to
the corresponding six release assets. Expected: 6/6 exact matches.

**Step 3: Review, merge, and deploy**

Add Copilot if the generated workflow did not, invoke monitoring, settle all
checks/threads, and admin squash-merge PR #4. Verify main
`Build & Deploy Registry` success.

Poll Pages until:
```bash
curl -fsS https://gocodealone.github.io/workflow-registry/v1/plugins/workflow-plugin-agent/manifest.json |
  jq -r .version
curl -fsS https://gocodealone.github.io/workflow-registry/v1/index.json |
  jq '[.[] | select(.name=="workflow-plugin-agent")] | length'
```
Expected: released version; bulk-index count `0`.

Rollback: revert the registry merge and redeploy. Plugin release remains valid
but is not eligible for downstream consumption until registry state is repaired.

### Task 5: Remediate SDK Advisories In Workflow-Plugin-Agent

**Repository:** `GoCodeAlone/workflow-plugin-agent`

**Files:**
- Create: `dependency_versions_test.go`
- Create: `provider/ollama_client_test.go`
- Modify: `orchestrator/container_manager_test.go`
- Modify only if upstream API requires it: `orchestrator/container_manager.go`
- Modify only if upstream API requires it: `provider/ollama_client.go`
- Modify: `go.mod`
- Modify: `go.sum`

**Step 1: Create worktree from released Task 3 master**

Branch: `fix/agent-sdk-security`; verify base contains Task 3 merge/tag.

**Step 2: Write failing module-version guard**

Parse `go.mod` with `golang.org/x/mod/modfile`. Require:
```go
map[string]string{
	"github.com/docker/docker": "v29.6.2+incompatible",
	"github.com/ollama/ollama":  "v0.32.4",
	"google.golang.org/grpc":    "v1.82.1",
}
```
Also reject `replace` directives for Docker/Ollama/gRPC.

Run: `go test . -run TestSecurityDependencyVersions -count=1`
Expected: FAIL on all old versions.

**Step 3: Add owner-wrapper compatibility coverage**

Add httptest-backed Ollama cases for:
- list response mapping and `name`/`model` fallback;
- pull progress;
- heartbeat success/failure;
- HTTP 500/malformed payload;
- context cancellation;
- malformed configured URL fallback.

Strengthen fake Docker coverage for inspect/pull/create/start/status/exec/
stop/remove options and representative upstream errors. Tests must exercise
`dockerAPIClient`, not reimplement SDK behavior.

Run focused tests before upgrades; expected existing behavior cases pass and
new version guard remains RED.

**Step 4: Upgrade through Go tooling**

Run:
```bash
go get github.com/docker/docker@v29.6.2+incompatible
go get github.com/ollama/ollama@v0.32.4
go get google.golang.org/grpc@v1.82.1
go mod tidy
```
Adapt only compile failures at `dockerAPIClient`/`ContainerManager` and
`OllamaClient`. Do not add local SDK copies or downstream compatibility shims.

**Step 5: Verify owner and real plugin boundary**

Run:
```bash
go test ./provider ./orchestrator -count=1
go test -race ./...
go vet ./...
golangci-lint run --new-from-rev=origin/master
go list -m github.com/docker/docker github.com/ollama/ollama google.golang.org/grpc
go mod why -m github.com/docker/docker
go mod why -m github.com/ollama/ollama
go mod why -m google.golang.org/grpc
```
Expected: target versions, no replaces, and full suite/lint green.

Run GoReleaser in the same disposable `rsync` source-copy pattern from Task 3
because its before hook rewrites `plugin.json` and runs `go mod tidy`.
Expected: six artifacts, no feature-worktree mutation, and wfctl capability
verification against the real snapshot binary exits 0.

**Step 6: Commit, review, merge, release**

Commits:
1. `test: guard agent SDK security versions`
2. `test: cover Docker and Ollama compatibility`
3. `fix: remediate agent SDK advisories`

Create PR #5, add Copilot, monitor all CI/security checks, and admin
squash-merge. Tag next patch (initial target v0.12.10), verify release artifacts
and checksums, then require the release dispatch to create/update PR #6.

Rollback: choose the newest API-compatible versions outside every advisory
range, revert wrapper adaptations as needed, and publish a new patch. Never
return to Docker <=28.5.2, Ollama <=0.20.2, or gRPC <1.82.1.

### Task 6: Publish Workflow-Plugin-Agent v0.12.10 In Registry

**Repository:** `GoCodeAlone/workflow-registry`

Repeat Task 4 against generated branch
`chore/sync-workflow-plugin-agent-v0.12.10`:
- verify dispatch was not skipped;
- verify repository-main version and 6/6 release hashes;
- run manifest/index tests;
- add Copilot, monitor, admin squash-merge;
- verify main deployment and Pages per-plugin version;
- verify public bulk index still omits the private plugin.

Rollback: revert/deploy registry only; stop before Task 7 until the released
plugin and registry projection agree.

### Task 7: Consume Remediated Plugin And gRPC In Ratchet-CLI

**Repository:** `GoCodeAlone/ratchet-cli`

**Files:**
- Create: `internal/releaseguard/dependency_test.go`
- Modify: `go.mod`
- Modify: `go.sum`
- Modify only for proven compatibility failures: existing plugin/provider/
  daemon client consumers

**Step 1: Create worktree from Task 2 released master**

Branch: `fix/ratchet-agent-sdk-security`.

**Step 2: Write failing dependency ownership guard**

Parse `go.mod` and assert:
- workflow-plugin-agent equals Task 5 release;
- gRPC equals v1.82.1;
- Docker/Ollama remain indirect only, with no replace or exclude;
- no replacement for workflow-plugin-agent/gRPC.

Run: `go test ./internal/releaseguard -run TestSecurityDependencyOwnership -count=1`
Expected: FAIL on plugin v0.12.8 and gRPC v1.81.1.

**Step 3: Consume released owner**

Run:
```bash
go get github.com/GoCodeAlone/workflow-plugin-agent@v0.12.10
go get google.golang.org/grpc@v1.82.1
go mod tidy
```
Use the locked Task 5 tag. If patch numbers advanced after lock, stop for the
formal manifest amendment/alignment/re-lock path. Do not promote Docker/Ollama
to direct requirements or add replaces/copied adapters.

**Step 4: Verify module graph and focused consumers**

Run:
```bash
go list -m github.com/GoCodeAlone/workflow-plugin-agent \
  github.com/docker/docker github.com/ollama/ollama google.golang.org/grpc
go mod why -m github.com/docker/docker
go mod why -m github.com/ollama/ollama
go test ./internal/releaseguard ./internal/provider ./internal/plugins \
  ./internal/client ./internal/daemon -count=1
go test ./internal/tui/pages -run 'Ollama|Provider' -count=1
```
Expected: target plugin/SDK versions; Docker/Ollama paths pass through
workflow-plugin-agent; provider, gRPC, and release guards green.

**Step 5: Verify full runtime and platforms**

Run:
```bash
go test ./...
go vet ./...
golangci-lint run --new-from-rev=origin/master
goreleaser check
goreleaser release --snapshot --clean --skip=publish
scripts/check-release-artifacts.sh --manifest-only dist
```
Launch the snapshot binary with bounded `--version`, `help`, and
`provider setup list --json`; expected 22 providers and no daemon hang.

PR CI must retain native `windows-2025` version/help, ConPTY, daemon lock,
provider durability, ACP lifecycle, CodeQL, and release-check jobs.

**Step 6: Commit, review, merge, release**

Commits:
1. `test: guard agent SDK dependency ownership`
2. `fix: consume remediated agent SDKs`

Create PR #7, add Copilot, monitor, and admin squash-merge after every check and
thread settles. Tag next patch (initial target v0.30.39) and run the full
ratchet release validation from Task 2.

Poll Dependabot until the seven Docker/Ollama/gRPC alerts close. If GitHub lags,
record target module graph evidence and continue bounded polling; never
reintroduce vulnerable versions.

Rollback: revert to the last released plugin only if it remains outside all
advisory ranges; otherwise fix forward in the owner plugin and publish another
consumer patch.

### Task 8: Retrospective, Plan Closeout, And Final Release

**Repository:** `GoCodeAlone/ratchet-cli`

**Files:**
- Create: `docs/retros/2026-07-25-ratchet-cli-ci-sdk-security-retro.md`
- Modify if a durable lesson exists: `docs/design-guidance.md`
- Modify: `docs/plans/2026-07-25-ratchet-cli-ci-sdk-security.md`
- Delete after verified completion: plan scope-lock sidecar

**Step 1: Build evidence ledger**

Record PR merge SHAs, Copilot/review rounds, all CI/main/release run IDs,
registry deploy/sync PRs, release tags/assets/hashes, Homebrew proof,
Dependabot state, and any design backports.

**Step 2: Write retrospective**

Invoke `autodev:post-merge-retrospective`. Score every D/P finding and gate.
Include the false-green registry root cause and whether release-state
verification caught it after repair. Update design guidance only for a durable
future-project principle.

**Step 3: Close the locked plan**

Mark all eight tasks/PR rows complete only after evidence exists. Run
`scope-lock-complete` with exact verification evidence and preserve generated
phase progress in workspace state, not as ratchet-cli untracked residue.

**Step 4: Verify docs**

Run:
```bash
git diff --check
go test ./internal/releaseguard -count=1
go test ./...
golangci-lint run --new-from-rev=origin/master
```
Expected: no machine path, all tests/lint green, manifest Status `Complete`.

**Step 5: Commit, review, merge, release**

Commit: `docs: close CI and SDK security plan`

Create PR #8, add Copilot, monitor all checks/threads, and admin squash-merge.
Tag next patch (initial target v0.30.40) and repeat complete ratchet release
validation. Reconcile workspace portfolio/projects/followups through an
isolated workspace PR, preserving the dirty primary workspace checkout.

Rollback: revert only inaccurate documentation/evidence in a new PR and publish
the next ratchet patch because every ratchet merge is released; do not rewrite
prior tags or phase history.
