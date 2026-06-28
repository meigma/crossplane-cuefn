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

