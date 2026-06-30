---
id: 005
title: Docs freshness audit + the required-resources feature (design → ship → release)
date: 2026-06-30
status: complete
repos_touched: [crossplane-cuefn]
related_sessions: [004]
---

## Goal

Two pieces of work, both developer-driven:

1. A **docs freshness examination** after session 004's changes (the developer
   suspected drift).
2. Investigate, design, and ship a feature: let a **CUE module request additional
   cluster data at render time** the way native Crossplane composition functions
   do (Crossplane's "extra resources" / v2 "required resources"). Ran under
   `ultracode` (workflow-per-task).

## Outcome

**Both goals fully met.** `master` at `13f8587` (+ tag `v0.1.1`).

1. **Docs audit (PR #30, merged):** a multi-agent audit examined 21 doc files,
   found 16 fresh + **10 confirmed stale spots across 5 files** (all from session
   004: the contract-v2 `out` root, the importable contract module,
   `--cache-dir`), and fixed them. The `out`-root conceptual error in
   `explanation/one-module-two-outputs.md` was the highest-impact.

2. **Required-resources feature — shipped end-to-end** across a verified design
   doc + 6 PRs + 2 releases. The mechanism is exactly the developer's intuition:
   a module **emits** `out.requirements` (selectors) when data is missing and
   **reads** `out.input.requiredResources` when Crossplane delivers it; Crossplane
   owns the fetch-and-re-invoke fixpoint; the function stays pure.
   - Contract `v0.2.0` published to the CUE Central Registry (#31 source, #33
     test, #32 release).
   - Product code: render (#34), function (#36), cli (#37).
   - **Proven on a real kind cluster** (#38, CI `e2e` green).
   - Docs (#39).
   - Product **`v0.1.1` cut** (#35): Release pipeline succeeded — binaries+SBOMs,
     signed multi-arch image + Function xpkg on ghcr, cosign + SLSA attestations.
   - **`v0.1.1` and `contract: v0.2.0` GitHub releases are DRAFTS** (repo
     `draft: true`), left for the maintainer to publish (outward-facing; not done
     without explicit instruction).

## Key Decisions

- **Engine SEED, not the contract field, guarantees first-pass concreteness.** The
  prototype proved an absent/empty-key `requiredResources` is a hard CUE error
  ("undefined field"), not "incomplete". So the engine seeds an empty `[]` per
  declared requirement (read `out.requirements` → seed → re-fill → read
  `out.resources`), and the optional contract field is `requiredResources?`.
- **"Exactly one of matchName/matchLabels" enforced at render time** (engine
  `readRequirements`), not by the closed contract — the disjunction-in-closed-struct
  form was unverified and deferred.
- **Current wire field, not deprecated:** the function uses
  `Requirements.Resources` / `request.GetRequiredResources` (v0.7.1 has no setter,
  so the proto selector is built by hand), never `extra_resources`.
- **CLI does a fixed two-pass + stabilization check**, not a re-implemented
  fixpoint loop (requirements are pure-of-stable-inputs → converge in two passes;
  the real loop is Crossplane's, covered by the integration test).
- **Cross-namespace reads are accepted as the supported upstream behavior** (per
  developer), not a cuefn security gap — RBAC-governed, documented.
- **Contract test split from contract source** (`test(contract)` #33 vs
  `feat(contract)` #31) so a test-only change doesn't cut a spurious product
  release (the root component includes `internal/contract/`). Mirrors session-004
  #20/#28.
- **Product release held until code-complete**, then cut after e2e + docs (the
  developer's choice); contract releases were published immediately (additive).
- **Merge autonomy:** mid-session the developer delegated the merge action — I
  squash-merged each approved PR + cleaned up, surfacing only the outward-facing
  release decision.

## Changes

- **Docs (#30):** `README.md`, `explanation/one-module-two-outputs.md`,
  `reference/{cli,module-contract}.md`, `how-to/local-toolchain.md` — refreshed
  for contract v2 + the contract module + CLI surface.
- **Contract (#31):** `contract/contract.cue` — `#Required`, `#Requirement`,
  optional `#Input.requiredResources?`, optional `#Transform.requirements?`. Test
  (#33): `internal/contract/contract_test.go`.
- **Render (#34):** `internal/render/engine.go` — `Inputs.RequiredResources`,
  `Requirement`, `Result.Requirements`, `readRequirements`, `seedRequiredResources`,
  the `Render` reorder; fixture `internal/test/common/testdata/required` + unit tests.
- **Function (#36):** `internal/function/function.go` — `requiredToInputs`,
  `setRequirements`, the capability Warning gate.
- **CLI (#37):** `internal/cli/render.go` + new `required_resources.go` —
  `--required-resources`, two-pass + stabilization, `loadRequiredObjects` (k8s
  multi-doc reader), `matchRequirements`; printed requirements; unit + gated
  integration tests.
- **E2E (#38):** `internal/test/e2e/` + `test/chainsaw/e2e/required-resources.yaml`
  — additive requirement in the e2e fixture + aggregate-to-crossplane ClusterRole.
- **Docs (#39):** `how-to/require-resources.md`, `explanation/required-resources-fixpoint.md`,
  reference updates, mkdocs nav.
- **Journal:** `NOTES.md`, this `SUMMARY.md`, `TECH_NOTES.md` (required-resources
  section); the temporary `DESIGN-required-resources.md` was promoted into
  TECH_NOTES and deleted at close.

## Open Threads

- **Drafts to publish (maintainer):** `v0.1.1` and `contract: v0.2.0` (and the
  older `v0.1.0` / `contract v0.1.0` from session 004) are unpublished GitHub
  release drafts.
- **Deferred follow-ups (non-blocking):** adopt a `cfg` requirement in
  `example/module` so the how-to's command runs against the shipped example
  (design deliberately kept `example/` out of the rollout); the function-side
  runtime **contract-major check** (carried from session 004 — only matters at a
  future `v1`).
- **Carried since session 002:** session-001 `.journal/001/DESIGN.md` + `PLAN.md`
  still flagged temporary — promote/delete.

## References

- Docs audit: PR #30. Required-resources: #31, #32, #33, #34, #35, #36, #37, #38,
  #39 — https://github.com/meigma/crossplane-cuefn/pull/30 … /39
- `.journal/TECH_NOTES.md` ("Required resources" section), `.journal/005/NOTES.md`
  (full running log, incl. the feasibility research + design rationale).
- Releases: `v0.1.1` (draft), `contract: v0.2.0` (draft + published to Central).

## Lessons

- **Adversarial verify earns its keep on the highest-risk PRs.** It caught two
  HIGH defects that would have failed CI or shipped wrong: the e2e fixture's
  namespace/name collision (would have failed the kind `e2e` job), and the docs
  how-to pointing its "test offline" command at the shipped `example/module`
  (which emits no requirements — the exact docs-drift the PR #30 audit found).
- **Verify the gate yourself, every PR.** Phantom gopls cross-worktree compiler
  diagnostics recurred (trust `go build`/`go vet`, session-003 lesson); real lint
  caught me twice (`protogetter` wants `GetRequirements()`; `golines` wrapping) and
  the YAML splitter had a real `---`-in-a-value bug. The mise-pinned
  `golangci-lint` cache is poisoned by deleted sibling worktrees → `cache clean`
  before the gate in fresh worktrees.
- **release-please path attribution is by file path, not commit scope:** a
  `feat(contract)` commit touching `internal/contract/` would bump the *product*;
  keep the contract test in a separate `test(contract)` PR. Product config uses
  `bump-patch-for-minor-pre-major` (feat → patch in 0.x), so the feature cut
  `0.1.1`, not `0.2.0`; the contract config does not, so it cut `0.2.0`.
- **The CUE seed/read-order is subtle:** `out.requirements` must be a pure
  function of stable inputs; if it references `input.requiredResources` it errors
  before the seed runs (the seed happens after `readRequirements`) unless the
  author defaults the field.
