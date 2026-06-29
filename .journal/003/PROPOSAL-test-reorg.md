---
status: TEMPORARY working doc — delete once the reorg lands (promote any durable bits into TECH_NOTES)
session: 003
date: 2026-06-28
subject: Relocate integration/E2E tests into internal/test/{integration,e2e,common}; dedupe helpers + tests
inputs: session-003 initial test survey + assess-test-reorg workflow (run wf_a5a87d1f-5e8, 5 agents)
---

# Proposal: Conform the integration/E2E test suite to the standard layout

> **This is a temporary planning doc**, in the spirit of session 001's `DESIGN.md`/`PLAN.md`.
> It captures the assessment + recommended remedy so we can execute against it. Delete it
> once the reorg is merged; fold any durable architecture facts into `TECH_NOTES.md`.

## Goal

Bring crossplane-cuefn's tests in line with the org standard used by our other repos:

- **All integration tests** live under `internal/test/integration`.
- **All E2E tests** live under `internal/test/e2e`.
- **Shared test infrastructure** (testcontainer setup, gates, path helpers, fixtures) lives in
  `internal/test/common`.

Two principles motivate the standard:

1. **Surveyability / de-duplication** — one place to read the whole integration surface.
2. **Real API boundary** — tests live *outside* the package they exercise, so they can only
   touch the exported API (black-box), which is what a real consumer sees.

While doing the move we also: (1) consolidate duplicated test helpers, and (2) consolidate
mostly-duplicate tests into single tests that preserve coverage.

---

## 1. Findings — both principles are currently broken

### Principle 1 (location): violated everywhere
Every integration/E2E test is co-located in the package it tests, scattered across six packages:
`internal/render`, `internal/function`, `internal/pkg`, `internal/cli`, `internal/schema`,
`internal/e2e`. Because they're scattered, the same infra is re-implemented per package and
duplicate tests went unnoticed.

### Principle 2 (boundary): one structural violation + several mechanical ones
- `render_test`, `function_test`, `pkg_test`, `schema_test` are already **external** test
  packages (good — already black-box).
- **`internal/cli` is white-box**: `publish_test.go`, `publish_function_test.go`,
  `publish_helpers_test.go` declare `package cli`. They are the only suite that *can* reach
  internals — and the only unexported production symbol any test actually depends on is
  `publishFunctionUse = "publish-function"` (`internal/cli/publish_function.go:32`), used at
  `publish_function_test.go:60,99,130,157,164`. Its sibling `publish_test.go` already uses the
  **string literal** `"publish"` (`:43,93,112`). **Verified.** → Substitute the literal
  `"publish-function"`; **nothing needs exporting.** The driver seam is already the exported
  `cli.NewRootCommand(cli.Options{…})` + `ExecuteContext`, exactly as `cmd/cuefn/main.go` uses it.

### Secondary evidence — helper duplication (verified)
- `requireDocker` ×4 (byte-identical): `render/ocifixtures_test.go:52`, `function/testregistry_test.go:28`,
  `pkg/testregistry_test.go:22`, `cli/publish_helpers_test.go:26`.
- `const registryImage = "registry:2.8.3"` ×5 (those four + `e2e/registry.go:27`).
- `testRegistry` type + `startRegistry` ×4; `publishModule` ×3 methods + 1 e2e free func.
- `manifestDigest` ×2 (`render:137`, `cli:86`) + e2e `resolveDigest`.
- `zeros` ×2 (`pkg/composition_test.go:143`, `cli/publish_test.go:174`).
- `freePort` ×2 — **and divergent**: `function/renderloop_test.go:49` keeps the listener open;
  `cli/function_test.go:19` closes then returns the port. **Verified.**
- `requireCrossplane` ×2 — **divergent**: `function/renderloop_test.go:33` probes
  `crossplane render --help` to defeat the moon proto PATH shim; `pkg/testregistry_test.go:33`
  is a bare `LookPath` (weaker — can silently run against a fake `crossplane`).

