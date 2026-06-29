# How to render a module locally

`cuefn render` evaluates a module against an XR and prints the result — no
cluster and no `crossplane` CLI. Use it as the inner dev loop while writing a
module.

## Render from a local directory

Point `--dir` at the module directory and `--xr` at an observed XR:

```sh
cuefn render cuefn.example/app@v0 \
  --dir example/module \
  --xr example/xr.yaml
```

The output is YAML: a `resources` map keyed by the module's resource names (each
with its rendered `object` and a `ready` value) and an optional `status`.

`--dir` serves the module from disk. A self-contained module renders fully
offline; a module that imports public schemas (the example imports
`cue.dev/x/k8s.io`) resolves those from the central registry on the first run and
caches them, so it stays cluster-free. Use `--cache-dir` to point that cache at a
specific writable directory.

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

Omit `--dir` to fetch the module itself over OCI. Set `CUE_REGISTRY` only to point
at a private or local module registry (add `+insecure` for plain-HTTP); public
dependencies still resolve from the central registry by default:

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
