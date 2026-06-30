# Module contract

A cuefn module is a CUE module that declares a platform API and a transform. The
same module drives both XRD codegen (authoring time) and rendering (runtime), so
the schema and the transform never drift. This page describes the contract the
CLI and the engine rely on.

All examples are the canonical example module shipped at `example/module/`.

## Module identity

The module is an ordinary CUE module with a `cue.mod/module.cue`:

```cue
module: "cuefn.example/app@v0"
language: {
	version: "v0.16.0"
}
source: {
	kind: "self"
}
deps: {
	"cue.dev/x/k8s.io@v0": {
		v:       "v0.7.0"
		default: true
	}
}
```

The module path and major version (`cuefn.example/app@v0`) form the
`path@version` reference used everywhere a `<module-ref>` is required.

The `deps` block records the module's dependencies. The example transform
instantiates its Kubernetes objects from the official Kubernetes schema
(`cue.dev/x/k8s.io`), so the module carries that as a dependency. Run
`cue mod tidy` in the module directory to populate `deps` after adding an import;
modern CUE records the resolved versions inline in `module.cue` (there is no
separate `cue.sum`). The dependency resolves from the default central registry
(`registry.cue.works`) — see [configuration](configuration.md#cue_registry).

## The API envelope: `#API`

`#API` is a concrete definition the CLI decodes to build the XRD envelope. A
single served version is supported.

```cue
#API: {
	group:   "platform.meigma.io"
	version: "v1alpha1"
	kind:    "XApp"
	plural:  "xapps"
	scope:   *"Namespaced" | "Cluster"
}
```

| Field | Required | Description |
|-------|----------|-------------|
| `group` | yes | API group of the composite resource. |
| `version` | yes | The single served API version. |
| `kind` | yes | Composite resource kind. |
| `plural` | yes | Plural name (XRD `names.plural`). |
| `scope` | yes | `"Namespaced"` or `"Cluster"`; a same-type disjunction with a default is allowed. |
| `shortNames` | no | Optional list of short names. |
| `categories` | no | Optional list of resource categories. |
| `printerColumns` | no | Optional additional printer columns. |

Multi-version XRDs and conversion are not supported; the module declares exactly
one served version.

## The spec schema: `#Spec`

`#Spec` is the authoritative, **closed** XR spec schema. It is the single source
of the spec's defaults and constraints: the CLI translates it to the XRD's
`.properties.spec`, and the runtime engine unifies the observed spec against it.

```cue
#Spec: {
	image:    string | *"ghcr.io/stefanprodan/podinfo:6.7.0"
	replicas: *1 | int & >=1 & <=10
}
```

Because `#Spec` is closed, fields the author did not declare are rejected. The
engine first projects Crossplane-reserved keys out of the observed spec so they
do not collide with the closed schema — see
[reserved-key projection](../explanation/reserved-key-projection.md).

Defaults (`*…`) and bounds (`>=1 & <=10`) live here once and apply in both
places: the API server fills XRD defaults, and the engine applies the same
defaults at render, so there is no drift between cluster-filled and
render-filled values.

## The status schema: `#Status` (optional)

`#Status` is the optional XR status schema. When present, the CLI emits
`.properties.status` in the XRD and the transform returns a value of this shape
to be patched onto the composite.

```cue
#Status: {
	ready: bool
	url:   string
}
```

When a module omits `#Status`, the generated XRD has no status schema and the
transform's `out.status` field is simply absent.

## The transform

The transform is a regular (non-definition) CUE field set nested under a single
top-level **`out`** field: the engine fills `out.input` and reads `out.resources`
and an optional `out.status`. The schema definitions (`#API`/`#Spec`/`#Status`)
stay top-level.

```cue
out: {
	input: {/* filled by the engine */}
	resources: {/* the composed Kubernetes objects */}
	status?: {/* optional; patched onto the composite */}
}
```

Nesting the transform under one `out` field keeps the well-known field set in a
single place that one closed definition can validate.

### Inputs the engine fills

```cue
out: input: {
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
```

| Fill point | Source |
|------------|--------|
| `out.input.spec` | The observed XR's `spec`, projected (reserved keys stripped) and unified against `#Spec`. |
| `out.input.metadata` | The XR's identifying metadata: `name` (and optional `namespace`). |
| `out.input.environment` | The merged `EnvironmentConfig` data from the pipeline context. |

Binding `out.input.spec: #Spec` is what ties render-time defaults/validation to
the same schema the XRD is generated from. The engine fills `out.input` by JSON
marshalling, so an integral spec value (e.g. a `float64` replica count from
YAML) unifies cleanly against a bounded integer field.

`out.input.environment` is populated in-cluster by an upstream
`function-environment-configs` step that merges the referenced
`EnvironmentConfig` into the pipeline context (context key
`apiextensions.crossplane.io/environment`). With `cuefn render`, the `--env` file
supplies it directly.

### Outputs the engine reads

The example instantiates each `object` from the official Kubernetes schema
(`cue.dev/x/k8s.io`) rather than hand-writing the maps, so `apiVersion`/`kind` are
supplied by the definition and any invalid field name, type, or shape fails at
render time instead of on apply. Writing the objects as plain maps is still valid;
importing the schema is the recommended practice.

```cue
import (
	appsv1 "cue.dev/x/k8s.io/api/apps/v1"
	corev1 "cue.dev/x/k8s.io/api/core/v1"
)

out: {
	resources: {
		deployment: {
			ready: "Ready"
			object: appsv1.#Deployment & { metadata: {/* ... */}, spec: {/* ... */} }
		}
		service: {
			ready: "NotReady"
			object: corev1.#Service & { metadata: {/* ... */}, spec: {/* ... */} }
		}
		config: {
			object: corev1.#ConfigMap & { metadata: {/* ... */}, data: {/* ... */} }
		}
	}

	status: #Status & {
		ready: true
		url:   "http://\(input.metadata.name).svc"
	}
}
```

**`out.resources`** is an author-keyed map. Each key is a stable, author-chosen name
used **verbatim** as the Crossplane composed-resource name. Each entry has:

| Field | Required | Description |
|-------|----------|-------------|
| `object` | yes | The fully rendered Kubernetes object. |
| `ready` | no | `"Ready"` or `"NotReady"`. An absent hint is treated as unspecified. |

The engine validates that every entry is concrete (`cue.Concrete(true)`); a
non-concrete `out.resources` (or `out.status`) is an error.

**`out.status`** is optional. When present it must be concrete and is patched onto
the composite.

### Readiness mapping

The module's readiness hint maps to the runtime/render readiness as follows:

| Module hint | `cuefn render` output | Crossplane readiness |
|-------------|-----------------------|----------------------|
| `"Ready"` | `"True"` | Ready |
| `"NotReady"` | `"False"` | Not ready |
| _(absent)_ | `Unspecified` | Unspecified |

## Author-time validation with the contract module

The contract above is published as a CUE module of **closed** definitions,
`github.com/meigma/crossplane-cuefn/contract@v0`. Importing it and unifying your
module against it validates the shape at author time (`cue vet` / editor) — a
misspelled or unknown field is rejected before render. The example module adopts
it:

```cue
import "github.com/meigma/crossplane-cuefn/contract@v0"

#API: contract.#API & {group: "platform.meigma.io", version: "v1alpha1", kind: "XApp", plural: "xapps"}
out: contract.#Transform & {input: {/* ... */}, resources: {/* ... */}, status: #Status & {/* ... */}}
```

`#Spec` and `#Status` are **not** wrapped — they are your own schemas and feed the
XRD codegen. Adoption is optional (the plain shape renders identically); it adds
the in-editor guarantee. See
[How to enforce the module contract](../how-to/enforce-the-contract.md).

## What a render produces

Rendering the example module against `example/xr.yaml` (`replicas: 2`) with no
environment yields:

```yaml
resources:
  config:
    object:
      apiVersion: v1
      kind: ConfigMap
      data:
        tier: unset
      metadata:
        name: demo
        labels: {app: demo, tier: unset}
    ready: Unspecified
  deployment:
    object:
      apiVersion: apps/v1
      kind: Deployment
      metadata:
        name: demo
        labels: {app: demo, tier: unset}
      spec:
        replicas: 2
        # ...
    ready: "True"
  service:
    object:
      apiVersion: v1
      kind: Service
      # ...
    ready: "False"
status:
  ready: true
  url: http://demo.svc
```

With an environment supplying `tier: production`, the `tier` label and the
ConfigMap datum become `production` instead of the `"unset"` default — proving
the environment flows through. Reproduce both with
[render locally](../how-to/render-locally.md).

## Runtime behaviors

Two engine behaviors are part of the contract, not optional add-ons.

### Reserved-key projection

Before unifying the observed spec against the closed `#Spec`, the engine strips
the Crossplane-reserved `crossplane` block and legacy machinery keys
(`compositionRef`, `resourceRefs`, `claimRef`, `environmentConfigRefs`, and
others). This lets `#Spec` stay closed without conflicting with fields the author
never declared. The stripping happens in the engine; the schema stays closed.
Full list and rationale in
[reserved-key projection](../explanation/reserved-key-projection.md).

### Digest verify-after-fetch

CUE references modules by **semver, not digest**. To lock the runtime to the
exact module bytes the Configuration was built from, the loader re-resolves the
tag on every load, computes the manifest digest, and verifies it against an
expected digest (from the Composition's `Input.ExpectedDigest`). A mismatch fails
the render. The publish-time half records the live digest. See the
[digest lock-step](../explanation/digest-lockstep.md).

## Authoring guardrails

The one real constraint on the schema definitions is that **type-crossing
disjunctions are not expressible**. A field whose type spans two kinds
(`string | int`, or a struct union `{a} | {b}`) generates an OpenAPI `oneOf`,
which Kubernetes structural schemas reject. `cuefn generate` detects this and
fails with a `DisjunctionError` naming the offending field.

Same-type disjunctions are fine and become an `enum`:

```cue
scope:    *"Namespaced" | "Cluster"   // ok — string enum
port:     80 | 443                    // ok — integer enum
mixed:    string | int                // rejected — DisjunctionError
```

This is a Kubernetes CRD limitation surfaced early, not a cuefn restriction.
