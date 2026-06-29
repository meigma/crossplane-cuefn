---
id: 004
title: Session 004
started: 2026-06-29
---

## 2026-06-29 07:05 — Kickoff
Goal for the session: not yet stated — session opened with `/session-new` and no
task. Will update this entry once the developer states their actual request.

Current state of the world:
- Product is **functionally complete and CI-proven** (PLAN P1→P8 all merged, PRs
  #4–#11). `master` at `5c9a363`; `ci` + `integration` + `e2e` green.
- Session 003 (PRs #12, #13) reorganized + consolidated the integration/E2E test
  suite under `internal/test/{integration,e2e,common}` (integration tests 23→17).
- Architecture + every phase + gotchas are documented top-to-bottom in
  `.journal/TECH_NOTES.md`.

Known open threads carried in (none blocking):
- Promote hardened bits of `.journal/001/DESIGN.md` + `PLAN.md` into TECH_NOTES and
  delete those temporary working docs.
- Two untouched Dependabot PRs (#1 attest, #2 cache) from session 001.
- Assorted non-blocking CI niceties (e2e/integration fail-if-skipped guards;
  per-job tool installs; `example/xrd.yaml` lost its "generated" header).

Plan: await the developer's request before doing substantive work.

## 2026-06-29 08:10 — Design exploration: author-time contract enforcement + k8s schema imports
No code changes this session yet — a design conversation that should drive future
work. Thread:

1. **Two Chainsaw suites explained.** `test/chainsaw/schema/` (P4, envtest, driven by
   `internal/test/integration/schema_chainsaw_test.go`, moon `schema-test`) proves the
   *generated XRD schema* is structural + defaults/prunes/round-trips status on a bare
   apiserver — testing the codegen against the K8s API server (the thing Crossplane
   itself delegates schema enforcement to), no Crossplane needed. `test/chainsaw/e2e/`
   (P8, kind, `internal/test/e2e`, moon `e2e-test`) proves the full
   publish→install→reconcile product loop. Different layer, different cost, different
   fixtures (XWidget/platform.example.com vs XApp/platform.meigma.io). Not redundant.

2. **The fixed module-contract wrapper.** Field NAMES are hardcoded in our Go (paths in
   `internal/render/engine.go` + `internal/schema/xrd.go`); only values/inner shapes vary.
   Definitions: `#API` (required keys group/version/kind/plural, optional scope), `#Spec`
   (required), `#Status` (optional). Top-level: `input` (`.spec`/`.metadata.{name,namespace}`/
   `.environment`, filled by engine), `resources` (map; author keys; each entry `{object,
   ready?}`), `status` (optional). Symmetry: `#Spec`↔`input.spec`, `#Status`↔`status`.

3. **Idea (developer): ship an importable `contract` CUE module** so authors unify their
   module against our defs and get author-time `cue vet`/in-editor enforcement BEFORE the
   function consumes it. Feasible + idiomatic. Guardrail: keep `#Spec`/`#Status`
   import-free (they feed the finicky OpenAPI codegen); constrain only the wrapper.

4. **Bigger vision (developer): authors should import the K8s API CUE schema and
   instantiate objects from it** (e.g. `apps.#Deployment & {…}`) instead of hand-writing
   maps — so no input variation yields an invalid k8s object; caught at author/render time,
   not apply time. This dissolves the "import = permanent OCI dep" objection: modules are
   never import-free under this model anyway.

5. **CONSEQUENCE / work item:** our **local load path assumes offline/self-contained
   modules** — `LocalLoader` wires a `nil` dependency registry (`internal/render/load.go:35`).
   Under the import-heavy model, `cuefn generate --dir` / `validate` / offline `render` must
   become **dependency-aware** (wire a registry or honor CUE's `cue mod tidy` module cache).
   Runtime OCILoader already wires `load.Config.Registry`; it's the local-dir commands that
   regress. This is the real engineering the vision implies.

6. **RESEARCH RESULT — the CUE Central Registry now publishes the K8s API.** Module
   `cue.dev/x/k8s.io` @ **v0.7.0**, curated/official, per-Go-package imports
   (`import "cue.dev/x/k8s.io/api/apps/v1"`), fetched via `cue mod tidy`. Caveat: the
   `cue.dev/x/` path **prefix is explicitly temporary** ("while its proper location is being
   decided") — schemas are permanent; a future one-shot `cue refactor imports` migrates the
   prefix. This settles the sourcing fork in favor of the registry route over `cue get go`
   vendoring (Stefan Prodan's `kubernetes-cue-schema`, Timoni's old source). Prior art for
   the whole pattern: **Timoni** (Stefan Prodan; our example already uses podinfo).

Cost to keep in view: the K8s OpenAPI schema is large → CUE eval time/memory; keep the
runtime `CUE_CACHE_DIR` warm; measure render latency with a realistic k8s-imported module.

Next candidate step (proposed, not yet approved): a spike — author a module that imports a
real k8s schema + a local `contract` def, builds a `#Deployment`, and run it through
`cuefn render` / `generate` / `validate` to measure how badly the local (nil-registry) path
breaks and how slow eval gets.

## 2026-06-29 09:30 — Plan approved: dependency-aware local CUE loading + example on official k8s module
Ran under ultracode. Explored (3 Explore agents: loader core, CLI wiring, blast radius) +
designed (1 Plan agent), grounded in code + vendored `modconfig` source. Plan written to
`/Users/josh/.claude/plans/yes-please-start-by-eager-glade.md` and **approved**.

Key technical findings driving the design:
- **`load.go` needs no change** — `LoadModule` already wires a non-nil `Loaded.Registry`
  (`modconfig.Registry`) into `load.Config.Registry` (load.go:35-37). The local path just
  needs a non-nil registry; `LocalLoader` passes `nil` today (loader.go:46-48).
- **Registry construction** is inlined in `NewOCILoader` (oci.go:82-98:
  `modconfig.Config{Env}` → `NewResolver`+`NewRegistry`); extract a shared `buildRegistry`.
- **Decision 3 is mostly free:** `modconfig.DefaultRegistry = "registry.cue.works"` is the
  built-in catch-all. Unset OR **prefix-scoped** `CUE_REGISTRY` keeps central as default
  (resolve.go:323-332); only a **bare** value replaces it (the deliberate offline/air-gap
  override our tests use). So "handle it ourselves" = give the local loader a real registry
  + document the prefix form. Optional `cue.dev=` hardening rejected by default.
- **Digest gate precedes CUE eval** (verifyDigest at oci.go:138 before ensureExtracted) →
  the e2e uses example/module only as digest-gated drift content, NEVER evaluated in-cluster.
  So converting example needs NO in-cluster public-registry access as long as the e2e
  primary fixture (testdata/module) stays self-contained.
- **CUE registry config file form** (`CUE_REGISTRY=file:...` with `moduleRegistries` +
  `defaultRegistry`) exists as the clean structured alternative if string-composition is
  ever needed — not needed for the common case.

User decisions: (1) DECOUPLE example from tests — tests use hermetic fixtures, example
validated only by xrd-check + a render smoke; (2) xrd-check stays in the blocking gate +
CI CUE-cache warming; (3) central is the always-on default, CUE_REGISTRY only for private
registries (achieved natively).

5 PRs, sequenced so tests are decoupled BEFORE the example changes:
- PR1 render core (buildRegistry + NewLocalLoader, zero-value stays offline + S1 routing test)
- PR2 CLI wiring (moduleLoader(dir,cacheDir) + --cache-dir on 4 cmds)
- PR3 test decoupling (shared hermetic fixture internal/test/common/testdata/module; repoint
  all sites; rename ExampleModuleRef→HermeticModuleRef cuefn.test/app; e2e drift; moon groups)
- PR4 example conversion (transform.cue → cue.dev/x/k8s.io imports; cue mod tidy; XRD unchanged)
- PR5 CI cache warming + example-check render smoke + docs
Dep graph: PR1∥PR3 independent; PR2 after PR1; PR4 after PR1+PR2+PR3; PR5 after PR4.
Main risk = S2 (k8s schema field shape — needs network at PR4 impl time).

Following the repo norm (sessions 002/003): one PR per phase, **human sign-off before each
merge** — I implement + verify + open the PR, then STOP for review; I do not merge.

Starting PR1.

## 2026-06-29 09:55 — PR1 open (#14): render core dependency-aware
Worktree `.wt/feat-render-dep-aware-load` (branch `feat/render-dep-aware-load`, off
origin/master). Changes:
- `internal/render/oci.go`: extracted `buildRegistry(cfg) (*modconfig.Resolver,
  modconfig.Registry, string, error)` from the inlined construction in `NewOCILoader`;
  `NewOCILoader` now a thin caller.
- `internal/render/loader.go`: added unexported `registry` field + `NewLocalLoader(dir,
  cfg)`; zero-value `LocalLoader{Dir}` unchanged (nil registry, offline). `load.go`
  untouched (already wires non-nil registry).
- `internal/render/registry_test.go` (internal `package render` test): `TestBuildRegistry_
  Routing` — 3 offline subtests via `Resolver.ResolveToLocation` proving decision 3:
  unset→central; prefix-scoped→private routes local, central stays catch-all; bare→replaces
  central. ALL PASS offline. **modconfig API confirmed:** `ResolveToLocation(mpath, version)
  (HostLocation, bool)`; `HostLocation`=`modresolve.Location{Host, Insecure, Repository,
  Tag}`; `modconfig.DefaultRegistry == "registry.cue.works"`.
- Verified: `moon run root:check` GREEN (10 tasks). New-worktree gotcha: had to `mise trust`
  the worktree first. (Benign lint warning references a stale file in another worktree
  `.wt/test-consolidate`; docs:build "errors" are mkdocs-material's insiders upsell banner.)
- PR #14 opened, base master. NOT merged (human gate). Awaiting review.

## 2026-06-29 10:40 — PR1 merged (#14); PR2 (#15) + PR3 (#16) open
- **PR1 #14 MERGED** to master (75a3c4d) after user "LGTM. Proceed" + CI green (ci/integration/e2e).
  Cleaned up worktree (background `wt remove --force`; squash-merge left local branch, deleted it).
- **PR2 #15** (`feat(cli): wire dependency-aware local loading and add --cache-dir`): `moduleLoader(dir,
  cacheDir)` builds `NewLocalLoader` for the local path; `--cache-dir` on render/generate/validate/publish;
  `resolveModuleDigest` threads cacheDir. Both build tags + cli tests + `moon run root:check` green.
  Worktree `.wt/feat-cli-dep-aware-loader`. CI running at report time. Awaiting merge sign-off.
- **PR3 #16** (`refactor(test): decouple the test suite from the example module`): independent of #14/#15.
  New hermetic fixture `internal/test/common/testdata/module` (frozen 3-readiness copy of example) +
  `testdata/renderloop` assets; `common.HermeticModuleDir/HermeticRenderloopDir`; repointed ALL load sites
  (engine/function/cli/oci/publish/renderloop/e2e-drift). grep confirms ZERO test refs to example/module.
  moon: cueModules→fixture, render-test→hermetic assets, xrd-check+example input, removed unused exampleAssets.
  **DESIGN CHANGE vs plan: did NOT rename ExampleModuleRef** — kept fixture module path cuefn.example/app@v0
  so the e2e drift (republishes under cuefn.example/app@v0.1.0) path-matches; less churn, same decoupling.
  Verified: build (default/e2e/envtest vet), `moon run root:check` green, gated oci-test + render-test +
  publish-test green locally. Worktree `.wt/refactor-decouple-tests-from-example`. Awaiting merge.

Gotcha recurring: each new worktree needs `mise trust` before `mise exec -- moon ...`. Also the
golangci-lint cache is SHARED across worktrees — a half-deleted sibling worktree poisoned PR2's lint
(stale cross-worktree findings); fix = fully remove the worktree + `golangci-lint cache clean`.

Dep graph remaining: PR4 (example→k8s) needs #14+#15+#16 merged. PR5 (CI cache + smoke + docs) needs PR4.

## 2026-06-29 11:20 — PR2+PR3 merged; PR4 (#17) open — SUCCESS CRITERION met
- **PR2 #15 + PR3 #16 MERGED** to master after user "LGTM. Please merge both and proceed." Both CI-green
  (ci/integration/e2e). master now at 8a5a48f (#16) ← 3a70f63 (#15) ← 75a3c4d (#14).
- **PR4 #17** (`feat(example): instantiate Kubernetes objects from cue.dev/x/k8s.io`): worktree
  `.wt/feat-example-k8s-schema` off updated master.
  - **S2 spike RETIRED (network worked in sandbox):** probed `cue.dev/x/k8s.io@v0` → resolves to v0.7.0.
    `apps/v1.#Deployment`/`core/v1.#Service`/`core/v1.#ConfigMap` **pin apiVersion/kind** as concrete
    defaults, require nothing beyond what the example already set, and accept int targetPort/containerPort/
    port (intstr disjunction unifies). `cue eval -c` clean, NO extra defaulted fields → rendered output
    structurally identical to the hand-written version.
  - `transform.cue` rewritten to instantiate from the k8s schema; `cue mod tidy` (in example/module) added
    the `deps` block (cue.dev/x/k8s.io@v0 v0.7.0 default:true; **no cue.sum** — modern CUE inlines deps);
    api.cue package doc updated (dropped "no external imports/offline"). #API/#Spec/#Status UNTOUCHED.
  - **Verified locally:** `cuefn render --dir example/module --xr example/xr.yaml` with NO CUE_REGISTRY →
    renders 3 resources from k8s schema (proves dep-aware local load + central default end-to-end);
    `cuefn generate --dir example/module` → XRD byte-identical (xrd-check clean); `moon run root:check`
    GREEN (root:test offline on the PR3 fixture, xrd-check resolves k8s dep from warm cache).
  - **CI note:** ci.yml sets NO CUE_REGISTRY → central default; xrd-check fetches k8s dep live (public,
    reachable on GH runners). CUE-module cache deferred to PR5 (resilience only; first fetch must work
    regardless). integration/e2e jobs use the hermetic fixture → unaffected by the example change.
  - PR #17 opened, base master. Watching the `ci` job (first blocking-gate live k8s fetch). Awaiting
    merge sign-off; PR5 (CI cache + example-check smoke + docs) is next and needs PR4 on master.

Recurring gotcha confirmed again: shared golangci-lint cache poisoned by deleted sibling worktrees →
`golangci-lint cache clean` fixes it. Probe trick: `mise which cue` to get the pinned binary, run it with
cwd in the target module dir (mise's `--cd` points tool resolution at the worktree, not the run dir).

## 2026-06-29 12:10 — PR4 merged; PR5 (#18) open — PLAN COMPLETE (all 5 PRs implemented)
- **PR4 #17 MERGED** (master 6fe9932) after user "LGTM. Go ahead and continue." + full CI green
  (ci 44s incl. the FIRST live k8s fetch in the blocking gate — proves no cache needed for correctness).
- **PR5 #18** (`docs: cover the k8s-schema example, registry defaults, and --cache-dir`): worktree
  `.wt/feat-example-ci-cache-and-docs`. Three parts:
  1. **CI cache** (ci.yml): deterministic `CUE_CACHE_DIR: ${{ github.workspace }}/.cue-cache` + a
     `Cache CUE modules` actions/cache step keyed on `example/module/cue.mod/module.cue` (restore-keys
     fallback). Resilience/speed only — first fetch works regardless (proven on #17).
  2. **example-check** (moon.yml): render smoke (`cuefn render --dir example/module` asserting
     Deployment/Service/ConfigMap), added to `check.deps` → blocking gate now 11 tasks. The light "doesn't
     break" validation the example gets instead of being in the unit suite.
  3. **Docs (11 files)** — delegated to a FORK (inherited full context): module-contract + quickstart show
     the cue.dev/x/k8s.io import + deps block + `cue mod tidy`; "offline --dir" softened across how-to +
     cli.md; configuration.md documents central-as-always-on-default + the prefix form (only bare
     CUE_REGISTRY replaces central) + --cache-dir; render/validate/generate Long help + engine.go doc
     de-"offline"-ed. Fork self-verified docs:build strict + go vet.
  - **Verified myself** (session-003 lesson — don't trust agent self-report): `moon run root:check` GREEN
    (11 tasks incl. example-check + docs:build); ci.yml valid YAML; spot-checked the doc snippets match the
    real transform. PR #18 opened, base master. Watching the `ci` job (the cache change touches it).

### Series summary (all merged except #18 awaiting final sign-off)
#14 render core (buildRegistry + NewLocalLoader + offline routing test) → #15 CLI wiring (moduleLoader +
--cache-dir) → #16 decouple tests (hermetic fixture) → #17 example→cue.dev/x/k8s.io → #18 CI cache + smoke
+ docs. Net: the local load path resolves OCI deps; the example instantiates k8s objects from the official
schema; central is the always-on default registry; tests are fully decoupled from the example.

## 2026-06-29 12:25 — PR5 #18 MERGED — ALL 5 PRs DONE, paused
**PR5 #18 MERGED** (master e734c79) after user "LGTM. Merge once its green. Then pause." + full CI green
(ci 1m22s, integration 2m46s, e2e 2m59s). All worktrees cleaned (only master + journal remain). No
leftover open PRs from this session (only the two pre-existing Dependabot #1/#2 from session 001).

Final master order: e734c79 (#18) ← 6fe9932 (#17) ← 8a5a48f (#16) ← 3a70f63 (#15) ← 75a3c4d (#14) ← 5c9a363.
NOTE: the local `master` checkout is still at 5c9a363 (5 behind origin) — all work happened in worktrees off
origin/master; fast-forward the main checkout when convenient (not required).

User asked to PAUSE. Stopping here; not starting new work. Plan fully delivered.

## 2026-06-29 13:30 — New phase: module-contract v2 + importable contract module (plan approved)
Design discussion → user confirmed the two remaining contract pieces. Ran 3 Explore agents +
designed; new plan APPROVED (overwrote the old plan file). Two coupled PRs.

Design decisions (locked by the user):
- **Root field = `out`**: nest the transform (`input`/`resources`/`status`) under one root field
  `out: {...}`; keep `#API`/`#Spec`/`#Status` as TOP-LEVEL definitions.
- **Registry = CUE Central via `github.com/meigma` path** (resolves with zero CUE_REGISTRY config;
  needs `cue login` to publish).
- **Enforcement = author-time only**: the engine just reads `out.*`; it does NOT embed/unify the
  contract. The published contract module is the single source of truth (authors `cue vet`).

Key exploration facts:
- **v2 is 4 path literals** in `internal/render/engine.go` (`FillPath("input")`→`"out.input"` :132,
  `LookupPath("input")`→`"out.input"` :134, `"resources"`→`"out.resources"` :152, `"status"`→
  `"out.status"` :175). `cue.ParsePath` handles dotted paths.
- **Codegen UNCHANGED**: `internal/schema/openapi.go:75-94` `definitionsOnly` keeps only top-level
  `#`-defs and drops regular fields → a single `out` field is dropped like input/resources/status
  today → XRD byte-identical. validate.go + function.go touch no transform paths. Blast radius =
  internal/render + per-module text.
- **Engine is schema-agnostic** (never references #Spec/#Status; author's `out.input.spec: #Spec`
  binding applies the schema).
- **GOTCHA: two Explore agents explored the STALE main checkout (5c9a363, pre-#14–#18)** — the main
  `master` worktree was never fast-forwarded. Confirmed current origin/master (e734c79) via git show:
  the hermetic fixture `internal/test/common/testdata/module` EXISTS and example/module imports k8s.
  Fixed: fast-forwarding the main checkout. Implementation worktrees branch off origin/master.
- **Central publishing** (Agent C + web): publish under `github.com/meigma/...` (cue login, GitHub
  auth) → resolves with no consumer config (Central is CUE's default). S1 spike: in-repo subdir
  module (`github.com/meigma/crossplane-cuefn/contract@v0`) vs dedicated repo.

8 engine-loaded modules need the `out` restructure (example, hermetic fixture, e2e fixture,
render/testdata {nostatus,badstatus,nonconcrete}, oci {consumer,mutable/v1,v2}). 4 codegen-only/
library fixtures untouched (schema/testdata {derisked,disjunction,nostatus}, oci/dep).

PRs: **PR A** = v2 `out` restructure (engine 4 literals + 8 modules + docs; breaking, offline-testable).
**PR B** = contract module (`contract/` dir + #API/#Resource/#Input/#Transform, closed) + publish to
Central + example imports it; hermetic fixtures stay import-free. Same per-PR human-sign-off norm.

Starting PR A.

## 2026-06-29 14:30 — PR A open (#19): module-contract v2 (`out` nesting)
Worktree `.wt/feat-module-contract-v2`. Changes (12 files):
- `internal/render/engine.go`: 4 literals → `out.input`/`out.resources`/`out.status` + a clear
  pre-v2 error when `out` is absent (used `cue/errors.New`, NOT stdlib). Schema-agnostic engine
  unchanged otherwise.
- 8 engine-loaded modules wrapped under `out: {...}` (locals `_name`/`_tier` moved inside out —
  hidden fields are allowed inside a closed struct, so this also works for PR B's `out:
  contract.#Transform & {...}`): example/module, internal/test/common/testdata/module (hermetic),
  internal/test/e2e/testdata/module, render/testdata/{nostatus,badstatus,nonconcrete},
  oci/{consumer,mutable/v1,v2}. CUE is whitespace-insensitive → wrapped via heredoc + `cue fmt`.
- Docs: module-contract.md + quickstart.md → `out` shape (no contract-module ref yet; that's PR B).
- 4 codegen-only/library fixtures untouched (schema/testdata/{derisked,disjunction,nostatus}, oci/dep).
- **Verified myself:** `cue mod`-level — example renders v2 (`cuefn render --dir example/module`);
  XRD byte-identical (codegen drops the regular `out` field like it dropped input/resources/status);
  `moon run root:check` GREEN (11 tasks incl example-check); gated oci-test + render-test + publish-test
  GREEN. e2e/funcpkg need an image rebuild → CI.
- PR #19 opened (BREAKING, pre-1.0, no shim). Awaiting merge sign-off. PR B (contract module +
  Central publish) needs #19 on master + the publishing decision (S1: in-repo subdir vs dedicated repo,
  cue login flow).

Tooling notes: `cue fmt <dir>` fails ("cannot be imported as a CUE package") — pass file paths or
`./...`. root:format only checks Go (golangci-lint fmt), not CUE. Recurring: cleared golangci-lint
cache (shared across worktrees) before the gate.

## 2026-06-29 15:30 — PR A #19 MERGED; PR B1 (#20) open: the contract module
- **PR A #19 MERGED** to master (c825fe6) after user "PR #19 LGTM, please merge" + full CI green
  (ci/integration/e2e — the e2e rebuilt the dev image with the v2 engine and passed → v2 works in-cluster).
- **User decided the contract path: in-repo `github.com/meigma/crossplane-cuefn/contract@v0`.** Researched
  Central publishing: NO separate registration form — `cue login` (one-off GitHub device flow) + push access
  to the meigma/crossplane-cuefn repo IS the authorization. Subdir modules supported. Flow (in contract/):
  `cue mod init --source=git <path>`, `cue login`, `cue mod publish v0.1.0`. CI-token + exact subdir git-tag
  form = confirm at publish time.
- **PR B1 #20** (`feat(contract): add the importable cuefn module contract`): worktree
  `.wt/feat-contract-module`. SPLIT from the original "PR B" because the example can't cleanly import an
  UNPUBLISHED contract (CI would block on the real import; a local-replace in the shipped example is a smell).
  So B1 = contract source + validation (mergeable, NO publish needed); B2 = example adoption + publish + docs
  (after the user's bootstrap publish).
  - `contract/cue.mod/module.cue` (cue mod init --source=git) + `contract/contract.cue`: closed `#API`,
    `#Resource` ({object, ready?}), `#Input` (out.input), `#Transform` (closed out wrapper). #Spec/#Status
    NOT wrapped (codegen guardrail).
  - `internal/contract/` (doc.go + contract_test.go): loads the contract via `render.LoadModule`(LocalLoader),
    proves closedness OFFLINE — conforming transform OK; `resorces` typo / bad `ready` / unknown #API key all
    REJECTED. 4 subtests pass.
  - moon: `contract/**/*.cue` → cueModules.
  - Verified: `go test ./internal/contract/...` pass; `cue vet ./contract/...` clean; `moon run root:check`
    GREEN (11 tasks). (root:format wanted a long line wrapped → `golangci-lint fmt` applied.)
  - PR #20 opened. Awaiting merge sign-off.
- **NEXT (needs user):** after #20 merges → user runs `cue login` (interactive device flow) once → then
  `cue mod publish v0.1.0` from contract/ (I can run the publish once auth is cached) → then PR B2 (example
  imports the real path + cue mod tidy + release publish job + docs). The hermetic fixtures stay import-free.

## 2026-06-29 16:30 — User: CI-managed publish via release-please (proper monorepo). Restructured.
User rejected manual publish; wants CI-managed publishing tied into release-please as a PROPER MONOREPO:
independent components, separate PRs per component, monorepo-style tags, independent semver. Auth = GitHub
OIDC (no secret) — user chose this.

Researched (agent + web), KEY findings:
- **Headless publish IS supported via OIDC**: `cue-labs/registry-login-action@v1` (SHA 66d40052...) exchanges
  the GH OIDC token for a short-lived registry token, writes ~/.config/cue/logins.json → `cue mod publish`
  works. Job needs `id-token: write`. ONE-TIME manual setup: trust entry at registry.cue.works/account/oidc
  for meigma/crossplane-cuefn (web UI, can't be done from CI). Fallback: `cue login --token` + a secret.
- **CUE ignores git tags**: `cue mod publish vX.Y.Z` uploads committed content (source:git, clean tree,
  major must match @v0). No tag-format constraint. Subdir module auto-folds the repo-root LICENSE (repo has
  LICENSE-APACHE/-MIT, no plain LICENSE → may warn; non-blocking).
- **release-please monorepo**: add `separate-pull-requests: true` + a `contract` package (release-type
  simple, component contract, tag-separator "/", include-component-in-tag true → tag `contract/v0.1.0`,
  which does NOT match release.yml's `v*` trigger). Per-component outputs `contract--version` etc.

**CRITICAL release-please constraint discovered:** a single squash commit touching BOTH contract/ AND root
files (test, moon, release config, workflow) attributes to BOTH components → would bump root too (draft
product release). So to keep components independent, the contract-RELEASE-triggering commit must touch ONLY
contract/. → restructured into a sequence:
- **#21** `ci(release): set up monorepo releases for the contract module` — release-please config (monorepo +
  contract component) + manifest (contract@0.0.0) + `.github/workflows/release-contract.yml` (OIDC publish on
  contract/v* tag) + moon cueModules glob. NON-bumping (ci) → merges with NO release. Worktree
  `.wt/build-contract-release-setup`. Verified: actionlint clean, JSON valid, root:check green (11 tasks).
- **#20** RESCOPED to `feat(contract)` = contract/*.cue ONLY (amended; removed the test + moon, force-pushed)
  → release-please turns this into the independent contract release PR.
- The closedness test (internal/contract, saved to /tmp/contract-test/) lands as a follow-up non-bumping
  `test(contract)` PR after #20 merges (it needs contract/ + must not bump the product).

**Merge order (matters for attribution):** #21 (wiring) FIRST → then #20 (source) → release-please opens a
`contract` release PR → merge it → tag contract/v0.1.0 → workflow publishes. OIDC trust setup before that
contract release PR is merged. Then B2 (example adoption) + the test PR.

release.yml convention mirrored: mise-action `cache: false` (fresh, mise.lock-verified) for the publish job;
ubuntu-24.04; permissions:{} top + per-job; SHA-pinned actions.
