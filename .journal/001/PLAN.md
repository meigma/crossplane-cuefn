# PLAN — crossplane-cuefn phased build (working draft)

> **Status: temporary working plan (session 001), revised after a 5-lens
> adversarial review.** Supplements [`DESIGN.md`](DESIGN.md): breaks the design
> into sequential phases, each a natural PR. No code here — phases describe intent,
> scope, and success criteria; agents implement. Starting point is the
> post-rebrand `master` (binary `cuefn`, module `github.com/meigma/crossplane-cuefn`,
> placeholder Cobra/Viper CLI; cobra/viper deps only; full mise/moon/melange/apko
> supply chain; no Crossplane/CUE deps yet).

## Sequencing overview

Decision #7 (DESIGN §13) puts the **runtime engine first**. Arrows below are
**merge order**, not the full dependency graph (true deps are noted per phase).

```
P1 engine(offline) → P2 OCI+deps+cache+digest-verify → P3 function + render(+ `cuefn render`)
   → P4 author-time schema CLI (generate + validate) → P5 Configuration publish
   → P6 function xpkg + release → P7 docs → P8 e2e on kind
```

**What proves what (no false "cluster-free" claims).** `cuefn render` (built in
P3) gives a cluster-free, `crossplane`-CLI-free functional check of engine output
(resources/status). `crossplane render` (P3) proves the function works inside a
real Crossplane function pipeline (needs the `crossplane` CLI + Docker for
`function-environment-configs`). Composed-resource **readiness** is a response
signal, asserted in **unit tests**, not in render output. True **API-server**
behaviors (XRD structural acceptance, defaulting, status subresource, pruning)
are proven by P4's **server-side dry-run/envtest** and P8's kind e2e — not by
render.

**Toolchain rule (cross-cutting).** Any phase that needs a new tool must pin it in
`mise.toml`/`mise.lock` and wire it into moon/CI in that phase: P3 adds the
`crossplane` CLI + `controller-tools`; P4 adds `apiextensions-apiserver` (dep) and
the server-side check tool (envtest/`kubectl`); P7 adds the `cue` CLI (for the
documented author workflow); P8 adds `kind`.

**Embedded de-risk spikes.** Two phases open with a short spike whose output is a
TECH_NOTES entry, because the draft rested on unproven assumptions: **P2** (what
actually fails with a real two-module dep graph; whether `load.Config{Registry}`
is needed or CUE auto-creates it from `CUE_REGISTRY`; nonroot `CUE_CACHE_DIR`
writability; the digest verify-after-fetch mechanism) and **P5** (confirm
Crossplane's xpkg builder is non-importable `internal/`; prototype a minimal
Configuration xpkg **and** an embed-runtime Function xpkg with go-containerregistry
against the xpkg media-type spec — de-risks both P5 and P6).

---

## Phase 1 — Render engine core + module contract (offline)

**Goal.** The shared `internal/render` engine that evaluates a CUE module from a
local directory into the v2 output contract (author-keyed resource map +
readiness + status), with the canonical example module.

**Description.**
- Add the engine deps (DESIGN §11): `cuelang.org/go`, `function-sdk-go`,
  `crossplane-runtime/v2`, `crossplane/apis/v2`, `apimachinery`, plus
  `go-cmp`/`testify`. (gRPC/protobuf, controller-tools, apiextensions-apiserver
  come in later phases that need them.)
- `internal/render` as a pure library: `Engine` + `ModuleLoader` port +
  `LocalLoader` adapter. Fill the module's `input.{spec,metadata,environment}` via
  **JSON marshalling** (avoids the float64-vs-int trap), evaluate, read the
  keyed-map `resources` (`{<name>: {object, ready?}}`) + optional `status`,
  validate concreteness over **both** `resources` and `status`, decode into an
  in-Go render result (resources keyed by the author's name with object +
  readiness; status map).
- **Reserved-key projection (DESIGN blocker #1):** project the observed XR's
  **user** spec into `input.spec` with `spec.crossplane` (and legacy machinery
  keys) **removed**, so a closed `#Spec` doesn't conflict. (The XR plumbing lands
  in P3, but the projection helper + its tests live here with the engine.)
