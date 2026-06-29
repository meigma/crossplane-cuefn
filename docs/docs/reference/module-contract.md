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
```

The module path and major version (`cuefn.example/app@v0`) form the
`path@version` reference used everywhere a `<module-ref>` is required.

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
transform's `status` field is simply absent.

## The transform

The transform is a regular (non-definition) CUE field set. The engine fills a
top-level `input` field and reads a top-level `resources` map and an optional
`status`.

### Inputs the engine fills

```cue
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
```

| Fill point | Source |
|------------|--------|
| `input.spec` | The observed XR's `spec`, projected (reserved keys stripped) and unified against `#Spec`. |
| `input.metadata` | The XR's identifying metadata: `name` (and optional `namespace`). |
| `input.environment` | The merged `EnvironmentConfig` data from the pipeline context. |

Binding `input.spec: #Spec` is what ties render-time defaults/validation to the
same schema the XRD is generated from. The engine fills `input` by JSON
marshalling, so an integral spec value (e.g. a `float64` replica count from
YAML) unifies cleanly against a bounded integer field.

`input.environment` is populated in-cluster by an upstream
`function-environment-configs` step that merges the referenced
`EnvironmentConfig` into the pipeline context (context key
`apiextensions.crossplane.io/environment`). With `cuefn render`, the `--env` file
supplies it directly.

### Outputs the engine reads

```cue
resources: {
	deployment: {
		ready: "Ready"
		object: { apiVersion: "apps/v1", kind: "Deployment", /* ... */ }
	}
	service: {
		ready: "NotReady"
		object: { apiVersion: "v1", kind: "Service", /* ... */ }
	}
	config: {
		object: { apiVersion: "v1", kind: "ConfigMap", /* ... */ }
	}
}

status: #Status & {
	ready: true
	url:   "http://\(input.metadata.name).svc"
}
```

**`resources`** is an author-keyed map. Each key is a stable, author-chosen name
used **verbatim** as the Crossplane composed-resource name. Each entry has:

| Field | Required | Description |
|-------|----------|-------------|
| `object` | yes | The fully rendered Kubernetes object. |
| `ready` | no | `"Ready"` or `"NotReady"`. An absent hint is treated as unspecified. |

The engine validates that every entry is concrete (`cue.Concrete(true)`); a
non-concrete `resources` (or `status`) is an error.

**`status`** is optional. When present it must be concrete and is patched onto
the composite.

### Readiness mapping

The module's readiness hint maps to the runtime/render readiness as follows:

| Module hint | `cuefn render` output | Crossplane readiness |
|-------------|-----------------------|----------------------|
| `"Ready"` | `"True"` | Ready |
| `"NotReady"` | `"False"` | Not ready |
| _(absent)_ | `Unspecified` | Unspecified |

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
