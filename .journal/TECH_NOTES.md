# Technical Notes

- Use hexagonal architecture at all times. Keep business logic isolated from CLI, filesystem, network, storage, and other external adapters.
- Prefer functional testing before calling any feature complete. Unit tests are useful, but they do not prove the tool works the way the design intends.
- Take an agile approach to development. Avoid waterfall: underspecify when useful, prototype early, learn from the result, and refine from working behavior.

## Project: crossplane-cuefn — what we're building

A **Crossplane v2 composition function (Go, function-sdk-go) that renders Kubernetes objects from CUE modules pulled from an OCI registry**, plus an operator CLI for the developer experience. One CUE module is the single source of truth for both the XRD schema and the transformation logic.

Two halves:

1. **Runtime function.** Go composite function. Input = an OCI reference to a CUE module. At reconcile it pulls/unpacks the module, injects the XR spec + any EnvironmentConfig under a well-known path, evaluates the CUE via the Go CUE SDK, and maps the module's returned Kubernetes objects into desired composed resources. A generic CUE rendering engine as a Crossplane function. (Prior art: `function-cue` is inline-only; `function-kcl` loads OCI/Git modules — we're the CUE analog that loads modules from a registry.)

2. **Operator DX (CLI we provide).** Operator authors ONE CUE module containing both the transformation code AND a full schema for the XR spec, and publishes it to an OCI registry. Our CLI, pointed at that module, does CUE→OpenAPI translation to generate an XRD spec, then packages + pushes a versioned Crossplane **Configuration** (XRD + a Composition wiring our function with the module ref). Ideally generate+push is one command, reusing Crossplane's own xpkg code if importable. End state: operator publishes a module + an auto-generated Configuration; cluster usage = install the Configuration and instantiate XRs.

   - **Bonus command:** validate a populated XR against a target CUE module's schema ahead of time (CUE unify/vet → good error messages) before it reaches the cluster.

Design seam: the "CUE module → resources" engine is shared core, reused by the runtime function, a local `render` path, and the `validate` command (clean hexagonal boundary).

### Reference spike (catalyst-infra, 2026-06-26)

Location: `/Users/josh/work/catalyst-infra/.wt/experiment-platform-mvp/platform/mvp/cuefn`. A working MVP of the **runtime half**. Productizing it into `meigma/crossplane-cuefn` (template-go base) is the task. Definitive, adoptable facts:

Stack (exact versions proven to work together): `cuelang.org/go v0.16.1`, `crossplane/function-sdk-go v0.7.1`, `crossplane-runtime/v2 v2.3.1`, `crossplane/apis/v2 v2.3.1`, `k8s.io/apimachinery v0.35.3`, `sigs.k8s.io/controller-tools v0.20.0`, `google.golang.org/protobuf`, testify. CUE's OCI transport is `cuelabs.dev/go/oci/ociregistry`. (Spike CLI used kong; our repo uses cobra/viper.)

