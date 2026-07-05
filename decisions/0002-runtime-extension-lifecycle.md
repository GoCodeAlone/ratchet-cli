# 0002 Runtime Extension Lifecycle Boundaries

**Date:** 2026-07-05
**Status:** Accepted

## Context

ratchet-cli already loads plugins and declares hooks, but it lacks a full extension lifecycle: marketplaces, update policy, runtime reload, plugin skill prompt integration, broad hook callsites, routines, and dynamic workflows.

Claude Code's current public model includes plugin marketplaces, plugin reload, skill/agent/hook bundles, broad lifecycle hooks, scheduled routines, and dynamic workflows. ratchet should adopt the useful primitives without losing its existing local trust model or duplicating Workflow messaging plugins.

## Decision

- Marketplaces are catalog/update metadata. They do not imply execution trust.
- Project/plugin hooks remain hash-trusted independently of marketplace/plugin install.
- Plugin skills are namespaced and selectively injected; full plugin skill content is not injected by default.
- Runtime reload is daemon-owned and must stop old plugin daemon processes before replacing capabilities.
- Dynamic workflows and routines are visible, bounded ratchet runtime objects layered over existing sessions/fleet/team/cron primitives.
- Slack/Discord/Teams delivery remains in Workflow messaging plugins; ratchet exports or bridges notification events only.

## Consequences

- Autodev-like plugins can become functional in ratchet without forcing every installed skill into every prompt.
- Marketplace autoupdate can be added without bypassing hook review.
- Workflow/routine work can proceed incrementally without hidden background autonomy.
- Direct notification delivery may take an extra Workflow bridge step, but avoids duplicated credentials and channel code in ratchet-cli.

