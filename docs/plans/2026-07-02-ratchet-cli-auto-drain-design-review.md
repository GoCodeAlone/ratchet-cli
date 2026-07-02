### Adversarial Review Report

**Phase:** design
**Artifact:** `docs/plans/2026-07-02-ratchet-cli-auto-drain-design.md`
**Status:** PASS

**Findings (Critical):**
- None.

**Findings (Important):**
- None.

**Findings (Minor):**
- `D1` [User-intent drift] [Goal/User Intent]: The user asked for background/auto-drain, while the design intentionally ships a foreground `watch` worker. Recommendation: keep daemon-owned scheduled drain explicitly out of scope and document it as the next phase after this policy-bound worker. _Resolution: accepted; design names daemon scheduler as deferred and policy matrix still blocks hidden background execution._
- `D2` [Security/privacy] [Runtime Behavior/Policy Boundary]: Watch output could accidentally become a prompt transcript if implementation prints queue item detail while reporting cycles. Recommendation: implementation tests must assert cycle output is aggregate-only and does not contain prompt bodies. _Resolution: accepted; plan must include docs/CLI tests for prompt-free watch output._
- `D3` [Platform assumptions] [Infrastructure Impact]: SIGINT/SIGTERM behavior differs on Windows, so runtime correctness should not depend on POSIX signal delivery tests. Recommendation: unit tests should drive cancellation through context and CI/local gates should include Windows builds. _Resolution: accepted; design already requires Windows builds, and the plan should make cancellation context-driven._

**Bug-class scan transcript:**

| Class | Result | Note |
|---|---|---|
| Project-guidance conflicts | Clean | No repo-local `docs/design-guidance.md` or `AGENTS.md` exists; design follows workspace guidance for Windows support, minimal duplication, and existing repo patterns. |
| Assumptions under attack | Finding | A1 is load-bearing: foreground `watch` may not satisfy future daemon-style background drain; accepted as this slice's policy-safe prerequisite. |
| Repo-precedent conflicts | Clean | Existing ACP client commands live in `cmd/ratchet/cmd_acp_client.go` and local queue execution lives in `internal/acpclient`; the design follows that split. |
| Artifact-class precedent | Clean | Similar harness docs/tests live in `docs/harness-emulation.md`, `docs/policy-matrix.md`, and `cmd/ratchet/harness_docs_test.go`; design places updates in the same artifact class. |
| YAGNI violations | Clean | Scheduling knobs are limited to interval, max-per-cycle, max-cycles, stop-when-empty, and JSON output. |
| Missing failure modes | Clean | Drain errors, cancel requests, empty queues, owner lock contention, and no argv reconstruction are named. |
| Security / privacy at architecture level | Finding | Watch output must not expand local prompt text; accepted as a test requirement. |
| Infrastructure impact | Clean | No cloud resources, migrations, network endpoints, or secrets are added. |
| Multi-component validation | Clean | Design requires binary smoke through the built CLI and fixture ACP agent, not mocks only. |
| Declared integration proof | Clean | The only runtime integration is the existing ACP child-process client path; design proves it through the real fixture agent. |
| Contributed UI rendering proof | Clean | No UI contribution. |
| Rollback story | Clean | Revert-only rollback is sufficient because no persistent schema changes are introduced. |
| Simpler alternative not considered | Clean | Shell loop / scheduler recipe is considered and rejected as insufficient. |
| User-intent drift | Finding | Foreground watch is narrower than daemon background drain, but intentionally matches the policy matrix prerequisite. |
| Existence / runtime-validity | Clean | Design mutates existing docs/tests and existing CLI surfaces; implementation plan must still verify command help and representative invocation. |

**Options the author may not have considered:**

1. Approved launch profiles: a later design could persist a reviewed launch profile and allow `watch` to reference that profile instead of repeating `--command`/`--arg`; that should not be bundled here because it would create durable execution authority.
2. Daemon scheduler: the daemon could own restartable queue workers with richer lifecycle status; that requires a larger policy design for ownership, audit, cancellation, and config.
3. File-notification wakeups: a platform-specific watcher could reduce polling latency; current polling is simpler and portable enough for this slice.

**Verdict reasoning:** PASS. The design is intentionally conservative and keeps the newly documented policy boundary intact. Minor risks are implementation constraints rather than design blockers: keep watch explicit, keep output prompt-free, and validate Windows by build plus context-driven cancellation tests.