Runtime architecture (proven, adopt mostly as-is):
- `internal/render` is the hexagonal core. `Engine` + `ModuleLoader` port; adapters `OCILoader` (real) and `LocalLoader` (tests/offline).
- **Module contract:** engine fills top-level `input: {spec, metadata:{name,namespace}, environment}` and reads top-level `resources: [<k8s objects>]`. Authors never touch Crossplane req/resp. Input filled via **JSON marshal** (not Go encode) to avoid float64-vs-int constraint conflicts. `resources` validated `cue.Concrete(true)` then decoded to `[]map[string]any`.
- **OCI load:** `modconfig.NewResolver(nil)` → `modregistry.NewClientWithResolver(r).GetModule(ctx, mv)` → `GetZip` → unzip stripping `<module>@<version>/` prefix into tempdir → `load.Instances(["."], &load.Config{Dir})`. Honors `CUE_REGISTRY` incl `+insecure`.
- **fn.go:** reads `Input.Module` (`path@version`), observed XR `spec`/metadata, env from context key `apiextensions.crossplane.io/environment`; deterministic composed name `<lowercase-kind>-<metadata.name>`; emits via `composed.New()` + set `.Object`.
- Pipeline **requires `function-environment-configs` upstream** to populate the env context (see example/composition.yaml).
- Packaging in spike: **ko** (distroless/static:nonroot, no Dockerfile) → `crossplane xpkg build --embed-runtime-image` → `crossplane xpkg push`. Function image must be **HTTPS** (Crossplane pkg manager is HTTPS-only); CUE modules can use plain-HTTP local registry. **Delta for our repo: replace ko with melange/apko/cosign** (template-go's supply chain).

Open / green-field (the DX half — not in the spike):
- **No CLI** for CUE→OpenAPI→XRD or Configuration packaging. Spike's XRD is hand-written; module schema is informal (`input.spec` is an OPEN struct with inline `| *default`s — provides defaults, does NOT enforce a schema, and carries no API metadata).
- **Module contract v2 (the linchpin to design):** add a strict named schema def (e.g. `#Spec`) + API metadata (group/kind/version/scope/plural) at well-known paths so ONE module drives runtime + XRD codegen + validate. Likely also unify `input.spec` against `#Spec` in the engine (authoritative defaults+validation at render).
- **Transitive OCI deps not handled:** `load.Config{Dir}` only (no `Registry`), so modules importing OTHER OCI modules won't resolve. Wire `load.Config.Registry` via `modconfig.NewRegistry` to support deps.
- **CUE→OpenAPI:** `cuelang.org/go/encoding/openapi`; must yield K8s *structural* schema (risk area A). **Configuration gen+push:** spike already shells `crossplane xpkg` — importing Crossplane's xpkg code is the stretch goal for one-command UX.
- **Digest lock-step (F):** spike pins module by tag `@v0.1.0`, not digest; generated Configuration must pin the digest it was built from.

### Codegen de-risk spike — PROVEN (2026-06-27)

Scratch: `…/scratchpad/codegen-spike` (throwaway). Built CUE `#Spec` → OpenAPI → wrapped as XRD → validated with the **API server's own** `apiextensions-apiserver/pkg/apiserver/schema` `NewStructural` + `ValidateStructural`. A PASS == a real cluster accepts the XRD. Risk A (CUE→OpenAPI structural fidelity) is **retired**: a realistic spec (required `!`, bounded int+default, string/int enums, regex, bool default, nested struct, list-of-objects, map patterns) generates a fully structural XRD.

Two implementation gotchas (both fixed in-spike, must carry into the real `internal/schema` codegen):
1. `openapi.Config{ExpandReferences:true}` is **buggy with bounded numbers** → "unsupported op for number &". Fix: generate with `ExpandReferences:false`, then inline `$ref`s ourselves (~30 LOC recursive inliner; cycle-detecting). K8s structural schemas forbid `$ref` anyway, so we must inline regardless.
2. `openapi.Generate` **rejects regular (non-`#`) top-level fields** ("unsupported top-level field input/resources"). Fix: reduce the module value to definitions-only before generating (`v.Fields(cue.Definitions(true))`, keep `sel.IsDefinition()`, FillPath into a fresh struct).

Author guardrail (the ONLY real constraint on schema CUE): **type-crossing disjunctions are not expressible.** `string|int` and struct unions `{a}|{b}` → `oneOf` → REJECTED structural. Same-type disjunctions (`"a"|"b"|"c"`, `80|443`) → `enum` → fine. This is a Kubernetes CRD limitation, not ours. Future nicety: detect `string|int` and emit `x-kubernetes-int-or-string: true`.

## Design + plan (session 001)

Authoritative spec: `.journal/001/DESIGN.md` (resolved decisions §13) and
`.journal/001/PLAN.md` (8 phases + falsifiable success criteria). Runtime engine
is the first implementation slice. Two non-obvious runtime traps surfaced by the
plan review (do not re-learn the hard way):

- **Strip `spec.crossplane`** (and legacy machinery keys) from the observed XR
  spec before unifying with the *closed* `#Spec` — otherwise `resources` never
  goes concrete in-cluster (the reference spike only worked because its
  `input.spec` was open).
- **No digest-by-ref:** CUE loads modules by **semver**, not OCI digest. Enforce
  the schema↔runtime lock-step by recording the expected manifest digest and
  **verifying it after fetch**, not by referencing the module by digest.

Output contract: module returns `resources: {<stableName>: {object, ready?}}` +
optional `status`; engine fills `input.{spec,metadata,environment}` via JSON
marshal (float64-vs-int safety). Module cache must use a writable `CUE_CACHE_DIR`
(function image is nonroot). Configuration/function xpkg are built in-process with
go-containerregistry (Crossplane's xpkg builder is `internal/`, not importable).

Validated **module-contract-v2** shape (one module = source of truth):
- `#API: {group, version, kind, plural, scope, ...}` — concrete; CLI `Decode`s it for the XRD envelope.
- `#Spec: {...}` — authoritative closed spec schema; CLI → OpenAPI → XRD `.properties.spec`; runtime unifies XR spec against it; `validate` checks against it. (Optionally `#Status`.)
- Transform (regular fields): `input: {spec: #Spec, metadata, environment}` + `resources: [...]`. Because `input.spec: #Spec`, the same defaults/constraints apply at render — XRD defaults (API-server-filled) and render defaults come from one place, so no drift.
Codegen reduces to definitions-only, so the transform fields never interfere with OpenAPI generation. Confirmed `controller-tools`-free; pure CUE Go API + `apiextensions-apiserver` for the accept-check.

## Phase 2 — OCI loading (implemented + merged 2026-06-28, PR #5)

`internal/render` now has an `OCILoader` adapter beside `LocalLoader`. The
`ModuleLoader` port returns a `Loaded{Dir, Registry modconfig.Registry, Cleanup}`
value; `LocalLoader` returns `Registry: nil` + no-op cleanup (P1 offline path is
byte-identical), `OCILoader` supplies a registry so the engine wires
`load.Config.Registry` only when present.

De-risk spike findings (now proven in code + tests):
- **Transitive deps** resolve through `load.Config.Registry`. We **inject the
  registry explicitly** rather than relying on CUE auto-creating one from
  `CUE_REGISTRY` when `Registry` is nil — the nil-auto path is process-global and
  races under parallel tests. No `cue.sum` is needed in the consumer fixture;
  programmatic load via the injected registry resolves/verifies deps.
- **Nonroot cache:** CUE honors `CUE_CACHE_DIR` (`internal/cueconfig.CacheDir`).
  Point it (or `OCIConfig.CacheDir`) at a writable non-`$HOME` path for a
  read-only-root / nonroot (uid 65532) runtime. `NewOCILoader` forces
  `CUE_CACHE_DIR` to the resolved dir so both CUE's modcache and the loader's
  extraction cache live there.
- **Digest verify-after-fetch:** refs are **semver, not digests**;
  `modregistry`'s `Module.ManifestDigest()` is verified after fetch against
  `OCIConfig.Expect`. `Expect` checks the **root module ref only** — transitive
  deps are immutable-by-version, so per-dep digest locks are out of scope.
- **CRITICAL (drives the cache design):** CUE's modcache is keyed by
  `module@version`, **NOT content digest**. Republishing different content under
  the same `v0.1.0` yields a different manifest digest, but a version-keyed cache
  would serve stale content. Hence the loader owns a **digest-keyed** cache for
  the **root** module: `<cache>/cuefn-oci/<alg>/<digest>/` (atomic temp+rename) +
  a `ref→digest` pointer file for offline serving. Transitive deps stay on CUE's
  version-keyed cache (versions are immutable, so safe).
- **Error classification:** `modregistry.ErrNotFound` (404 / NAME_UNKNOWN) →
  non-existent-ref error; transport/dial failures → fall back to the cached
  pointer, else an unreachable-registry error. All wrap `%w` and name the ref.
- **Test gotcha:** `t.TempDir()` cleanup fails with EPERM (`unlinkat`) because CUE
  marks extracted dependency files read-only — chmod the tree writable before
  removal (the `cacheDir(t)` helper does this).
- Fixtures are published via the **Go `modregistry` API** (`modzip.CreateFromDir`
  + `PutModule`), no `cue` CLI; tests use a `registry:2.8.3` testcontainer and
  `t.Skip` when Docker is absent. (Removed the spike's `<module>@<version>/`
  prefix strip — `GetZip` returns module-root-relative entries.)

Open follow-ups carried forward (non-blocking):
- **CI does not *assert* the Docker-backed OCI tests ran** — they `t.Skip`
  without Docker, so a green check could one day not exercise OCI. GitHub
  `ubuntu-latest` has Docker preinstalled (they do run today). When hardening CI,
  make it fail-rather-than-skip (assert Docker present for the integration tests).
- Minor untested offline branch: pointer exists but the digest extraction dir is
  missing (`cannot reach registry … and no cached copy`).

## Phase 3 — function + render loop (implemented + merged 2026-06-28, PR #6)

The library is now a Crossplane v2 composition function, kept as a clean adapter
over the untouched `internal/render` core.
- `input/v1beta1`: `Input` type (`Module` semver ref + optional `ExpectedDigest`
  for the lock-step) with controller-gen deepcopy (`controller-gen` pinned via a
  go.mod `tool` directive, wired into moon `generate`/`generate-check`).
- `internal/function`: `RunFunction` adapter — decodes `Input`, builds
  `render.Inputs` from the observed XR (spec via reserved-key projection +
  metadata) and env from `request.GetContextKey(apiextensions.crossplane.io/environment)`,
  calls `Engine.Render`, sets desired composed resources keyed by author names
  verbatim with readiness → proto enum, patches desired composite status, sets a
  `FunctionSuccess` condition, returns a single FATAL `(rsp, nil)` on any
  malformed/unreachable input. `OCILoaderFactory` folds `ExpectedDigest` into
  `OCIConfig.Expect`. All Crossplane proto/req/resp translation lives here, not in
  the core (the core reaches it only through the `ModuleLoader` port).
- `cuefn function` (gRPC serve, mTLS/insecure) + `cuefn render` (cluster-free
  offline eval). `apko.yaml` gained `cmd: function` so the image serves by default.
- `crossplane` CLI pinned in mise (v2.3.3). Example assets: XRD, pipeline
  Composition (`function-environment-configs` → `cuefn`), XR, EnvironmentConfig,
  functions.yaml.
- Proven locally (Docker + crossplane present): the real `crossplane render` loop
  runs end-to-end and asserts an EnvironmentConfig value (`tier: production`)
  overrides the module default — env flows through; the apko image serves gRPC.

### DECISION — Chainsaw is the e2e harness for API-server-facing tests (2026-06-28)

For anything that needs a **real Kubernetes API server**, use **Chainsaw**
(Kyverno's declarative apply/assert e2e tool) rather than hand-rolled client-go.
Rationale: declarative `apply`/`assert` with built-in eventual-consistency
polling, and a Chainsaw CI job **runs-or-fails (no silent `t.Skip`)**.
- **P8 (kind e2e):** primary use — install Configuration + Function, instantiate
  XR, assert reconcile to `Ready`, composed resources, status, env-merge, and the
  digest-drift guard.
- **P4 (server-side XRD checks):** use Chainsaw against a real apiserver (prefer
  **envtest** — lightweight, no kubelet; these are pure apiserver behaviors). The
  generated XRD `openAPIV3Schema` is wrapped as a real **CRD** and applied to the
  apiserver to exercise structural acceptance + defaulting + status round-trip
  (a bare apiserver has no Crossplane controllers, so test the schema via a CRD,
  not by installing the XRD and expecting an XR API to appear). Keep the fast
  `apiextensions-apiserver` `NewStructural`/`ValidateStructural` check as a Go
  unit test alongside it.
- **NOT for:** the render engine, OCI loader, `RunFunction` proto tests, `cuefn
  render`, `crossplane render` loop, or the apko gRPC smoke — those stay
  Go + testcontainers (no API server involved). Optional later nicety: assert
  `crossplane render` output via `crossplane render | chainsaw assert`.
- Confirm the current Chainsaw version + the exact envtest→kubeconfig bridge
  pattern at P4 implementation time before wiring (don't trust memory of the API).
- Pin Chainsaw (+ setup-envtest at P4, kind at P8) in mise; give the e2e suites a
  dedicated moon/CI task.

### Phase 3 follow-ups carried forward (non-blocking)

- **CI-execution-assurance (recurring, now 3 items):** the Go integration tests
  self-skip without their tool — P2 OCI tests skip without Docker; P3 render-loop
  skips if a generic `crossplane` shim shadows the mise binary on `moon ci`'s PATH;
  and the apko image test passes explicit `function` args that override the OCI
  `CMD`, so apko's default `cmd: function` is asserted only manually, not by a test.
  Fix when hardening CI (fold into P8): assert tools present (fail-rather-than-skip)
  and add a no-args image test. (Chainsaw fixes this for the *cluster* suites only.)
- Hexagonal seam: `internal/render` core exposes `resource.Ready` (a
  function-sdk-go type) — spec-permitted, from P1/P2, but an adapter type on the
  core boundary.
- Partial `RunFunction` error-path coverage (input-decode / set-desired failure
  branches not exercised).
- `example/functions.yaml` (committed quickstart) isn't the exact artifact the
  render-loop test validates (the test writes its own with a per-run target/port).

## Phase 4 — schema CLI (implemented + merged 2026-06-28, PR #7)

`internal/schema` turns CUE `#API`/`#Spec`/`#Status` into a structural XRD via the
de-risked recipe, plus `cuefn generate` and `cuefn validate`.
- `openapi.go` (generate with `ExpandReferences:false` + `checkDisjunctions`/
  `walkForOneOf` to reject type-crossing disjunctions naming the field),
  `inline.go` (cycle-detecting `$ref` inliner), `structural.go`
  (`apiextensions-apiserver` `NewStructural`+`ValidateStructural` self-check on
  every generate), `xrd.go` (envelope from `#API`; `.properties.status` emitted
  IFF `#Status`), `validate.go` (unify XR spec vs `#Spec`, `Concrete(true)` so a
  passing validate proves defaults applied), `errors.go` (`DisjunctionError`).
- P1's `engine.go` was refactored to extract a shared **`render.LoadModule`**
  (new `internal/render/load.go`), reused by `internal/schema` — so schema depends
  on render (sibling core packages, no cycle, no edge-adapter leakage).
- **Chainsaw + envtest** debut (the DECISION above): `chainsaw 0.2.15` +
  `setup-envtest 0.20.4` pinned in mise; a build-tagged (`envtest`),
  self-skipping `moon run root:schema-test` task wraps the generated XRD schema as
  a real CRD and applies it to an envtest apiserver. Proven (ran it):
  structural CRD acceptance, API-server **defaulting** (`replicas`→3),
  unknown-field **pruning**, and a **status subresource round-trip**. The
  envtest↔chainsaw bridge: `setup-envtest use 1.35.0 -p path` provides
  KUBEBUILDER_ASSETS; the Go harness boots envtest and points chainsaw at it.

### LESSON — workflow harness crash + recovery (the P4 run)

The first P4 workflow **crashed fatally** after ~48 min: a *direct* `await
agent({schema})` (the Implement agent) hit the StructuredOutput retry cap (5
failed validations) and threw; unlike `parallel()` (which coalesces failures to
null), a direct throw aborts the whole run. The implementation was already
written to the worktree (uncommitted) and survived. Fixes applied to
`phase-build.workflow.js`:
- **Big work agents (understand/plan/implement/fix) now return FREE TEXT** — no
  schema — so a StructuredOutput failure can never abort a run. Schemas remain
  only on small decision-driving agents (verify/critic/check/PR).
- Added a **`startAt:"verify"` finish-mode** to drive an existing worktree through
  verify→fix→PR without re-implementing (used to recover P4 → PR #7).
- General rule for future phases: the deliverable is the code on disk + the small
  verdicts, never a big structured summary.

### CI-EXECUTION-ASSURANCE DEBT (consolidated — address in P8 CI hardening)

Recurring across phases: heavy/tool-gated tests **self-skip silently** when their
tool (or network for asset download) is absent, so a green CI check may never run
the proof. Instances so far: P2 OCI tests (Docker), P3 render-loop (a generic
`crossplane` shim shadowing the mise binary), P3 apko image default-`cmd` (the
test overrides CMD so `cmd: function` is unasserted), P4 `schema-test`
(chainsaw/envtest + envtest-asset download). They are mise-pinned and almost
certainly run on GitHub runners, but it is "probably," not "asserted." **P8 CI
hardening:** make these suites **fail-rather-than-skip** in CI (assert tools
present), add a no-args apko image test, and add an `example/xrd.yaml` drift check
(it is a committed generated artifact with no guard). Chainsaw cluster suites are
already run-or-fail; this debt is the Go testcontainers/build-tagged suites.

## Phase 5 — Configuration packaging + publish (implemented + merged 2026-06-28, PR #8)

`internal/pkg` + `cuefn publish` build and push an installable Crossplane
**Configuration** xpkg from one CUE module, closing the digest lock-step.
- `internal/pkg`: `image.go` (`BuildXpkgImage`), `composition.go`
  (`GenerateComposition`, embeds `input/v1beta1.Input{Module, ExpectedDigest}` as
  the cuefn pipeline step), `meta.go` (`dependsOn` the function), `configuration.go`
  (`BuildConfigurationImage`), `push.go` (`Push` — the only network seam).
- `internal/render/oci.go` gained `OCILoader.ResolveDigest` (live manifest digest,
  no cache fallback) — reused by publish.
- `cuefn publish <module-ref> --package … --function-ref/--function-version/--dir`.
- Proven (gated, ran under mise exec): push→pull byte-identical round-trip;
  `crossplane xpkg extract` accepts the Configuration; the **real resolved digest**
  flows into the Composition input and the P3 runtime accepts it / rejects drift.

### XPKG PACKAGING SPIKE FINDINGS (de-risks P6)

- **`crossplane/crossplane/internal/xpkg` is NON-importable** (under `internal/`).
  The importable escape hatch is **`crossplane-runtime/v2/pkg/xpkg`**, exporting
  `Layer`, `AnnotateLayers`, `ExtractPackageYAML`, and the
  `StreamFile`/`PackageAnnotation`/`AnnotationKey` constants.
- **Validated assembler layout:** `empty.Image` → `xpkg.Layer(stream,
  "package.yaml", base, …)` → `mutate.AppendLayers`/`Config` → `xpkg.AnnotateLayers`
  (`io.crossplane.xpkg=base`) → `remote.Write`. We own the YAML bytes, so we skip
  the heavier `xpkg.Builder`/`parser.Backend` plumbing.
- **`crossplane xpkg inspect`/`validate` DO NOT EXIST in crossplane 2.3.3** — only
  `build/extract/batch/init/install/push`. Use **`crossplane xpkg extract`** as the
  "crossplane accepts it" check (it parses the full package stream). Note this
  deviation from the spec's "inspect" wording.
- A prototype **Function** xpkg passes the same path — but only over `empty.Image`,
  NOT a real runtime base. The embed-runtime (non-empty base) path of
  `BuildXpkgImage` is implemented + documented (internal/pkg/doc.go) but **first
  exercised for real in P6**.

### Phase 5 follow-ups carried into P6 / later (non-blocking)

- **DEP-BLOAT (P6 decision):** the importable `crossplane-runtime/.../xpkg` drags
  sigstore/cosign + pkcs7 + cloud-credential SDKs into the **same `cuefn` binary
  that serves as the function runtime** (cmd/cuefn imports internal/pkg via
  `publish`). This will inflate the P6 function runtime image even though signing
  is out of scope for the runtime. **P6 must decide:** exclude `publish` from the
  image binary via a build tag (keep the runtime lean) vs. accept the measured size.
- **`--dir` publish footgun:** XRD/Composition are generated from the LOCAL module
  dir while `ExpectedDigest` is resolved from the REGISTRY — unpublished local edits
  can ship an inconsistent package. Documented, not guarded.
- **`dependsOn[].function`** uses the deprecated typed field; P8 must confirm the
  cluster package manager still resolves the function dependency via this form.
- crossplane-CLI validation here is annotation/parse-level (kinds present via
  `extract`), not schema-level; full structural acceptance is P4 envtest + P8 install.

## Phase 6 — Function xpkg + release wiring + CI-hardening (merged 2026-06-28, PR #9)

Ships the function as a signed Crossplane **Function** xpkg.
- `internal/pkg/function.go`: `BuildFunctionImage` — first real use of the
  embed-runtime (non-empty base) path, appending the `package.yaml` layer over the
  apko `crossplane-cuefn:dev` image while preserving its entrypoint/cmd. `meta.go`
  `GenerateFunctionMeta` (kind Function) + the embedded Input CRD
  (`internal/pkg/inputcrd/cuefn.meigma.io_inputs.yaml`, controller-gen from
  `input/v1beta1`). `push.go` `PushIndex` (multi-arch). `cuefn publish-function`.
- `release.yml` (`function-package-release`: multi-arch index, keyless cosign,
  syft SBOM + attest-sbom; `attest-function-package` → SLSA L3), `release-dry-run.yml`,
  `security-scan.yml` wired for the function xpkg. Proven: `crossplane xpkg extract`
  accepts it; cosign sign+verify; the packaged image serves gRPC.
- **DEP-BLOAT resolved:** `publish`/`publish-function` (and `internal/pkg` →
  sigstore/cosign + cloud-cred SDKs) sit behind a `//go:build !noxpkg` seam
  (`internal/cli/packaging.go` / `packaging_noxpkg.go`); `melange.yaml` builds the
  **image binary with `-tags=noxpkg`** → **−12.1 MiB / −23%** (53.5→41.3 MiB
  stripped). A `build-image-binary` moon task (in `check`) keeps the lean path green.

### CI-EXECUTION-ASSURANCE DEBT — RESOLVED (was tracked P2→P5)

`ci` had been **red on master since Phase 2**: the GitHub `ci` job ran `moon ci`
(all tasks, incl. the heavy Docker/crossplane suites), which were flaky/failing on
the runner. It slipped through because the per-phase gate was local
`moon run root:check` (excludes the heavy suites) + `MERGEABLE`, and `ci` is **not
a required check**. **LESSON: verify the real `ci` Action at every gate, not just
local `moon run root:check`.** The fix (folded into P6):
- Heavy tests **env-gate on `CUEFN_INTEGRATION`** (set only by the dedicated moon
  tasks) via the four `require*` helpers (`requireDocker`/`requireCrossplane`/
  `requireBinary`/`requireDevImage`) — so plain `go test ./...` skips them (no
  build-tag file-splitting needed; fast error-path tests stay in the gate).
- The blocking `ci` job runs **`moon run root:check`** (NOT `moon ci`); the heavy
  tasks are not `check` deps, so they're excluded. (`runInCI: false` was rejected:
  it also makes a task un-runnable via `moon run` when `CI=true`.)
- New **non-blocking `.github/workflows/integration.yml`** runs all five heavy
  suites (oci/schema/publish/funcpkg/render-loop) visibly on PRs + master.
- Three real CI-env bugs fixed along the way: (1) `image-local` mise task used
  `set -o pipefail` under dash → pin `shell = "bash -c"`; (2) the `moon run`/`CI`
  conflict above; (3) the render-loop `crossplane render` step — bump `--timeout`
  to 10m (cold env-configs pull) AND **bind the function server on `0.0.0.0`** (not
  `127.0.0.1`) so crossplane's function container reaches it via the Linux Docker
  bridge gateway (172.17.0.1).
- **Result:** `ci` green (~1m) + `integration` green (~4m, all five heavy suites
  genuinely run + pass in CI).

## Phase 7 — documentation (merged 2026-06-28, PR #10)

Diátaxis docs set under `docs/docs/` + mkdocs nav: quickstart (tutorial), how-to
×8, reference ×4 (module-contract, cli, configuration, input), explanation ×4
(one-module-two-outputs, digest-lockstep, reserved-key-projection, noxpkg-split).
`cue` CLI pinned in mise (0.16.1) for the documented `cue mod publish` step. Hard
gate `docs:build --strict` (in `root:check`). CLI reference = exactly the six
shipped commands (function/render/generate/validate/publish/publish-function;
`noxpkg` drops the last two). Folded in two fixes at the gate: documented the
`:8080` metrics endpoint, and fixed a `--from-daemon` quickstart bug (pull before
extract).

### `cuefn function` metrics on :8080 (finding from a leaked process)

`cuefn function` calls `sdk.Serve` (function-sdk-go), which **also serves
Prometheus metrics on a fixed `:8080`** alongside the gRPC `--address` — there is
no flag. A leaked test/verifier `cuefn function` collided with the user's app on
:8080. Documented in `reference/cli.md`. **Open:** a `--metrics-address`
flag/disable — feasibility depends on whether `sdk.Serve` exposes a metrics option
(investigate in P8; if function-sdk-go hardcodes it, document the limitation).

### Open follow-ups → Phase 8 (the finale ties these up)

- Kind e2e + CI e2e (the P8 core): install Configuration + Function, instantiate
  XR, assert reconcile/Ready/status/defaulting/env-merge/**digest-drift guard** via
  **Chainsaw** (the recorded decision). Crossplane pkg manager is **HTTPS-only** —
  the e2e needs the Function/Configuration packages reachable over a registry the
  in-cluster pkg manager accepts.
- No-args apko image test (assert the default `cmd: function` serves, not just the
  arg-override path).
- `example/xrd.yaml` drift check (committed generated artifact, currently unguarded).
- The `--metrics-address` flag (above), if feasible.

## Phase 8 — kind e2e + finale (merged 2026-06-28, PR #11) — PLAN COMPLETE

The full author→publish→install→instantiate→reconcile loop, proven on a real kind
cluster via **Chainsaw**, green in CI on ubuntu (`e2e` workflow, ~4.5m). `internal/e2e/`
(build tag `e2e`, gated) + `test/chainsaw/e2e/` + `.github/workflows/e2e.yml`
(non-blocking but always-running).
- **Registry crux solved (dual registry):** (A) an **HTTPS, CA-trusted** `registry:2`
  on the kind network for the Crossplane xpkgs — a self-signed CA minted in-process,
  wired into Crossplane via helm `registryCaBundleConfig` AND into each node's
  containerd (`/etc/containerd/certs.d/.../hosts.toml`); pushes go to localhost over a
  CA-trusting transport. (B) a plain-HTTP `+insecure` registry for the CUE modules the
  function fetches at render (`CUE_REGISTRY=...+insecure`).
- **Two non-obvious gotchas:** Crossplane's CEL package-ref validation **rejects a
  dotless registry host** (use e.g. `cuefn-e2e-registry.test`); avoid host-publishing
  `:5000` (macOS Control Center).
- Chainsaw asserts on the live cluster: XR `Ready=True` + composed Deployment/Service/
  ConfigMap; status from `#Status`; API-server defaulting of an omitted field;
  EnvironmentConfig `tier: production` on composed resources; and the **digest-drift
  guard** → `Synced=False` after republishing different content under the same version.

### Real bugs the e2e surfaced + fixed (carry forward as facts)

- **`pkg.GenerateComposition` emitted the `function-environment-configs` step with NO
  input** → generated Configurations merged no EnvironmentConfig (`tier` stayed
  `unset`). Fixed: optional `CompositionInput.EnvironmentConfigRefs` + a `cuefn publish
  --environment-config` flag. (Configs published before this fix would not merge env.)
- **`cuefn function` ignored Crossplane's injected mTLS certs dir** → wired the standard
  `TLS_SERVER_CERTS_DIR` default so the packaged Function serves mTLS in-cluster.

### Cleanups done

- `--metrics-address` flag (default `:8080`, empty disables) wired to real
  `sdk.WithMetricsServer` (function-sdk-go v0.7.1 exposes it); enable/disable tests.
- No-args apko image test (`TestImageServesFunction_NoArgs` — default `cmd: function`).
- `root:xrd-check` drift guard (in the `check` gate): `cuefn generate` vs committed
  `example/xrd.yaml`.
- Pinned `kind 0.31.0`, `kubectl 1.35.3`, `helm 3.18.6` in mise.

### Remaining minor follow-ups (optional, non-blocking)

- `example/xrd.yaml` lost its "generated" header (collateral of the exact-match
  xrd-check; cuefn generate emits none). Could have `cuefn generate` emit a header or
  make xrd-check comment-tolerant.
- The e2e job has no explicit fail-if-skipped guard (it demonstrably runs — 4.5m in CI).
- `ci` mise-setup installs ALL pinned tools, so a transient download 403 of any tool
  (hit crossplane.io once) fails the fast gate; could scope per-job tool installs.
- e2e loop calls `internal/pkg` library funcs directly (CA-trusting push), not the
  `cuefn publish` CLI binary — same code path, but the CLI binary isn't in the live loop.

## Test layout — integration/E2E under `internal/test` (session 003, PRs #12 + #13)

Org standard, now enforced here: **all infra-dependent tests live OUTSIDE the package they
exercise, under `internal/test`** (black-box, real exported API). Co-locating integration/E2E
tests in their home package is no longer allowed. Genuine UNIT tests (no infra) STAY in their
home package.

- **`internal/test/common`** (`package common`, non-`_test.go`, imports `testing` on purpose):
  the ONE home for shared test infra. `Registry` (testcontainers OCI registry: `StartRegistry`,
  `Host`/`CUERegistry`/`Env`/`Publish`/`ManifestDigest`/`Stop`, + free `PublishModule`/
  `ManifestDigestAt` for the e2e plain-HTTP registry); gates `RequireDocker`/`RequireCrossplane`
  (uses the `crossplane render --help` shim-probe, NOT bare `LookPath`)/`RequireBinary`/
  `RequireDevImage`; `RepoRoot`/`FreePort`/`CacheDir`; serve helpers `BuildBinary`/`ServeFunction`/
  `WaitForFunction`/`WriteFunctions`; runtime bases `FakeRuntimeBase`/`WriteRuntimeBaseTarball`/
  `RuntimeBaseImage`; kinds parsing `StreamKinds`/`ExtractKinds`/`SplitStream`; typed fixtures
  `FixtureXRD`/`BuildFixtureConfiguration`/`FixtureFunction`/`StepName`; render accessors
  `Object`/`ToInt`; consts `ExampleModuleRef`/`DevImage`/`RegistryImage`/`Zeros`. It imports ONLY
  `internal/pkg` + `internal/render` (no cycle); never `internal/cli`/`function`/`schema`/`test/e2e`.
- **`internal/test/integration`** (`package integration_test`, external): all `CUEFN_INTEGRATION`-
  gated integration tests (17). Files: `oci`, `renderloop`, `image`, `funcpkg`, `push`,
  `publish` (`//go:build !noxpkg`), `publish_function` (`//go:build !noxpkg`), `schema_chainsaw`
  (`//go:build envtest`).
- **`internal/test/e2e`** (`package e2e`, `//go:build e2e`): the kind harness — `e2e_test.go` +
  `cluster.go` + `registry.go` + `doc.go` + `testdata/module`. `TestE2E_Kind`.

Conventions: asset paths use `common.RepoRoot(t)` (NOT `../..` relatives). testdata stays in its
source package (`internal/render/testdata/oci`, `internal/schema/testdata/derisked`) except the
e2e fixture which travels in `internal/test/e2e/testdata`. moon gated tasks
(`oci`/`render`/`publish`/`funcpkg`/`schema`/`e2e-test`) target `internal/test/{integration,e2e}`;
their `-run` regexes **partition** the suite (no test run twice, no dead alternation). Unit tests
run in the BLOCKING `check` gate via `root:test`.

Consolidation (#13): merged the round-trip/validate/supply-chain duplicates into table/subtest
form and dropped library round-trips the CLI E2Es already cover (digest-stability + real-base
moved into the CLI E2Es). Integration tests 23→17, no assertion lost.

**GOTCHA (caused a 61 MiB binary in #12, fixed in #13):** never `git add -A` after a bare
`go build ./cmd/cuefn` — it writes `./cuefn` to the repo root. `/cuefn` is now gitignored.


## Dependency-aware loading + module-contract v2 + the contract module (session 004, PRs #14–#29)

### Dependency-aware local loading (#14, #15)
`render.NewLocalLoader(dir, cfg)` attaches a `modconfig.Registry` so a local module
that imports an OCI dep (e.g. `cue.dev/x/k8s.io`) resolves at load; the zero-value
`render.LocalLoader{Dir}` stays offline (nil registry) for hermetic tests. Both go
through `buildRegistry` (extracted from `NewOCILoader`). `load.go` already wires a
non-nil `Loaded.Registry`. **Central is the default for free:** `modconfig`'s
`DefaultRegistry = registry.cue.works` is the catch-all when `CUE_REGISTRY` is unset
OR prefix-scoped; only a bare value replaces it (the deliberate offline/air-gap
override the tests use). CLI: `moduleLoader(dir, cacheDir)` + a `--cache-dir` flag on
render/generate/validate/publish.

### The k8s example + test decoupling (#16, #17, #18)
`example/module` instantiates its objects from `cue.dev/x/k8s.io/api/{apps/v1,core/v1}`
(`apps/v1.#Deployment` pins apiVersion/kind, no surprise required fields). The tests
are **decoupled from the example**: the hermetic self-contained fixture is
`internal/test/common/testdata/module` (loaded via `common.HermeticModuleDir(t)`); the
example is validated only by the blocking `xrd-check` (XRD drift) + `example-check`
(render smoke), never woven into unit/integration tests. CI warms a CUE module cache
(`.github/workflows/ci.yml`, keyed on `example/module/cue.mod/module.cue`).

### Module-contract v2 — the `out` root (#19)
The transform nests under a single top-level **`out`** field: the engine fills
`out.input` and reads `out.resources`/`out.status` (4 `cue.ParsePath` literals in
`internal/render/engine.go`). `#API`/`#Spec`/`#Status` stay **top-level definitions**.
Codegen is UNCHANGED — `internal/schema/openapi.go` `definitionsOnly` drops the regular
`out` field exactly like it dropped the old transform fields, so the generated XRD is
byte-identical. A pre-v2 module (no `out`) gets a clear engine error.

### The contract module (#20, #21, #28, #29)
`contract/` is a published CUE module **`github.com/meigma/crossplane-cuefn/contract@v0`**
(on the CUE Central Registry; resolves with zero `CUE_REGISTRY` config since central is
default). It exposes **closed** `#API`, `#Resource` (`{object, ready?}`), `#Input`
(`out.input`), `#Transform` (the closed `out` wrapper). Authors import it and unify:
`#API: contract.#API & {…}`, `out: contract.#Transform & {…}` → `cue vet` rejects a
misspelled/unknown field (e.g. `out.resorces: field not allowed`). **GUARDRAIL:**
`#Spec`/`#Status` are NOT wrapped — they feed the XRD codegen and stay the author's
import-free schemas. Enforcement is **author-time only** (the engine does NOT embed the
contract; the published module is the single source of truth). `internal/contract` is
the offline closedness regression test. The example is the reference adoption (and the
live drift canary — it imports the contract AND is rendered by the function).

**Versioning policy:** the contract's major is welded to the function's major (both
`v0`), enforced by `bump-minor-pre-major`. Within `v0`: fix→patch, feature→minor;
breaking stays in `0.x`; `v1` only as a deliberate coordinated bump when the function
goes `v1`. Authors pin `@v0` (the compatibility anchor).

### Release automation — release-please monorepo + OIDC (#21, #24–#27)
Two independently-versioned components: the **product** (`.`, tag `v*` — the `cuefn`
binary distributed as binaries + image + Function xpkg, one version) and the
**contract** (`contract`, tag `contract/v*`, release-type `simple`, `bump-minor-pre-major`
so it stays on `v0`). `separate-pull-requests: true`; `exclude-paths: ["contract"]` on
root so contract-only commits never bump the product. **Publishing is headless via
GitHub OIDC** — `.github/workflows/release-contract.yml` uses
`cue-labs/registry-login-action` (no stored secret; one-time trust at
`registry.cue.works/account/oidc`) + `cue mod publish` from `contract/`.
- **release GitHub App:** `MEIGMA_RELEASE_APP_ID` (var) + `MEIGMA_RELEASE_APP_PRIVATE_KEY`
  (secret) are configured (they were MISSING before — release-please had never run).
- **First-release gotcha:** a never-released component's first release defaults to
  `1.0.0` (overriding `bump-minor-pre-major`). Use `release-as: "0.1.0"` to bootstrap,
  then remove it. CUE ignores git tags — `cue mod publish vX.Y.Z` just uploads the
  committed `contract/` content (major must match `@v0`).
- Non-bumping commit types (`ci`/`docs`/`test`/`chore`/`build`) cut no release.

## Required resources — a module requests cluster data (session 005, PRs #31–#39, product v0.1.1)

A CUE module can request additional cluster objects at render time — Crossplane's
"extra resources" (renamed **required resources** in v2). The module **emits**
selectors under `out.requirements`; Crossplane fetches them and re-invokes; the
fetched objects arrive under `out.input.requiredResources` keyed by the same
author-chosen name. The function is a **pure** function of (observed + delivered +
input) → (desired + requirements); **Crossplane owns the fetch/re-invoke fixpoint**
(`internal/xfn` `FetchingFunctionRunner`, `MaxRequirementsIterations=5`, stop when
`proto.Equal` of two consecutive `Requirements`; non-final desired is discarded;
missing → key-present-empty). Use the CURRENT wire field `Requirements.Resources`
/ `request.GetRequiredResources`, NOT deprecated `extra_resources`.

- **Contract (`contract@v0.2.0`, published):** added optional `#Transform.requirements?`
  (`[string]: #Requirement`) and `#Input.requiredResources?` (`[string]: [...#Required]`);
  `#Requirement{apiVersion, kind, matchName?, matchLabels?, namespace?}`; `#Required`
  open. Backward-compatible minor bump. "Exactly one of matchName/matchLabels" is NOT
  enforced by the closed contract (the disjunction-in-closed-struct form is unverified,
  deferred) — the **engine** enforces it at render time.
- **Engine SEED is load-bearing (`internal/render/engine.go`):** `Render` reads
  `out.requirements` (`readRequirements`: nil-when-absent, concrete, exactly-one) →
  `seedRequiredResources` (one non-nil empty `[]` per declared requirement name, only
  if absent) → re-fill `out.input` → read `out.resources`. The seed is what keeps a
  data-dependent guard concrete on the first pass — an absent/empty-key
  `requiredResources` is a HARD CUE error ("undefined field"), NOT "incomplete", so an
  optional contract field alone does NOT save it. `cue.Concrete(true)` is never relaxed.
  Author idiom: guard data-dependent fields with `for i,x in input.requiredResources.<name>`
  or `if len(...)>0`, and keep `out.requirements` a **pure function of stable inputs**
  (spec/metadata) — referencing `requiredResources` inside `out.requirements` errors
  before the seed runs unless the author defaults the field.
- **Function edge (`internal/function/function.go`):** `requiredToInputs` (request →
  inputs, empty-but-present preserved) + `setRequirements` (Result → `rsp.Requirements.Resources`,
  proto built by hand — v0.7.1 has NO setter). Emits on every successful call (fixpoint);
  `HasCapability(...CAPABILITY_REQUIRED_RESOURCES)` gates only a non-fatal Warning.
- **CLI:** `cuefn render --required-resources <file|dir>` does a fixed two-pass
  (render → `matchRequirements` → render) + `reflect.DeepEqual` stabilization error;
  prints emitted `requirements`. `loadRequiredObjects` uses the k8s multi-doc YAML reader
  (`k8s.io/apimachinery/.../util/yaml`) so a `---` inside a value is not mis-split.
- **XRD codegen UNAFFECTED** (runtime-only); the step `input/v1beta1.Input` is UNCHANGED.
- **RBAC (operational):** required-resource reads go through Crossplane's **core
  controller ServiceAccount**, not the function pod → operators grant a ClusterRole
  labeled `rbac.crossplane.io/aggregate-to-crossplane: "true"` for each requested kind.
  The kind e2e (`test/chainsaw/e2e/required-resources.yaml`) proves the full loop with
  this grant. Cross-namespace reads are intentional, RBAC-governed upstream behavior
  (not a cuefn boundary); scope a read to the XR's namespace with
  `namespace: input.metadata.namespace`.
- **Enforcement stays author-time-only at the contract layer** (the engine does not
  embed the contract). Carried follow-up: a function-side runtime check of a module's
  imported contract major — only matters when the function/contract reach `v1`.

## Session 006 — DX hardening (product v0.1.2, PRs #40–#46)

A 6-persona consumer-impersonation DX sweep (`.journal/006/DX-REPORT.md`) found the
documented install path was broken end-to-end. The fixes below changed durable
behavior — read before touching publish/install, the cache, or codegen.

- **Configuration install is single-apply now (#44).** Crossplane installs a
  `dependsOn` Function under a DERIVED name = `xpkg.ToDNSLabel(name.ParseReference(ref)
  .Context().RepositoryStr())` — host-stripped, DNS-labelized, **no hash**
  (`ghcr.io/meigma/function-cuefn` → `meigma-function-cuefn`). `pkg.DerivedFunctionName`
  (`internal/pkg/names.go`) computes it; `cuefn publish` defaults the Composition's
  `functionRef.name` to it, so ONE `kubectl apply` of the Configuration resolves the
  step with no hand-installed Function. **Never hand-install a Function on the same
  package source** — the package Lock dedups by **source**, so a duplicate bricks every
  package ("node … already exists"). `--function-name` still overrides. Proven on a
  live cluster (the spike); `TestDerivedFunctionName` pins the mappings.
- **`function-environment-configs` is conditional (#44).** Emitted only with
  `cuefn publish --environment-config <name>` — then with a selector, its own derived
  functionRef name, AND a 2nd `dependsOn` entry to
  `xpkg.crossplane.io/crossplane-contrib/function-environment-configs`
  (`--environment-config-function-ref/-version` flags, default `>=v0.7.2`). The default
  Configuration is a single cuefn step. (Was always-emitted + never in dependsOn →
  first reconcile failed "cannot find an active FunctionRevision".)
- **Function renders with no DRC (#40).** `render.resolveCacheDir` probes the OS user
  cache via MkdirAll and falls back to `<tmp>/cuefn-cache` when uncreatable (the nonroot
  read-only-root `/.cache` case). Precedence: explicit `--cache-dir` > `CUE_CACHE_DIR` >
  OS cache > temp. Only a hardened `readOnlyRootFilesystem` deployment still needs
  `CUE_CACHE_DIR` + an `emptyDir`.
- **XRD codegen emits defaults for required-defaultable fields (#41).**
  `internal/schema/materializeDefaults` (in `GenerateXRD`, after `$ref` inline, before
  `selfCheck`) sets `{}`/`[]` defaults on required, empty-satisfiable fields (a
  fully-defaultable struct, a keyless map, a zero-min list) so the apiserver accepts
  `spec: {}` exactly as CUE does — restoring the no-drift guarantee. Genuinely-required
  (no-default scalar, `minItems>0`) is left required. Example/derisked XRDs are
  byte-identical (no required containers there).
- **CUE error formatting (#45):** `internal/cueerr.Wrap` walks CUE's structured
  `errors.Errors` (not the rendered tree), drops the "empty disjunction" wrapper + the
  misleading default-branch "conflicting values", dedupes, preserves `Unwrap`. Replaced
  the duplicated `wrapCUE` in `internal/render` (engine.go, load.go) + `internal/schema`.
- **Author + ops docs (#46):** the correct author check is **`cue vet -c=false ./...`**
  (bare `cue vet` fails on a required-no-default field). New how-to
  `docs/docs/how-to/configure-the-runtime.md` — in-cluster `DeploymentRuntimeConfig`
  (`CUE_REGISTRY` PREFIX form keeps central as fallback; container name
  `package-runtime`; bound via `runtimeConfigRef`) + the write-side RBAC for composed
  native kinds (aggregate-to-crossplane). **B1:** committed function refs are pinned and
  auto-bumped — `example/{functions,deploy/functions}.yaml` carry
  `# x-release-please-version` and are in the root component's `extra-files` (proven:
  auto-bumped to `function-cuefn:v0.1.2`).
- **No moving `:v0`/`:latest` tag by design** — refs are pinned + release-please-bumped.
  Deferred (not bugs): M1 per-Input registry routing; M3 render `--strict` #Spec guard;
  L3 "incomplete value" wording; `CUEFN_*` env not wired (only `CUE_*` work);
  `additionalProperties:false` is deliberately prune-not-reject; the
  `example/deploy/functions.yaml` self-host Function name still `cuefn` (mismatches the
  derived `function-cuefn` for that path).

## Session 007 — README revamp + CLI distribution (product v0.1.3)

The README is now a **landing page** (#47): value prop + a verified minimal CUE
example + a command table + a docs hand-off; dev/toolchain/supply-chain content
lives in CONTRIBUTING.md. The docs `install-the-cli.md` how-to is the canonical
install reference.

### Release artifacts (post-#48)
`.goreleaser.yaml` builds **darwin/linux/windows** (windows amd64 only; arm64
ignored) and archives as **tar.gz (unix) + zip (windows)** named
`cuefn_<version>_<os>_<arch>.<ext>`, each bundling `LICENSE-*` + `README.md`.
Per-archive SBOMs; `checksums.txt` covers the archives (so the SLSA attestation
subjects are what users download). **`ghd` was removed** — `ghd.toml` + the staging
script are gone; `release.yml` uploads GoReleaser's Archive/SBOM/Checksum artifacts
straight from `dist/artifacts.json`. Binaries are authenticated by **SLSA/GitHub
attestations via the isolated `attest.yml`** — NOT cosign (no `signs:` block; adding
one would regress the L3 signing-token isolation).

### Six install methods
`brew install meigma/tap/cuefn` (formula, mac+linux) · `scoop install meigma/cuefn`
(windows) · `mise use -g "github:meigma/crossplane-cuefn[bin=cuefn]"` (native
`github:` backend — zero registry, verifies attestations+SLSA by default; **aqua was
deliberately dropped**) · `nix profile install github:meigma/crossplane-cuefn`
(in-repo `flake.nix`, `buildGoModule`, source build → immune to draft-first;
`vendorHash` tracks go.sum; version via release-please extra-files; `nix.yml` guards
staleness) · `curl -fsSL …/install.sh | bash` (verified) · `go install …@latest`.

### Taps: templated from checksums, pushed post-publish (CRITICAL)
Homebrew/Scoop are populated by **`.github/workflows/release-distribute.yml`** on the
**`release: released`** event (post-publish, so asset URLs resolve). It does NOT use
GoReleaser's brews/scoops (removed) and does NOT rebuild: it downloads the published
`checksums.txt` and templates the formula + manifest from those exact hashes via
`.github/scripts/render_tap_manifests.py`, then `push_tap.sh` git-pushes to the taps
(tokens `HOMEBREW_TAP_TOKEN`/`SCOOP_BUCKET_TOKEN`, set from 1Password `Homelab`
"Meigma scoop/tap token"). **Why:** a GoReleaser rebuild is NOT byte-reproducible
across separate CI runs (proven: same v2.16.0 + same commit, different archive bytes
— LICENSE/README mtimes), so rebuilt hashes did not match the uploaded archives and
`brew install` failed with a checksum mismatch. To fix a bad tap entry for an existing
tag, re-run `release-distribute` via `workflow_dispatch tag=vX.Y.Z`.

### Gotchas future agents must know
- **`install.sh` latest resolution** uses the GitHub API releases list and greps the
  newest product `"tag_name":"vX.Y.Z"` — it does NOT use the `/releases/latest` web
  redirect (which tracks most-recently-*published* and can surface a `contract/*`
  draft during churn). The repo has BOTH `v*` (product) and `contract/v*` releases.
- **`go install` version stamping**: `cmd/cuefn/main.go` `resolveBuildInfo()` reads
  `debug.ReadBuildInfo()` when version=="dev" (ldflags unset). `go install …@vX.Y.Z`
  → module version; `go build` from a checkout → vcs.revision/time. Release builds
  (GoReleaser/Nix/melange set version via ldflags) skip the fallback. NOTE: a tag cut
  before this fix (e.g. v0.1.3) still reports `dev` on `go install @that-tag`.
- **The release dry-run** (`release-dry-run.yml`) rehearses tap rendering: it runs
  `render_tap_manifests.py` against the dry-run `checksums.txt` and asserts every
  rendered hash is present in `checksums.txt` (the guard that would have caught the
  #49 bug). `.github/scripts/` is in its change-detect filter.
- **Post-release sanity check is worth it**: passing mise/nix/install.sh-with-version
  did NOT catch the broken brew/scoop hashes or the wrong "latest" — only running the
  real `brew install` / default `install.sh` did.
