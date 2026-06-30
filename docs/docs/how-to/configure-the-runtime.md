# How to configure the function runtime (in-cluster)

The installed cuefn Function resolves CUE modules itself, at render time. Public
dependencies on the CUE central registry work out of the box, but two in-cluster
concerns need operator configuration: pointing the function at a **non-central
module registry**, and granting RBAC for **composed kinds beyond the core
workloads**. Both are configured through standard Crossplane mechanisms.

## Point the function at your module registry

The function fetches the module recorded in the Composition from its own
`CUE_REGISTRY`, which defaults to the central registry (`registry.cue.works`). A
module published anywhere else — a private, internal, or local registry — must be
routed by injecting `CUE_REGISTRY` into the function pod through a
`DeploymentRuntimeConfig` bound to the Function with `runtimeConfigRef`. Crossplane
names the function's runtime container `package-runtime`.

```yaml title="runtime.yaml"
apiVersion: pkg.crossplane.io/v1beta1
kind: DeploymentRuntimeConfig
metadata:
  name: cuefn-runtime
spec:
  deploymentTemplate:
    spec:
      selector: {}
      template:
        spec:
          containers:
            - name: package-runtime
              env:
                - name: CUE_REGISTRY
                  value: "your.org=registry.internal"
```

Use the **prefix form** (`your.org=registry.internal`) so the central registry
stays the catch-all fallback for public dependencies such as `cue.dev/x/k8s.io`; a
bare value (`registry.internal`) *replaces* central and breaks those. For a
plain-HTTP local or in-cluster registry, add the `+insecure` suffix
(`your.org=registry.internal:5000+insecure`). Multiple teams share one
`DeploymentRuntimeConfig` by comma-joining prefixes
(`teamA=regA,teamB=regB`) — a per-team apply of a single bare value would clobber
the others.

Bind it to the function. When the function was auto-installed via a Configuration's
`dependsOn`, patch the installed object (do not hand-install a second Function — it
would poison the package Lock):

```sh
kubectl apply -f runtime.yaml
kubectl patch function.pkg.crossplane.io meigma-function-cuefn --type merge \
  -p '{"spec":{"runtimeConfigRef":{"name":"cuefn-runtime"}}}'
```

When you install the Function yourself, set `runtimeConfigRef` directly in its
manifest — see the committed `example/deploy/functions.yaml` for the full shape.

!!! note "The cache needs no configuration"
    The function writes its module cache to a temp directory on the container's
    writable layer, so a default install renders without any cache setup. Only a
    hardened Deployment with `readOnlyRootFilesystem: true` (no writable `/tmp`)
    needs `CUE_CACHE_DIR` pointed at a mounted `emptyDir`, as
    `example/deploy/functions.yaml` shows. See
    [Configuration & environment](../reference/configuration.md) for the cache
    precedence.

### Troubleshooting "module not found"

If an XR fails to reconcile with `module … not found`, the function could not
resolve the module's registry. The usual cause is `CUE_REGISTRY` set only in your
local shell (for `cuefn render` / `cue mod publish`) but never injected into the
pod. Apply the `DeploymentRuntimeConfig` above and confirm it landed:

```sh
kubectl get deployment -n crossplane-system -l pkg.crossplane.io/function=meigma-function-cuefn \
  -o jsonpath='{.items[0].spec.template.spec.containers[?(@.name=="package-runtime")].env}'
```

## Grant RBAC for composed native kinds

A Composition can render any Kubernetes object, but Crossplane's controller only
creates the kinds its aggregated `crossplane` ClusterRole permits. The default
role covers the common workload and core kinds (Deployments, Services, ConfigMaps,
Secrets, ServiceAccounts); composing anything beyond that — a StatefulSet, an
Ingress, a Job, a PVC — fails the reconcile with `…is forbidden: …cannot create`.

Grant the controller the missing kinds with a `ClusterRole` carrying the
aggregation label Crossplane absorbs (confirm against your Crossplane version which
kinds the default role already covers, so the grant adds only what is missing):

```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: crossplane-cuefn-composed-kinds
  labels:
    rbac.crossplane.io/aggregate-to-crossplane: "true"
rules:
  - apiGroups: ["apps"]
    resources: ["statefulsets"]
    verbs: ["create", "update", "patch", "delete", "get", "list", "watch"]
  - apiGroups: ["networking.k8s.io"]
    resources: ["ingresses"]
    verbs: ["create", "update", "patch", "delete", "get", "list", "watch"]
```

This is the **write** side of RBAC — granting the controller permission to create
composed objects. It is distinct from the **read** side that
[required resources](require-resources.md#4-grant-the-in-cluster-rbac) need, which
grants `get`/`list`/`watch` on requested objects. Both use the same
aggregate-to-crossplane label but cover different kinds and verbs.

## See also

- [Serve the function](serve-function.md) — running the function locally.
- [Publish a Configuration](publish-configuration.md) — building the package whose
  `dependsOn` installs the function.
- [Configuration & environment](../reference/configuration.md) — `CUE_REGISTRY`
  and `CUE_CACHE_DIR` precedence.
