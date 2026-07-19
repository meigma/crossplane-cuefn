---
id: 012
title: Review the v0.1.7 CLI workflow prompt
started: 2026-07-19
---

## 2026-07-19 10:55 — Kickoff
Goal for the session: Review the attached prompt grounded in cuefn v0.1.7, establish the intended work and its boundaries, and use that context to guide today's session.
Current state of the world: Product v0.1.7 is released from master at 4577625; the CLI now includes the static `cuefn check` gate alongside `cuefn test` and `cuefn validate`, and session 011 is closed.
Plan: Read the attached prompt after session setup, compare its assumptions with the current CLI and repository context, then identify the smallest useful next slice before implementation.

## 2026-07-19 10:56 — Prompt reviewed
The attached prompt defines an additive publishing enhancement: use public CUE and go-containerregistry APIs to publish a normal two-layer CUE module with an explicit `org.opencontainers.image.source` manifest annotation, label the resulting Configuration xpkg with the same source URL, and bind the Composition to the exact digest produced in that transaction.

The first learning slice is deliberately a disposable-registry prototype, not a full CLI design. It must prove media-type and layer preservation, immutable-version retry behavior, registry digest agreement, and compatibility with `render.OCILoader`. Only after that proof should the session compare the three proposed CLI shapes and choose the least surprising backward-compatible surface.

Execution boundaries are explicit: keep the implementation behind `!noxpkg`; do not shell out or add credential flags; do not broaden into generic annotations, release automation, or registry-specific APIs; preserve developer-owned untracked files; use an isolated Worktrunk branch; open but do not merge a PR; and do not cut a release.