- Canonical `example/module/` (with `cue.mod`, **no external imports** — kept
  dependency-free so the offline criterion stays valid) exercising `#API`,
  `#Spec`, `#Status`, and a transform producing a keyed resource map with
  readiness, env-driven output, and a returned status.

**Out of scope.** OCI loading; gRPC function; codegen; CLI subcommands.

**Depends on.** Nothing (first phase).

**Success criteria.**
- `go build ./...` and `go test ./...` green.
- **Keyed output:** the render result's resource keys equal the module's map keys
  **verbatim** (not a derived `<kind>-<name>`).
- **Readiness:** all three states are represented — `Ready`, `NotReady`, and
  **absent → unspecified** (not silently defaulted).
- **Status:** a module returning `status` yields it; a module with no
  `#Status`/`status` renders cleanly; a **non-concrete `status`** is rejected.
- **float64-vs-int proof:** an XR spec integer arriving as a JSON number (e.g.
  `replicas: 2`) unifies against a bounded `int` field (`*1 | int & >=1 & <=10`)
  and renders — the falsifiable proof the JSON-marshal fill (not Go-encode) is in
  place.
- **Reserved-key proof:** an observed spec containing `spec.crossplane.compositionRef`
  renders successfully (the key is stripped before unifying with closed `#Spec`).
- **Error paths (observable):** a violated `#Spec` constraint and a non-concrete
  `resources` each return a non-nil error whose message contains the offending
  field/key, with no panic.

---

## Phase 2 — OCI loading: transitive deps, nonroot cache, digest verification

**Goal.** Load a module (and any transitive CUE deps) from an OCI registry per
`CUE_REGISTRY`, with a nonroot-safe cache and a working digest-drift guard.

**Description.**
- **Open with a de-risk spike** (output → TECH_NOTES): build a real two-module dep
  graph (consumer imports a shared dep), and establish (a) what actually fails
  today and whether an explicit `load.Config{Registry}` is required or CUE
  auto-creates the registry from `CUE_REGISTRY` when nil; (b) where CUE writes its
  module cache and how to point it at a writable dir for a nonroot/read-only-root
  container (`CUE_CACHE_DIR`); (c) the digest verify-after-fetch mechanism (CUE
  refs are **semver**, not digests — so verify the fetched module's manifest
  digest against an expected value rather than referencing by digest).
- Implement `OCILoader`: resolve + fetch via the CUE module registry API
  (`modconfig`/`modregistry` → zip → unzip, strip `<module>@<version>/` prefix),
  evaluate through the P1 engine. Honor `CUE_REGISTRY` incl. `+insecure`. Use
  CUE's module cache with `CUE_CACHE_DIR` set to a writable path; prefer CUE's
  built-in modcache over a parallel cache unless the spike shows otherwise.
- **Digest verification:** accept an optional expected manifest digest; after
  fetch, verify it matches and reject on mismatch (the runtime half of DESIGN's
  digest lock-step).
- Test fixtures are **published via the Go modregistry API** (no `cue` CLI
  dependency) to a throwaway local OCI registry (testcontainers). Use a
  **separate consumer+dep fixture pair** for the transitive-dep test; keep
  `example/module/` dependency-free.

**Out of scope.** gRPC function; codegen; cosign verification of modules.

**Depends on.** Phase 1 (engine + contract).

**Success criteria.**
- **Loader equivalence:** the P1 example module (no deps) renders **byte-for-byte
  identical** via `OCILoader` and `LocalLoader`.
- **Transitive deps:** a module importing an OCI-only dependency renders the
  expected output via `OCILoader` (the spike's conclusion on how the local
  baseline resolves the dep is documented).
- **Digest-keyed cache, not tag-keyed:** republishing the same version/tag with
  changed content yields a different digest and re-fetches/renders the new content.
- **Cross-process cache:** a fresh process/Engine serves a previously-cached
  module with the registry stopped/unreachable.
- **Digest drift rejected:** loading with an expected digest that no longer matches
  the fetched manifest fails with a clear error.
