# DESIGN — crossplane-cuefn (working draft)

> **Status: temporary working design (session 001).** This is the consolidated
> design we've converged on, plus the still-open questions. It is a scratchpad to
> drive the build, not a final spec — expect churn. Promote durable decisions to
> `TECH_NOTES.md` as they settle; this file can be deleted once the design is
> stable and reflected in real docs/code.

## 1. Goal

A **Crossplane v2 composition function (Go)** that renders Kubernetes resources
from **CUE modules pulled from an OCI registry**, plus an **operator CLI
(`cuefn`)** that makes one CUE module the single source of truth for both the
platform API (the XRD) and its transformation logic.

Author experience, end to end:

1. Write one CUE module: a strict **schema** for the XR spec + a **transform**
   that renders Kubernetes objects.
2. Publish the module to an OCI registry (`cue mod publish`).
3. Run `cuefn` to generate an XRD (CUE→OpenAPI) and package + push a versioned
   Crossplane **Configuration** (XRD + a Composition wired to this function).
4. Install the Configuration; instantiate XRs. The function pulls the module and
   renders resources per XR.

This is the CUE analog to `function-kcl` (modules from OCI), going beyond
`function-cue` (inline-only).

## 2. Architecture (two halves, one shared core)

```
                ┌─────────────────────────── shared core ───────────────────────────┐
                │  internal/render  (CUE module → resources)                         │
                │  internal/schema  (CUE #Spec → structural OpenAPI/XRD)             │
                └────────────────────────────────────────────────────────────────────┘
   RUNTIME (in-cluster)                         AUTHOR-TIME (cuefn CLI, local)
   ── function gRPC server ──                   ── cuefn generate / package / validate / render ──
   pulls module from OCI,                       reads same module, emits XRD + Configuration,
   injects XR spec+env, returns resources       validates XRs, renders locally for debugging
```

The "CUE module → resources" engine and the "CUE #Spec → XRD" codegen are plain
Go libraries reused by both the runtime and the CLI. Clean hexagonal seam.

## 3. Module contract v2 (the linchpin)

One module exposes well-known **definitions** (schema/metadata, consumed by the
CLI) and the **transform** (regular fields, consumed by the runtime).

```cue
package app

// Metadata the CLI Decodes to build the XRD envelope. Concrete values.
#API: {
	group:   "platform.meigma.io"
	version: "v1alpha1"
	kind:    "XApp"
	plural:  "xapps"
	scope:   *"Namespaced" | "Cluster"
}

// Authoritative schema for the XR spec — ONE source feeding:
//   • CLI:     CUE→OpenAPI→XRD .properties.spec
//   • runtime: unify the incoming XR spec against it (defaults + validation)
//   • validate: check a populated XR against it
#Spec: {
	image!:   string
	replicas: *1 | int & >=1 & <=10
	tier:     *"dev" | "staging" | "prod"
	// ...
}

// Transform — regular fields. Runtime fills `input`, reads `resources`.
input: {
	spec:        #Spec        // ties runtime defaults/validation to the same schema
	metadata:    {name: string, namespace?: string, ...}
	environment: {...}        // merged EnvironmentConfig
}
resources: [ /* finished k8s objects derived from input */ ]
```

Because `input.spec: #Spec`, the XRD's API-server-filled defaults and the
render-time defaults come from the same place — no schema/runtime drift.

## 4. Runtime function (adopt the spike's `internal/render` ~as-is)

- `Engine` + `ModuleLoader` port; adapters `OCILoader` (real) / `LocalLoader`
  (tests/offline).
- **Contract:** fill top-level `input: {spec, metadata, environment}`, read
  top-level `resources: [...]`. Fill via **JSON marshal** (avoids float64-vs-int
  constraint conflicts). Validate `resources` `cue.Concrete(true)`, decode to
  `[]map[string]any`.
- **OCI load:** `modconfig.NewResolver` → `modregistry.GetModule` → `GetZip` →
  unzip (strip `<mod>@<ver>/` prefix) → `load.Instances`. Honors `CUE_REGISTRY`
  (incl. `+insecure`).
- **fn.go:** read `Input.Module` (`path@version`), observed XR spec/metadata, env
  from pipeline context key `apiextensions.crossplane.io/environment`; map each
  rendered object → `desired[<name>]` via `composed.New()`.
- **Pipeline requirement:** `function-environment-configs` must run upstream to
  populate the env context.

## 5. Codegen: CUE #Spec → structural XRD (`internal/schema`) — de-risked

Proven recipe (scratch spike validated against the API server's own
`apiextensions-apiserver` structural validator):

1. Load module; `Decode` `#API`; reduce module value to **definitions only**
   (the OpenAPI encoder rejects regular top-level fields like `input`/`resources`).
2. `openapi.Generate(defs, {ExpandReferences: false})` — **must be false**;
   `true` is buggy with bounded numbers (`unsupported op for number &`).
3. **Inline `$ref`s** ourselves (cycle-detecting) → self-contained schema (K8s
   structural schemas forbid `$ref`).
4. Wrap `Spec` as `openAPIV3Schema.properties.spec`; emit XRD
   (`apiextensions.crossplane.io/v2`) from `#API`.
