# Alignment Report: TUI Binary Verification

**Design:** `docs/plans/2026-07-03-ratchet-cli-tui-binary-verification-design.md`
**Plan:** `docs/plans/2026-07-03-ratchet-cli-tui-binary-verification.md`
**Status:** PASS

## Coverage

| Design Requirement | Plan Task(s) | Status |
|---|---|---|
| Keep release `ratchet` and test-only `ratchet-tui-smoke` separated by build tags. | Task 1, Task 3, Task 7, Task 10, Task 11 | Covered |
| Build and drive a credential-free Unix PTY TUI smoke binary through real Bubble Tea flow. | Task 1, Task 2, Task 3 | Covered |
| Prove daemon-backed provider, session, trust, chat, jobs, and clean shutdown paths without leaking host state. | Task 2, Task 3, Task 4 | Covered |
| Split slash-command and shortcut evidence into PTY-proven, focused-proven, and deferred-runtime classes. | Task 3, Task 5, Task 6, Task 12 | Covered |
| Keep docs truthful and fail when automated evidence is removed or overclaimed. | Task 6, Task 12, Task 13 | Covered |
| Add releaseguard and wrapper checks for snapshot archives, packaged binaries, checksums, cask/tap material, and redaction. | Task 7, Task 8, Task 10, Task 11 | Covered |
| Keep GoReleaser v2.16+ validation executable: supported `homebrew_casks`, deprecated `brews` rejected, legacy Formula handled as tap cleanup. | Task 8, Task 9, Task 10, Task 11 | Covered |
| Gate Homebrew tap cleanup/preflight/postcheck with state-proof SHA and rollback evidence. | Task 9, Task 10, Task 11, Task 13 | Covered |
| Add release preflight and draft-asset postcheck before undrafting published releases. | Task 10, Task 11, Task 13 | Covered |
| Build for Windows honestly without adding runner classes: cross-build plus package/archive inspection only. | Task 1, Task 10, Task 11, Task 13 | Covered |
| Preserve redaction for temp paths, prompts, trust bodies, release/tap/artifact diagnostics, and workflow-command failures. | Task 3, Task 4, Task 6, Task 7, Task 10, Task 11 | Covered |
| Keep the work split into six PRs with explicit closeout and retro. | Scope Manifest, Task 13 | Covered |

## Scope Check

| Plan Task | Design Requirement | Status |
|---|---|---|
| Task 1 | Build-tagged smoke binary boundary, source manifest, client/socket containment, Windows negative package boundary. | Justified |
| Task 2 | Smoke daemon service, safe RPC wiring, job panel error visibility. | Justified |
| Task 3 | PTY runtime proof for the TUI smoke binary and representative command/shortcut rows. | Justified |
| Task 4 | Release-shaped startup smoke, daemon shutdown RPC, temp state cleanup. | Justified |
| Task 5 | Command-surface classification, help/autocomplete alignment, focused shortcut contracts, PTY rerun after fixture expansion. | Justified |
| Task 6 | Documentation evidence guards and overclaim prevention. | Justified |
| Task 7 | Releaseguard package/wrapper, artifact matrix checks, packaged binary execution proof, shared redaction. | Justified |
| Task 8 | GoReleaser Homebrew cask guard, deprecated `brews` rejection, tap preflight shape. | Justified |
| Task 9 | Homebrew tap cleanup prerequisite and recorded tap state proof. | Justified |
| Task 10 | CI release-check, non-race smoke jobs, Windows temp output guard, tap preflight. | Justified |
| Task 11 | Release workflow preflight, draft assets, tap postcheck, Windows archive proof without runner changes. | Justified |
| Task 12 | Final docs and harness table evidence boundaries. | Justified |
| Task 13 | Release, retro, workspace state, and scope closeout. | Justified |

## Manifest Trace

`plan-scope-check.sh --plan docs/plans/2026-07-03-ratchet-cli-tui-binary-verification.md` returned PASS. The manifest declares 6 PRs and 13 tasks; every task heading exists and every task appears in exactly one PR row.

## Drift Items

None.
