### Adversarial Review Report

**Phase:** design
**Artifact:** `docs/plans/2026-07-10-ratchet-cli-provider-drain-managed-hooks-design.md`
**Status:** PASS

Required framing: Find at least three things wrong with this design, even if
they seem minor. Bias toward misconceptions, unstated assumptions, and missed
alternatives; reflexive approval is forbidden.

User intent checked: unify CLI/TUI provider support without future drift, then
implement the next two approved ratchet-cli improvements; preserve Go-native,
Windows, secret, plugin, release, and autonomous-delivery constraints.

**Findings (Critical):**

- None.

**Findings (Important):**

- `D1` [Existence/runtime-validity] [Shared Provider Setup Catalog]: The design
  required a catalog test against the runtime provider registry, but the
  orchestrator registry's factory map is private and exposes no type query.
  Recommendation: add a defensive, sorted upstream `ProviderTypes()` API,
  release it, and test ratchet against a real registry. _Resolution: design and
  ADR 0003 now make this an explicit upstream prerequisite and fourth scoped
  PR._
- `D2` [Repo precedent / naming] [Daemon-Supervised Background ACP Drain]: The
  proposed `ratchet acp queue background <queue>` path does not exist in the
  current command hierarchy; queues belong to ACP client session records under
  `ratchet acp client`, and the durable identifier is a session ID.
  Recommendation: use `ratchet acp client background ... <session-id>` and
  persist the session ID. _Resolution: design corrected._
- `D3` [Security / assumptions] [Daemon-Supervised Background ACP Drain]: A
  stored profile's `Trusted` boolean is currently accepted without proving its
  stored trust hash still equals `DescriptorHash()`. Background resume would
  convert that existing gap into unattended execution after command/env/cwd
  edits. Recommendation: require `Trusted && Hash == DescriptorHash()` at start
  and resume, pin that hash in policy, and harden the shared registry path.
  _Resolution: design and ADR 0004 corrected with negative integration proof._
- `D4` [Security / missing failure modes] [Managed Hook Policy and Audit]: A
  fixed path does not by itself prove administrator ownership, and an audit
  append performed only after execution cannot prevent unaudited launch.
  Recommendation: reject symlinks/non-regular/insecure ownership or DACLs, and
  durably append `started` before managed execution. _Resolution: design and ADR
  0005 corrected; terminal audit degradation is explicit._
- `D5` [Artifact-class precedent / failure mode] [Daemon manager lifecycle]:
  existing TUI smoke construction deliberately disables cron/background work.
  A manager created implicitly by `NewService` could execute persisted host
  state in tests. Recommendation: constructor-inject daemon ownership and use a
  disabled manager in test/smoke constructors. _Resolution: design and ADR 0004
  corrected._

**Findings (Minor):**

- None after revision.

**Bug-class scan transcript:**

| Class | Result | Note |
|---|---|---|
| Project-guidance conflicts | Clean | Revised design reuses plugin SDKs, existing secrets/Redactor, Go, JSONL audit, Windows builds, and per-PR release gates from workspace guidance. |
| Assumptions under attack | Finding | Profile trust validity and implicit test-manager disablement were load-bearing; D3/D5 resolved them. |
| Repo-precedent conflicts | Finding | Existing command hierarchy is `acp client` and queue state is session-owned; D2 corrected both. |
| Artifact-class precedent | Finding | Daemon smoke constructors explicitly omit background schedulers; D5 requires the same injection pattern. |
| YAGNI violations | Clean | No remote policy service, TypeScript SDK, arbitrary scheduler, provider SDK, or self-mutation loop is included. |
| Missing failure modes | Finding | Insecure managed files, pre-launch audit failure, profile drift, test state leakage, and retry amplification are now specified. |
| Security / privacy at architecture level | Finding | D3/D4 closed unattended trust and administrator-boundary gaps; secrets/content remain excluded from policy and audit. |
| Infrastructure impact | Clean | Local files and daemon workers only; no production resources, IAM, migrations, or deployment approval required. |
| Multi-component validation | Clean | Revised matrix requires real catalog/registry, TUI/daemon, daemon/ACP process, and hook loader/plugin/runner proofs. |
| Declared integration proof | Clean | Explicit integration matrix marks every named upstream/runtime/deferred boundary and its proof. |
| Contributed UI rendering proof | Clean | No plugin-contributed UI exists; ratchet's own Bubble Tea wizard has PTY content/navigation proof. |
| Rollback story | Clean | Per-PR rollback preserves stored provider data, stops workers, and requires managed-policy coordination before enforcement removal. |
| Simpler alternative not considered | Clean | Separate tables, daemon-rendered UI, detached shell workers, trust-store seeding, and remote policy were considered and rejected. |
| User-intent drift | Clean | The three ratchet features directly match the approved request; the upstream API exists only to make no-drift enforcement real. |
| Existence / runtime-validity | Finding | D1 found the missing upstream registry query; actual proto, ACP store/profile, hook loader, plugin merge, and Windows release surfaces exist. |

**Options the author may not have considered:**

1. Generate the catalog from a plugin-owned descriptor schema. This would
   remove more metadata duplication but requires moving product-facing labels,
   auth steps, and TUI field policy into the runtime plugin. The read-only type
   query is the smaller contract and keeps UI policy in ratchet.
2. Run background drains as daemon routines. Existing routines provide visible
   lifecycle, but they do not own ACP session claim/cancel/profile semantics.
   A dedicated manager that delegates to `DrainQueue` avoids a second queue
   abstraction while preserving daemon supervision.
3. Treat managed hooks as one more trusted source. This is simpler but cannot
   enforce managed-only after plugin reload or protect policy from local
   disable state, so it does not meet administrator-policy intent.

**Verdict reasoning:** Five Important issues were found and resolved in the
design/ADRs before planning. The revised artifact now names its upstream
contract, established command/state identifiers, trust validity, secure policy
file boundary, audit ordering, daemon injection pattern, integration matrix,
and four-PR release shape. No Critical or unresolved Important finding remains.
