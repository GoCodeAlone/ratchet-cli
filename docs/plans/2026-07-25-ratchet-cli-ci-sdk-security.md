# Ratchet CLI CI And SDK Security Maintenance Implementation Plan

> **For the implementing agent:** REQUIRED SUB-SKILL: Use autodev:executing-plans to implement this plan task-by-task.

**Goal:** Repair release-driven agent registry publication, migrate ratchet-cli and workflow-plugin-agent to current GitHub Action runtimes, remediate Docker/Ollama/gRPC advisories at the owning boundary, consume the released plugin from ratchet-cli, and publish verified cross-platform releases.

**Architecture:** Fix workflow-registry alias resolution before the next plugin tag. Isolate Actions changes from SDK changes; release workflow-plugin-agent before changing ratchet-cli's module graph. Use existing SDK wrappers, releaseguard, registry sync, GoReleaser, native Windows jobs, and Homebrew publication as authorities.

**Tech Stack:** Go 1.26.4 for ratchet-cli/workflow-plugin-agent and Go
1.26.5 for workflow-registry validation, Bash, GitHub Actions, GoReleaser
v2/action v7, `wfctl`, workflow-registry, Docker v29.6.2 via Moby client
v0.5.0/API v1.55.0, Ollama v0.32.4, gRPC v1.82.1, Homebrew.

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

| PR # | Title | Tasks | Branch | Completion |
|---|---|---|---|---|
| 1 | `fix: resolve short plugin release aliases` (`workflow-registry`) | Task 1 | `fix/plugin-release-alias-resolution` | Complete |
| 2 | `ci: update ratchet Action runtimes` (`ratchet-cli`) | Task 2 | `chore/ratchet-action-runtimes` | Complete |
| 3 | `ci: update agent plugin Action runtimes` (`workflow-plugin-agent`) | Task 3 | `chore/agent-action-runtimes` | Complete |
| 4 | `chore: sync workflow-plugin-agent to v0.12.9` (`workflow-registry`, generated) | Task 4 | `chore/sync-workflow-plugin-agent-v0.12.9` | Complete |
| 5 | `fix: remediate agent SDK advisories` (`workflow-plugin-agent`) | Task 5 | `fix/agent-sdk-security` | Complete |
| 6 | `chore: sync workflow-plugin-agent to v0.12.10` (`workflow-registry`, generated) | Task 6 | `chore/sync-workflow-plugin-agent-v0.12.10` | Complete |
| 7 | `fix: consume remediated agent SDKs` (`ratchet-cli`) | Task 7 | `fix/ratchet-agent-sdk-security` | Complete |
| 8 | `docs: close CI and SDK security plan` (`ratchet-cli`) | Task 8 | `docs/ci-sdk-security-closeout` | Documentation complete in #146; integration pending |

**Status:** Scope implementation complete 2026-07-26T01:34:02Z;
integration pending for ratchet-cli #146 and `v0.30.40`.

The scope-lock completion timestamp records the immutable implementation scope
and removal of its sidecar. It does not assert that this document's own PR or
release already exists. A commit cannot contain evidence of its future merge
SHA or tag workflow, so the workspace phase-progress ledger is the
authoritative append-only record for #146 integration, `v0.30.40`, and final
workspace reconciliation.

## Completion Evidence

| Task | Exact integration record | PR and settled integration workflow runs |
|---|---|---|
| 1 | workflow-registry #629, merge `571f73ac6e8c5907d00ab44dccf9ec98cdcec176` | PR `30173902649`, validation `30173904211`; main policy `30174254928`, deploy `30174255060`, validation `30174255076`; short-alias dispatch `30174621567` |
| 2 | ratchet-cli #144, merge/tag `c391e5a70e144fd9ce58ca78eb4ce7ea09b5b6e4`, `v0.30.38` | PR policy `30175540305`, quality `30175540316`, CI `30175541257`; main quality `30176219749`, policy `30176219893`, CI `30176219970`; release `30176230104` |
| 3 | workflow-plugin-agent #42, merge/tag `9cbc0009bb2eb629eb9f086862e1b2fe2bf4a104`, `v0.12.9` | PR policy `30177484712`, CI `30177485909`; main policy `30177705277`, CI `30177705520`; release `30177716512` |
| 4 | workflow-registry #630, merge `47b7f2ef05d29f9dacc3cf79ad72436a16910773` | PR policy `30178160119`; main policy `30178210748`, validation `30178211056`, deploy `30178211090` |
| 5 | workflow-plugin-agent #43, merge/tag `36882286121c93ad8b0974b0427025df2a191b56`, `v0.12.10` | PR policy `30179275063`, CI `30179275723`; main policy/CodeQL `30179495585`, CI `30179495789`, graph `30179497415`; release `30179497372` |
| 6 | workflow-registry #631, merge `fd9cad072476ceaa3d1c7fdbdd838f60f9979e0b` | release dispatch `30179794046`; PR policy `30179872519`, validation `30179873264`; main policy `30180022517`, validation `30180022653`, deploy `30180022660` |
| 7 | ratchet-cli #145, merge/tag `5afbfd0fafcefe7ecc47e41bd876537a873a7e88`, `v0.30.39` | PR quality `30181979630`, policy `30181979637`, CI `30181980216`; main quality `30182445433`, policy/CodeQL `30182445525`, CI `30182445914`, graph `30182446782`; release `30182453196` |
| 8 | ratchet-cli #146, `docs/ci-sdk-security-closeout` | retrospective, guidance, evidence ledger, scope-lock sidecar removal, local verification, and independent review are complete; merge SHA, `v0.30.40`, and workspace PR evidence remain pending in external phase progress |

