# 0010. Pin Hook Audit Anchor

**Status:** Accepted
**Date:** 2026-07-14
**Decision-makers:** project maintainers
**Related:** `docs/plans/2026-07-10-ratchet-cli-provider-drain-managed-hooks-design.md`

## Context

Owner-private audit files and leaf directories do not preserve canonical audit
history when another principal can rename an ancestor. Limiting recursive
creation depth bounds persistence work but does not establish that trust.

## Decision

Managed audit paths have exactly two private namespace levels beneath an
existing trusted anchor. Every read or append pins that anchor for the complete
transaction, validates the anchor and its ancestry against untrusted
rename/delete rights, and revalidates identity before release. Unix accepts
only current-user/root-owned ancestry without untrusted mode or supported
native ACL mutation rights, with trusted-owner sticky directories supported.
Darwin rejects mutation-capable allow ACEs, including inherit-only entries,
before namespace creation and revalidates ACLs on private directories/files;
deny-only ACLs remain valid. Linux accepts POSIX access/default ACLs because their effective access
mask is reflected in `st_mode`, and rejects recognized non-POSIX ACL xattrs.
Other Unix targets support the portable POSIX mode/ACL contract only.
Windows evaluates owner and DACL mutation rights and holds directory handles
without delete sharing.

Rejected: leaf-only validation because it cannot protect the canonical path;
arbitrary-depth traversal because it expands race and durability surfaces.

## Consequences

- Custom paths must retain the fixed `<anchor>/<private>/<private>/<file>` shape.
- Existing namespace directories remain owner-private; read does not create them.
- Audit reads require each of the six schema keys exactly once.
- Privileged owners remain trusted and can administratively alter the tree.
- Relaxing the layout later requires a new ancestry and persistence proof.

## Proof

`TestManagedHookAuditRejectsMutationACLOnTrustedAnchor` covers Darwin anchor
and ancestor write/delete allow ACLs; inheritance and existing-object siblings
prove private namespace/file ACL validation. Its deny-only sibling protects
normal macOS homes.
`TestManagedHookAuditRejectsAnchorReplacementDuringAppend` and
`TestManagedHookAuditRejectsAnchorReplacementBeforeReadUnlock` exercise full
transactions. Windows additionally runs
`TestManagedHookAuditWindowsReadPinsTrustedAnchor`. Process-lock scope,
relative paths, empty reads, exact JSON keys, and Linux ACL model detection have
focused regressions. Each security fix has a recorded revert-and-restore proof.
