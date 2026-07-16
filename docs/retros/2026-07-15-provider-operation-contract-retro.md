# Retro: Provider Operation Contract

**PR:** #140 - fix(provider): expose applied operation state
**Merged:** 2026-07-16
**Branch:** `feat/provider-operation-contract`
**Design:** `docs/plans/2026-07-15-ratchet-cli-provider-operation-contract-design.md`
**Plan:** `docs/plans/2026-07-15-ratchet-cli-provider-operation-contract.md`
**Related ADRs:** `decisions/0011-expose-provider-applied-state.md`

## Adversarial-review findings, scored

| Phase | Finding | Severity | Outcome |
|---|---|---|---|
| design | D1: lifecycle lacked human documentation | Important | Resolved upfront: README lifecycle and docs guard shipped. |
| design | D2: generated-client proof missed the real command boundary | Important | Prescient: the built CLI/daemon smoke exposed startup recovery behavior before merge. |
| design | D3: state-only proof could miss failure metadata or raw errors | Important | Prescient: implementation added result/failure assertions and a classified provider-error boundary. |
| design | D4: secret providers may ignore cancellation | Minor | Inconclusive: explicitly bounded out of scope; no new cancellation behavior shipped. |
| plan | P1: substring table could not prove the exact diagnostic | Critical | Resolved upfront: standalone exact-equality RED/GREEN regression shipped. |
| plan | P2: one-PR manifest contradicted required post-merge closeout | Critical | Resolved upfront: manifest locked at five tasks and two PRs. |
| plan | P3: proposed installed command was not a valid help surface | Important | Resolved upfront: release proof used `provider setup list --json`. |
| plan | P4: snapshot verification was not directly executable | Important | Resolved upfront: exact GoReleaser and manifest commands were locked. |

## Merge and release evidence

| Boundary | Evidence |
|---|---|
| PR | #140 admin squash-merged as `afd4c80d738924f3aecf151fc4930e6a9df50e6f`; PR CI `29486495170` and Copilot 15/15-file review green with no comments. |
| Merge commit | CI `29487319908` and CodeQL `29487319469`, `29487319512` green; CI included race, release snapshot, Windows build/release/ConPTY, TUI smoke, lint, and vet. |
| Release | `v0.30.34` at `afd4c80`; workflow `29488160366` green; public, non-draft, non-prerelease. |
| Homebrew | Formula and Cask both `0.30.34`; four macOS/Linux hashes matched the release manifest. |
| Runtime | Direct and installed outputs: `ratchet 0.30.34 (afd4c80d738924f3aecf151fc4930e6a9df50e6f, 2026-07-16T09:56:11Z)` within 15 seconds. |
| Catalog | Installed `ratchet provider setup list --json` returned 22 entries within 30 seconds. |

| Archive | SHA-256 |
|---|---|
| `ratchet_darwin_amd64.tar.gz` | `3bf207ef4d5b4e2f322853d5e4f8422cac26303bde923cb30d2855782a21b963` |
| `ratchet_darwin_arm64.tar.gz` | `930a465009f0d39a3c6e5771f55be8eeaaf0423b38b86fc76d63628b4e1f1718` |
| `ratchet_linux_amd64.tar.gz` | `bc91492181dbd1b236a3cbcd5d7a9ccc1e55a2047743d140ffbe908626a74528` |
| `ratchet_linux_arm64.tar.gz` | `0e59cc6bf015df8b0af3cc6f404b264951d2e679d5297e45e90baafa1ba459f6` |
| `ratchet_windows_amd64.zip` | `5ae29d5d1c9f27f8b2f7b84375a5e35aa4d8cb31a815be802e4526f84ddcd3b9` |
| `ratchet_windows_arm64.zip` | `7a5d35f24f978dfb536549f183ee960f20d4456394b19e3777aa4bfc8a2f1a8c` |

`checksums.txt` validated all six platform archives.

## Gate misses

