# How to validate an XR

`cuefn validate` checks a populated XR's `spec` against a module's `#Spec` before
it reaches a cluster, applying `#Spec` defaults and reporting the first violation
with its field path.

## Validate against a local module

```sh
cuefn validate example/xr.yaml --dir example/module
```

On success it prints `example/xr.yaml: valid` to stderr and exits zero. A passing
validate proves the spec satisfies `#Spec` with defaults applied.

## Validate against a published module

Fetch the module over OCI with `--module` (set `CUE_REGISTRY` first):

```sh
export CUE_REGISTRY=localhost:5000+insecure
cuefn validate xr.yaml --module cuefn.example/app@v0.1.0
```

## Interpreting failures

The command exits non-zero on the first violation and names the offending field.
Typical violations against the example `#Spec` (where `replicas` is bounded
`1..10`):

- `replicas: 0` or `replicas: 99` — out of bounds.
- an undeclared spec field — rejected by the closed `#Spec`.
- a missing required field with no default.

Because validation uses the same CUE evaluation as the runtime engine, a spec
that passes here will not be rejected for schema reasons at render time. See the
[CLI reference](../reference/cli.md#validate).
