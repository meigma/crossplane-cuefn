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