| Issue | Gate that missed | Why it slipped | Fix idea |
|---|---|---|---|
| Startup inventory calls could expose provider-supplied text. | adversarial-design-review (plan) | The plan asserted no raw errors at the operation response but did not enumerate sibling startup inventory calls crossing the same diagnostic boundary. | When a design introduces a sanitizer boundary, inventory every sibling call from that provider before naming the proof complete. |
| Background daemon launch could not prove foreground diagnostic text. | adversarial-design-review (plan) | The built smoke proved behavior but `StartBackground` disconnects launcher output, so its negative text assertion was vacuous for startup diagnostics. | Require each diagnostic claim to name the process/output boundary that can actually observe it. |

Local full-diff review caught both misses before merge. Copilot reviewed all 15
files and generated no comments. Every PR and merge-commit CI job was green.

## Implementation-review findings, scored

| Finding | Earliest applicable gate | Outcome |
|---|---|---|
| Provider inventory calls could return raw provider text. | adversarial-design-review (plan) | Gate miss; local full-diff review required shared classified sanitization. |
| Background launch output could not observe foreground diagnostics. | adversarial-design-review (plan) | Gate miss; manager-level tests replaced the vacuous proof. |
| README implied broader retryability than permanent/context/inventory failures allow. | requesting-code-review | Correctly caught; wording now distinguishes retryable and fail-stop failures. |
| A proposed `service.go` helper exceeded the locked file set. | requesting-code-review | Correctly caught; helper was rejected and removed. |
| Context diagnostics needed the same provider-text redaction guarantee. | requesting-code-review | Correctly caught; final shared boundary covers the path. |
| A fallback could discard an already committed result. | requesting-code-review | Correctly caught; result preservation regression shipped. |

## Missed skill activations

The canonical repository activation log was unavailable; artifact and PR
evidence below is recorded without inventing hook evidence.

| Gate | Fired? | Notes |
|---|---|---|
| brainstorming | unverified | Approved design and self-challenge exist; activation log unavailable. |
| adversarial-design-review (design) | unverified | Committed PASS report with D1-D4; activation log unavailable. |
| writing-plans | unverified | Locked implementation plan exists; activation log unavailable. |
| adversarial-design-review (plan) | unverified | Committed PASS report with P1-P4; activation log unavailable. |
| alignment-check | unverified | Committed PASS alignment report; activation log unavailable. |
| subagent-driven-development | unverified | Commit history and local review evidence exist; activation log unavailable. |
| finishing-a-development-branch | unverified | PR evidence exists; activation log unavailable. |
| finishing Step 1e (doc-reconciliation) | yes | PR body records `Doc-reconciliation: clean`. |
| pr-monitoring | unverified | All 13 PR checks, merge CI, and both CodeQL runs settled green; activation log unavailable. |
| post-merge-retrospective | unverified | Retro artifact exists; canonical activation log unavailable. PR2 verification, merge, and release follow. |

## What worked

- Real built CLI -> daemon -> SQLite/file-secret proof found the restart recovery
  requirement before merge and now proves `APPLIED` -> `COMMITTED`.
- Exact diagnostic and metadata-only assertions prevented substring-only and
  state-only tests from passing vacuously.
- Three local full-diff review rounds covered sibling inventory sanitization;
  Copilot and all PR/base checks then settled cleanly.
- `v0.30.34` published seven assets: one checksum manifest and six
  checksum-validated platform archives; direct and Homebrew
  binaries reported merge `afd4c80` and the installed catalog returned 22 providers.

## What didn't

- The plan did not identify that background launch output cannot observe
  foreground startup diagnostics; manager-level tests had to supply that proof.
- The first full race run exposed a pre-existing cleanup-fairness wait race;
  focused stress and the exact rerun passed, but deterministic waiting remains a follow-up.
- Shared disk pressure removed a Go build temp directory during closeout
  baseline verification; an isolated short `GOTMPDIR` rerun passed affected packages.

## Plugin-level follow-ups

No plugin change is warranted from one retro. If either miss recurs, extend
plan review to enumerate sibling provider calls at sanitization boundaries and
to require an observable process/output boundary for diagnostic assertions.

## Project guidance updates

| Guidance file | Change | Reason |
|---|---|---|
| `docs/design-guidance.md` | no change | Existing privacy and real-consumer boundary rules already cover both lessons; execution and plan review need to apply them more completely. |
