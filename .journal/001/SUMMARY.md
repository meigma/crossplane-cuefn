---
id: 001
title: Ramp, rebrand, and design the CUE-over-OCI Crossplane function
date: 2026-06-28
status: complete
repos_touched: [crossplane-cuefn]
related_sessions: []
---

## Goal

Ramp on Crossplane v2 + CUE, define what we're building (a Crossplane v2
composition function that renders Kubernetes resources from CUE modules pulled
from an OCI registry, plus an operator CLI), turn the `template-go` scaffold into
a properly branded `crossplane-cuefn` repo, and produce a design + phased
implementation plan. **No product code this session** — by design, implementation
starts next session at PLAN Phase 1.

## Outcome

**Goal met.** Delivered, in order:
1. Grounded research on Crossplane v2 (Go composition functions, Compositions,
   Configurations) and CUE (types/constraints/modules, OCI publish/consume, the
   Go API). Captured in NOTES + TECH_NOTES.
2. Studied the developer's reference spike (`catalyst-infra/.../platform/mvp/cuefn`)
   — a working proof of the runtime half — and recorded the adoptable stack +
   contract.
3. Ran a **codegen de-risk spike** (scratch) proving CUE `#Spec` → OpenAPI →
   **structural** XRD works, validated by the API server's own
   `apiextensions-apiserver` structural validator. Retired the biggest unknown.
4. **Rebranded the repo** `template-go` → `crossplane-cuefn` — PR #3, **merged**.
5. Locked the design via 7 one-at-a-time decisions → `DESIGN.md`.
6. Authored a phased build plan → `PLAN.md`, then hardened it with a **5-lens
   adversarial review workflow** (2 blockers + ~40 findings, all triaged/integrated).

## Key Decisions

- **Product = one CUE module is the source of truth** for both the XRD (schema)
  and the transform (runtime). Distinguishes us from `function-cue` (inline-only);
  closest analog is `function-kcl`.
- **7 design decisions** (DESIGN §13): one `cuefn` binary (`cuefn function` = gRPC
  server + image entrypoint); full output contract (author-keyed `resources` map +
  readiness + `status`); single XRD version in v1; self-contained xpkg packaging
  via go-containerregistry; transitive CUE module deps supported; runtime engine is
  the first slice.
- **Binary `cuefn`, dual-license Apache-2.0 OR MIT, reset to 0.1.0** (rebrand).
- **Two blocker fixes from the plan review** (technical, not product changes):
  - `spec.crossplane` would conflict with a *closed* `#Spec` → the engine **strips
    Crossplane-reserved spec keys** before unifying (the spike's `input.spec` was
    open, hiding this).
  - CUE references modules by **semver, not OCI digest** → digest lock-step is done
    by recording semver ref + expected manifest digest and **verifying after
    fetch**, not by referencing-by-digest.
- **De-risk-by-spike** is the working method: PLAN Phases 2 and 5 each open with a
  short spike before committing to an approach.

## Changes

- PR #3 (merged): full rebrand — module path `github.com/meigma/crossplane-cuefn`;
  binary `cuefn` (env prefix `CUEFN`, `internal/templateinfo`→`internal/appinfo`);
  rebranded moon/GoReleaser/ghd/melange/apko/mise/release-please/workflows +
  `.github/scripts/*` + vendored apko/melange/mise skills; rewrote README/docs/
  SECURITY; reset to 0.1.0; `is_template=false`; dual `LICENSE-APACHE`/`LICENSE-MIT`;
  removed `DELETE_ME.md`. Verified: zero `template-go` residue, `moon run root:check`
  green, `goreleaser check` valid, `.github/scripts` py tests 11/11.
- Journal (this session, on `journal/jmgilman`): `DESIGN.md`, `PLAN.md`, plus
  TECH_NOTES sections (Project, Reference spike, Codegen de-risk spike).

## Open Threads

- **Implementation not started** — next session begins at **PLAN Phase 1** (render
  engine core + module contract, offline).
- The 8 phases are each a planned PR; runtime-first (P1 engine → P2 OCI → P3
  function → P4 schema CLI → P5 Configuration publish → P6 function xpkg → P7 docs
  → P8 kind e2e).
- Two embedded de-risk spikes still to run: P2 (transitive-dep reality + nonroot
  `CUE_CACHE_DIR` + digest verify-after-fetch) and P5 (confirm Crossplane's xpkg
  builder is non-importable `internal/`; prototype Configuration + Function xpkg
  with go-containerregistry).
- Two unrelated Dependabot PRs (#1 attest, #2 cache) are open and untouched.
- DESIGN.md/PLAN.md are marked temporary working docs — promote hardened bits into
  TECH_NOTES and delete once reflected in code.

## For the next agent (start here)

1. Read `.session.md`, then `.journal/SKILLS.md` (load `git`, `worktrunk`; load
   `cue` when touching CUE), then `.journal/TECH_NOTES.md`.
2. Read `.journal/001/DESIGN.md` (the spec, incl. the resolved decisions and the
   two blocker fixes) and `.journal/001/PLAN.md` (the phased plan + success
   criteria). These are authoritative.
3. Port from the proven reference spike at
   `/Users/josh/work/catalyst-infra/.wt/experiment-platform-mvp/platform/mvp/cuefn`
   (`internal/render`, `fn.go`, loaders) — adapt to the richer contract
   (keyed-map resources + readiness + status, reserved-key stripping).
4. Stack (proven to interoperate): `cuelang.org/go v0.16.1`,
   `function-sdk-go v0.7.1`, `crossplane-runtime/v2 v2.3.1`,
   `crossplane/apis/v2 v2.3.1`, `apimachinery v0.35.3`,
   `apiextensions-apiserver v0.35` (codegen), `controller-tools v0.20`,
   go-containerregistry (xpkg). CLI is cobra/viper (repo default).
5. Start a fresh implementation worktree off `origin/master` (per `.session.md`);
   open one PR per phase.

## References

- `.journal/001/DESIGN.md`, `.journal/001/PLAN.md`, `.journal/001/NOTES.md`
- `.journal/TECH_NOTES.md` — "Project", "Reference spike", "Codegen de-risk spike"
- PR #3 (merged): https://github.com/meigma/crossplane-cuefn/pull/3
- Reference spike: `/Users/josh/work/catalyst-infra/.wt/experiment-platform-mvp/platform/mvp/cuefn`
- Prior art: `crossplane-contrib/function-cue` (inline-only), `function-kcl` (OCI modules)
- Codegen scratch spike (throwaway): `…/scratchpad/codegen-spike` (proved CUE→structural XRD)

## Lessons

- The OpenAPI encoder's `ExpandReferences:true` is buggy with bounded numbers
  ("unsupported op for number &") and it rejects non-`#` top-level fields — codegen
  must use `ExpandReferences:false` + a manual `$ref` inliner and reduce to a
  definitions-only value first. The only author guardrail: no type-crossing
  disjunctions (`string|int`, struct unions) in schema defs.
- Validate adopted-spike assumptions against current library reality before
  planning around them — the plan review caught that CUE may already resolve
  transitive deps via `CUE_REGISTRY` (so the "wire the resolver" framing may be a
  non-problem) and that the real risks were elsewhere (cache writability, digest).
