# Ratchet CLI ConPTY and Split Publish Plan

Date: 2026-07-04
Design: `docs/plans/2026-07-04-ratchet-cli-conpty-split-publish-design.md`

## Scope Manifest

**PR Count:** 1
**Tasks:** 10
**Estimated Lines of Change:** ~900

**Out of scope:**
- Changing dogfooded compute runner policy or self-hosted runner setup.
- Driving the production `ratchet` release binary through Windows ConPTY.
- Replacing GoReleaser cask generation with hand-generated Homebrew Ruby.

**PR Grouping:**

| PR # | Title | Tasks | Branch |
|------|-------|-------|--------|
| 1 | feat: prove windows conpty and split publish | Task 1, Task 2, Task 3, Task 4, Task 5, Task 6, Task 7, Task 8, Task 9, Task 10 | feat/conpty-split-publish |

**Status:** Locked 2026-07-04T06:22:29Z

### Task 1: Record And Lock Plan

Create the design and plan artifacts, run adversarial review, alignment check,
and scope-lock.

Verification:

```sh
tests/plan-scope-check.sh --plan docs/plans/2026-07-04-ratchet-cli-conpty-split-publish.md
```

### Task 2: Configure GoReleaser Cask Generation Without Upload

Set `homebrew_casks[].skip_upload: true` in `.goreleaser.yaml`. Update
`internal/releaseguard` config parsing and validation so tests require that
GoReleaser generates cask material without mutating the tap during the first
release step.

Verification:

```sh
go test ./internal/releaseguard -run 'GoReleaser|Tap' -count=1
goreleaser check
```

### Task 3: Add Generated-Cask Tap Publish Guard

Add a script and releaseguard support that copy
`dist/homebrew/Casks/ratchet-cli.rb` into a clean tap checkout, reject missing
or forbidden generated content, commit and push only when the cask changed, and
write `RATCHET_RELEASE_GUARD_TAP_COMMITS`.

Verification:

```sh
go test ./internal/releaseguard -run 'Tap|Workflow' -count=1
```

### Task 4: Restructure Tag Release Workflow

Change `.github/workflows/release.yml` so the GitHub draft release is published
and draft assets are checked before the Homebrew tap is mutated. Keep public
undraft after tap postcheck.

Verification:

```sh
go test ./internal/releaseguard -run Workflow -count=1
```

### Task 5: Enforce Split-Publish Ordering

Add workflow/config tests that fail if the first GoReleaser release step can
mutate Homebrew, if tap publish occurs before draft asset checks, or if GitHub
undraft occurs before tap postcheck.

Verification:

```sh
go test ./internal/releaseguard -count=1
```

### Task 6: Add Windows Smoke Client And Daemon

Add `tui_smoke && windows` client/daemon helpers that use loopback-only gRPC
inside a test-owned temp root, while preserving the same disabled MCP/plugin/
autoresponder/cron constraints as the Unix smoke daemon.

Verification:

```sh
GOOS=windows GOARCH=amd64 go test -c -tags tui_smoke ./internal/client
GOOS=windows GOARCH=amd64 go test -c -tags tui_smoke ./internal/daemon
```

### Task 7: Add Windows ConPTY Smoke Test

Add a Windows-only `tui_smoke` TUI smoke test using
`github.com/ActiveState/termtest`, building `cmd/ratchet-tui-smoke` and driving
the visible TUI through a ConPTY-backed terminal.

Verification:

```sh
GOOS=windows GOARCH=amd64 go test -c -tags tui_smoke ./internal/tui
```

On Windows:

```powershell
go test -tags tui_smoke ./internal/tui -run WindowsConPTY -count=1
```

### Task 8: Add Windows CI Job

Add a `windows-2025` CI job that runs the Windows ConPTY smoke test with
`-tags tui_smoke`.

Verification:

```sh
go test ./internal/releaseguard -run Workflow -count=1
```

### Task 9: Update Public Evidence Docs

Update README, RATCHET, `docs/harness-emulation.md`, and
`docs/policy-matrix.md` so they describe split-publish, Windows ConPTY smoke
coverage, and remaining release-binary boundary honestly.

Verification:

```sh
go test ./cmd/ratchet -run TestHarnessDocs -count=1
```

### Task 10: Verify, PR, Monitor, Merge

Run focused local verification, create the PR, monitor checks/review, fix any
failures, and admin merge after checks are green or after local verification is
passing and checks are delayed.

Verification:

```sh
go test ./cmd/ratchet -run TestHarnessDocs -count=1
go test ./internal/releaseguard -count=1
goreleaser check
goreleaser release --snapshot --clean --skip=publish
scripts/check-release-artifacts.sh --manifest-only dist
```

