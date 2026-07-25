# Ratchet CLI CI And SDK Security Maintenance Design

**Status:** Approved
**Date:** 2026-07-25
**Scope:** Release-driven registry synchronization, current GitHub Action runtimes, owner-first Docker/Ollama remediation, gRPC advisory remediation, downstream consumption, and release closeout across `workflow-registry`, `workflow-plugin-agent`, and `ratchet-cli`.

## Context

The released ratchet-cli `v0.30.37` tree is clean and its native Windows,
Linux, macOS, GoReleaser, Homebrew, and releaseguard paths are green. Current
primary-source inventory on 2026-07-25 found:

| Surface | Current | Target |
|---|---|---|
| `actions/checkout` | v4 | v7 |
| `actions/setup-go` | v5 | v7 |
| `actions/upload-artifact` | v4 (ratchet-cli) | v7 |
| `actions/github-script` | v7 | v9 |
| `codecov/codecov-action` | v4 (ratchet-cli) | v7 |
| `golangci/golangci-lint-action` | v8 (ratchet-cli) | v9 |
| `goreleaser/goreleaser-action` | v7 | retain v7 |
| Docker Go module | v28.5.2 | v29.6.2 |
| Ollama Go module | v0.18.3 | v0.32.4 |
| gRPC Go module | v1.81.1 | v1.82.1 |

Official latest releases are checkout v7.0.1, setup-go v7.0.0,
upload-artifact v7.0.1, github-script v9.0.0, codecov v7.0.0,
golangci-lint-action v9.3.0, Docker v29.6.2, Ollama v0.32.4, and gRPC v1.82.1.
The workflows follow the repositories' existing major-tag convention.

Dependabot reports seven open alerts in both `workflow-plugin-agent` and
ratchet-cli: five Docker alerts, one Ollama alert, and one gRPC alert.
Docker and Ollama enter ratchet-cli through `workflow-plugin-agent`; gRPC is a
direct ratchet-cli dependency and an indirect plugin dependency through
Workflow's external-plugin protocol.

The plugin's `v0.12.8` release successfully dispatched `plugin: agent` to
workflow-registry, but run `29143345613` skipped synchronization while reporting
success. `scripts/resolve-sync-plugin-filter.sh` maps an unprefixed name to
`workflow-plugin-<name>` only when both paths exist. The registry has only
`plugins/workflow-plugin-agent`, so release-driven sync failed. A later full
daily sync repaired v0.12.8; the release boundary remains defective.

## Global Design Guidance

Source: `docs/design-guidance.md`; workspace `docs/PORTFOLIO.md`,
`docs/PROJECTS.md`, and `docs/FOLLOWUPS.md`.

| Guidance | Design response |
|---|---|
| Reuse SDK/plugin ownership; no parallel integrations. | Upgrade Docker/Ollama only in `workflow-plugin-agent`; ratchet-cli consumes a released plugin version. |
| Preserve native Windows, macOS, and Linux release paths. | Keep existing native Windows jobs and six-platform release assets; action guards reject removed coverage. |
| Bound and inspect failure. | Split CI-runtime and SDK changes, require release/registry evidence per merge, and reject warning-only registry skips. |
| Release only merge commits and verify installed output. | Every ratchet-cli merge gets a patch release with checksum, Homebrew, bounded version, and provider-catalog proof. |
| Prefer current portfolio capability. | Use existing release dispatch, `wfctl plugin registry-sync`, Docker SDK, Ollama SDK, gRPC, releaseguard, and registry tests. |

Initiative: workspace `misc tools & libs`, current phase "Actions maintenance,
owner-first Docker/Ollama remediation, then ratchet-cli consumption."

## Approaches

| Approach | Trade-off | Decision |
|---|---|---|
| Four maintenance PRs plus registry repair/sync/closeout | More releases, but CI runtime, SDK API churn, registry automation, and downstream consumption remain independently diagnosable and rollbackable. | Choose. |
| One PR per code repository | Fewer merges, but combines action-runtime migration with SDK/module churn and makes release failures ambiguous. | Reject. |
| Ratchet-only module overrides | Fast alert suppression, but duplicates dependency ownership and can diverge from the plugin's tested module graph. | Reject per operator direction. |

