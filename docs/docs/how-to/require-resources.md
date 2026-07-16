# How to require cluster resources

Sometimes a module needs to read an existing cluster object before it can render
— a `ConfigMap` whose data it copies, a `Secret` it references, a sibling
resource it depends on. cuefn supports this the way native Crossplane composition
functions do: a module **emits** selectors under `out.requirements`, Crossplane
fetches the matching objects, and they reappear under
`out.input.requiredResources` keyed by the same author-chosen name.

This guide shows how to declare a requirement, guard a data-dependent resource on
the fetched objects, test it offline with `cuefn render`, and grant the RBAC the
in-cluster fetch needs. For *why* the loop converges, see
[Required resources and the fixpoint](../explanation/required-resources-fixpoint.md);
for the field-by-field contract, see
[the module contract](../reference/module-contract.md#requiring-cluster-resources).

These fields live in the contract module from **v0.2.0** onward. If you adopt the
contract for author-time validation, run `cue mod tidy` so `deps` resolves
`github.com/meigma/crossplane-cuefn/contract@v0` to v0.2.0 or newer.

!!! note "Use observed resources for composed children"
    Do not emit a requirement just to read a Job, Deployment, or other object the
    same module already creates. Crossplane supplies composed children through
    `out.input.observedResources` without a fetch loop or read RBAC. See
    [derive readiness from observed resources](derive-readiness-from-observed-resources.md).

## 1. Emit a requirement

A requirement is one selector under `out.requirements`, keyed by a name you
choose. The engine enforces that each entry sets **exactly one** of `matchName`
(an exact `metadata.name`) or `matchLabels` (a label subset):

```cue
import "github.com/meigma/crossplane-cuefn/contract@v0"

out: contract.#Transform & {
	requirements: cfg: contract.#Requirement & {
		apiVersion: "v1"
		kind:       "ConfigMap"
		matchName:  input.spec.configName
		namespace:  input.metadata.namespace
	}
	// ...input / resources / status...
}
```

| Selector field | Required | Meaning |
|----------------|----------|---------|
| `apiVersion` | yes | API version of the kind to fetch. |
| `kind` | yes | Kind to fetch. |
| `matchName` | one of | Exact `metadata.name`. |
| `matchLabels` | one of | Map of labels; an object matches when it carries all of them. |
| `namespace` | no | Namespace to read. Omit to read a cluster-scoped kind, or to list a namespaced kind across all namespaces. |

!!! warning "Requirements must be a pure function of stable inputs"
    Build the selector only from `input.spec`, `input.metadata`, and
    `input.environment` — never from `input.requiredResources` (the fetched
    data). Crossplane re-invokes the function until two consecutive responses
    emit byte-identical requirements (the `proto.Equal` fixpoint). A selector
    that depends on what was fetched changes every pass and never converges;
    Crossplane then fails the composite with `requirements didn't stabilize`,
    and `cuefn render` mirrors that with its own stabilization error. See
    [the fixpoint explanation](../explanation/required-resources-fixpoint.md).

## 2. Guard the data-dependent resource

Read the fetched objects back under `input.requiredResources[name]` and only
build the resource that needs them when something was delivered. The engine seeds
an empty list for every requirement name before it reads `out.resources`, so the
guard collapses to a concrete (empty) result on the first pass — no author
boilerplate needed.

The idiomatic guard is a `for` comprehension, which emits nothing on the seeded
empty list and one resource per fetched object on later passes:

```cue
out: resources: {
	// No-ops on the seeded [] (first pass -> out.resources is a concrete {});
	// emits one Deployment per fetched ConfigMap once Crossplane delivers them.
	for i, cm in input.requiredResources.cfg {
		"deployment-\(i)": {
			ready: "Ready"
			object: {
				apiVersion: "apps/v1"
				kind:       "Deployment"
				metadata: name: "\(input.metadata.name)-\(i)"
				spec: image: cm.data.image
			}
		}
	}
}
```

To gate a single resource on the first match instead, guard it with
`if len(...) > 0` and read element `[0]`:

```cue
out: resources: {
	if len(input.requiredResources.cfg) > 0 {
		app: {
			ready: "Ready"
			object: {
				apiVersion: "apps/v1"
				kind:       "Deployment"
				metadata: name: input.metadata.name
				spec: image: input.requiredResources.cfg[0].data.image
			}
		}
	}
}
```

Either form works because the engine guarantees
`input.requiredResources.cfg` exists as a concrete list. The deciding factor is
the concrete list, not the guard expression.

### Supporting cluster-scoped XRs

A selector that writes `namespace: input.metadata.namespace` becomes
non-concrete when the XR is cluster-scoped (no namespace), which fails the
render. If your module supports `scope: "Cluster"`, default `metadata.namespace`
and emit the selector's `namespace` only when it is set:

```cue
out: {
	input: metadata: {
		name:      string
		namespace: string | *""
	}
	requirements: cfg: contract.#Requirement & {
		apiVersion: "v1"
		kind:       "ConfigMap"
		matchName:  input.spec.configName
		if input.metadata.namespace != "" {
			namespace: input.metadata.namespace
		}
	}
}
```

An omitted selector `namespace` reads cluster-wide; scope reads to the XR's own
namespace by writing `namespace: input.metadata.namespace` as above.

## 3. Test it offline

`cuefn render` always prints the requirements a module emits, so you can see what
to supply even before you have any objects. The commands below render the module
you built in steps 1-2 — the one that emits the `cfg` requirement and guards a
Deployment on `input.requiredResources.cfg` — served from its own directory with
`--dir`. (The shipped `example/module` emits no requirements, so point `--dir` at
your own module here.)

Rendering it against an XR named `myapp` with `spec.configName: app-cfg` in
namespace `default` prints the selector from step 1:

```sh
cuefn render cuefn.example/myapp@v0 \
  --dir ./my-module \
  --xr ./xr.yaml
```

```yaml
requirements:
  cfg:
    apiVersion: v1
    kind: ConfigMap
    matchName: app-cfg
    namespace: default
resources: {}
```

With nothing delivered, the `for` comprehension over the seeded empty
`input.requiredResources.cfg` emits nothing, so the guarded Deployment is absent
and `resources` is empty — exactly the first pass Crossplane sees.

Now supply a flat bag of real cluster objects with `--required-resources`. It
accepts a single YAML file or a directory of them (multi-document files are
fine); each object is matched against the emitted selectors — files are not
keyed by name:

```sh
cat > /tmp/cfg.yaml <<'EOF'
apiVersion: v1
kind: ConfigMap
metadata:
  name: app-cfg
  namespace: default
data:
  image: ghcr.io/example/app:1.2.3
EOF

cuefn render cuefn.example/myapp@v0 \
  --dir ./my-module \
  --xr ./xr.yaml \
  --required-resources /tmp/cfg.yaml
```

The ConfigMap's name and namespace match the emitted selector, so it is delivered
to `input.requiredResources.cfg` and the guarded Deployment renders with
`spec.image` taken from `cm.data.image`:

```yaml
requirements:
  cfg:
    apiVersion: v1
    kind: ConfigMap
    matchName: app-cfg
    namespace: default
resources:
  deployment-0:
    ready: Ready
    object:
      apiVersion: apps/v1
      kind: Deployment
      metadata:
        name: myapp-0
      spec:
        image: ghcr.io/example/app:1.2.3
```

`cuefn render` runs the same fixed two-pass loop Crossplane converges to and
fails with a stabilization error if the module's requirements are not pure (see
the [CLI reference](../reference/cli.md#render)).

## 4. Grant the in-cluster RBAC

In-cluster, required resources are fetched by Crossplane's **core controller**
ServiceAccount — not the function pod. For every kind a module can request, the
operator must grant that controller `get`/`list`/`watch`. The portable way is a
`ClusterRole` carrying the aggregation label, which Crossplane's aggregated
`crossplane` ClusterRole absorbs:

```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: crossplane-cuefn-required-configmaps
  labels:
    rbac.crossplane.io/aggregate-to-crossplane: "true"
rules:
  - apiGroups: [""]
    resources: ["configmaps"]
    verbs: ["get", "list", "watch"]
```

This is an **operator responsibility**: cuefn ships no RBAC for arbitrary
requested kinds, because it cannot know which kinds an author will request.
Without the rule the fetch silently returns nothing, the delivered bucket stays
empty, and any guarded resource never renders — a silent under-render. See
[why reads go through the core controller](../explanation/required-resources-fixpoint.md#reads-and-rbac).

This grants the **read** side (`get`/`list`/`watch` on requested objects).
Composing native kinds beyond the core workloads needs the separate **write**
grant — see
[configure the function runtime](configure-the-runtime.md#grant-rbac-for-composed-native-kinds).

## See also

- [Required resources and the fixpoint](../explanation/required-resources-fixpoint.md)
  — why the loop converges and the function stays pure.
- [Module contract: requiring cluster resources](../reference/module-contract.md#requiring-cluster-resources)
  — the exact `#Requirement` / `#Input.requiredResources` / `#Transform.requirements` fields.
- [How to render a module locally](render-locally.md) — the inner dev loop this
  guide builds on.
