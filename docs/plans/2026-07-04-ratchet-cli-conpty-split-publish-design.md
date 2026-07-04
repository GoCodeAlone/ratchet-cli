# Ratchet CLI Windows ConPTY and Split Publish Design

Date: 2026-07-04
Status: locked for execution
Branch: `feat/conpty-split-publish`

## Goal

Close the two release follow-ups left by the TUI verification work:

1. Prove a real Windows interactive terminal path with ConPTY, not just
   cross-compiled Windows archives.
2. Split release publishing so Homebrew tap mutation happens only after draft
   GitHub release assets are verified and before the GitHub release is made
   public.

## Sources Checked

- GoReleaser Homebrew Casks docs say `skip_upload: true` writes the generated
  cask to `dist` and leaves publishing to the user:
  <https://goreleaser.com/customization/publish/homebrew_casks/>.
- GoReleaser release docs document draft/existing-release behavior and artifact
  upload controls:
  <https://goreleaser.com/customization/publish/scm/>.
- GoReleaser OSS split/merge is a Pro-only feature, so this plan does not rely
  on `goreleaser continue --merge`:
  <https://goreleaser.com/customization/general/partial/>.
- Microsoft ConPTY docs require independent synchronous input/output channels
  and warn that handle lifetime mistakes can deadlock:
  <https://learn.microsoft.com/en-us/windows/console/creating-a-pseudoconsole-session>
  and
  <https://learn.microsoft.com/en-us/windows/console/createpseudoconsole>.
- GitHub Actions docs confirm hosted runners run jobs on GitHub-managed VMs:
  <https://docs.github.com/en/actions/concepts/runners/github-hosted-runners>.

## Global Design Guidance

The workspace guidance says to inspect repo-local conventions, avoid duplicate
plumbing, and prefer existing libraries. Ratchet already has:

- `internal/releaseguard` for release workflow invariants;
- `tui_smoke` build-tagged test-only interactive proof;
- GoReleaser as the release artifact and Homebrew cask generator;
- docs guards that keep public evidence claims honest.

The follow-ups should extend those surfaces instead of adding a new public
helper binary or a second release system.

## Design

### Split Publish

Keep `.goreleaser.yaml` as the single release config, but set
`homebrew_casks[].skip_upload: true`. GoReleaser still generates
`dist/homebrew/Casks/ratchet-cli.rb`, but it no longer pushes the tap during the
first `goreleaser release --clean` step.

The tag release workflow becomes:

1. Run `goreleaser check`, snapshot, manifest guard, and tap preflight.
2. Run `goreleaser release --clean`, publishing only the draft GitHub release
   and generated local cask material.
3. Resolve and download the draft release assets.
4. Run `draft-assets` releaseguard against downloaded release artifacts.
5. Clone `GoCodeAlone/homebrew-tap`.
6. Copy the GoReleaser-generated `dist/homebrew/Casks/ratchet-cli.rb` into the
   tap, commit, and push it.
7. Run `tap-postcheck` against the exact path-changing cask commit.
8. Undraft the GitHub release.

If steps 4-7 fail, the GitHub release remains draft and the tap either remains
unchanged or has an exact commit SHA available for revert.

### Windows ConPTY

Do not implement raw `CreatePseudoConsole` calls in ratchet-cli. Add a
Windows-only `tui_smoke` test using `github.com/ActiveState/termtest`, whose
`xpty` module uses ActiveState's ConPTY wrapper on Windows. The test builds the
existing test-only `ratchet-tui-smoke` binary and drives the same visible TUI
boundary used by the Unix PTY proof.

The existing smoke daemon/client are currently Unix-socket-only. For Windows,
add `tui_smoke && windows` variants that bind a loopback-only gRPC listener on
`127.0.0.1:0` inside the test-owned temp root. The Windows helper must:

- keep MCP, plugin loading, autoresponders, and cron disabled like the Unix
  smoke daemon;
