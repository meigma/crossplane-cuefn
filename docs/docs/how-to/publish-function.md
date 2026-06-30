# How to publish the Function

`cuefn publish-function` assembles the cuefn **Function** package over the
apko-built runtime image and pushes it. The package image *is* the runtime image
plus a `package.yaml` layer (Function metadata + the embedded `Input` CRD), so it
both installs as a Crossplane Function and serves `cuefn function`.

Most operators install the published `ghcr.io/meigma/function-cuefn` package and
never run this command. Use it to ship your own build of the function.

## Prerequisite: build the runtime image

`publish-function` layers over a runtime image base. Build one locally first:

```sh
mise run image-local      # builds crossplane-cuefn:dev and image.tar
```

See [Build the runtime image](build-image.md).

## Write a local package (dry run)

`--output` writes a single-arch `.xpkg` instead of pushing — useful for
inspection:

```sh
cuefn publish-function --runtime-image image.tar --output function.xpkg
crossplane xpkg extract --from-xpkg function.xpkg -o out.gz   # accepts it; embeds the Input CRD
```

The bundled mise task wraps the same call:

```sh
mise run package-function -- --output function.xpkg
```

## Push a single-arch package

```sh
cuefn publish-function \
  --runtime-image image.tar \
  --package registry.example.com/function-cuefn:v0.1.1
```

## Push a multi-arch index

A Function package image runs on every node arch, so a real install needs a
multi-arch index. Pass `--runtime-image` once per arch base:

```sh
cuefn publish-function \
  --runtime-image image-amd64.tar \
  --runtime-image image-arm64.tar \
  --package registry.example.com/function-cuefn:v0.1.1
```

The destination registry must serve **HTTPS** (Crossplane's package manager is
HTTPS-only). `--insecure` is for a plain-HTTP throwaway registry only.

## Signing

Signing is left to cosign at the edge — keyless in CI, a local key for proofs —
not done by this command. The release pipeline builds the multi-arch index over
the signed runtime image, signs it keyless, and attaches SBOM and SLSA
provenance. Full flag list: [CLI reference](../reference/cli.md#publish-function).
