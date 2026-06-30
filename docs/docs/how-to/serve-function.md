# How to serve the function

`cuefn function` serves the composition function over gRPC. Crossplane connects
to it to render XRs. In a cluster it runs as the installed Function package; for
local development you run it yourself and point `crossplane render` at it.

## Serve locally for `crossplane render`

Crossplane's local render connects to a development function over plain gRPC, so
serve with `--insecure`:

```sh
export CUE_REGISTRY=localhost:5000+insecure
cuefn function --insecure --address :9443
```

Then run `crossplane render` against the example assets, with `functions.yaml`
marking cuefn as a `Development` runtime so render connects to your local server
instead of pulling an image:

```sh
crossplane render example/xr.yaml example/composition.yaml example/functions.yaml \
  --extra-resources example/environmentconfig.yaml
```

!!! note "Bind address in containers"
    When Crossplane's render runs the function in a separate container (the
    `integration` path), bind on `0.0.0.0` (e.g. `--address 0.0.0.0:9443`) so the
    function is reachable across the Docker bridge, not just on loopback.

## Serve with mTLS

Without `--insecure`, the function expects mTLS material in `--tls-certs-dir`:
`tls.key`, `tls.crt`, and the client CA `ca.crt`. This is how Crossplane connects
to the installed Function in-cluster; the package manager provisions the certs.

```sh
cuefn function --tls-certs-dir /tls
```

## Cache directory

The function caches fetched modules. By default it writes the cache to a temp dir
on the container's writable layer, so the nonroot, read-only-root runtime image
needs no extra configuration. Override the location with a flag or env var:

```sh
cuefn function --cache-dir /var/cache/cuefn
# or: CUE_CACHE_DIR=/var/cache/cuefn cuefn function
```

If you harden the Deployment with a read-only root filesystem (no writable
`/tmp`), mount an `emptyDir` and point `CUE_CACHE_DIR` at it.

See [Configuration & environment](../reference/configuration.md) for the cache
and registry details, and the [CLI reference](../reference/cli.md#function) for
every flag.
