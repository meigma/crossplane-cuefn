---
id: 004
title: Dependency-aware CUE loading, k8s example, module-contract v2, and the published contract module
date: 2026-06-30
status: complete
repos_touched: [crossplane-cuefn]
related_sessions: [001, 002, 003]
---

## Goal

Opened with no fixed goal; the developer's questions drove a multi-part initiative
(ran under ultracode throughout), in order:

1. Make the **local** CUE module-load path dependency-aware and convert the example
   to instantiate Kubernetes objects from the official `cue.dev/x/k8s.io` schema —
   decoupling the example from the test suite (the example is for examples, not a
   test dependency).
2. **Module-contract v2:** nest the transform (`input`/`resources`/`status`) under a
   single `out` root so one closed definition can validate it.
3. Ship an **importable contract module** — closed CUE definitions authors unify
   against for author-time (`cue vet`) validation — published and versioned
   independently via release-please + GitHub OIDC.

Hard constraint (per the repo norm): one PR per phase, human sign-off before each
merge.

## Outcome

**Goal fully met across 16 human-signed-off PRs (#14–#29).** `master` at `e41d12a`.
Both halves shipped, and the repo cut its **first real automated releases**:

- **Product `v0.1.0`** released (GoReleaser binaries + signed melange/apko image +
  Function xpkg + cosign + attestations; draft GitHub release awaiting publication).
- **Contract `v0.1.0` published to the CUE Central Registry** and verified resolving
  from a clean module with zero `CUE_REGISTRY` config.

## Key Decisions

- **Dependency-aware local loader, central-as-default:** `render.NewLocalLoader`
  attaches a registry (zero-value `LocalLoader` stays offline). `modconfig` already
  keeps `registry.cue.works` as the catch-all default for unset/prefix-scoped
  `CUE_REGISTRY`; only a bare value overrides it. So no string surgery — central
  resolution is free.
- **Tests decoupled from the example:** a self-contained hermetic fixture
  (`internal/test/common/testdata/module`) is the one module the suites load; the
  example carries the public deps. The example is validated only by `xrd-check` +
  a `example-check` render smoke, never woven into unit/integration tests.
- **Module-contract v2 = `out` nesting** (root field `out`); `#API`/`#Spec`/`#Status`
  stay top-level definitions. Engine change is 4 path literals; codegen unchanged
  (XRD byte-identical). Author-time-only enforcement — the engine does NOT embed the
  contract; the published module is the single source of truth.
- **Contract module = `github.com/meigma/crossplane-cuefn/contract@v0`** on the CUE
  Central Registry (in-repo `contract/` dir; resolves with zero config). Exposes
  closed `#API`/`#Resource`/`#Input`/`#Transform`. `#Spec`/`#Status` are NOT wrapped
  (they feed the XRD codegen and stay the author's import-free schemas).
- **Versioning policy:** the contract's major is welded to the function's major
  (both `v0`), enforced by `bump-minor-pre-major`. Within `v0`: fix→patch,
  feature→minor; breaking stays in `0.x`; `v1` only as a deliberate coordinated bump
  when the function goes `v1`. Authors pin `@v0`.
- **CI-managed publishing, no secret:** release-please monorepo (`separate-pull-requests`,
  independent `contract` component, `exclude-paths:["contract"]` on root) + a
  `release-contract.yml` that authenticates to Central via **GitHub OIDC**
  (`cue-labs/registry-login-action`) and runs `cue mod publish`. The contract's
  `contract/v*` tag never collides with the product's `v*`.

## Changes

16 squash-merged PRs on `master`:

- **Dep-aware loader series:** #14 render core (`buildRegistry`/`NewLocalLoader` +
  offline routing test), #15 CLI wiring (`moduleLoader` + `--cache-dir`), #16
  decouple tests (hermetic fixture), #17 example → `cue.dev/x/k8s.io`, #18 CI CUE
  cache + render smoke + docs.
- **Contract series:** #19 module-contract v2 (`out`), #20 contract source, #21
  monorepo release wiring + OIDC publish workflow, #24/#25/#27 release-please version
  fixes (both bootstrap to `v0.1.0` via `release-as`, then removed), #28 closedness
  test (`internal/contract`), #29 example adopts the contract + docs.
- **Release artifacts:** product `v0.1.0` (draft) + `contract: v0.1.0` (draft) GitHub
  releases; contract published to Central; product image/xpkg on ghcr.
- Journal (`journal/jmgilman`): `NOTES.md` (full running log), this `SUMMARY.md`,
  `TECH_NOTES.md` additions (the contract module + release-please + v2 facts).

## Open Threads

- **Two draft GitHub releases** (`v0.1.0`, `contract: v0.1.0`) await the maintainer's
  publication after inspection (draft:true is deliberate).
- **Possible enhancement:** a function-side runtime check of a module's imported
  contract major, for an explicit mismatch error (we chose author-time-only for now).
- Pre-existing, untouched: two Dependabot PRs (#1 attest, #2 cache) from session 001;
  assorted non-blocking CI niceties from prior sessions.
- The session-001 working docs (`.journal/001/DESIGN.md`, `PLAN.md`) are still marked
  temporary — promote any load-bearing bits and delete (carried since session 002).

## For the next agent (start here)

1. Read `.session.md`, `.journal/SKILLS.md` (load `git`, `worktrunk`; load `cue` for
   CUE work), then `.journal/TECH_NOTES.md` top-to-bottom — it documents the whole
   architecture incl. the contract module, the release-please monorepo, and the
   versioning policy.
2. **Releases are now automated** via release-please (the App creds + OIDC trust are
   configured). A `feat`/`fix` touching product code → product release PR (`v*`); a
   `feat`/`fix` touching `contract/` → contract release PR (`contract/v*`, stays on
   `v0`). Non-bumping types (`ci`/`docs`/`test`/`chore`/`build`) cut nothing.
3. The contract is `github.com/meigma/crossplane-cuefn/contract@v0` on Central; the
   example is the reference adoption.

## References

- PRs (all merged): #14–#29 — https://github.com/meigma/crossplane-cuefn/pull/14 … /29
- `.journal/TECH_NOTES.md` (durable record), `.journal/004/NOTES.md` (full log)
- Plan file (contract v2 + module): `/Users/josh/.claude/plans/yes-please-start-by-eager-glade.md`
- Contract module on Central: `github.com/meigma/crossplane-cuefn/contract@v0` (v0.1.0)
- Research sources: CUE central-registry publishing (OIDC `cue-labs/registry-login-action`),
  `cue.dev/x/k8s.io` (curated k8s schema), release-please monorepo config.

## Lessons

- **release-please had never run** in this repo — every run since the start failed at
  the GitHub-App-token step (creds were never configured). Set
  `MEIGMA_RELEASE_APP_ID` + `MEIGMA_RELEASE_APP_PRIVATE_KEY` from 1Password
  (`op read "op://Homelab/meigma-release-please/key.pem" | gh secret set …`; an `op`
  file attachment resolves via `op://vault/item/filename`).
- **release-please first-release defaults to `1.0.0`** for a never-released component,
  overriding `bump-minor-pre-major` (which only keeps an *existing* `0.x` in `0.x`).
  Resetting the manifest to `0.0.0` made it worse (→`1.0.0`). The deterministic fix is
  `release-as: "0.1.0"`, removed after the bootstrap.
- **A second release PR conflicts** on the shared manifest/CHANGELOG after the first
  merges; re-running release-please does NOT auto-rebase it — delete its branch and
  re-run so it regenerates fresh.
- **Verify the engine vs codegen seam:** v2 was 4 path literals + every module
  restructured; the codegen drops the regular `out` field exactly like the old
  transform fields, so the XRD stayed byte-identical (proven, not assumed).
- **Recurring infra:** new worktrees need `mise trust`; the golangci-lint cache is
  shared across worktrees and gets poisoned by deleted siblings → `golangci-lint
  cache clean` before the gate. The local `master` checkout drifts behind `origin`
  when all work happens in worktrees → fast-forward it before spawning explore agents
  (two agents explored stale master this session).
