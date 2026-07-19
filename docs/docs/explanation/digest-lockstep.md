# The schema-runtime digest lock-step

A Configuration is built from a specific version of a CUE module. The runtime
should render against the **exact same bytes** that the XRD was generated from —
otherwise the schema the API server enforces and the transform the engine runs
could diverge even though they share a module path and version. This page
explains how cuefn keeps them aligned without referencing modules by digest.

## Why versions are not enough

CUE references modules by **semver**, not by content digest. A version like
`v0.1.0` is immutable *by convention*, but nothing stops a module from being
republished under the same tag with different content. If the runtime trusted the
version alone, a republished `v0.1.0` could change the transform out from under a
Configuration that was generated and validated against the original content.

Worse, CUE's own module cache is keyed by `module@version`. Once a version is
extracted, it is served from disk without re-resolving the tag. That is exactly
right for transitive dependencies (whose versions really are immutable in
practice), but it means a version-keyed cache would happily serve stale root
content after a republish.

## Verify after fetch, do not reference by digest

cuefn cannot simply reference the module by digest — CUE's loader works in
semver terms. Instead it **verifies the digest after fetch**:

1. **Author side (`cuefn publish`).** For an existing module, cuefn resolves the
   ref's **live** OCI manifest digest and records it in the Composition's cuefn
   `Input` as `expectedDigest`. With `--publish-module`, it instead prepares the
   exact canonical manifest locally, builds the Configuration around that digest,
   publishes the immutable module version, and re-resolves the tag before pushing
   the Configuration. Neither path takes a digest from the module cache.
2. **Runtime side (`cuefn function`).** The function reads `expectedDigest` and
   passes it to the loader's expectation map. On every load the loader
   re-resolves the tag, computes the fetched module's manifest digest, and
   compares it to the expectation. A mismatch fails the render with a clear
   error; an empty expectation skips the check.

The version still selects the module; the digest is a guard verified afterward,
not a reference.

## The digest-keyed cache

To make this efficient and tamper-evident, `OCILoader` keeps a second cache
beside CUE's version-keyed one, keyed by the **root** module's manifest digest
(`<cache>/cuefn-oci/<alg>/<digest>/`). Every load re-resolves the tag to its
current digest:

- **Same digest** → cache hit, served from disk.
- **New digest** (republished content) → cache miss, re-fetch, re-render.

A `ref → digest` pointer file records the last-known digest so a fresh process
can serve from cache when the registry is briefly unreachable, while a reachable
registry always resolves digest-fresh. A non-existent ref is an error; a
transport failure falls back to the pointer when a cached extraction exists.

## Scope

Only the **root** module ref is digest-verified. Transitive dependencies resolve
through CUE's version-keyed cache and are not individually digest-locked: their
versions are immutable in the dependency graph, so a per-dependency lock would
add cost without closing a real gap. The lock-step exists to pin the one module
a Configuration was built from to the one the runtime renders.
