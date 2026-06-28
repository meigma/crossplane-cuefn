# DESIGN — crossplane-cuefn (working draft)

> **Status: temporary working design (session 001).** The consolidated design we
> converged on, including the 7 resolved decisions (see §13). A scratchpad to
> drive the build, not a final spec. Promote durable decisions to `TECH_NOTES.md`
> as they harden; delete this file once reflected in real docs/code.

## 1. Goal

A **Crossplane v2 composition function (Go)** that renders Kubernetes resources
from **CUE modules pulled from an OCI registry**, plus an **operator CLI
(`cuefn`)** that makes one CUE module the single source of truth for both the
platform API (the XRD) and its transformation logic.

Author experience, end to end:

1. Write one CUE module: a strict **schema** for the XR spec (`#Spec`), an
   optional status schema (`#Status`), and a **transform** that renders k8s objects.
2. Publish the module to an OCI registry (`cue mod publish`).
3. Run `cuefn publish` to generate an XRD (CUE→OpenAPI) and build + push a
   versioned Crossplane **Configuration** (XRD + a Composition wired to this
   function, pinned to the module **digest**).
4. Install the Configuration; instantiate XRs. The function pulls the module and
   renders resources per XR.

CUE analog to `function-kcl` (modules from OCI), beyond `function-cue` (inline).

## 2. Architecture (two halves, one shared core, ONE binary)

```
                ┌─────────────────────────── shared core ───────────────────────────┐
                │  internal/render  (CUE module → resources + status + readiness)    │
                │  internal/schema  (CUE #Spec/#Status → structural OpenAPI/XRD)      │
                │  internal/pkg     (XRD + Composition → Configuration xpkg → push)   │
                └────────────────────────────────────────────────────────────────────┘
   RUNTIME: `cuefn function`                    AUTHOR-TIME: `cuefn generate|publish|validate|render`
   gRPC server; pulls module from OCI,          same engine; emits XRD + Configuration,
   injects XR spec+env, returns resources       validates XRs, renders locally
```

One `cuefn` binary (cobra). `cuefn function` is the in-cluster gRPC server and
the apko image entrypoint; the other subcommands are the operator CLI. Shared
logic is plain Go libraries under `internal/`.

## 3. Module contract v2 (the linchpin)

```cue
package app

// Metadata the CLI Decodes to build the XRD envelope. Concrete. Single version.
#API: {
	group:   "platform.meigma.io"
	version: "v1alpha1"            // exactly one served version in v1
	kind:    "XApp"
	plural:  "xapps"
	scope:   *"Namespaced" | "Cluster"
	// niceties folded into XRD generation when present:
	shortNames?: [...string]
	categories?: [...string]
	printerColumns?: [...{name: string, jsonPath: string, type: string}]
}

// Authoritative XR spec schema — ONE source feeding CLI codegen, runtime
// defaults/validation, and `cuefn validate`.
#Spec: {
	image!:   string
	replicas: *1 | int & >=1 & <=10
	tier:     *"dev" | "staging" | "prod"
	// ...
}

// Optional XR status schema — drives the XRD status subresource and types the
// status the transform returns.
#Status: {
	url?:   string
	ready?: bool
}

// Transform — regular fields. Runtime fills `input`, reads `resources`/`status`.
input: {
	spec:        #Spec            // ties runtime defaults/validation to the schema
	metadata:    {name: string, namespace?: string, ...}
	environment: {...}            // merged EnvironmentConfig
}

// Desired composed resources, keyed by a STABLE author-chosen name (the key is
// the Crossplane composed-resource name). `ready` is an optional readiness hint.
resources: [Name=string]: {
	object: {...}                                  // finished k8s object
	ready?: "Ready" | "NotReady"                   // -> Crossplane readiness; default unspecified
}

// Optional status patched onto the composite (XR).
status?: #Status
```

`input.spec: #Spec` keeps XRD-defaults (API-server-filled) and render-defaults
from the same source → no drift.

