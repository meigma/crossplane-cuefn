# crossplane-cuefn

Define a Crossplane platform API as a single [CUE](https://cuelang.org) module —
its schema **and** the Kubernetes resources it renders — and crossplane-cuefn
gives you both halves of running it:

- the **`cuefn` CLI** generates the XRD and packages an installable Crossplane
  **Configuration** from that module, and
- a **[Crossplane v2](https://crossplane.io) composition function** pulls the same
  module from an OCI registry at runtime and renders your resources against each
  composite resource (XR).

One source of truth, in CUE, for both the API you install and the resources it
produces — no Go, no patch-and-transform pipelines.

> **Status: early development.** Every surface works end to end and is covered by
> CI, but expect them to change.

## Install

**Homebrew** (macOS/Linux):

```sh
brew install meigma/tap/cuefn
```

**Scoop** (Windows):

```sh
scoop bucket add meigma https://github.com/meigma/scoop-bucket && scoop install meigma/cuefn
```

**mise** (installs from GitHub releases, verifying their attestations):

```sh
mise use -g "github:meigma/crossplane-cuefn[bin=cuefn]"
```

**Nix**:

```sh
nix profile install github:meigma/crossplane-cuefn
```

**Shell** (Linux/macOS — verifies checksums, and SLSA provenance when `gh` is present):

```sh
curl -fsSL https://raw.githubusercontent.com/meigma/crossplane-cuefn/master/install.sh | bash
```

**Go**:

```sh
go install github.com/meigma/crossplane-cuefn/cmd/cuefn@latest
```

See [installing the CLI](https://meigma.github.io/crossplane-cuefn/how-to/install-the-cli/)
for prebuilt archives and provenance verification. The composition function is
published as a signed Crossplane **Function** package at
`ghcr.io/meigma/function-cuefn`, which Crossplane pulls automatically when you
install a generated Configuration.

## Example

One module is the whole platform API — the schema and the transform that renders
its resources:

```cue
// app.cue
package app

// #API and #Spec are the API: its identity, and a closed, defaulted spec schema.
#API: {
	group:   "platform.example.com"
	version: "v1alpha1"
	kind:    "XApp"
	plural:  "xapps"
}
#Spec: {
	image:    string
	replicas: *1 | int & >=1 & <=10 // defaults to 1, bounded 1–10
}

// out.* is the transform: the engine fills `out.input` from each XR, and reads
// back the resources you return.
out: {
	input: {
		spec: #Spec
		metadata: name: string | *"app"
	}
	resources: deployment: object: {
		apiVersion: "apps/v1"
		kind:       "Deployment"
		metadata: name: input.metadata.name
		spec: {
			replicas: input.spec.replicas
			selector: matchLabels: app: input.metadata.name
			template: {
				metadata: labels: app: input.metadata.name
				spec: containers: [{name: "app", image: input.spec.image}]
			}
		}
	}
}
```

Render it against an XR locally — no cluster, no registry:

```sh
cuefn render example.com/app@v0 --dir . --xr xr.yaml
```

Then package it as an installable Configuration — the XRD, a Composition wired to
the function, and the function as a dependency — in one command:

```sh
cuefn publish example.com/app@v0.1.0 --package registry.example.com/xapp:v0.1.0
```

`kubectl apply` that package as a Crossplane `Configuration` and the `XApp` API is
live on your cluster. The
[Quickstart](https://meigma.github.io/crossplane-cuefn/quickstart/) walks the
whole loop — module to a running XR — step by step.

## Commands

| Command | What it does |
| --- | --- |
| `cuefn render` | Render a module against an XR locally — no cluster. |
| `cuefn generate` | Emit the structural XRD from the module's schema. |
| `cuefn validate` | Check an XR's spec against the module's schema before apply. |
| `cuefn publish` | Package an installable Crossplane Configuration from the module. |
| `cuefn publish-function` | Package the composition function as a Crossplane Function. |
| `cuefn function` | Serve the composition function over gRPC (the runtime). |

See the [CLI reference](https://meigma.github.io/crossplane-cuefn/reference/cli/)
for every flag.

## Documentation

Full documentation: **<https://meigma.github.io/crossplane-cuefn>**

- **[Quickstart](https://meigma.github.io/crossplane-cuefn/quickstart/)** — write a
  module, publish it, install a Configuration, and watch an XR render.
- **How-to guides** — one task at a time: render locally, generate an XRD,
  validate, publish, serve the function, and configure the runtime.
- **Reference** — the [module contract](https://meigma.github.io/crossplane-cuefn/reference/module-contract/),
  the CLI, configuration & environment, and the Input CRD.
- **Explanation** — the design: one module / two outputs, the digest lock-step,
  and the lean runtime image.

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) for local setup, the toolchain, and the
release process. Security issues go through [SECURITY.md](SECURITY.md).

## License

Licensed under either of [Apache License, Version 2.0](LICENSE-APACHE) or
[MIT license](LICENSE-MIT) at your option (SPDX: `Apache-2.0 OR MIT`). Unless you
state otherwise, any contribution you submit for inclusion in this project shall
be dual licensed as above, without any additional terms or conditions.
