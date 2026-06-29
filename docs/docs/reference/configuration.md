# Configuration & environment

This page describes how `cuefn` resolves configuration: the `CUEFN_*` flag
environment variables, the CUE environment (`CUE_REGISTRY`, `CUE_CACHE_DIR`), and
the digest lock-step settings the runtime honors.

## Flag environment variables (`CUEFN_*`)

`cuefn` binds its flags through Viper with the prefix `CUEFN`. Any flag can be
set through an environment variable: uppercase the flag name and replace `-` and
`.` with `_`.

| Flag | Environment variable |
|------|----------------------|
| `--message` | `CUEFN_MESSAGE` |
| `--address` | `CUEFN_ADDRESS` |
| `--cache-dir` | `CUEFN_CACHE_DIR` |
| `--tls-certs-dir` | `CUEFN_TLS_CERTS_DIR` |
| `--insecure` | `CUEFN_INSECURE` |

Precedence follows Viper's defaults: an explicitly set command-line flag wins
over the environment variable, which wins over the flag's default. `AutomaticEnv`
is enabled, so the variables are read without per-flag binding.

## CUE environment

The OCI loader honors the standard CUE environment. When `cuefn function` or any
fetching command runs, these select the registry and cache.

### `CUE_REGISTRY`

Selects the OCI registry CUE modules are fetched from. **You usually do not need
to set it.** When `CUE_REGISTRY` is unset, the central registry
(`registry.cue.works`) is the default, so a module's public dependencies — for
example the official Kubernetes schema `cue.dev/x/k8s.io` the example module
imports — resolve automatically.

Set `CUE_REGISTRY` only to point at a **private or override** registry. To serve
your own modules from a private registry while keeping central as the default for
everything else, use the **prefix form** — central stays the catch-all, so no
trailing `,registry.cue.works` is needed:

```sh
# your.org/* from the private registry; cue.dev/x/k8s.io etc. still from central
export CUE_REGISTRY='your.org=registry.internal'
```

A **bare** value *replaces* the central default for all modules — the deliberate
single-registry override for an air-gapped mirror or an offline/local registry. It
supports the `+insecure` suffix for a plain-HTTP registry (e.g. a localhost
throwaway):

```sh
export CUE_REGISTRY=localhost:5000+insecure
```

The longest matching module-path prefix wins, so you can combine entries (a
comma-separated list) to route different prefixes to different registries.

`CUE_REGISTRY` selects the **CUE module** registry only. It is independent of the
registry a Configuration or Function package is pushed to, which must be HTTPS
(Crossplane's package manager is HTTPS-only).

### `CUE_CACHE_DIR` / `--cache-dir`

Points the module cache at a writable directory. `--cache-dir` is available on the
fetching commands (`render`, `generate`, `validate`, `publish`, `function`) and
(like `OCIConfig.CacheDir` in code) takes precedence over `CUE_CACHE_DIR`, which
takes precedence over the OS user-cache default.

!!! warning "Nonroot runtime requires a writable cache"
    The function runtime image runs as a nonroot user (uid 65532) on a read-only
    root filesystem. CUE cannot write its module cache to `$HOME`, so the cache
    **must** be a writable non-`$HOME` path. In a Deployment, mount an
    `emptyDir` and point the cache at it with `--cache-dir` or `CUE_CACHE_DIR`.

### Two caches, by design

`OCILoader` keeps two caches under the resolved cache root:

- **CUE's own module cache**, keyed by `module@version`. It serves transitive
  dependencies, whose versions are immutable, so a version-keyed cache is
  correct.
- **The loader's digest-keyed extraction cache** (under `<cache>/cuefn-oci/`),
  keyed by the **root** module's manifest digest. Republished content under the
  same tag yields a new digest, a cache miss, and a re-render. A `ref → digest`
  pointer file lets a fresh process serve the last-known digest from cache when
  the registry is unreachable.

This split is why a version-keyed cache cannot mask republished root-module
content. The reasoning is in the
[digest lock-step](../explanation/digest-lockstep.md).

## Digest lock-step settings

The schema↔runtime lock-step is configured from one field on each side.

| Side | Setting | Effect |
|------|---------|--------|
| Author (publish) | resolved manifest digest | `cuefn publish` resolves the module's live digest and records it in the Composition's cuefn `Input`. |
| Runtime (serve) | `Input.ExpectedDigest` | The function folds it into the loader's `Expect` map; the loader verifies the fetched manifest digest against it and fails the render on a mismatch. |

When `ExpectedDigest` is empty the module is resolved by version with no digest
check. Only the root module ref is verified; transitive dependencies are
immutable by version and are not digest-checked. The publish-time digest
resolution always queries the live registry (no cache fallback), so the recorded
digest is the real published one.