### Release Assets

Every archive digest matched the downloaded `checksums.txt`. Each
`checksums.txt` digest is GitHub's independent SHA-256 asset digest.

| Release | Tagged merge and release run | Asset SHA-256 |
|---|---|---|
| ratchet-cli `v0.30.38` | `c391e5a70e144fd9ce58ca78eb4ce7ea09b5b6e4`; `30176230104` | `checksums.txt=4166656c018898630bb45abe99790752c220980becc7f98643025138513b2eba`<br>`ratchet_darwin_amd64.tar.gz=541f1d0e994f6022cc38d7713ee9fbc0175f407c30d30345de78e1aab53bb465`<br>`ratchet_darwin_arm64.tar.gz=35ca63b0fa391a32c701dd5ddfeb00999d4cfafb6b223f283e5e50c5b87a552e`<br>`ratchet_linux_amd64.tar.gz=936b0e147fbce3c6a60efff6c97c55dd6eb436550837e6b0763836e2545ed064`<br>`ratchet_linux_arm64.tar.gz=8fe9a1dee6a7501ebd728d932ebd4d67c8c5ed5a3502c2efe74bad54aa16e7f1`<br>`ratchet_windows_amd64.zip=ca46c610cc17ed5a0f70181557097cfacf88cde2cd18e067ae484dc6cba6ff65`<br>`ratchet_windows_arm64.zip=153225dd1c8c419aac84ff0c410f0607936ca0889d28ceac5e2c320f6eed5d37` |
| workflow-plugin-agent `v0.12.9` | `9cbc0009bb2eb629eb9f086862e1b2fe2bf4a104`; `30177716512` | `checksums.txt=9fe1d7f2885627afd63905688782be70b7a6a2a358be77dba44257de2ec133c1`<br>`workflow-plugin-agent_0.12.9_darwin_amd64.tar.gz=4ffe5a9b1c201c4001875aa0c4e83d093f62c0a04151e0ae1c0ae2911c3d7125`<br>`workflow-plugin-agent_0.12.9_darwin_arm64.tar.gz=7d897602b19cb4b4f63cb7bcb381a90d23edef1d1791be5e7494c6946da572bc`<br>`workflow-plugin-agent_0.12.9_linux_amd64.tar.gz=8729528716ee21e19a344c3703652af310b26f56056c50c0e0c867f9fdc74dcd`<br>`workflow-plugin-agent_0.12.9_linux_arm64.tar.gz=fc8bdc51a784427bafeca7fd3189a9fbf89319acbb88dbc5746198b5ec821654`<br>`workflow-plugin-agent_0.12.9_windows_amd64.tar.gz=2677567303142642190f5837f40f0408ae9e6f325faceeddb61d9bcdd2638b1d`<br>`workflow-plugin-agent_0.12.9_windows_arm64.tar.gz=350c3c9d33da95d0029c46e6f225333abee10802d7037af88bb62a058dbdb347` |
| workflow-plugin-agent `v0.12.10` | `36882286121c93ad8b0974b0427025df2a191b56`; `30179497372` | `checksums.txt=d179a49b85941055f01b732e95900305dd39536c6fc587a53fcb49be12905442`<br>`workflow-plugin-agent_0.12.10_darwin_amd64.tar.gz=6b7d21893f667b8b300bfd4088d44fa0fe77336501203f3dece862969527926c`<br>`workflow-plugin-agent_0.12.10_darwin_arm64.tar.gz=2b918649721e57c9b682fc08b28d3b51ff7d9eeff52edb72b3d094cbec327a3d`<br>`workflow-plugin-agent_0.12.10_linux_amd64.tar.gz=8f0b54e37364d6c46f9633f46e2ca08da8a4d30207e1b9e593e1c976ab5d8355`<br>`workflow-plugin-agent_0.12.10_linux_arm64.tar.gz=73d18c8aa76cef081042908bb647bb98886fe8c9f44414e0856331d3c2154066`<br>`workflow-plugin-agent_0.12.10_windows_amd64.tar.gz=8649ef86991fc75dcbffe3066df91fab9106e6a3af6f5a08e617c42bb0dd0a06`<br>`workflow-plugin-agent_0.12.10_windows_arm64.tar.gz=1c7ecbcac7de11b558ec2a7ea486a66b0bb3c663518fb2bc18bb7c94bfc2dcc1` |
| ratchet-cli `v0.30.39` | `5afbfd0fafcefe7ecc47e41bd876537a873a7e88`; `30182453196` | `checksums.txt=d5b5694c74dfb60a2acf89ba4d0be52c17370d808af6e28341d0daee47f6bfd6`<br>`ratchet_darwin_amd64.tar.gz=ed04649972523be151462d97460861b4b44bf1e8dd0b5b4eb3605435b015c1ca`<br>`ratchet_darwin_arm64.tar.gz=2df75c0f42e034f89d9bab9da237654ec2c9619bfe5e0e0222765ff5454322d7`<br>`ratchet_linux_amd64.tar.gz=436cb75407e22cc19696fcf8a0d90e9e6dea36ce121da5f212f08ee7be8d0f2b`<br>`ratchet_linux_arm64.tar.gz=ff88d8a82f4402a063cf89b0bb4b59263848bd8dceb085662e66d2ca7c3f45f1`<br>`ratchet_windows_amd64.zip=750ea62768e21e0464abbff1e9f83e0dfc0eb20ebd35df1be4e2e0022066fc4b`<br>`ratchet_windows_arm64.zip=3fb046551196816a3c7fb8d330223519fb3a24118aa78ed5ffb5ce0a0e4b829c` |

