# How to render a module locally

`cuefn render` evaluates a module against an XR and prints the result — no
cluster, no `crossplane` CLI, no registry. Use it as the inner dev loop while
writing a module.

## Render from a local directory

Point `--dir` at the module directory and `--xr` at an observed XR:

```sh
cuefn render cuefn.example/app@v0 \
  --dir example/module \
  --xr example/xr.yaml
```

The output is YAML: a `resources` map keyed by the module's resource names (each
with its rendered `object` and a `ready` value) and an optional `status`.

## Supply an environment

In-cluster, `input.environment` comes from an upstream
`function-environment-configs` step. Locally, supply it with `--env`; the file's
top-level keys become `input.environment`:

```sh
echo 'tier: production' > /tmp/env.yaml
cuefn render cuefn.example/app@v0 \
  --dir example/module \
  --xr example/xr.yaml \
  --env /tmp/env.yaml
```

With the example module, this changes the rendered `tier` label and ConfigMap
datum from the `"unset"` default to `production`.

## Render from the registry

Omit `--dir` to fetch the module over OCI instead. Set `CUE_REGISTRY` first
(add `+insecure` for a plain-HTTP registry):

```sh
export CUE_REGISTRY=localhost:5000+insecure
cuefn render cuefn.example/app@v0.1.0 --xr example/xr.yaml
```

## Notes

- A spec that violates `#Spec` (out-of-bounds, missing required, wrong enum)
  fails the render with the offending field path — the same evaluation the
  served function uses.
- `cuefn render` reuses the runtime engine, so what you see here matches what
  Crossplane renders in-cluster.

See the [CLI reference](../reference/cli.md#render) for every flag and the
[module contract](../reference/module-contract.md) for the output shape.
