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