## Design

### Release-Driven Registry Sync

- Fix the existing general alias resolver, not the agent manifest: when
  `plugins/<name>` is absent and `plugins/workflow-plugin-<name>` exists, accept
  the prefixed candidate only when its normalized manifest repository is
  exactly `GoCodeAlone/workflow-plugin-<name>` (accepting the registry's
  established HTTPS or git+SSH representation, with no arbitrary owner).
- Preserve the existing core-vs-external disambiguation when both paths exist.
- Invalid names, owner/repo traversal forms, unknown plugins, and mismatched
  repository metadata continue to skip safely.
- Extend `tests/test-resolve-sync-plugin-filter.sh` with the exact
  `agent`-only-prefixed regression and negative mismatched-repository coverage.
- Merge and validate workflow-registry main before publishing another agent
  plugin tag. Main deployment must complete before the first release dispatch.
- Each subsequent plugin release must create or update the expected generated
  sync PR; merge it only after registry validation is green, then prove the
  repository-main and Pages per-plugin manifest versions plus all six
  checksums. Because the agent plugin is private, prove the public bulk index
  omits it. The existing registry contract intentionally publishes per-plugin
  manifest metadata; private release assets still require repository access.
- A green dispatch run is insufficient by itself. After every run, compare the
  repository-main manifest version with the released tag; `skip=1`,
  `changed=0`, or no generated PR while versions differ is a failed
  publication gate. Re-run with the canonical prefixed plugin name only after
  the resolver defect is fixed; do not continue to consumer release.

### GitHub Action Runtimes

- Ratchet-cli: update checkout/setup-go/upload-artifact/github-script/codecov/
  golangci-lint majors; retain GoReleaser v7.
- `internal/releaseguard/workflow_test.go` is the ratchet authority for current
  action majors and for retained release/native-Windows jobs. Add assertions
  before changing YAML.
- Workflow-plugin-agent: update checkout/setup-go/github-script majors; retain
  GoReleaser v7 and the SHA-pinned repository-dispatch v4 action.
- Add a small parsed workflow contract test in workflow-plugin-agent before
  changing YAML. It asserts action majors, Go 1.26, release publication order,
  registry notification, and the existing six-platform GoReleaser contract.
- The current github-script bodies use injected `github`, `context`, and `core`
  plus Node's `fs`; neither repository imports `@actions/github`, so v9's ESM
  breaking change requires no script rewrite. Tests reject that import pattern.
- Do not change runner labels or permissions in this phase.

### Owner-First SDK Remediation

- In workflow-plugin-agent, upgrade Docker to v29.6.2, Ollama to v0.32.4, and
  gRPC to v1.82.1 with `go get`/`go mod tidy`; do not copy or fork SDK code.
- Keep `orchestrator.dockerAPIClient` and `provider.OllamaClient` as the narrow
  compatibility boundaries. Adapt only compile-time API changes at those
  boundaries.
- Add/strengthen fake Docker lifecycle tests for image inspection/pull,
  create/start/inspect/exec/stop/remove, option propagation, and error paths.
- Add httptest-backed Ollama tests for list, pull progress, health, malformed
  server URL fallback, context cancellation, and server errors using the real
  upstream client through the wrapper.
- Run the plugin's full race suite, vet, strict contract validation,
  GoReleaser snapshot, and cross-builds before merge.
- Release the plugin from each merge commit. The SDK release cannot be consumed
  until its registry sync PR is merged and checksums match.

### Ratchet CLI Consumption

- Upgrade ratchet-cli to the released SDK-remediated plugin and gRPC v1.82.1;
  run `go mod tidy`; add no Docker/Ollama override or replace directive.
- Prove `go mod why` routes Docker/Ollama through workflow-plugin-agent and
  `go list -m` resolves Docker v29.6.2, Ollama v0.32.4, gRPC v1.82.1, and the
  expected plugin tag.
- Exercise real ratchet consumers: provider catalog, Ollama wrapper-backed
  setup/model paths with local HTTP fakes, Docker orchestrator package tests,
  daemon/client gRPC tests, releaseguard, and native Windows CI.
- Confirm the seven Dependabot alerts are dismissed/closed after GitHub
  re-evaluates the merged module graph.