### Secondary evidence — fixture coupling across unit/integration (verified)
The integration suite in `internal/pkg/push_test.go` consumes fixtures defined in **unit**
`_test.go` files of the same package: `buildFixtureConfiguration` (`configuration_test.go:16`),
`fixtureFunction` + `fakeRuntimeBase` (`function_test.go:19,34`), transitively `fixtureXRD` /
`splitStream` / `stepName` / `zeros`. Likewise `oci_test.go` uses `object`/`toInt` defined in the
**unit** `engine_test.go:29,38` (which stays in `internal/render`). Moving the integration tests
to another package severs these (Go test helpers are visible only to their own test binary), so
the shared fixtures must be **promoted to an importable package** or the integration tests won't
compile. **Verified.**

---

## 2. Recommended target layout

**One flat `internal/test/integration` package, one `internal/test/e2e` package, one importable
`internal/test/common` support package.** Reject per-area sub-packages (`integration/render`,
`integration/pkg`, …).

Rationale:
- **No Test-name collisions** — the full set of `Test*` names is already unique, so a flat package
  is fine. The real collision risk was the duplicated helpers; hoisting them into `common` removes
  it entirely.
- **Per-test registries stay** — `TestOCI_ServesFromCacheWhenRegistryDown` stops the registry
  mid-test and `TestOCI_RepublishedTagRefetched` mutates a tag, so a shared package-level container
  would cause interference. testcontainers uses random host ports, so per-test registries are
  parallel-safe. A flat package co-locates without forcing sharing.
- **Build tags are file-level** — `schema_chainsaw_test.go` keeps `//go:build envtest`; the two
  migrated publish files keep/gain `//go:build !noxpkg`; the e2e harness lives in a *separate*
  directory because it ships non-test `.go` files (`cluster.go`, `registry.go`) that also need the
  `e2e` tag.
- **moon `-run` regexes survive Phase 1 verbatim** — each task's package path collapses to a single
  `./internal/test/integration/...` (e2e → `./internal/test/e2e/...`) and every test name is
  preserved, so no regex churn during the move.

```
internal/test/
├── common/                       # package common — importable, non-_test.go (imports testing)
│   ├── registry.go               # Registry{}: StartRegistry, Host/CUERegistry/Env/Publish/ManifestDigest/Stop; const RegistryImage
│   ├── gates.go                  # RequireDocker, RequireCrossplane (shim-probe variant), RequireBinary, RequireDevImage
│   ├── paths.go                  # RepoRoot, FreePort, CacheDir
│   ├── serve.go                  # BuildBinary, ServeFunction, WaitForFunction, WriteFunctions
│   ├── runtime.go                # FakeRuntimeBase, WriteRuntimeBaseTarball, RuntimeBaseImage
│   ├── kinds.go                  # StreamKinds, ExtractKinds, PackageYAMLBytes, SplitStream/YAMLDoc
│   ├── fixtures.go               # FixtureXRD, BuildFixtureConfiguration, FixtureFunction, StepName  (see Q4)
│   ├── render.go                 # Object, ToInt  (shared by staying engine_test.go AND migrating oci_test.go)
│   └── consts.go                 # ExampleModuleRef, DevImage, Zeros
├── integration/                  # package integration_test
│   ├── oci_test.go               # TestOCI_*  (+ newOCIEngine, containerSpec, variant as unexported locals)
│   ├── renderloop_test.go        # TestRenderLoop_CrossplaneRender
│   ├── image_test.go             # TestImageServesFunction_NoArgs (+ TestImageServesFunction unless folded — C2)
│   ├── funcpkg_test.go           # TestFunctionPackageServesGRPC
│   ├── push_test.go              # TestConfigurationRoundTrip, TestXpkgValidate, TestFunctionXpkg{Validate,RoundTrip,Cosign,SBOM}, TestFunctionIndexRoundTrip
│   ├── publish_test.go           # //go:build !noxpkg — TestPublish_EndToEnd
│   ├── publish_function_test.go  # //go:build !noxpkg — TestPublishFunction_{EndToEnd,MultiArchIndex,OutputLocalXpkg}
│   └── schema_chainsaw_test.go   # //go:build envtest — TestSchema_Chainsaw (+ its single-consumer helpers)
└── e2e/                          # package e2e — //go:build e2e
    ├── e2e_test.go               # TestE2E_Kind
    ├── cluster.go                # kind/Crossplane/TLS cluster harness
    ├── registry.go               # TLS/CA dual-registry harness
    └── doc.go                    # the only untagged file in this dir
```

