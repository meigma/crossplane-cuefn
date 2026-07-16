---
id: 008
title: Native observed-resource readiness
started: 2026-07-16
---

## 2026-07-16 13:19 — Kickoff
Goal for the session: Begin the reviewed Catalyst proposal to add a backward-compatible, explicitly opted-in `out.input.observedResources` path, carrying full observed composed objects through the contract, function adapter, render engine, standalone and Crossplane render flows, compatibility coverage, live readiness proof, documentation, and contract/product releases.
Current state of the world: The proposal at `/Users/josh/work/catalyst-infra/.wt/journal-jmgilman/.journal/044/cuefn-observed-resources-implementation-proposal.md` has been reviewed. Local `master` and `origin/master` both point to `9a4f3ca` (product `v0.1.3`), contract `v0.2.0` remains the latest contract release, and release PR #55 for product `v0.1.4` is open and unmerged. The proposal's source- and SDK-level assumptions still need code-level revalidation before edits. Sessions 001–007 are closed and the personal journal worktree is synced and clean; the main checkout's pre-existing untracked `.claude/` and `xr.yaml` are out of scope and must be preserved.
Plan: Start with a focused prototype proving explicit CUE field opt-in plus lossless observed-object round-trip, then build the smallest vertical contract/engine/adapter slice and extend it through CLI and render integration. Once behavior is stable, add the dedicated readiness fixture and live two-XR proof, tighten compatibility and failure-path coverage, update compact docs, and carry the implementation through exact-head CI and published-artifact verification.

## 2026-07-16 13:47 — Observed-resource vertical slice proven
Created Worktrunk branch `feat/observed-resource-readiness` from `origin/master` at `9a4f3ca`. The contract now exposes an open optional observed-resource map; the renderer fills it only when the module materializes a regular field, sends a concrete `{}` on the first pass, and leaves optional-only and legacy closed modules byte-for-byte unchanged. The function adapter carries full observed composed object bodies under stable resource keys while excluding connection details. Standalone `cuefn render` now accepts raw file/directory snapshots via `--observed-resources`, validates the standard composition-resource-name annotation, and rejects missing, empty, or duplicate stable keys.

Focused contract/render/function/CLI/integration tests and the full Go lint configuration pass. `moon run root:render-test --summary minimal` also passed and visibly executed all three real Crossplane render cases: normal render, required-resource fixpoint, and observed-resource delivery. The observed case used a physical Deployment name distinct from the `workload` annotation and reflected an open vendor status field, proving stable-key and full-object fidelity across the real protocol. Next: build the dedicated Job/Deployment/ConfigMap readiness module, offline transition matrix, and two-XR live Chainsaw proof.
