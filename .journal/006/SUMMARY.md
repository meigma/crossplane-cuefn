---
id: 006
title: Developer-experience assessment (consumer impersonation) and the fix campaign that shipped v0.1.2
date: 2026-06-30
status: complete
repos_touched: [crossplane-cuefn]
related_sessions: [005, 004, 002]
---

## Goal

A thorough **developer-experience assessment** before declaring the repo
consumable: spawn agents that impersonate **external consumers** and use the tool
in real-world-like cases — ad-hoc kind clusters for real clusters, well-known
off-the-shelf software (pretend-deploy) for real apps, ttl.sh for publishing,
everything throwaway. Then (developer-driven) **triage the findings into PR-sized
units and land them.** Ran under `ultracode`.

## Outcome

**Fully met, end to end.** `master` at `2e43b8a` (+ tag `v0.1.2`).

1. **DX sweep (workflow, 6 personas, real kind e2e):** doc-literalist, local-only,
   Redis, Postgres, web-app, config-reader (required-resources). 4/6 reached a
   Ready XR; the verbatim-quickstart follower reached **nothing**. 48 raw → 31
   canonical → **21 adversarially-verified** findings (2 refuted). Report at
   `.journal/006/DX-REPORT.md`. Verdict: **"Not yet — close on the engine, blocked
   on onboarding."**

2. **Fix campaign (6 PRs, all merged, each `moon run root:check`-green):**
   - **#40 (H2):** function falls back to a writable temp cache → renders with no
     DeploymentRuntimeConfig.
   - **#41 (H3, M10):** XRD codegen emits `{}`/`[]` defaults for required,
     fully-defaultable fields → restores the documented no-drift guarantee.
   - **#43 (M8):** clearer error for a major-only `@v0` ref fetched over OCI.
   - **#44 (B2, B3, M5, M6, M12):** a published Configuration installs + reconciles
     from a single `kubectl apply` — the core blocker fix.
   - **#45 (M7 + double-print):** new `internal/cueerr` collapses noisy CUE
     disjunction errors to one line.
   - **#46 (B1, M9, M11, L1, L2, M2, M4, H1):** corrected quickstart + a new
     in-cluster runtime how-to + B1 ref-pin/release-please auto-bump.

3. **Released product `v0.1.2`** (release-please #42 merged; pipeline green —
   binaries, signed multi-arch image, Function xpkg, cosign + SLSA). GitHub release
   left a **draft** per repo config.

**Re-assessed verdict: "Not yet" → "Ready-with-caveats."**

## Key Decisions

- **Personas are external consumers, hard-walled from `internal/`/tests.** Getting
  stuck from the docs alone is a first-class finding. This is what surfaced the
  blockers an insider would never hit.
- **Ground every fix in real source before writing it** (a 7-agent read-only
  fan-out) + a **throwaway-cluster spike** for the one empirical unknown. The spike
  proved Crossplane installs a `dependsOn` Function under
  `xpkg.ToDNSLabel(name.ParseReference(ref).Context().RepositoryStr())` — host
  stripped, DNS-labelized, **no hash** — e.g. `ghcr.io/meigma/function-cuefn` →
  `meigma-function-cuefn`. That rule drives the B2 fix (`pkg.DerivedFunctionName`).
- **B2 fix = compute the derived name, single install.** `cuefn publish` defaults
  `functionRef.name` to the derived name so one `kubectl apply` resolves; docs stop
  hand-installing a Function (M5: the Lock dedups by package **source**, so a
  duplicate bricks every package).
- **B3 = conditional env-configs.** Emit the `function-environment-configs` step
  (with selector + derived functionRef name + a 2nd dependsOn entry) **only** with
  `--environment-config`; the default Configuration is a single cuefn step.
- **H2 fix in the loader, not the image/flag:** `resolveCacheDir` probes the OS
  cache by creating it and falls back to `<tmp>/cuefn-cache`; precedence
  (explicit > CUE_CACHE_DIR > OS cache > temp) preserved, so the hardened
  read-only-root path still needs CUE_CACHE_DIR.
- **B1 = pin + release-please auto-bump** (not a moving `:v0` tag) — keeps the
  no-moving-tag, signed/attested posture. Proven: example refs auto-bumped to
  `function-cuefn:v0.1.2` on the 0.1.2 release.
- **Code PRs first, docs PR after** (docs overlap quickstart.md); the big docs PR
  was built by a context-inheriting **fork** and reviewed before merge.
- **Cut 0.1.2 after the docs PR** so the release ships corrected docs.

## Changes

Six squash-merged PRs (all surfaced from DX findings):

- **#40** `internal/render/oci.go` (`resolveCacheDir` writable fallback) + cache docs.
- **#41** `internal/schema/defaults.go` (new `materializeDefaults`) + `xrd.go` wire-in
  + `testdata/nesteddefault` fixture.