Testdata/chainsaw assets: **stay where they are**, referenced via `common.RepoRoot(t)` (see §6).
`example/**`, `test/chainsaw/**`, and `internal/schema/testdata/derisked` are first-class /
shared-with-unit-tests and must stay. `internal/render/testdata/oci` and
`internal/e2e/testdata/module` *could* move (single consumer each) but staying + `RepoRoot` is the
smaller diff with zero fileGroup churn — recommended.

---

## 3. Helper consolidation plan — `internal/test/common`

A non-`_test.go` package (imports `testing`) reachable from `integration`, `e2e`, and any staying
unit `_test.go`. Proposed surface (harmonized across the audits):

```go
package common

// Registry: unified throwaway OCI registry (replaces testRegistry ×4 + startRegistry ×4)
type Registry struct { /* host, cueRegistry string; container *registry.RegistryContainer */ }
const RegistryImage = "registry:2.8.3"                               // was 5 copies
func StartRegistry(t *testing.T) *Registry                           // gates RequireDocker
func (r *Registry) Host() string
func (r *Registry) CUERegistry() string                             // host+"+insecure"
func (r *Registry) Env(cacheDir string) []string                    // CUE_REGISTRY + CUE_CACHE_DIR
func (r *Registry) Publish(t *testing.T, ref, srcDir string)        // was publishModule ×3
func (r *Registry) ManifestDigest(t *testing.T, ref string) string  // was manifestDigest ×2
func (r *Registry) Stop(t *testing.T)                               // idempotent (render offline test needs it)
func PublishModule(t *testing.T, host, ref, srcDir string)          // free-func mirror for the e2e TLS registry

// Gates
func RequireDocker(t *testing.T)
func RequireCrossplane(t *testing.T) string   // ADOPT the render --help shim-probe variant (strict superset)
func RequireBinary(t *testing.T, bin string) string
func RequireDevImage(t *testing.T) (dockerPath, image string)

// Paths / port / cache
func RepoRoot(t *testing.T) string            // walk to go.mod (lift pkg/push_test.go:275) — depth-independent
func FreePort(t *testing.T) int               // RECONCILE the two divergent copies (see Q6)
func CacheDir(t *testing.T) string            // chmod-then-rm CUE modcache temp (was render-only)

// Binary run / gRPC serve
func BuildBinary(t *testing.T) string
func ServeFunction(t *testing.T, bin, bindAddr, dialAddr, cueRegistry, cacheDir string)
func WaitForFunction(t *testing.T, addr string)   // folds the inline 30s dial loop in image_test.go
func WriteFunctions(t *testing.T, dir, target string) string

// Synthetic runtime base
func FakeRuntimeBase(t *testing.T, arch string) v1.Image        // unify divergent layer counts (256/2 vs 128/1)
func WriteRuntimeBaseTarball(t *testing.T, arch string) string
func RuntimeBaseImage(t *testing.T) v1.Image                    // real image.tar/$CUEFN_RUNTIME_IMAGE else fake

// Stream / kind parsing
func StreamKinds(stream []byte) map[string]bool                // unify extractKinds tail + streamKinds
func ExtractKinds(t *testing.T, bin string, img v1.Image, base string) map[string]bool
func PackageYAMLBytes(img v1.Image) ([]byte, error)
type YAMLDoc struct{ /* ... */ }
func SplitStream(t *testing.T, stream []byte) []YAMLDoc

// Render-result accessors (shared by staying engine_test.go + migrating oci_test.go)
func Object(t *testing.T, res render.Result, name string) map[string]any
func ToInt(t *testing.T, v any) int

// Pure / consts
func Zeros(n int) string
const ExampleModuleRef = "cuefn.example/app@v0.1.0"
const DevImage         = "crossplane-cuefn:dev"

// Typed Crossplane fixtures (depend only on exported internal/pkg API — see Q4)
func FixtureXRD(t *testing.T) *xv2.CompositeResourceDefinition
func BuildFixtureConfiguration(t *testing.T) pkg.Configuration
func FixtureFunction(t *testing.T) pkg.Function
func StepName(t *testing.T, step any) string
```