### Releases And Closeout

- Sequence:
  1. workflow-registry alias fix and main deployment;
  2. ratchet-cli Actions release v0.30.38;
  3. workflow-plugin-agent Actions release v0.12.9;
  4. generated registry sync to v0.12.9;
  5. workflow-plugin-agent SDK release v0.12.10;
  6. generated registry sync to v0.12.10;
  7. ratchet-cli consumer release v0.30.39;
  8. ratchet-cli retrospective/plan closeout release v0.30.40.
- Version targets assume no intervening release. If another merge advances a
  repository, use the next patch versions without changing feature scope.
- Every PR receives Copilot review, all reported checks/threads settle, then an
  admin squash merge. Delayed checks do not waive local verification or
  required repository gates.

## Error Handling

| Failure | Result |
|---|---|
| Registry dispatch uses unknown/unsafe name | Resolver emits `skip=1`; no path access or PR; release verification still fails if the repository manifest is stale. |
| Valid short name has prefixed external manifest | Resolver emits canonical prefixed directory and sync proceeds. |
| Action major breaks workflow input/runtime | Contract test or PR CI fails; revert only that action PR. |
| github-script ESM incompatibility | Script step fails before publication; draft remains unpublished and tap is unchanged. |
| Docker/Ollama API drift | Compile/focused wrapper test fails in owner repo; adapt wrapper without downstream override. |
| Plugin release succeeds but registry sync fails | Stop before consumer bump; repair/re-run sync and merge generated PR. |
| Dependabot reevaluation lags | Prove resolved module versions locally/GitHub dependency graph; poll boundedly and record delayed alert state without reintroducing vulnerable versions. |
| Ratchet release validation fails | Leave draft/unpublished state per releaseguard and issue a corrected patch from a merge commit. |

## Security Review

- Dependency changes remove known vulnerable Docker, Ollama, and gRPC ranges.
- Existing GitHub token/secret names and least-privilege permissions remain
  unchanged; no credential values enter logs, tests, PR bodies, or registry
  manifests.
- Registry aliasing is constrained by filename regex, path rejection, manifest
  existence, and exact normalized `GoCodeAlone/workflow-plugin-<name>`
  repository identity; it is not an arbitrary prefix fallback.
- The private agent entry remains absent from the public bulk index. The
  existing public per-plugin manifest must contain metadata only and no
  credential values; private release downloads remain access-controlled by
  GitHub.
- GitHub Actions remain major-tag pinned to match repository convention.
  Release integrity still relies on current action publishers and existing
  checksum/draft/tap guards.
- No new network listener, auth/authz path, secret store, or command executor.

## Infrastructure Impact

- GitHub-hosted CI/release jobs change JavaScript runtimes through action
  majors; runner labels and permissions remain unchanged.
- workflow-registry main triggers its existing Pages/build deployment. This is
  an approved publication path, not a new environment or destructive change.
- Plugin release dispatch creates registry synchronization PRs; no direct
  mutation bypasses review.
- No cloud resource, IAM, database, storage, migration, or production runtime
  resource changes.

## Multi-Component Validation

| Boundary | Proof |
|---|---|
| plugin release -> repository dispatch -> registry resolver | Release job success, dispatch run logs canonical prefixed plugin, generated sync PR. |
| registry sync -> release assets | Repository-main and Pages per-plugin manifest tag/checksums match all six plugin archives; public bulk index omits the private plugin; registry validators and main deployment green. |
| action contract -> hosted runner | Parsed workflow tests plus actual PR CI and tag release jobs. |
| Docker SDK -> plugin wrapper | Fake-client lifecycle tests and full plugin race suite. |
| Ollama SDK -> HTTP service wrapper | Real upstream client against httptest list/pull/health/error endpoints. |
| gRPC -> Workflow plugin and ratchet daemon/client | Plugin external adapter tests and ratchet daemon/client suites. |
| plugin release -> ratchet consumer | Released module graph, provider/orchestrator tests, full ratchet suite. |
| ratchet release -> supported OS/package | Linux/macOS/Windows archives, checksums, Formula/Cask, Homebrew binary, native Windows CI. |

Declared integrations:

| Integration | Class | Validation |
|---|---|---|
| GitHub Actions | runtime-integrated | PR/tag jobs run the changed action majors |
| workflow-registry | runtime-integrated | dispatch creates reviewed manifest PR; repository main and Pages per-plugin manifest expose the version; bulk index preserves private exclusion |
| Docker SDK | runtime-integrated | plugin wrapper fake lifecycle; ratchet consumes released wrapper |
| Ollama SDK | runtime-integrated | httptest-backed upstream client behavior; ratchet provider paths |
| gRPC | runtime-integrated | plugin adapter plus ratchet daemon/client tests |
| Homebrew tap | runtime-integrated | releaseguard, matching hashes, installed bounded version probe |

## Assumptions

| ID | Assumption | Challenge | Fallback |
|---|---|---|---|
| A1 | Current action majors support existing inputs and hosted runners. | Node/runtime or input changes may be undocumented. | Isolated action PRs; actual PR/tag jobs are release gates. |
| A2 | github-script v9 accepts Node built-in `require("fs")`. | ESM migration could forbid all `require`. | Replace only built-in access with `import()` if actual release job disproves it. |
| A3 | Docker v29.6.2 preserves the narrow client operations used by the plugin. | `types`/option shapes may move. | Adapt the owner wrapper and interface; do not expose SDK types downstream. |
| A4 | Ollama v0.32.4 preserves list/pull/heartbeat semantics. | Response fields or endpoints may change. | Normalize in `provider.OllamaClient`, verified against httptest responses. |
| A5 | Registry prefix mapping can be generalized safely by repository identity. | Another short name may ambiguously map to core/external manifests. | Preserve direct/core precedence; skip on identity mismatch. |
| A6 | No intervening release consumes planned patch numbers. | Parallel work may tag first. | Recompute next patch versions at each release; scope/ordering stay fixed. |
| A7 | GitHub reevaluates Dependabot after module graph changes. | Alert closure may lag. | Treat resolved versions as security proof; continue bounded polling and record GitHub lag. |

## Self-Challenge

1. **Simplest solution:** downstream `go get` overrides could clear alerts with
   one PR, but would leave the owner plugin vulnerable and violate explicit
   ownership direction.
2. **Fragile assumption:** A3/A4 span large SDK version jumps. Narrow wrapper
   tests and an owner-first release keep breakage contained before ratchet
   consumption.
3. **Unasked scope:** an organization-wide Action sweep would be broader than
   this initiative; only ratchet-cli and workflow-plugin-agent are updated.
4. **Partial failure:** successful plugin publication with skipped registry sync
   previously looked green. The repaired resolver and mandatory generated PR
   evidence make that boundary explicit.
5. **Repo precedent:** cross-repo provider work already releases
   workflow-plugin-agent before ratchet consumption; this design follows that
   pattern.

## Non-Goals

- Reimplement Docker, Ollama, gRPC, Workflow registry sync, GoReleaser, or
  Homebrew behavior.
- Change runners, permissions, provider APIs, agent UX, daemon schemas,
  workflow DSL, or self-improving harness behavior.
- Upgrade unrelated application dependencies or sweep Actions across the
  organization.
- Fold the nested TUI test-build or startup-reconciliation follow-ups into this
  security-maintenance scope.

## Rollback

- Action PRs are independently revertible. If a tag release fails, revert the
  offending action major in a new merge and publish the next patch; never move
  or overwrite a release tag.
- SDK rollback may return only to versions outside every known vulnerable
  range. If latest APIs prove incompatible, select the newest compatible safe
  release and document the exact advisory boundary.
- Registry alias fix is additive; revert it if a false mapping is observed.
  Existing fully qualified/prefixed dispatches and daily full sync remain.
- Module and registry formats do not migrate; no data rollback is required.

## Release And PR Model

Eight planned PRs: registry resolver fix; ratchet Actions; plugin Actions;
generated registry v0.12.9 sync; plugin SDK remediation; generated registry
v0.12.10 sync; ratchet consumer; ratchet retrospective/plan closeout. Registry
has no tag-based product release; its main merge is validated through the
existing build/deploy workflow. Every plugin and ratchet merge receives the
next patch release and the verification described above.