### Homebrew And Runtime

| Release | Tap authority | Bounded runtime proof |
|---|---|---|
| `v0.30.38` | homebrew-tap `1b4d937812ef42f12cc0eb893b7f6e6a8a4b824b`; Formula/Cask version `0.30.38`; four Darwin/Linux hashes match the release table | Task 2 recorded a bounded Homebrew installation, but its exact historical `brew list` and installed-path transcript were not retained; retained released-binary output: `ratchet 0.30.38 (c391e5a70e144fd9ce58ca78eb4ce7ea09b5b6e4, 2026-07-25T21:58:27Z)`; setup catalog count `22` |
| `v0.30.39` | homebrew-tap `0d855b5694597114e1918457c1ab0415ab74ac0b`; Formula/Cask version `0.30.39`; four Darwin/Linux hashes match the release table | `brew list --versions ratchet-cli` returned `ratchet-cli 0.30.39`; installed binary returned `ratchet 0.30.39 (5afbfd0fafcefe7ecc47e41bd876537a873a7e88, 2026-07-26T01:24:28Z)`; setup catalog count `22` |

Registry #630 and #631 each projected all six plugin archive hashes exactly,
served private per-plugin manifests at `v0.12.9` and `v0.12.10`, and retained a
bulk-index count of zero. Open Dependabot alert counts were zero for both
workflow-plugin-agent and ratchet-cli after Task 7.

Copilot was requested on every authored/generated PR but did not materialize a
review. Task 7 instead completed four local adversarial review rounds and ended
`SHIP-IT` with no Critical or Important findings. All GitHub review-thread
queries for Tasks 1-7 were empty at merge.

Two execution backports preserved scope: Task 1 raised workflow-registry's
Workflow-derived Go floor from 1.26.4 to 1.26.5 after sibling `main` moved, and
Task 5 replaced the nonexistent Docker module pin with the published Moby
client/API split. Neither added an SDK copy, downstream override, task, or PR.

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
	"github.com/GoCodeAlone/workflow-plugin-authz": "v0.6.0",
	"github.com/moby/moby/api":                    "v1.55.0",
	"github.com/moby/moby/client":                 "v0.5.0",
	"github.com/ollama/ollama":                    "v0.32.4",
	"google.golang.org/grpc":                      "v1.82.1",
}
```
The committed guard rejects a direct legacy `github.com/docker/docker`
requirement and replacement directives for the tracked modules or legacy
Docker. Task 5's GREEN verification separately requires `go list -m all` to
contain no legacy Docker module.

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
go get github.com/moby/moby/client@v0.5.0
go get github.com/moby/moby/api@v1.55.0
go get github.com/GoCodeAlone/workflow-plugin-authz@v0.6.0
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
go list -m github.com/GoCodeAlone/workflow-plugin-authz \
  github.com/moby/moby/client github.com/moby/moby/api \
  github.com/ollama/ollama google.golang.org/grpc
go mod why -m github.com/GoCodeAlone/workflow-plugin-authz
go mod why -m github.com/moby/moby/client
go mod why -m github.com/moby/moby/api
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
return to the legacy Docker module/Docker <=28.5.2, Ollama <=0.20.2, or
gRPC <1.82.1.

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
- Moby client/API and Ollama remain indirect only, the legacy Docker module is
  absent, and none has a replace or exclude;
- no replacement for workflow-plugin-agent/gRPC.
- the selected module graph resolves those exact versions and contains no
  legacy Docker module;
- the production package-import graph from `internal/daemon` and `cmd/ratchet`
  reaches Moby client/API and Ollama only through workflow-plugin-agent,
  excluding test-only imports, across all six GoReleaser OS/architecture
  targets.

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
  github.com/moby/moby/client github.com/moby/moby/api \
  github.com/ollama/ollama google.golang.org/grpc
go mod why -m github.com/moby/moby/client
go mod why -m github.com/moby/moby/api
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