5. (Optional self-check) run `NewStructural` + `ValidateStructural` to fail fast.

**Author guardrail (only real constraint):** type-crossing disjunctions
(`string | int`, struct unions `{a} | {b}`) → `oneOf` → not expressible as a K8s
structural schema. Same-type disjunctions (enums) are fine. Possible future
nicety: detect `string | int` and emit `x-kubernetes-int-or-string`.

## 6. Configuration packaging & distribution

- The CLI emits a Crossplane **Configuration** = generated XRD + a generated
  **Composition** (pipeline: `function-environment-configs` → this function with
  the module ref as input) + `crossplane.yaml`
  (`meta.pkg.crossplane.io`, `spec.dependsOn` this function).
- **Digest lock-step:** the generated Composition pins the module by **digest**
  (not just tag) so the XRD schema and the runtime transform can never drift.
- Build/push as an `xpkg`. (Mechanism is an open question — see below.)
- Registry split (from the spike): the **function image / packages** need an
  HTTPS registry (Crossplane's package manager is HTTPS-only); **CUE modules**
  can live on any OCI registry (incl. plain-HTTP local).

## 7. Validate command (bonus, easy win)

`cuefn validate <xr.yaml> --module <ref>`: unify the XR's spec against the
module's `#Spec` via the CUE Go API; report violations with CUE's (good) error
messages. Nicer DX than OpenAPI validation. Pure library reuse of `internal/render`'s
loader + the schema.

## 8. CLI surface (cobra, on the existing scaffold)

Proposed subcommands (names TBD):

- `cuefn generate` — module → XRD (stdout/file).
- `cuefn package` / `publish` — module → Configuration xpkg → push (the "one
  command" that does generate + package + push).
- `cuefn validate` — XR vs module `#Spec`.
- `cuefn render` — local eval (module + sample XR/env → resources) for debugging,
  mirrors `crossplane render`.
- runtime server — see Q (binary topology).

## 9. Repo structure (proposed)

```
cmd/cuefn/            # operator CLI (+ maybe `serve` subcommand for the function)
internal/render/      # CUE module → resources (shared)
internal/schema/      # CUE #Spec → structural XRD (shared)
internal/pkg/         # Configuration/xpkg assembly + push
internal/cli/         # cobra commands (existing)
example/              # runnable demo module + XRD + Composition + XR
package/              # function xpkg metadata (crossplane.yaml) [if function ships as xpkg]
```

## 10. Supply chain / packaging the function

Repo already has melange/apko/cosign + GoReleaser + Release Please. A Crossplane
function ships as an **xpkg** (OCI image wrapping an embedded runtime image) —
the template currently builds a plain runtime image. Adapting melange/apko output
into an xpkg (and signing it) is deferred but needed. (See Q.)

## 11. Stack / dependencies (proven together in the spike)

`cuelang.org/go v0.16.1` · `crossplane/function-sdk-go v0.7.1` ·
`crossplane-runtime/v2 v2.3.1` · `crossplane/apis/v2 v2.3.1` ·
`k8s.io/apimachinery v0.35.3` · `k8s.io/apiextensions-apiserver v0.35` (codegen
structural check) · `sigs.k8s.io/controller-tools v0.20` (input deepcopy) ·
`google.golang.org/protobuf`. CLI uses cobra/viper (repo default; spike used kong).

## 12. Decided vs deferred

**Decided:** product shape; module contract v2 (`#API`+`#Spec`+transform); the
`input{spec,metadata,environment}`→`resources` runtime contract; OCI module load
via `modregistry`+`CUE_REGISTRY`; the codegen recipe + guardrail; env from
pipeline context; digest lock-step; dual Apache-2.0/MIT; binary `cuefn`; 0.1.0.

**Deferred (later slices):** xpkg packaging adaptation; transitive OCI module
deps (TBD); the `crossplane`-import vs shell-out decision; int-or-string escape
hatch.

## 13. Open questions (to resolve one at a time)

1. **Binary topology** — one `cuefn` binary with a `serve`/`function` subcommand
   for the runtime, vs a separate function-server binary (`cmd/function`)?
2. **Output contract scope** — does the module return only `resources`, or also
   XR `status` and/or readiness? Or keep v1 resources-only and lean on
   `function-auto-ready` downstream?
3. **Resource naming** — keep the spike's deterministic `<kind>-<name>` keys, or
   let authors control the desired-resource name (map key / annotation)?
4. **Module API-metadata scope (v1)** — minimal (`group/version/kind/plural/scope`)
   vs richer now (status schema, additional printer columns, multiple versions)?
5. **Configuration packaging mechanism** — shell out to the `crossplane` CLI
   (simple, external dependency) vs self-contained Go via go-containerregistry
   (no external dep, more code) vs import Crossplane's xpkg packages (if exported)?
6. **Transitive CUE module deps (v1)** — support modules that import other OCI
   modules now (wire `load.Config.Registry`), or restrict v1 to self-contained
   modules?
7. **First build slice** — codegen (`generate`) first, runtime engine first, or
   the validate command first?
