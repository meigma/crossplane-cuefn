# One module, two outputs

cuefn is built around a single idea: a platform API should be defined **once**,
and that one definition should drive both the cluster's view of the API and the
machinery that renders it. This page explains why the project is shaped that way
and what follows from it.

## The drift problem

A Crossplane platform API has two artifacts that must agree:

- an **XRD** (`CompositeResourceDefinition`) — the schema the API server
  enforces on every XR, including defaults and validation, and
- a **Composition** — the transform that turns an XR into composed resources.

When these are authored separately, they drift. A field added to the transform
but not the schema is silently unvalidated; a default in the schema that differs
from the transform's assumption produces surprising output. Keeping two
hand-written documents in lock-step is a maintenance tax.

## One CUE module

cuefn collapses both artifacts into one CUE module. The module declares the
schema as CUE definitions (`#API`, `#Spec`, optional `#Status`) and the transform
as regular fields (`input`, `resources`, `status`). The transform binds
`input.spec: #Spec`, so the schema is literally the same value the transform
renders against.

From that one module, two outputs are produced:

- **Authoring time.** `cuefn generate` / `cuefn publish` reduce the module to its
  definitions, translate `#Spec`/`#Status` to OpenAPI, and wrap the result as a
  structural XRD inside a Configuration package.
- **Runtime.** The function fetches the same module and evaluates the transform
  against the observed XR, returning composed resources.

Because both outputs come from one source, the schema the API server enforces
and the schema the engine renders against cannot disagree. A default lives in
`#Spec` once; the API server fills it on the XR and the engine applies the same
default at render.

## The shared core

This duality is reflected in the code. The "CUE module → resources" logic is a
single core package (`internal/render`) behind a `ModuleLoader` port. The runtime
function, the local `render` path, the `generate`/`validate` codegen, and
`publish` all reach the module through that one core — adapters at the edges
(OCI loader, gRPC server, CLI) never duplicate evaluation logic. What you see
from `cuefn render` locally is what Crossplane renders in-cluster, because it is
the same engine.

## Two registries {#two-registries}

A consequence worth calling out: cuefn deals with two distinct kinds of OCI
artifact, and they have different registry requirements.

- The **CUE module** is pushed with `cue mod publish`. It may live on any OCI
  registry, including a plain-HTTP local one (`CUE_REGISTRY=…+insecure`).
- The **Configuration** and **Function** packages are Crossplane xpkgs, pushed by
  `cuefn publish` / `cuefn publish-function`. Crossplane's package manager pulls
  them over **HTTPS only**, so their destination registry must serve HTTPS.

Conflating the two is a common setup mistake. The module registry and the package
registry are independent; only the package registry has to be HTTPS.

A module may also carry its own **public dependencies** — the example imports the
official Kubernetes schema (`cue.dev/x/k8s.io`) to build its objects, and the
cuefn **module contract** (`github.com/meigma/crossplane-cuefn/contract@v0`, an
independently-versioned CUE module also on central) to validate its shape at
author time. Those resolve from the **central registry** (`registry.cue.works`),
which is CUE's default when `CUE_REGISTRY` is unset, so they need no extra
configuration. If you point `CUE_REGISTRY` at a private module registry, use the
prefix form (`your.org=registry.internal`) so central stays the fallback for
public dependencies — see
[configuration](../reference/configuration.md#cue_registry).
