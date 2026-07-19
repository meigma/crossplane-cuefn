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

## 2026-07-19 11:09 — Implementation plan drafted
Created `IMPLEMENTATION_PLAN.md` as the standalone review plan for this session. It is organized as short proof-driven slices: isolate the work, prove canonical annotated-module preparation and promotion with public APIs, reproduce `source.kind: "git"` packaging fidelity, add immutable publication, wire the Configuration label and two-artifact transaction, close the acceptance matrix, update only affected docs, and open an unmerged PR after local and hosted verification.

Current-source inspection confirmed the manifest interception seam is viable through `ociregistry.Funcs`, but also exposed a required boundary not explicit in the original prompt: Catalyst's real modules use Git source mode, while CUE's tracked-file/VCS helper is internal. The plan therefore makes pure-Go linked-worktree/tracked-file/clean-state parity a hard prototype gate and refuses a `CreateFromDir`-only shortcut. It also records that public CUE APIs cannot perform the CLI's internal tidy check, so `cue mod tidy --check` remains an explicit author preflight rather than a false cuefn parity claim.

The CLI shape remains a working hypothesis until the first prototype. The leading option is additive `publish` flags (`--publish-module` plus explicit `--source`), because it preserves the old path and lets one transaction build both artifacts around the same precomputed digest without making `--dir` silently mutate a registry.

## 2026-07-19 11:17 — Metadata interface generalized
Correction to the earlier source-specific CLI hypothesis: after review, the public metadata interface is now an agreed repeatable `--metadata <key=value>` array rather than a one-purpose `--source` flag. `org.opencontainers.image.source=<URL>` remains the primary documented example, but combined publication applies the same deterministic metadata map to the CUE module's manifest annotations and the Configuration's image-config labels.

The revised plan specifies first-`=` parsing, `StringArray` behavior, exact value preservation, order-independent digests, duplicate/collision rejection, known-key validation for the standard source URL, and no `--source` alias. `--publish-module` remains the explicit registry-mutation switch and requires `--dir`; metadata is optional. Without `--publish-module`, metadata labels only the Configuration because cuefn does not own the existing module artifact. The plan explicitly avoids separate low-level annotation/label flags and artifact-specific overrides in this slice.
