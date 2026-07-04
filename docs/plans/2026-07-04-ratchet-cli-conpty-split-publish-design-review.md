# Adversarial Review Report

**Phase:** design
**Artifact:** `docs/plans/2026-07-04-ratchet-cli-conpty-split-publish-design.md`
**Status:** PASS

## Findings

- D1 [assumption, important]: The design assumes GoReleaser writes the generated
  cask to `dist/homebrew/Casks/ratchet-cli.rb` when upload is skipped. Fix:
  require local snapshot verification and workflow tests for that exact path.
  Resolution: covered by Tasks 2, 3, and 10.
- D2 [failure mode, important]: Tap push can fail after draft asset validation,
  leaving a valid draft release but no install path update. Fix: keep the
  GitHub release draft, record exact tap commit only after push, and undraft
  only after postcheck. Resolution: covered by Tasks 3-5.
- D3 [runtime validity, important]: A Windows CI job that only cross-compiles
  would not prove ConPTY. Fix: use a Windows-only `tui_smoke` test that drives
  a ConPTY-backed terminal and keep cross-compilation as supporting evidence.
  Resolution: covered by Tasks 7-8.

## Checks

- Project guidance: clean; design follows existing releaseguard, GoReleaser,
  and `tui_smoke` conventions instead of adding public helper binaries.
- Repo precedent: clean; release gating remains in `internal/releaseguard` and
  workflows, matching prior release artifact guard work.
- YAGNI: clean; one Windows hosted job and one explicit tap publish script are
  the minimum to close the two stated follow-ups.
- Security/privacy: clean after D2; no wildcard listener or host plugin loading
  is allowed in Windows smoke.
- Infrastructure impact: clean; no self-hosted runner or dogfooded compute
  runner change is included.
- Rollback: clean; design has repo revert, draft release deletion, and exact
  tap commit revert paths.