- **Error paths:** a non-existent ref, a malformed ref, and an unreachable registry
  each return a non-nil wrapped error (naming ref/cause), no panic.
- **Nonroot cache:** the cache works with `CUE_CACHE_DIR` set to a non-`$HOME`
  writable path (the in-image proof lands in P3/P6).

---

## Phase 3 — Function + `cuefn function` + `cuefn render` + render loop

**Goal.** Wire the engine into a Crossplane v2 composition function (`cuefn
function`), add the local `cuefn render` author command, and prove an end-to-end
`crossplane render` loop.

**Description.**
- Add `google.golang.org/protobuf` and `sigs.k8s.io/controller-tools` (deepcopy);
  pin the `crossplane` CLI in `mise.toml`/`mise.lock`. Wire both into moon/CI for
  the render-loop test.
- Define the function `Input` type under `input/v1beta1` (module **semver** ref;
  an optional **expected-digest** field for the lock-step; minimal options),
  kubebuilder markers + generated deepcopy.
- `RunFunction`: read `Input`, the observed XR (spec via the P1 reserved-key
  projection + metadata), env from context key
  `apiextensions.crossplane.io/environment`; invoke the engine; set desired
  composed resources keyed by the module's author names with readiness translated
  to `resource.Ready`; patch the desired composite `status`; set a success
  condition; fatal on malformed input.
- `cuefn function` Cobra subcommand (`function.Serve`, mTLS/insecure flags). Update
  `apko.yaml` entrypoint/args to `cuefn function` (the bare `cuefn` root prints
  help and would not serve).
