# 0001. Use GoFiles Count For Smoke List Boundary

**Status:** Accepted
**Date:** 2026-07-03
**Decision-makers:** Codex
**Related:** `docs/plans/2026-07-03-ratchet-cli-tui-binary-verification-design.md`, `docs/plans/2026-07-03-ratchet-cli-tui-binary-verification.md`

## Context

Task 1 originally required untagged `go list ./cmd/ratchet-tui-smoke` to fail.
The same task also requires an untagged package test in that directory. Go lists
a package that contains only untagged `_test.go` files, so raw `go list` exits
0 even when the package has zero non-test buildable files.

## Decision

Use `go list -f '{{len .GoFiles}}'` to prove the default package has zero
non-test buildable files, and keep `go build ./cmd/ratchet-tui-smoke` as the
failing default-build proof. Reject tagging the package test because that would
remove the untagged verification entrypoint required by the plan.

## Consequences

The smoke package can keep untagged tests while remaining absent from release
builds. Reviewers must treat zero `.GoFiles` plus default `go build` failure as
the package-boundary proof; raw `go list` exit status alone is not sufficient.
