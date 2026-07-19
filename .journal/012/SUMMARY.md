---
id: 012
title: Publish CUE modules with generic metadata and release v0.1.8
date: 2026-07-19
status: complete
repos_touched: [crossplane-cuefn]
related_sessions: [011, 007, 004]
---

## Goal

Review the v0.1.7-grounded publishing prompt, reduce it to a proof-driven
implementation plan, and then ship a backward-compatible workflow that can
publish a CUE module and Configuration together with shared generic metadata
and an exact module-digest binding.

## Outcome

**Fully met, through a verified public release.** The prompt was reviewed and
reframed around a repeatable generic `--metadata key=value` interface rather
than a source-specific flag. The implementation shipped in PR #79, release PR
#80 published product **v0.1.8**, and the complete binary, runtime image,
Function package, provenance, signature, Homebrew, and Scoop paths were
verified. Local `master`, `origin/master`, and tag `v0.1.8` all resolve to
`ae226d05b6dc38a6652c117c1713f5c63df75d87`.

## Key Decisions

- **Use repeatable `--metadata key=value`, not a single source flag** — this
  exposes one generic, deterministic metadata map while keeping
  `org.opencontainers.image.source` as the primary documented example.
- **Keep registry mutation explicit with `--publish-module`** — combined
  publication requires `--dir`; without it, metadata labels only the
  Configuration because cuefn does not own the pre-existing module artifact.
- **Prepare both artifacts before mutating the registry** — the transaction
  binds the Composition to the exact prepared module digest and avoids
  publishing a module before discovering a local Configuration-build failure.
- **Make module publication immutable and retry-safe** — an absent version is
  pushed, an identical digest is reused, and a conflicting digest is rejected
  without moving the version.
- **Use public CUE and OCI APIs end-to-end** — canonical CUE module assembly is
  intercepted only at the public OCI manifest boundary to add annotations;
  no product subprocess or registry-specific API was added.
- **Package Git source mode from a clean tracked-file view** — whole-worktree
  cleanliness, linked worktrees, repository-root license inheritance, and VCS
  annotations are preserved; go-git v6 alpha was required for repositories
  using `extensions.worktreeConfig`.
- **Verify the draft before publication** — checksums, SLSA provenance,
  Cosign signatures, multi-architecture indexes, runtime execution, and
  Crossplane-native Function extraction all passed before making v0.1.8 public.

## Changes

- `internal/modulepublish/` — canonical module preparation, tracked-file Git
  archive construction, metadata annotation, immutable promotion, retry, and
  digest verification.
- `internal/cli/publish.go` — repeatable metadata parsing plus the explicit
  combined `--publish-module` transaction and safe partial-failure diagnostics.
- `internal/pkg/` — deterministic OCI image-config labels on generated
  Configuration packages.
- `internal/test/integration/publish_test.go` — disposable-registry acceptance
  coverage for media types, annotations/labels, digest lockstep, immutable
  retry/conflict behavior, loader compatibility, and partial failure.
- `README.md` and `docs/docs/` — combined and legacy publication workflows,
  metadata semantics, and the external `cue mod tidy --check` preflight.
- `go.mod`, `go.sum`, and `flake.nix` — go-git v6 alpha dependency graph and
  the corresponding Nix vendor hash.
- Release v0.1.8 — five platform archives, five SBOMs, checksums, signed and
  attested runtime/Function OCI indexes, and updated Homebrew/Scoop manifests.

## Open Threads

- go-git v6 is still an alpha dependency; reassess the pinned version when a
  stable v6 release supports `extensions.worktreeConfig`.
- Release jobs still emit non-fatal `artifact-metadata:write` storage warnings
  and use deprecated `actions/attest-sbom`; direct signature, provenance, and
  SBOM verification passed, so modernization remains separate technical debt.
- Credential flags, registry-specific APIs, low-level annotation/label
  overrides, and artifact-specific metadata maps remain intentionally out of
  scope.
- Pre-existing Dependabot PRs remain untouched. Developer-owned untracked
  `.claude/` and `xr.yaml` were preserved.

## References

- Plan: `.journal/012/IMPLEMENTATION_PLAN.md`
- [PR #79 — generic metadata and combined module publication](https://github.com/meigma/crossplane-cuefn/pull/79)
- [PR #80 — product v0.1.8 release](https://github.com/meigma/crossplane-cuefn/pull/80)
- [Product v0.1.8](https://github.com/meigma/crossplane-cuefn/releases/tag/v0.1.8)
- [Release build run 29702515936](https://github.com/meigma/crossplane-cuefn/actions/runs/29702515936)
- [Distribution run 29703014106](https://github.com/meigma/crossplane-cuefn/actions/runs/29703014106)

## Lessons

- CUE's public module APIs can preserve canonical media types and layer layout
  while a narrow public OCI manifest interception adds deterministic metadata;
  rebuilding the module format by hand is unnecessary.
- Git source fidelity is a functional requirement, not incidental packaging:
  directory walking would publish ignored/untracked files and miss linked
  worktree behavior.
- A real repository using `extensions.worktreeConfig` was the decisive gate:
  the initial go-git v5 prototype passed synthetic tests but failed the real
  source layout, while v6 alpha plus a hermetic regression proved the boundary.
