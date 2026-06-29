# Quickstart

In this tutorial we will take a platform API from a blank directory to a running
Crossplane resource: write one CUE module, render it locally, publish it, build
and install a Configuration, install the function, and watch an XR render. By the
end you will have seen the whole cuefn loop end to end.

The repository ships a complete example under `example/`. We will build the same
shape from scratch so each step is visible.

## Prerequisites

- `cuefn` and `cue` on your PATH. With this repo: `mise install` (see
  [local toolchain](how-to/local-toolchain.md)), then `mise exec -- ...` or build
  `cuefn` with `go build -o bin/cuefn ./cmd/cuefn`.
- An OCI registry for the CUE module. A plain-HTTP local registry is fine.
- For the cluster steps: a Crossplane v2 cluster and an **HTTPS** registry for
  the Configuration and Function packages.

## Step 1 — Write the module

Create a module directory with a `cue.mod/module.cue`:

```cue title="cue.mod/module.cue"
module: "cuefn.example/app@v0"
language: {
	version: "v0.16.0"
}
source: {
	kind: "self"
}
```

We will instantiate the Kubernetes objects from the official schema, which adds a
`deps` block. You do not write it by hand — `cue mod tidy` (below) fills it in.

Declare the platform API and the spec schema. `#API` is the XRD envelope; `#Spec`
is the closed, authoritative spec schema (with defaults and bounds):

```cue title="api.cue"
package app

#API: {
	group:   "platform.meigma.io"
	version: "v1alpha1"
	kind:    "XApp"
	plural:  "xapps"
	scope:   *"Namespaced" | "Cluster"
}

#Spec: {
	image:    string | *"ghcr.io/stefanprodan/podinfo:6.7.0"
	replicas: *1 | int & >=1 & <=10
}

#Status: {
	ready: bool
	url:   string
}
```

Now the transform. The engine fills `input` (spec, metadata, environment); we
read it and return an author-keyed `resources` map and a `status`. We instantiate
each object from the official Kubernetes schema (`cue.dev/x/k8s.io`) so
`apiVersion`/`kind` come from the definition and an invalid object is caught here,
at render time, not on apply:

```cue title="transform.cue"
package app

import (
	appsv1 "cue.dev/x/k8s.io/api/apps/v1"
	corev1 "cue.dev/x/k8s.io/api/core/v1"
)

input: {
	spec: #Spec
	metadata: {
		name:       string | *"app"
		namespace?: string
		...
	}
	environment: {
		tier: string | *"unset"
		...
	}
}

_name: input.metadata.name
_tier: input.environment.tier

resources: {
	deployment: {
		ready: "Ready"
		object: appsv1.#Deployment & {
			metadata: {name: _name, labels: {app: _name, tier: _tier}}
			spec: {
				replicas: input.spec.replicas
				selector: matchLabels: app: _name
				template: {
					metadata: labels: {app: _name, tier: _tier}
					spec: containers: [{
						name:  "app"
						image: input.spec.image
						ports: [{containerPort: 9898}]
					}]
				}
			}
		}
	}
	config: {
		object: corev1.#ConfigMap & {
			metadata: {name: _name, labels: {app: _name, tier: _tier}}
			data: tier: _tier
		}
	}
}

status: #Status & {
	ready: true
	url:   "http://\(_name).svc"
}
```

Binding `input.spec: #Spec` is the key move: the schema the XRD is generated from
is the same value the transform renders against, so the two never drift.

Resolve the new dependency, which records it in `cue.mod/module.cue`:

```sh
cue mod tidy
```

This fetches `cue.dev/x/k8s.io` from the default central registry
(`registry.cue.works`) — no `CUE_REGISTRY` needed for public dependencies — and
writes a `deps` block. There is no separate `cue.sum`; modern CUE records the
resolved versions inline.

## Step 2 — Render it locally

Before touching a registry or cluster, render the module against an XR. Create a
sample XR:

```yaml title="xr.yaml"
apiVersion: platform.meigma.io/v1alpha1
kind: XApp
metadata:
  name: demo
  namespace: default
spec:
  image: ghcr.io/stefanprodan/podinfo:6.7.0
  replicas: 2
```