> **Reserved-key projection (plan-review blocker #1).** Crossplane v2 XRs nest
> machinery under `spec.crossplane` (compositionRef, resourceRefs, …). The engine
> must project the XR's **user** spec into `input.spec` with the reserved
> `crossplane` key (and any legacy machinery keys) **removed**, so a *closed*
> `#Spec` doesn't conflict and `resources` can go concrete in-cluster. `#Spec`
> stays closed (authoring stays strict); the stripping happens in the engine, not
> the contract.

## 4. Runtime function (`internal/render` + `cuefn function`)

Port the spike's engine, adapted to the richer contract:

- `Engine` + `ModuleLoader` port; `OCILoader` (real) / `LocalLoader` (tests).
- **Fill** top-level `input: {spec, metadata, environment}` via JSON marshal
  (avoids float64-vs-int). **Read** `resources` (keyed map) + optional `status`.
  Validate `cue.Concrete(true)` over the rendered output.
- **Map output:** for each `resources[name]`, set
  `desired[resource.Name(name)] = {Resource: composed.New(object), Ready: ready→resource.Ready}`.
  Patch `status` onto the desired composite. (Readiness uses the SDK's
  `resource.Ready` enum; absent `ready` → unspecified.)
- **OCI load with deps:** `modconfig.NewResolver`/`NewRegistry` →
  `modregistry.GetModule` → `GetZip` → unzip → `load.Instances`. Transitive deps
  resolve through CUE's module machinery from `CUE_REGISTRY` (verify in the
  Phase-2 spike whether an explicit `load.Config{Registry}` is even needed, or
  CUE auto-creates the registry when nil). Honors `+insecure`.
- **Module cache (nonroot-safe).** Use CUE's module cache; set **`CUE_CACHE_DIR`**
  to a writable path and give the function pod a writable cache mount (the image
  runs as nonroot uid 65532, often with a read-only root fs). Don't assume the
  default `$HOME/.cache`.
- **fn.go:** read `Input.Module` (`path@version` — CUE refs are **semver**, not
  OCI digests), observed XR spec/metadata (reserved keys stripped, above), env
  from context key `apiextensions.crossplane.io/environment`.
- **Pipeline requirement:** `function-environment-configs` upstream populates env.
- **First slice ends** at a working `crossplane render` loop against an example
  module (publish module → run `cuefn function` → render → see resources+status).

## 5. Codegen: CUE → structural XRD (`internal/schema`) — de-risked