- `cuefn render <module> --xr <file> [--env <file>]`: local eval to stdout
  (resources + status), reusing the engine + loaders — a cluster-free, crossplane-CLI-free
  functional surface (also covers DESIGN §8's `render` command).
- `example/`: hand-written XRD (codegen lands in P4), Composition
  (`function-environment-configs` → `cuefn`), XR, EnvironmentConfig, **and
  `functions.yaml`** (`render.crossplane.io/runtime: Development` for `cuefn` +
  registration for `function-environment-configs`).
- Table-driven `RunFunction` tests (protobuf-aware compare; JSON struct mocks).

**Out of scope.** XRD generation; validate; packaging.

**Depends on.** Phases 1–2.

**Success criteria.**
- **RunFunction (observable, not "tests green"):** desired resources are keyed by
  the module's author names; a `ready` hint maps to `resource.Ready`, absent →
  unspecified; the module's `status` is patched onto the desired composite; a
  success condition is set; malformed/unreachable input yields a FATAL result with
  a message naming the cause — all asserted via protobuf-level unit tests.
- **`cuefn render`** on the example module + sample XR/env emits the expected
  resources + status (the cluster-free functional check).
- **`crossplane render`** (with `cuefn function --insecure` running, module from
  the P2 registry, Docker available for `function-environment-configs`) produces
  the expected resources, and an **env-driven field differs from its `#Spec`
  default** because of the EnvironmentConfig value (proves env flows through).
- **Image serves:** the apko-built image, run as its entrypoint, starts the gRPC
  FunctionRunnerService (container smoke check), not the help text.

---

## Phase 4 — Author-time schema CLI: `cuefn generate` + `cuefn validate`

**Goal.** Generate a Kubernetes-structural XRD from `#API`/`#Spec`/`#Status`, and
validate a populated XR against `#Spec` — the two author-time schema commands,
both built on the loader + CUE (validate merged here per review; it needs only the
loader + `#Spec`).

**Description.**
- Add `k8s.io/apiextensions-apiserver` (structural self-check) and an actual
  server-side check tool (envtest or a pinned `kubectl` for `--dry-run=server`).
- `internal/schema` (de-risked recipe, DESIGN §5 / TECH_NOTES "Codegen de-risk
  spike"): definitions-only reduction; `openapi.Generate` with
  `ExpandReferences:false`; cycle-detecting `$ref` inliner; XRD envelope from
  `#API` (single served version; `spec` from `#Spec`; `status` subresource from
  `#Status` when present; printerColumns/categories/shortNames); structural
  self-check (`NewStructural` + `ValidateStructural`); clear error on type-crossing
  disjunctions (the one guardrail).
- `cuefn generate` (module → XRD YAML) and `cuefn validate <xr> --module <ref>`
  (unify XR spec against `#Spec`; CUE-error messages; scriptable exit codes;
  applies `#Spec` defaults).
- The codegen test/example `#Spec` must include the **de-risked constructs**:
  a bounded-number-with-default (the `ExpandReferences:true` bug case), at least
  one `$ref`/nested definition (forces the inliner), plus enum, regex, bool
  default, list-of-objects, and a map pattern.

**Out of scope.** Composition/Configuration packaging; multi-version XRDs.

**Depends on.** Phase 1 (`#API`/`#Spec`/`#Status` + loader). Independent of P3.

**Success criteria.**
- **Structural + de-risked:** the generated XRD passes `ValidateStructural` **and**
  a server-side `kubectl apply --dry-run=server` / envtest accept — using the
  de-risked `#Spec` above (so passing re-proves the spike's fixes).
- **Fidelity to the module:** required (`!`) fields are `required`; `#Spec`
  defaults + numeric bounds appear in the schema; the `#API` envelope
  (group/kind/plural/scope/version) maps correctly; a `status` subresource + schema
  exists **iff** `#Status` is present; printerColumns/categories/shortNames carry
  through.
- **Status round-trip:** applying an XR against the generated XRD (dry-run/envtest)
  preserves the `#Status`-derived fields (not pruned).
- **validate:** out-of-bounds, wrong-enum, and missing-required XRs are each
  rejected with a field-located message + non-zero exit; a valid XR passes (exit
  zero); an XR omitting a defaulted field validates (proves defaults applied).
- A type-crossing disjunction in a schema definition produces a clear error naming
  the field.

---

## Phase 5 — Configuration packaging + push: `internal/pkg` + `cuefn publish`

**Goal.** Build and push an installable, drift-guarded Crossplane **Configuration**
from a single module, with no external `crossplane` CLI.

**Description.**
- **Open with the xpkg packaging spike** (output → TECH_NOTES): confirm
  `crossplane/internal/xpkg` is non-importable; prototype assembling a **minimal
  valid Configuration xpkg** and an **embed-runtime Function xpkg** with
  go-containerregistry against the xpkg media-type/annotation spec, validated by
  `crossplane xpkg`/inspect. This de-risks both this phase and P6.
- Generate the Configuration contents: the XRD (P4), a Composition (pipeline
  `function-environment-configs` → the `cuefn` function), and `crossplane.yaml`
  (`meta.pkg.crossplane.io`, `dependsOn` the function). The Composition input
  records the module **semver ref + the expected manifest digest** the XRD was
  generated from (the lock-step the P2/P3 runtime verifies).
- `internal/pkg`: assemble + push the Configuration xpkg from Go
  (go-containerregistry), per the spike's layout.
- `cuefn publish` (generate + package + push; flags for module ref, Configuration
  image ref, function ref/version).

**Out of scope.** The function's own xpkg (P6); in-cluster install (P8).

**Depends on.** Phase 2 (digest resolution), Phase 4 (XRD generation).

**Success criteria.**
- **Externally checkable, not self-referential:** the embedded YAMLs parse as the
  expected Crossplane kinds/apiVersions (XRD `apiextensions.crossplane.io/v2`;
  Composition; `meta.pkg.crossplane.io` Configuration with `dependsOn` the
  function); a push→pull round-trip from the test registry re-yields identical
  contents.
- **Lock-step recorded:** the Composition input carries the module semver ref and
  the **exact expected digest** the XRD was generated from; a follow-up render
  using that input has the P3 runtime verify the digest (tie-in proven in P8).
- `cuefn publish` builds and pushes a Configuration xpkg with correct media types
  / annotations.

---

## Phase 6 — Function xpkg packaging + release wiring

**Goal.** Ship the function as a signed Crossplane **Function** xpkg through the
release pipeline (the deferred packaging from DESIGN §10).

**Description.**
- `package/crossplane.yaml` (`meta.pkg.crossplane.io`, kind `Function`) **plus the
  embedded Input CRD** generated from the `input/v1beta1` type.
- Build the function xpkg per P5's spike layout: embed the apko-built runtime image
  (entrypoint `cuefn function`), cosign-sign, push to an HTTPS registry
  (Crossplane's package manager is HTTPS-only).
- Update `release.yml`, `release-dry-run.yml`, `security-scan.yml` for the
  function-xpkg artifact; carry SBOM + provenance attestations.

**Out of scope.** Multi-version/conversion; Configuration packaging (P5); the
actual install (P8).

**Depends on.** Phase 3 (function + entrypoint), Phase 5 (shared xpkg builder).

**Success criteria.**
- The release **dry-run** runs green in CI and produces a Function xpkg with the
  correct package media types/annotations, the embedded runtime image, and the
  embedded Input CRD.
- `cosign verify` succeeds; SBOM and provenance attestations are present and
  verify with a named command.
- The packaged image, run, serves the gRPC FunctionRunnerService.
- ("Installable as a Crossplane Function" is proven in P8.)

---

## Phase 7 — Documentation

**Goal.** A self-contained documentation set for the contract and the CLI.

**Description.**
- Diátaxis pages + mkdocs nav (today nav is only `Home`): a **reference** for the
  module contract (`#API`/`#Spec`/`#Status`; the `resources`/`status` output; the
  reserved-key projection and digest-verify behaviors), a **CLI reference**
  (`generate`/`validate`/`publish`/`render`/`function`), and a **quickstart**
  (author → `cue mod publish` → `cuefn publish` → install → instantiate).
- Pin the `cue` CLI in mise for the documented `cue mod publish` author workflow.

**Out of scope.** The cluster e2e (P8).

**Depends on.** Phases 3–6 (the commands/behaviors being documented exist).

**Success criteria.**
- `docs:build --strict` passes (the hard gate already in `moon run root:check`).
- The CLI reference documents exactly the shipped subcommands (incl. `render`); no
  documented command is unimplemented.

---

## Phase 8 — End-to-end on a real cluster (kind) + CI e2e

**Goal.** Prove the full author→publish→install→instantiate→reconcile loop and the
distinctive in-cluster behaviors, automated where practical.

**Description.**
- Pin `kind`; stand up a kind cluster + Crossplane + install the Function (P6) and
  a Configuration published by `cuefn publish` (P5); instantiate an XR; assert
  reconciliation.
- Cover the behaviors proven nowhere else: API-server XRD defaulting (the DESIGN §3
  "no drift" claim), the status subresource, readiness conditions, the
  EnvironmentConfig merge, and the digest-drift guard.

**Out of scope.** Performance; deferred features (DESIGN §12).

**Depends on.** Phases 1–7.

**Success criteria (concrete cluster observables).**
- The XR reaches `Ready=True` and the expected composed Deployment + Service exist.
- The XR `status` is populated from the module's `#Status`.
- A `#Spec`-defaulted field **omitted** in the XR is filled by the API server
  (proves XRD defaulting / no-drift).
- An EnvironmentConfig value appears in a composed resource.
- **Digest-drift guard:** republishing the module's content under the same version
  causes the function to reject the drifted module (the P2/P5 lock-step, end to
  end).
- At least the kind-based path is an automated CI gate; any manual fallback is a
  scripted runbook ending in these same assertions (no "to the extent practical"
  hedging).

---

## Cross-cutting expectations (every phase)

- Hexagonal boundaries: logic in `internal/*` libraries; adapters at the edges
  (loaders, gRPC, OCI, CLI). (TECH_NOTES.)
- Functional testing before "done", not just unit tests. (TECH_NOTES.)
- New tools pinned in `mise.toml`/`mise.lock` and wired into moon/CI in the phase
  that introduces them.
- Every error-path criterion asserts: no panic; the exact result/severity (FATAL /
  non-nil wrapped error / specific exit code); message contains a named token
  (field path, module ref, or root cause).
- `moon run root:check` green; Conventional-Commit PR title; squash merge.
- Promote hardened decisions from DESIGN/PLAN into `TECH_NOTES.md` (especially the
  two embedded spike outputs); keep these working docs current as reality diverges.
