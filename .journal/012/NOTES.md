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

## 2026-07-19 11:28 — Execution branch established
Revalidated the live baseline before implementation: local `master` and `origin/master` both resolve to `4577625107450c6cbdc9fb350698fe0b95bea1bb`, GitHub release `v0.1.7` remains published, and the main checkout still has only the pre-existing untracked `.claude/` and `xr.yaml` paths.

Created the isolated Worktrunk branch `feat/publish-metadata` from that exact remote base at `.wt/feat-publish-metadata`. The feature worktree is clean, tracks no `.journal` files, and has the repository's pinned mise configuration trusted. Execution will stop after an unmerged PR and exact-head hosted-check verification; no release work is authorized.

## 2026-07-19 12:31 — Public-API prototype and transaction proven
Implemented the approved interface behind the existing `!noxpkg` publish path. The new `internal/modulepublish` adapter asks CUE's public `modregistry` API to assemble the canonical scratch-config/two-layer module in memory, intercepts only the public OCI manifest push to add deterministic annotations, captures the exact manifest/blobs before mutation, and promotes them through the standard CUE registry resolver. Publication is immutable: an absent version is pushed, an identical digest is reused, and a different digest is rejected before tag mutation, followed by an exact-digest re-resolution after push.

The hard Git gate passed without product subprocesses. An initial `go-git` v5.19.1 prototype handled a synthetic linked worktree but failed against Catalyst's real linked worktree because that repository enables Git's `extensions.worktreeConfig`. The narrow adapter now uses the official `go-git` v6 alpha API, which supports that extension. It requires whole-worktree cleanliness, packages only tracked module files, inherits a tracked repository-root `LICENSE` for nested modules, and records HEAD revision/committer time as CUE VCS annotations. Tests also prove ignored files stay out and generated-metadata collisions fail.

The CLI now accepts repeatable `--metadata key=value` and explicit `--publish-module` (which requires `--dir`). Parsing splits only the first `=`, preserves values, rejects empty/duplicate entries, and validates the standard source key as an absolute HTTP(S) URL. Combined mode prepares both artifacts before mutation, records the prepared module digest in the Composition, publishes/reuses the module, then pushes the identically labeled Configuration. Metadata without `--publish-module` labels only the Configuration. The old no-new-flags path remains unchanged.

Enabled throwaway-registry tests passed for canonical media types/layers, matching module annotations and Configuration labels, runtime loader digest acceptance, deterministic retry/reuse, conflicting-version refusal, metadata-only non-mutation, and module-success/Configuration-failure diagnostics with both refs plus safe-retry guidance. The no-xpkg binary build also passes, and the affected README, quickstart, how-to, CLI reference, and explanatory pages now document the combined and legacy workflows plus the external `cue mod tidy --check` preflight.

## 2026-07-19 12:46 — Real-module gate and local verification complete
Ran the prepared-artifact path against Catalyst Infra's clean Phoenix linked worktree at `crossplane/phoenix`, using module version `github.com/cardano-foundation/catalyst-infra/crossplane/phoenix@v0.1.1`. After moving the isolated Git adapter to `go-git` v6 alpha.4, preparation succeeded with the repository's real `worktreeConfig` extension. Replaced the environment-specific probe with a hermetic regression that enables the same extension before creating a linked worktree; no Catalyst paths or temporary fixtures remain in the implementation diff.

Final local evidence is green: focused package tests, `root:oci-test`, `root:publish-test`, `root:check`, the full 19-task `moon ci` graph including Kind/Chainsaw E2E, `git diff --check`, and the `noxpkg` build. Docker inspection found no remaining testcontainers. The implementation worktree tracks no `.journal` files, no release/version files changed, and the main checkout still contains only the pre-existing untracked `.claude/` and `xr.yaml` paths.

## 2026-07-19 13:01 — PR open and exact-head checks green
Opened PR #79, `feat(publish): publish modules with generic metadata`, from `feat/publish-metadata` into `master`. The first hosted Nix run exposed the expected fixed-output maintenance consequence of the new Go dependency graph: `flake.nix` still carried the old `vendorHash`. Updated only that hash to the exact value reported by Nix, reproduced the workflow command locally with a successful binary smoke test, and pushed the focused `fix(nix): refresh Go module vendor hash` follow-up.

The final PR head is `86a0fbda6d7deeb6f2637dcf3b3f20ca8ea72322`. Hosted checks are green on that exact SHA: CI, integration, E2E, Nix flake build, GitHub Pages, Kusari Inspector, amd64/arm64 Melange dry runs, container image dry run, and binary release dry run. PR: https://github.com/meigma/crossplane-cuefn/pull/79. It remains open and unmerged; no release was created.
