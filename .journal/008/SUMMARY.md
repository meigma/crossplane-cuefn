---
id: 008
title: Native observed-resource readiness
date: 2026-07-16
status: complete
repos_touched: [crossplane-cuefn]
related_sessions: [005, 007]
---

## Goal

Implement the reviewed Catalyst proposal for a backward-compatible, explicitly
opted-in `out.input.observedResources` path. Carry complete observed composed
objects through the contract, engine, function, standalone and Crossplane render
flows; derive conservative readiness; preserve legacy behavior; document it; and
prove the contract and product from published artifacts.

## Outcome

**Fully met.** The implementation, contract release, product release, and
published-artifact proof are complete. `master` is synchronized at
`8eebd929b75346d5a5672a5c858546c562b5b560`.

- PR #64 shipped the additive observed-resource contract and full runtime/CLI
  path, including the Crossplane v2.3.3 directory-loader correction requested in
  review. Exact-head CI, Integration, E2E, render-loop, local image, CUE,
  Chainsaw, lint, and documentation gates passed.
- PR #65 released contract v0.3.0. CUE Central serves it at
  `sha256:a84e8381392580bdb30132860550d7e38ee491d52215cb1f4822e3c4f22d030e`.
- PR #55 released product v0.1.4. The public release has five archives, five SPDX
  SBOMs, and `checksums.txt`; checksums, tag/SHA attestations, cosign identities,
  the runtime image, and the Function xpkg were independently verified.
- PR #66 added bounded retries for transient GitHub release uploads after an API
  incident. The corrected original tag-push run passed all ten release jobs, and
  the release-distribution workflow populated verified Homebrew and Scoop taps.
- A disposable fresh-cache proof resolved the published contract and immutable
  runtime/Function digests, then passed both the focused observed-readiness
  Chainsaw scenario and the complete Kind E2E suite.

## Key Decisions

- **Observed resources remain explicitly opt-in.** The engine fills
  `out.input.observedResources` only when a module materializes that regular
  field, supplies `{}` on the first pass, and leaves legacy or optional-only
  modules byte-for-byte unchanged.
- **Stable composition keys are the API.** Runtime objects are addressed by
  Crossplane's composed-resource key, not their physical Kubernetes name.
  Standalone snapshots derive the same key from the standard composition-resource
  annotation.
- **Pass complete objects, not a readiness projection.** Modules receive the full
  unstructured observed body so vendor status fields remain usable; connection
  details stay excluded because they are a separate sensitive channel.
- **Readiness predicates are conservative and identity-aware.** Job, Deployment,
  and conditionless ConfigMap examples reject stale or wrong objects before
  accepting readiness, and live tests prove independent migration/workload gates.
- **Mirror Crossplane's snapshot loader exactly.** Directory inputs use only
  sorted immediate lowercase `.yaml`/`.yml` entries, ignore nested fixtures, and
  error when no immediate YAML exists. Observed and required resource flags share
  this behavior.
- **Release provenance comes from the tag-push run.** A workflow-dispatch rerun
  produced branch-context attestations and was canceled before publication; the
  original `v0.1.4` tag-push run was rerun so every subject binds to the tag and
  release commit.
- **Contract and product releases have different publication surfaces.** Contract
  v0.3.0 is public through CUE Central; its GitHub draft remains intentionally
  unpublished because the release distributor expects product assets.

## Changes

- `contract/contract.cue` and `internal/contract/contract_test.go` — additive open
  observed-resource map with closed-contract compatibility coverage.
- `internal/render/engine.go` and render fixtures/tests — explicit opt-in,
  first-pass normalization, full-object fidelity, and legacy compatibility.
- `internal/function/function.go` and tests — observed composed objects mapped by
  stable key without connection details.
- `internal/cli/observed_resources.go`, `internal/cli/required_resources.go`, and
  tests — snapshot flag, annotation validation, duplicate detection, multi-document
  support, and Crossplane-aligned file/directory loading.
- `internal/test/common/testdata/readiness/`, render-loop fixtures, and
  `internal/test/integration/` — offline transition matrices and real Crossplane
  render coverage.
- `test/chainsaw/e2e/observed-readiness.yaml` and `internal/test/e2e/` — live
  two-XR pending-to-ready proof through normal reconciliation.
- `docs/docs/how-to/derive-readiness-from-observed-resources.md` plus CLI, input,
  contract, quickstart, and required-resource documentation — author and operator
  guidance for the new boundary.
- `.github/workflows/release.yml` — bounded retry around transient draft-asset
  uploads.
- Releases — contract v0.3.0 in CUE Central; public product v0.1.4; verified
  Homebrew and Scoop distribution.

## Open Threads

- PR #67 is the routine Release Please v0.1.5 proposal containing PR #66's upload
  retry and mechanical version bumps. It is not unfinished observed-resource work
  and remains open for a future release decision.
- The `contract/v0.3.0` GitHub release remains a draft intentionally; CUE Central
  is the published contract surface.
- Pre-existing Dependabot PRs and the developer-owned untracked `.claude/` and
  `xr.yaml` remain untouched.

## References

- Proposal: `/Users/josh/work/catalyst-infra/.wt/journal-jmgilman/.journal/044/cuefn-observed-resources-implementation-proposal.md`
- Implementation: https://github.com/meigma/crossplane-cuefn/pull/64
- Contract release: https://github.com/meigma/crossplane-cuefn/pull/65
- Product release: https://github.com/meigma/crossplane-cuefn/pull/55
- Upload retry: https://github.com/meigma/crossplane-cuefn/pull/66
- Product v0.1.4: https://github.com/meigma/crossplane-cuefn/releases/tag/v0.1.4
- Tag-push release run: https://github.com/meigma/crossplane-cuefn/actions/runs/29540656555/attempts/3
- Distribution run: https://github.com/meigma/crossplane-cuefn/actions/runs/29544904576
- `.journal/008/NOTES.md` and `.journal/TECH_NOTES.md`

## Lessons

- Matching an upstream CLI requires copying its edge semantics, not just its happy
  path: recursive loading and accepting an empty directory both created dangerous
  standalone-versus-Crossplane drift.
- Rerunning a release via `workflow_dispatch` can silently change provenance from
  a tag to the default branch. Verify event, ref, source SHA, and attestations
  before publishing; rerun the original tag-triggered run when provenance matters.
- Published-pair testing catches integration gaps that source-checkout tests
  cannot: resolve the released contract with fresh caches, pull OCI artifacts by
  digest, verify the in-cluster resolved Function image, and exercise the actual
  readiness transition.