- pass the resolved loopback target directly from daemon startup to client
  connection without env-discovered host state;
- stay behind `tui_smoke`, never included in release builds.

Add a required CI job on `windows-2025` that runs the Windows ConPTY smoke test.
This changes the job OS only for ratchet-cli CI; it does not change any
dogfooded compute runner setup.

## Security Review

- Homebrew publishing continues to use the existing `HOMEBREW_TAP_TOKEN` only
  in the tap-publish step. The first GoReleaser release step no longer needs to
  push to the tap.
- The tap publish script must reject missing generated cask files, forbidden
  smoke tokens, dirty tap state, and no-op commits for a tag that should update
  the cask.
- Windows smoke uses loopback only and a temp root. It must not open a wildcard
  listener, load host plugins, or use host MCP discovery.
- Public docs must say Windows ConPTY is smoke-proven only for the test-only
  `ratchet-tui-smoke` path until the release binary itself is driven through
  ConPTY.

## Infrastructure Impact

- Adds one GitHub-hosted Windows CI job.
- Release workflow still produces a draft GitHub release before public
  publication.
- Homebrew tap mutation moves from GoReleaser's Homebrew publisher to an
  explicit post-draft-asset-check step that publishes GoReleaser-generated cask
  content.

## Multi-Component Validation

- `go test ./internal/releaseguard -count=1`
- `go test ./cmd/ratchet -run TestHarnessDocs -count=1`
- `go test -tags tui_smoke ./internal/tui -run TUIBinarySmoke -count=1`
  on Unix hosts
- `go test -tags tui_smoke ./internal/tui -run WindowsConPTY -count=1`
  on Windows hosts
- `goreleaser check`
- `goreleaser release --snapshot --clean --skip=publish`
- `scripts/check-release-artifacts.sh --manifest-only dist`
- `GOOS=windows GOARCH=amd64 go test -c -tags tui_smoke ./internal/tui`

## Assumptions

- GitHub-hosted `windows-2025` runners have the ConPTY-capable Windows console
  APIs needed by ActiveState `xpty`.
- GoReleaser continues to write generated casks at
  `dist/homebrew/Casks/ratchet-cli.rb` when `homebrew_casks[].skip_upload` is
  true.
- The tap repository already has only the managed cask path after prior cleanup.

## Rollback

- Revert this repo PR to restore GoReleaser tap publishing and remove the
  Windows ConPTY CI job.
- If a tap commit is pushed but postcheck fails, keep the GitHub release draft
  and revert the exact cask commit recorded by the release workflow.
- Delete the draft GitHub release if artifact checks fail before public
  publication.

## Backport Notes

- 2026-07-04: Full default tests showed the smoke-source manifest still encoded
  a Unix-only build-tag invariant. Corrected invariant: every non-test smoke
  runtime file must be explicitly manifest-listed with its exact platform build
  tag (`tui_smoke`, `tui_smoke && !windows`, or `tui_smoke && windows`), and
  unmanifested non-test Go files must not contain Unix or Windows smoke tokens.

## Locked Scope Manifest

PR count: 1

Tasks:

1. Record this design and lock scope.
2. Add `skip_upload: true` to the Homebrew cask config and update releaseguard
   config validation.
3. Add a tap publish script/guard that copies the generated GoReleaser cask,
   commits, pushes, and records the exact path-changing commit.
4. Restructure `.github/workflows/release.yml` so draft asset checks happen
   before tap mutation and public undraft happens after tap postcheck.
5. Update release workflow tests to enforce split-publish ordering and forbid
   tap mutation inside the first GoReleaser publish step.
6. Add Windows `tui_smoke` daemon/client support over loopback only.
7. Add Windows ConPTY smoke test using `github.com/ActiveState/termtest`.
8. Add a `windows-2025` CI job for the Windows ConPTY smoke test.
9. Update README, RATCHET, harness docs, and policy matrix claim boundaries.
10. Run local verification, open PR, monitor checks, admin merge when green.
