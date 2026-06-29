# How to generate an XRD

`cuefn generate` turns a module's `#API`/`#Spec`/`#Status` into a structural
Crossplane v2 XRD. Use it to inspect the schema the API server will enforce, or
to commit a generated XRD as an artifact.

## Generate from a local directory

```sh
cuefn generate cuefn.example/app@v0 --dir example/module
```

The XRD is written to stdout. The envelope (`group`, `kind`, `plural`, `scope`,
`version`) comes from `#API`; the spec and status schemas (with defaults and
bounds) come from `#Spec` and `#Status`.

## Write to a file

```sh
cuefn generate cuefn.example/app@v0 \
  --dir example/module \
  --output example/xrd.yaml
```

## Generate from the registry

Omit `--dir` to fetch the module over OCI (set `CUE_REGISTRY` first):

```sh
export CUE_REGISTRY=localhost:5000+insecure
cuefn generate cuefn.example/app@v0.1.0 -o xrd.yaml
```

## If generation fails on a disjunction

If a `#Spec`/`#Status` field crosses types (`string | int`, or a struct union),
`cuefn generate` fails with a `DisjunctionError` naming the field — Kubernetes
structural schemas cannot express a type-crossing `oneOf`. Same-type enums
(`"a" | "b"`, `80 | 443`) are fine. See the
[authoring guardrails](../reference/module-contract.md#authoring-guardrails).

The XRD is also generated (and embedded in a Composition) by
[`cuefn publish`](publish-configuration.md). Use `generate` when you only want the
schema.