Proven recipe (validated with `apiextensions-apiserver`'s structural validator):

1. Load module; `Decode` `#API`; reduce to **definitions only** (encoder rejects
   regular top-level fields).
2. `openapi.Generate(defs, {ExpandReferences: false})` — `true` is buggy with
   bounded numbers.
3. **Inline `$ref`s** ourselves (cycle-detecting) → self-contained schemas.
4. Emit XRD (`apiextensions.crossplane.io/v2`) from `#API`: one served `version`,
   `openAPIV3Schema.properties.spec` from `#Spec`, `…properties.status` from
   `#Status` (when present), plus shortNames/categories/printerColumns.
5. (self-check) `NewStructural` + `ValidateStructural` to fail fast.

**Author guardrail:** no type-crossing disjunctions (`string|int`, struct unions)
in schema definitions — they become `oneOf`, which isn't a structural schema.
Same-type disjunctions (enums) are fine. Future: detect `string|int` →
`x-kubernetes-int-or-string`.

## 6. Configuration packaging & distribution (`internal/pkg`)

- `cuefn publish` emits a Configuration = generated XRD + generated Composition
  (pipeline: `function-environment-configs` → this function with the module ref
  as input) + `crossplane.yaml` (`meta.pkg.crossplane.io`, `dependsOn` this
  function).
- **Self-contained build/push:** assemble + push the Configuration xpkg from Go
  (go-containerregistry). Crossplane's xpkg builder lives under `internal/` and is
  **not importable**, so implement against the xpkg media-type/annotation spec
  (confirm + prototype in a packaging spike). **No dependency on an external
  `crossplane` CLI.**
- **Digest lock-step (plan-review blocker #2).** CUE loads modules by **semver**,
  not OCI digest, so we can't *reference* the module by digest. Instead: the CLI
  resolves the module version → manifest digest at generate time and records BOTH
  the semver ref and the expected digest in the Composition input; the runtime
  loads by semver and **verifies the fetched module's manifest digest matches the
  expected one**, rejecting drift. (Validate the exact mechanism in the Phase-2
  spike — recorded module sum vs. manifest-digest check.)
- **Registry split:** function image/packages need HTTPS (Crossplane pkg manager
  is HTTPS-only); CUE modules can use any OCI registry (incl. plain-HTTP local).

## 7. `cuefn validate` (bonus)

Unify a populated XR's spec against the module's `#Spec` via the CUE Go API;
report violations with CUE's error messages. Reuses the loader + schema.

## 8. CLI surface (cobra, one binary)

- `cuefn generate` — module → XRD (stdout/file).
- `cuefn publish` — module → Configuration xpkg → push (generate + package + push).
- `cuefn validate` — XR vs module `#Spec`.
- `cuefn render` — local eval (module + sample XR/env → resources+status), mirrors
  `crossplane render`.
- `cuefn function` — gRPC server (image entrypoint; `function.Serve`).

## 9. Repo structure (proposed)

```
cmd/cuefn/            # single binary; cobra root + subcommands incl. `function`
internal/cli/         # cobra command wiring (existing)
internal/render/      # CUE module → resources+status+readiness (shared)
internal/schema/      # CUE #Spec/#Status → structural XRD (shared)
internal/pkg/         # XRD+Composition → Configuration xpkg + push
internal/config/      # viper config (existing)
example/              # runnable demo module + XRD + Composition + XR + envconfig
package/              # function xpkg metadata (crossplane.yaml) for the function image
```

## 10. Supply chain / packaging the function (deferred)

The function ships as an **xpkg** (OCI image wrapping the runtime image). The repo
currently builds a plain runtime image via melange/apko; adapting that into a
signed function xpkg is a later slice. Keep melange/apko/cosign; add the xpkg
wrap + push + sign.

## 11. Stack / dependencies (proven together in the spike)

`cuelang.org/go v0.16.1` · `crossplane/function-sdk-go v0.7.1` ·
`crossplane-runtime/v2 v2.3.1` · `crossplane/apis/v2 v2.3.1` ·
`k8s.io/apimachinery v0.35.3` · `k8s.io/apiextensions-apiserver v0.35` (codegen
structural check) · `sigs.k8s.io/controller-tools v0.20` (input deepcopy) ·
`google.golang.org/protobuf` · `go-containerregistry` (xpkg) · cobra/viper (CLI).

## 12. Deferred (later slices)

Function **xpkg** packaging adaptation; multi-version XRDs + conversion;
int-or-string escape hatch; richer status/printer features beyond the basics.

## 13. Resolved decisions (session 001)

1. **Binary topology:** ONE `cuefn` binary; `cuefn function` is the gRPC server
   and image entrypoint.
2. **Output contract:** full — `resources` (with readiness) + XR `status` +
   function-driven readiness.
3. **Resource naming:** author-controlled, `resources` is a **keyed map**
   (`{<stableName>: {object, ready?}}`).
4. **API versioning:** **single** served version per module in v1 (status,
   printer columns, categories, shortNames included; multi-version deferred).
5. **Config packaging:** **self-contained** in `cuefn` (go-containerregistry;
   reuse Crossplane's xpkg builder if importable). No external `crossplane` CLI.
6. **Module deps:** **support transitive** OCI CUE deps in v1 (registry resolver
   wired into evaluation; cache by digest).
7. **First slice:** the **runtime engine + `cuefn function`** — end-to-end
   `crossplane render` loop against an example module.
