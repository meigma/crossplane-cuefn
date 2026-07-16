# Input CRD

The cuefn function reads its per-step configuration from an `Input` object
embedded in a Composition pipeline step. This page describes that type.

## Type

`Input` is `apiVersion: cuefn.meigma.io/v1beta1`, `kind: Input`. It is embedded
directly in a Composition pipeline step's `input`; the function decodes it with
`request.GetInput` before rendering.

```yaml
- step: cuefn
  functionRef:
    name: cuefn
  input:
    apiVersion: cuefn.meigma.io/v1beta1
    kind: Input
    module: cuefn.example/app@v0.1.0
    # expectedDigest: sha256:...
```

## Fields

| Field | Required | Description |
|-------|----------|-------------|
| `module` | yes | The CUE module to fetch and evaluate, in `path@version` semver form (e.g. `cuefn.example/app@v0.1.0`). Resolved against the function's `CUE_REGISTRY`; the version selects the module, not a digest. |
| `expectedDigest` | no | The OCI manifest digest the resolved module must match, in `sha256:...` form. When set, the loader verifies the fetched manifest digest against it and fails the render on a mismatch (the runtime half of the [digest lock-step](../explanation/digest-lockstep.md)). When empty, the module is resolved by version with no digest check. |

The function decodes **only** the typed fields above. The `Input` type embeds
`ObjectMeta`, but only so that controller-gen emits a CRD for it; any `metadata`
an author sets is ignored.

!!! note "Runtime resource data does not change this type"
    A module that [requires cluster resources](../how-to/require-resources.md)
    adds no fields here. Required resources arrive in the RunFunction *request*
    (Crossplane fetches them), and the selectors come from the *module output*
    (`out.requirements`) — neither is step configuration. The `Input` type stays
    `{module, expectedDigest}`, so adopting the feature needs no CRD regeneration.
    The same is true of [observed composed resources](module-contract.md#observing-composed-resources):
    Crossplane supplies them on the request, and explicit module opt-in controls
    whether cuefn exposes them under `out.input.observedResources`.

## Embedded CRD

A `CustomResourceDefinition` for `Input` (`inputs.cuefn.meigma.io`) ships inside
the Function xpkg. Crossplane reads it from the package to validate pipeline
step inputs. It is generated from the Go type by controller-gen and lives at
`internal/pkg/inputcrd/cuefn.meigma.io_inputs.yaml`;
[`cuefn publish-function`](cli.md#publish-function) embeds it in the Function
package.

A `cuefn publish`-generated Composition records both fields automatically: the
`module` ref you published and the `expectedDigest` it resolved at publish time.