**Must be generalized:** `RequireCrossplane` must adopt the shim-probe variant (do not regress to
the bare `LookPath` — it's a silent-coverage hole); `FakeRuntimeBase` layer count unified;
`Registry` carries `cueRegistry` even where unused; expose `Host()` + a free `PublishModule` so the
e2e TLS registry (a different type) can reuse publish logic.

**Stays out of `common` (suite-specific, travels with its test):** `newOCIEngine`,
`containerSpec`, `variant`, the oci-only refs; `assertBaseLayerAnnotation`, `compositionInput`
(cli publish E2E only); the schema chainsaw helpers; the e2e `Cluster`/TLS `Registry` harness.

---

## 4. Test consolidation plan (do as **Phase 2**, after the move lands green)

Load-bearing fact (verified): the CLI publish commands call the **exact same exported
`internal/pkg` functions** the library round-trip tests call directly (`runPublish` →
`pkg.BuildConfigurationImage` + `pkg.Push`; `pushFunctionImage` → `pkg.BuildFunctionImage` +
`pkg.Push`; `pushFunctionIndex` → `pkg.BuildFunctionIndex` + `pkg.PushIndex`). So at the exported
boundary, each pkg round-trip is a strict subset of the corresponding CLI E2E's code path.

| # | Merge | Merged test asserts | Coverage preserved |
|---|---|---|---|
| C1 | `TestConfigurationRoundTrip` + `TestFunctionXpkgRoundTrip` → one table test | push→pull digest stability + byte-identical `package.yaml` for each artifact | each artifact keeps its own assertions (superseded by C3 if folded into CLI E2E) |
| C2 | fold `TestImageServesFunction` into `TestFunctionPackageServesGRPC` | the *packaged* image serves gRPC under `function --insecure` | packaged image dominates base image; base-preservation already covered by unit `TestBuildFunctionImage_EmbedsRuntime`. **Keep `TestImageServesFunction_NoArgs` separate** (only no-args/default-`cmd` coverage) |
| C4 | fold `TestFunctionIndexRoundTrip` into `TestPublishFunction_MultiArchIndex` | index pulls back with 2 manifests **and both platforms** (port the platform check the CLI test lacks) | unit `TestBuildFunctionIndex` also covers index logic registry-free |
| C5 | `TestXpkgValidate` + `TestFunctionXpkgValidate` → one crossplane-gated table test | `crossplane xpkg extract` accepts each artifact; expected kinds present | shared `RequireCrossplane` + `ExtractKinds`. **Do NOT fold into `TestPublish_EndToEnd`** (that uses the *library* extractor; this proves the external `crossplane` binary accepts the package) |
| C6 | `TestFunctionXpkgCosign` + `TestFunctionXpkgSBOM` → `TestFunctionXpkgSupplyChain` | push once, then independently-gated `t.Run("cosign")` / `t.Run("syft")` subtests | identical setup today; per-subtool gates kept inside subtests; halves registry containers |

**C3 (judgment call — see Q2):** `TestConfigurationRoundTrip` + `TestFunctionXpkgRoundTrip` may be
**deleted** as redundant with the CLI E2Es, *if* two narrow properties first migrate into the CLI
E2Es: (1) a digest-stability assertion (compare the `Push`-reported digest to the pulled-back
digest), and (2) a real-runtime-base path in `TestPublishFunction_EndToEnd`
(`--runtime-image image.tar`/`$CUEFN_RUNTIME_IMAGE` when present). The overlap audit argues
byte-for-byte identity after a registry round-trip "tests go-containerregistry, not our code." The
conservative fallback is C1 (keep one merged config-case round-trip).

**Stay separate (distinct coverage):** `TestImageServesFunction_NoArgs`; `TestPublish_EndToEnd`
(uniquely asserts the xpkg base-layer annotation, the Composition input records the **real resolved
digest**, and the runtime loader **accepts it / rejects drift**); `TestSchema_Chainsaw`;
`TestE2E_Kind`.

**"Unit" tests — KEEP in their home package, do NOT migrate:** `TestPush_UnreachableDestination`,
`TestPush_MalformedReference`, `TestPublish_MalformedModuleRef`, `TestPublish_RequiresPackage`,
`TestPublishFunction_RequiresFlags`, `TestPublishFunction_OutputLocalXpkg` (no infra — closed-port
/ malformed-ref / arg-validation / local-file paths). Their shared fixtures get promoted to
`common` and re-imported.

---

## 5. Per-file migration map

| Current file / symbol | New location | Export / refactor required |
|---|---|---|
| `render/oci_test.go` (`TestOCI_*`) | `integration/oci_test.go` | import `common`; oci-only helpers travel as unexported; `object`/`toInt`→`common`; `exampleRef`→`common.ExampleModuleRef`; paths→`RepoRoot` |
| `render/ocifixtures_test.go` | **DELETE** | all helpers → `common` |
| `render/engine_test.go` (unit) | **STAY** | switch `object`/`toInt` → `common.Object`/`ToInt` |
| `function/renderloop_test.go` | `integration/renderloop_test.go` | serve/build/wait/write/freePort/requireCrossplane → `common`; paths → `RepoRoot` |
| `function/image_test.go` | `integration/image_test.go` | `requireDevImage`/`devImage`→`common`; inline gRPC loop→`common.WaitForFunction`; C2 folds `TestImageServesFunction` |
| `function/funcpkg_smoke_test.go` | `integration/funcpkg_test.go` | `common.WaitForFunction`/`DevImage`; receives C2 |
| `function/testregistry_test.go` | **DELETE** | → `common` |
| `function/function_test.go` (unit) | **STAY** | none |
| `pkg/push_test.go` (integration) | `integration/push_test.go` | uses promoted `common` fixtures/helpers; paths→`RepoRoot` |
| `pkg/push_test.go` (`TestPush_*` error paths) | **STAY** in `internal/pkg` | import promoted fixtures from `common` |
| `pkg/testregistry_test.go` | **DELETE** | → `common` (drop the weaker `requireCrossplane`) |
| `pkg/helpers_test.go` (`fixtureXRD`,`yamlDoc`,`splitStream`) | **PROMOTE** to `common` | staying unit tests re-import |
| `pkg/configuration_test.go` | **STAY**; promote `buildFixtureConfiguration`+`stepName` | only exported `pkg.*` used |
| `pkg/function_test.go` | **STAY**; promote `fixtureFunction`+`fakeRuntimeBase` | layer count unified |
| `pkg/composition_test.go` (incl. `zeros`) | **STAY**; `zeros`→`common.Zeros` | dedupe |
| `pkg/meta_test.go` | **STAY** | none |
| `cli/publish_test.go` (`TestPublish_EndToEnd`) | `integration/publish_test.go` (**add `//go:build !noxpkg`**) | `package cli`→external; `assertBaseLayerAnnotation`/`compositionInput` travel; `zeros`→`common`; registry helpers→`common`; paths→`RepoRoot` |
| `cli/publish_test.go` (`TestPublish_Malformed*`/`_Requires*`) | **STAY** (unit) | consider `!noxpkg` for `-tags noxpkg` correctness |
| `cli/publish_function_test.go` | `integration/publish_function_test.go` (**keep `//go:build !noxpkg`**) | replace `publishFunctionUse`→literal `"publish-function"`; `writeRuntimeBaseTarball`/`streamKinds`→`common` |
| `cli/publish_helpers_test.go` | **DELETE** | → `common` |
| `cli/function_test.go`/`render_test.go`/`validate_test.go` (unit) | **STAY** | `freePort`→`common.FreePort` or keep local (Q6) |
| `schema/chainsaw_test.go` (`TestSchema_Chainsaw`) | `integration/schema_chainsaw_test.go` (**keep `//go:build envtest`**) | helpers travel; paths→`RepoRoot` |
| `schema/xrd_test.go`/`validate_test.go` (unit) | **STAY** | none |
| `e2e/e2e_test.go` (`TestE2E_Kind`) | `e2e/e2e_test.go` (**keep `//go:build e2e`**) | `publishModule`/`resolveDigest`→`common.PublishModule`/`ManifestDigest`; paths→`RepoRoot` |
| `e2e/cluster.go`,`registry.go`,`doc.go` | `internal/test/e2e/` | move wholesale; `registryImage`→`common.RegistryImage` |

---

## 6. Build / CI changes

**moon.yml task command rewrites** (test names unchanged in Phase 1 → `-run` regexes preserved):
- `oci-test` → `./internal/test/integration/...` (`-run TestOCI`)
- `render-test` → `./internal/test/integration/...` (`-run TestRenderLoop`)
- `publish-test` → `./internal/test/integration/...` (regex unchanged)
- `funcpkg-test` → `./internal/test/integration/...` (regex unchanged; optionally add `@group(cueModules)` to `inputs` for parity)
- `schema-test` → `-tags envtest ./internal/test/integration/...` (keep setup-envtest wrapper + `-run`)
- `e2e-test` → `-tags e2e ./internal/test/e2e/...` (keep `-run TestE2E_Kind -timeout 30m`)

**moon.yml fileGroups:** `goSources` (`internal/**/*.go`) already covers `internal/test/**` — no
change. `chainsawAssets`/`exampleAssets` — no change (assets stay put). `cueModules` — change
**only if** testdata moves (recommend it doesn't). `check` graph + `test` task — no change (the six
gated tasks aren't in `check`; migrated tests self-skip under plain `go test ./...`).

**Workflows:** `integration.yml`/`e2e.yml` call tasks **by name** → no edits unless a task is
renamed. `ci.yml`/`release.yml`/`security-scan.yml` — no edits.

**Build tags:** keep `envtest` on the schema file and `e2e` on all four e2e files (incl. non-test
`cluster.go`/`registry.go`; `doc.go` untagged). Keep `!noxpkg` on the migrated
`publish_function_test.go` **and add `!noxpkg` to the migrated `publish_test.go`** — verified that
`publish`/`publish-function` are only registered under `!noxpkg` (`packaging.go` vs the
`packaging_noxpkg.go` no-op), so testing `publish` without the tag is a latent mismatch.

**Fixture paths — `RepoRoot` helper, not directory moves.** Build every asset path as
`filepath.Join(common.RepoRoot(t), "example/module")` etc. Depth-independent; permanently removes
the `../../`→`../../../` fragility the depth-2→3 move introduces.

---

## 7. Risks & mitigations

1. **Relative-path silent breakage** — every `../../` is one segment short at depth 3. → Convert all
   to `RepoRoot`-joined paths in the same change; grep migrated files for `"../../"` and `"testdata`
   to confirm none remain.
2. **Cross-file helper loss** (`object`/`toInt`, the pkg fixtures) — staying unit tests lose helpers
   that move. → Promote to `common`; land the `common` promotion + staying-file import edits
   atomically.
3. **Lone unexported dep `publishFunctionUse`** — external package can't see it. → Substitute the
   literal `"publish-function"`; do **not** export it (would leak a UI string into the package API).
4. **Losing `!noxpkg` semantics** — dropping/forgetting the tag breaks `-tags noxpkg` builds
   (commands unregistered). → Keep `!noxpkg` on both migrated publish files; the rest of the
   integration package is tag-free.
5. **Losing `envtest`/`e2e` tags** — would either compile the heavy harness into the default `test`
   task or silently exclude the test from its task. → Preserve tags file-for-file; verify
   `go test ./internal/test/integration/...` (no tags) does **not** compile the chainsaw file, and
   `go test ./...` does not pull e2e.
6. **`RequireCrossplane` regression** — bare `LookPath` lets crossplane-gated tests pass against the
   moon proto fake `crossplane`. → Adopt the shim-probe variant; comment the reason.
7. **Container reuse / parallelism** — a flat package tempts a shared registry, which breaks the two
   state-mutating OCI tests. → Keep per-test `StartRegistry`.
8. **moon `-run` drift after Phase 2** — deleting/renaming tests (C3/C4/C5) can leave a regex that
   silently matches nothing. Note `TestFunctionXpkg*`/`TestFunctionIndex*` are currently selected by
   **both** `publish-test` and `funcpkg-test` (they run twice). → Re-derive every regex after Phase
   2; assert with `go test -list` that each task still matches the intended set.
9. **Fixture-promotion compile breakage** — staying unit files currently *define* the fixtures; after
   promotion they must *import* them. → Atomic change.
10. **`common` imports `testing`** — a non-`_test.go` package importing `testing`/testcontainers is
    built by `go build ./...` and some linters flag it. → Acceptable for a test-support package;
    scope-exclude in `.golangci.yml` if it objects.
11. **`freePort` semantic divergence** — the two copies behave differently; picking wrong silently
    breaks binding or races. → Reconcile to one `common.FreePort`; document the chosen semantics;
    audit both call sites.
12. **C3 coverage loss** — if the pkg round-trips are deleted, the digest-stability + real-base
    assertions must land in the CLI E2Es *first*. → Make those edits a precondition of deletion.

---

## 8. Open decisions (need your call — my recommendation in **bold**)

1. **Scope:** Phase 1 only (relocate + dedupe helpers, preserve every test), or Phase 1 + Phase 2
   (the merges + C3)? → **Both, sequentially** — land Phase 1 green first, then Phase 2 as a
   separate PR so the `-run` regex changes are isolated.
2. **C3 — delete the pkg round-trips** once digest-stability + real-base move into the CLI E2Es? →
   **Delete** (they test go-containerregistry, not our code), with C1 as the fallback if you'd
   rather keep one.
3. **Schema chainsaw home:** `internal/test/integration` (envtest-tagged) vs an e2e-adjacent
   sibling? → **`internal/test/integration`** (maximizes surveyability; it relocates cleanly via
   exported `render.LoadModule`/`schema.GenerateXRD`).
4. **Typed fixtures home:** `internal/test/common` vs a dedicated `internal/pkg/pkgtest`? →
   **`common`** (one shared-fixture home; one import for the staying unit tests).
5. **testdata:** stay-put + `RepoRoot`, or co-locate the single-consumer testdata with its tests? →
   **Stay-put + `RepoRoot`** (smallest diff, zero fileGroup churn).
6. **`freePort` contract:** which semantics for `common.FreePort`, and does the staying unit
   `cli/function_test.go` adopt it? → **Pick one and have both call sites adopt it** (recommend
   close-then-return; the server bind tolerates the brief window in practice — confirm against the
   renderloop usage).
7. **Out-of-scope white-box unit tests** (`cli` `render`/`function`/`validate`, the publish
   arg-validation tests): leave white-box, or also push external per Principle 2? → **Leave
   white-box** — Principle 2 targets integration/E2E; these are genuine unit tests with no infra.
8. **Fix the latent `publish_test.go` tag now** (adding `!noxpkg` excludes it from `-tags noxpkg`
   builds rather than failing them)? → **Yes** — correct behavior; the `noxpkg` build has no publish
   command to test.

---

## Appendix — provenance

- Initial survey: session 003, first task (24 Go integration/E2E test functions across 6 packages
  + 3 Chainsaw `Test`s in 2 declarative suites).
- Assessment: `assess-test-reorg` workflow, run `wf_a5a87d1f-5e8` — four parallel read-only auditors
  (helper / boundary / overlap / wiring) + a synthesis pass; full reports persisted under the
  session's task output. Load-bearing claims independently re-verified against the tree before this
  doc (the `!noxpkg` registration asymmetry, the lone `publishFunctionUse` dep + literal `"publish"`,
  `object`/`toInt` in the staying `engine_test.go`, the two divergent `freePort` copies).