Render it from the directory:

```sh
cuefn render cuefn.example/app@v0 --dir . --xr xr.yaml
```

`--dir` serves the module from disk; its `cue.dev/x/k8s.io` dependency resolves
from the central registry on the first run (and is cached after), so this stays
cluster-free. You will see a `resources` map (a Deployment with `replicas: 2`
marked `"True"`, a ConfigMap marked `Unspecified`) and a `status`. Add `--env` to
see the environment flow through:

```sh
echo 'tier: production' > env.yaml
cuefn render cuefn.example/app@v0 --dir . --xr xr.yaml --env env.yaml
```

The `tier` label and ConfigMap datum change from `unset` to `production`. This
`--dir` render is your fast inner loop — iterate here until the output is right.

## Step 3 — Publish the module

Publish the module to its CUE registry. A plain-HTTP local registry needs the
`+insecure` suffix:

```sh
export CUE_REGISTRY=localhost:5000+insecure
cue mod publish v0.1.0
```

!!! warning "Publish the module before the Configuration"
    `cuefn publish` records the module's **registry** digest. Always
    `cue mod publish` first; otherwise the Configuration can pin a digest that
    does not match your source. This ordering avoids the `--dir` footgun.

## Step 4 — Build and push the Configuration

Now turn the published module into an installable Configuration. The Configuration
and Function packages go to an **HTTPS** registry (Crossplane's package manager is
HTTPS-only), distinct from the CUE module registry:

```sh
cuefn publish cuefn.example/app@v0.1.0 \
  --package registry.example.com/xapp-configuration:v0.1.0
```

`cuefn publish` generates the XRD, builds a Composition wired to
`function-environment-configs` → `cuefn`, records the module ref **and** its
resolved manifest digest (the runtime [digest lock-step](explanation/digest-lockstep.md)),
and pushes the package. You can confirm the package parses:

```sh
# --from-daemon reads from the local Docker daemon, so pull the pushed package first
docker pull registry.example.com/xapp-configuration:v0.1.0
crossplane xpkg extract --from-daemon registry.example.com/xapp-configuration:v0.1.0 -o out.gz
```

## Step 5 — Install the Function and the Configuration

Install the cuefn Function and your Configuration into the cluster:

```yaml title="install.yaml"
apiVersion: pkg.crossplane.io/v1
kind: Function
metadata:
  name: cuefn
spec:
  package: ghcr.io/meigma/function-cuefn:v0
---
apiVersion: pkg.crossplane.io/v1
kind: Configuration
metadata:
  name: xapp
spec:
  package: registry.example.com/xapp-configuration:v0.1.0
```

```sh
kubectl apply -f install.yaml
```

Crossplane installs the Configuration's XRD and Composition, and resolves the
function dependency the Configuration declared.

## Step 6 — Instantiate an XR and observe the result

Optionally supply an `EnvironmentConfig` the Composition references:

```yaml title="environment.yaml"
apiVersion: apiextensions.crossplane.io/v1beta1
kind: EnvironmentConfig
metadata:
  name: app-environment
data:
  tier: production
```

Apply it and the XR (the same `xr.yaml` from Step 2):

```sh
kubectl apply -f environment.yaml
kubectl apply -f xr.yaml
```

Watch the composite and its composed resources appear:

```sh
kubectl get xapp demo -o yaml          # status.ready, status.url patched by the module
kubectl get deployment,configmap -l app=demo
```

The Deployment carries `replicas: 2` from the XR spec, and the `tier` label and
ConfigMap datum read `production` from the `EnvironmentConfig` — the same output
you saw locally in Step 2, now rendered by the in-cluster function.

## What you built

One CUE module became two artifacts — an XRD-bearing Configuration and the
transform the runtime evaluates — from a single source of truth. To go deeper:

- The [module contract](reference/module-contract.md) — the full schema and
  transform surface.
- [One module, two outputs](explanation/one-module-two-outputs.md) — why it is
  shaped this way.
- The how-to guides for each command in isolation.