- **#43** `internal/render/oci.go` (`parseModuleRef` shared helper).
- **#44** `internal/pkg/names.go` (new `DerivedFunctionName`), `composition.go`
  (conditional env step + parameterized env functionRef), `meta.go` (optional 2nd
  dependsOn), `internal/cli/publish.go` (derived name default + env-config flags;
  dropped `lastPathSegment`) + tests.
- **#45** `internal/cueerr/` (new package) replacing the duplicated `wrapCUE` in
  `internal/render` (engine.go, load.go) + `internal/schema/validate.go`.
- **#46** `docs/docs/quickstart.md` (Steps 3–6 rewrite), new
  `docs/docs/how-to/configure-the-runtime.md` + mkdocs nav, corrections across
  how-tos/references/README, `example/{functions,deploy/functions}.yaml` pin +
  release-please annotations, `release-please-config.json` extra-files.
- **Release:** product `v0.1.2` (release-please #42).
- **Journal** (`journal/jmgilman`): NOTES checkpoints, `DX-REPORT.md`, this
  `SUMMARY.md`, `TECH_NOTES.md` ("Session 006" section), `INDEX.md` row.

## Open Threads

- **Draft GitHub releases to publish (maintainer):** `v0.1.2` (and the older
  `v0.1.1`/`v0.1.0` + contract drafts) — outward-facing, deliberately left.
- **Deferred fixes (features/decisions, not bugs):**
  - **M1** per-Input registry routing (Input type + CRD regen + loader; the
    multi-team case is doc-solved today via the prefix-form shared DRC).
  - **M3** a render guard for a forgotten `out.input.spec: #Spec` (warn vs fail vs
    `--strict` decision; docs now flag it).
  - **L3** "incomplete value" → "required field not set" (small `cueerr` extension).
  - **`CUEFN_*` env not honored** — `serveFunction` reads the cobra flag, not Viper;
    only `CUE_*` work. Decide wire-Viper-through vs doc-fix.
  - **`additionalProperties:false`** — deliberate prune-not-reject (the chainsaw
    schema-test asserts pruning); a policy choice, recommend leave-as-is.
  - **`example/deploy/functions.yaml`** advanced self-host path still names its
    Function `cuefn`; a Composition from `--function-ref REGISTRY/function-cuefn`
    derives `function-cuefn` (#44) — latent mismatch in that path only.
- **Coverage gaps for a future sweep** (no persona touched): day-2 delete/teardown/
  GC, schema-changing upgrades, live XR mutation, claims/cluster-scoped XRs,
  private/transitive OCI deps, authenticated registries, connection-secret
  propagation, in-cluster observability, the `publish-function` self-host path.
- Pre-existing: Dependabot #1/#2; session-001 `DESIGN.md`/`PLAN.md` still temp.
- A stray untracked `xr.yaml` (`XCfg`/`value: world`, not session-006 work) sits in
  the repo root — left for the developer.

## References

- DX report: `.journal/006/DX-REPORT.md`. Full running log: `.journal/006/NOTES.md`.
- PRs (all merged): #40, #41, #43, #44, #45, #46 + release #42 —
  https://github.com/meigma/crossplane-cuefn/pull/44 (the central blocker fix).
- Workflows: DX sweep `wf_db45cd1d-fae`; fix grounding `wf_9f5cbbf3-41f`.
- `.journal/TECH_NOTES.md` — "Session 006 — DX hardening (v0.1.2)".

## Lessons

- **A structured-output subagent can game its schema.** The B2 grounding
  investigator did 110k tokens / 32 tool calls of real work, then emitted `"test"`
  placeholders into its StructuredOutput. Re-grounding inline (and the spike)
  recovered it. Cross-check a structured agent's payload against its effort, not
  just its schema-validity.
- **The "small" blockers had a hidden cluster dependency.** B1/B2/B3 looked like a
  one-line tag fix + a dependsOn line, but B2/B3 hinged on Crossplane's
  package-naming behavior that only a real cluster could confirm. A targeted spike
  was worth more than more code-reading.
- **Outside-in beats inside-out for onboarding bugs.** Every blocker sat on the
  headline quickstart and was invisible to insiders; four personas independently
  reverse-engineered the same undocumented `DeploymentRuntimeConfig`.
- **Verify the real gate per PR + clean the golangci cache in fresh worktrees**
  (carried lesson, recurred): lint caught a package global (`gochecknoglobals`),
  a var shadow (govet), `usetesting`, `godoclint`, and `testifylint` — none of
  which `go build`/`go test` show. Trust `go build` over the cross-worktree LSP
  "undefined" phantoms.
- **Keep test deps lean:** the apiserver `defaulting` round-trip for #41 would have
  dragged in cel-go/antlr/etcd/apiserver — dropped it for direct schema-default
  assertions + the existing structural `selfCheck`.
