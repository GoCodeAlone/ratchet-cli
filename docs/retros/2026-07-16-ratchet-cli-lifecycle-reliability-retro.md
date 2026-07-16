# Retro: Ratchet CLI Lifecycle Reliability

**PR:** #142 - fix: harden lifecycle cleanup and process smoke
**Merged:** 2026-07-16
**Release:** `v0.30.36`
**Branch:** `fix/reliability-followups`
**Design:** `docs/plans/2026-07-16-ratchet-cli-lifecycle-reliability-design.md`
**Plan:** `docs/plans/2026-07-16-ratchet-cli-lifecycle-reliability.md`
**Related ADRs:** None

## Adversarial-review findings, scored

| Phase | Finding | Severity | Outcome |
|---|---|---|---|
| design | D1: persistent cleanup failures could flood logs | Important | Resolved upfront: equivalent errors are suppressed for one minute and reset after recovery. |
| design | D2: moving real-start smoke did not replace its five-second bound | Important | Resolved upfront: the three lifecycle smokes use a shared 30-second transition bound and a five-minute CI command bound. |
| design | D3: Linux-only process proof conflicted with native-platform guidance | Important | Resolved upfront: the existing `windows-2025` job ran the same selector and passed. |
| design | D4: notification write and peer handling were conflated | Minor | Prescient: stress exposed the SDK's asynchronous handler goroutine; the design was backported and tests gained an explicit peer barrier. |
| plan | P1: canceled manager context could mask the intended query failure | Important | Resolved upfront: the closed-database test uses an unstarted manager and live context. |
| plan | P2: reverting a signature would prove compilation, not behavior | Important | Resolved upfront: revert proof preserved signatures and disabled close, query, and suppression behavior independently. |
| plan | P3: timeout cleanup could leak or block on child reap | Important | Prescient: it identified the unreaped-child risk and required kill-and-drain cleanup, but its remediation missed that the drain itself needed an independent bound; Copilot found the gap and `eb03f01` added a second bounded window. |
| plan | P4: a docs closeout PR creates a second patch release | Minor | Inconclusive: the overhead is real, but it preserves truthful post-merge evidence and the explicit release-per-merge policy. |

## Gate misses

| Issue | Gate that missed | Why it slipped | Fix idea |
|---|---|---|---|
| Copilot found an unbounded `<-child.done` after the process timeout and kill. | `adversarial-design-review` (plan), then `requesting-code-review` | P3 introduced kill-and-drain cleanup without bounding the drain; the downstream pre-PR review also missed that independent wait. | Require an independent bound around every timeout-triggered cleanup join or receive. |

No PR or merge-commit CI failure occurred. The initial ACP exact-once stress
failure was caught locally before PR creation and corrected with a design
backport rather than a runtime sleep.

## Missed skill activations

The canonical `.claude/autodev-state/in-progress.jsonl` activation log is
unavailable. Committed artifacts and PR evidence prove outputs, not
hook-recorded activations.

| Gate | Fired? | Notes |
|---|---|---|
| brainstorming | activation log unavailable | Design artifact exists. |
| adversarial-design-review (design) | activation log unavailable | Committed report contains D1-D4. |
| writing-plans | activation log unavailable | Locked six-task, two-PR plan exists. |
| adversarial-design-review (plan) | activation log unavailable | Committed report contains P1-P4. |
| alignment-check / scope-lock | activation log unavailable | Alignment artifact and verified lock exist. |
| subagent-driven-development | activation log unavailable | Task commits and verification evidence exist. |
| finishing-a-development-branch | activation log unavailable | PR #142 contains the required verification and scope evidence. |
| finishing Step 1e (doc-reconciliation) | yes | PR #142 records one corrected ACP scheduling assumption. |
| pr-monitoring | activation log unavailable | All checks settled green; one Copilot thread was fixed, replied to, resolved, and re-reviewed with no new comments. |
| post-merge-retrospective | activation log unavailable | This retro supplies the required output artifact. |

## What worked

- Behavioral revert proofs showed the cleanup tests detect dropped close,
  query, and log-suppression behavior instead of only API drift.
- Repeated cancellation tests exposed the ACP SDK scheduling assumption before
  PR creation; the correction preserved immediate teardown and exact receiver
  proof without adding a sleep.
- Dedicated Linux and native Windows lifecycle selectors passed while
  in-process race coverage retained the concurrency boundary.
- PR #142 and merge-commit `f1ff79a` CI settled green; release run
  `29507752893` published `v0.30.36` with seven assets, six valid checksums,
  matching Formula/Cask hashes, and a Homebrew-installed 22-provider runtime
  proof.

## What didn't

- No retained independent reviewer output was available before PR creation;
  the downstream Copilot review caught the missing post-kill wait bound.
- The pre-PR review path failed to carry P3 through to an independently bounded
  drain even though it was the exact cleanup path under scrutiny.

## Plugin-level follow-ups

No plugin-level change is warranted from this single review miss. The post-kill
wait gap is captured as project guidance; another occurrence should promote it
into the shared code-review checklist.

## Project guidance updates

| Guidance file | Change | Reason |
|---|---|---|
| `docs/design-guidance.md` | updated | Future lifecycle designs must bound cleanup joins independently and distinguish notification send completion from receiver handling. |
