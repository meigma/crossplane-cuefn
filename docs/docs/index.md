---
title: crossplane-cuefn
slug: /
description: Crossplane v2 composition function that renders Kubernetes resources from CUE modules.
---

# crossplane-cuefn

A [Crossplane v2](https://crossplane.io) composition function that renders
Kubernetes resources from **CUE modules distributed over an OCI registry**,
paired with `cuefn`, an operator CLI that turns one CUE module into a versioned
Crossplane Configuration.

## What it does

A platform author writes **one** CUE module that holds both halves of a platform
API:

- a closed **schema** for the composite resource's `spec` (`#API` / `#Spec` /
  optional `#Status`), and
- a **transform** that renders the desired Kubernetes objects from that spec plus
  any merged `EnvironmentConfig`.

That single module is the source of truth for two outputs:

- **At authoring time**, the `cuefn` CLI translates the module's schema to a
  structural XRD and packages it — with a Composition wired to the function — as
  an installable Crossplane **Configuration**.
- **At runtime**, the composition function pulls the same module from the OCI
  registry and evaluates it against each composite resource (XR), returning the
  rendered objects as Crossplane desired resources and a patched status.

Define the platform API once, in CUE; publish the module and the generated
Configuration; install it and instantiate XRs.

## Where to go next

- **[Quickstart](quickstart.md)** — write a module, publish it, build a
  Configuration, install it, and observe a rendered XR end to end.
- **How-to guides** — focused runbooks for one task at a time:
  [render locally](how-to/render-locally.md),
  [generate an XRD](how-to/generate-xrd.md),
  [validate an XR](how-to/validate-xr.md),
  [publish a Configuration](how-to/publish-configuration.md),
  [publish the Function](how-to/publish-function.md), and
  [serve the function](how-to/serve-function.md).
- **Reference** — the authoritative facts:
  [module contract](reference/module-contract.md),
  [CLI](reference/cli.md),
  [configuration & environment](reference/configuration.md), and the
  [Input CRD](reference/input.md).
- **Explanation** — the design behind the surfaces:
  [one module, two outputs](explanation/one-module-two-outputs.md),
  the [digest lock-step](explanation/digest-lockstep.md),
  [reserved-key projection](explanation/reserved-key-projection.md), and the
  [lean runtime image](explanation/noxpkg-split.md).

The [README](https://github.com/meigma/crossplane-cuefn#readme) covers the
development workflow and the supply-chain layer.
