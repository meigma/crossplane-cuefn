# How to build the runtime image

The function runtime image is built **without a Dockerfile**: melange compiles
the binary into a signed Wolfi apk, and apko assembles it into a minimal,
multi-arch, nonroot image (uid 65532, no shell, read-only root). The binary is
compiled with `-tags noxpkg` so the image stays lean — see
[the noxpkg split](../explanation/noxpkg-split.md).

## Build and run locally

The bundled mise task builds the host-arch image and loads it into Docker. It
uses melange's Docker runner, so Docker must be running:

```sh
mise run image-local                       # builds crossplane-cuefn:dev and image.tar
docker run --rm crossplane-cuefn:dev --version
```

The generic image has `/usr/bin/cuefn` as its entrypoint and `function` as its
default command, so a no-args run serves gRPC while callers can replace the
subcommand (for example, with `render`). Function xpkg assembly specializes this
config for Crossplane without changing the generic image.

## Use the image to package the Function

`mise run image-local` also writes `image.tar`, the runtime base
[`cuefn publish-function`](publish-function.md) layers over:

```sh
cuefn publish-function --runtime-image image.tar --output function.xpkg
```

The `package-function` mise task wraps this:

```sh
mise run package-function -- --package localhost:5005/function-cuefn:dev --insecure
```

## Notes

- The Wolfi base intentionally floats to the latest packages (fresh CA bundle and
  timezones, low CVE surface); exact resolved versions are recorded in the
  per-build SBOM and provenance attestation rather than pinned.
- `version`, `commit`, and `date` are stamped into the binary via melange
  `--vars-file`.
- The release pipeline builds each architecture natively (no QEMU), signs the
  image keyless with cosign, and attaches SBOM + SLSA L3 provenance.
