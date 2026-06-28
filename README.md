# crossplane-cuefn

A [Crossplane v2](https://crossplane.io) composition function, written in Go,
that renders Kubernetes resources from **CUE modules distributed over an OCI
registry** — paired with `cuefn`, an operator CLI that turns one CUE module into
a versioned Crossplane Configuration.

> **Status: early development.** The toolchain, CI, and release scaffolding are
> in place and exercised. The composition-function runtime (`cuefn function`) and
> the local author command (`cuefn render`) work end to end against a CUE module
> over OCI; Configuration packaging and XRD codegen are still under construction.
> Expect the surfaces described below to change.

## Commands

- `cuefn function` serves the Crossplane v2 composition function over gRPC. It
  fetches the CUE module named in each pipeline step's `Input` from the OCI
  registry (`CUE_REGISTRY`), evaluates it against the observed XR and the merged
  `EnvironmentConfig`, and returns the rendered objects as desired composed
  resources plus a patched composite status. Crossplane connects over mTLS by
  default; pass `--insecure` for local `crossplane render`. This is the image's
  default command.
- `cuefn render <module-ref> --xr <file> [--env <file>] [--dir <dir>]` evaluates a
  module against an XR locally and prints the rendered resources and status as
  YAML — cluster-free and crossplane-CLI-free. `--dir` serves the module from a
  local directory offline; otherwise it is fetched over OCI.

The `example/` directory contains a runnable render loop: an XRD, a pipeline
Composition (`function-environment-configs` → `cuefn`), an XR, an
`EnvironmentConfig`, and a `functions.yaml`. With the module published to a
registry and `cuefn function --insecure` running, `crossplane render` over those
assets produces the composed resources and an env-driven field sourced from the
`EnvironmentConfig`.

## The idea

A platform author writes **one** CUE module that contains both:

- a **schema** for the composite resource's `spec`, and
- a **transform** that renders the desired Kubernetes objects from that spec
  plus any merged `EnvironmentConfig`.

That single module is the source of truth for two things:

- **At runtime**, the composition function pulls the module from an OCI registry
  and evaluates it against each composite resource (XR), returning the rendered
  objects as Crossplane desired resources.
- **At authoring time**, the `cuefn` CLI translates the module's schema to an
  OpenAPI/XRD via CUE, and packages it — together with a Composition wired to the
  function — as a versioned Crossplane Configuration ready to install.

The result: define a platform API once, in CUE; publish the module and an
auto-generated Configuration; install and instantiate XRs.

## Loading modules from an OCI registry

The render engine (`internal/render`) is pure and offline; where a module's bytes
come from is a pluggable `ModuleLoader` port. Two adapters ship today:

- `LocalLoader` serves a fixed directory — used for tests and offline
  development.
- `OCILoader` fetches a module (and its transitive CUE dependencies) from an OCI
  registry using the CUE module-registry protocol.

`OCILoader` is configured with `OCIConfig` and honours the standard CUE
environment:

- **`CUE_REGISTRY`** selects the registry, including the `+insecure` suffix for a
  plain-HTTP (e.g. localhost) registry. It is read from `OCIConfig.Env`; when
  `Env` is nil the process environment is used. An explicit
  `load.Config{Registry}` is **not required** for dependency resolution — when it
  is nil CUE's loader auto-creates a registry from `CUE_REGISTRY` over the same
  `Env`. `OCILoader` supplies its own registry anyway so it is built from the
  loader's controlled `Env` (with `CUE_CACHE_DIR` forced) and shares one modcache
  with the digest-aware root client.
- **`CUE_CACHE_DIR`** (or `OCIConfig.CacheDir`, which takes precedence) points the
  module cache at a **writable, non-`$HOME`** path. The function image runs
  nonroot on a read-only root filesystem, so the cache must live somewhere
  writable — set this to an `emptyDir`/tmp path in that deployment.
- **`OCIConfig.Expect`** optionally pins the expected manifest digest for a ref.
  CUE references modules by **semver, not digest**, so the loader verifies the
  fetched module's manifest digest against the expected value *after* fetch and
  rejects a mismatch — the runtime half of the schema↔runtime digest lock-step.

### Two caches, by design

A CUE module version is immutable by convention, and CUE's own module cache is
keyed by `module@version`: once a version is extracted it is served from disk
without re-resolving the tag. That is correct for **transitive dependencies**
(resolved through CUE's version-keyed cache), but it cannot detect a **root
module** whose content changed under the same tag.

So `OCILoader` handles the root module digest-aware: every load re-resolves the
tag to its current manifest digest and keys a small loader-owned extraction cache
(under `<cache>/cuefn-oci/`) by that digest. Republished content under the same
tag yields a new digest, a cache miss, and a re-render. A `ref → digest` pointer
lets a fresh process serve the last-known digest from cache when the registry is
unreachable, while a reachable registry always resolves digest-fresh. A
non-existent ref propagates as an error; a transport failure falls back to the
cache when possible.

## Local bootstrap

Prerequisites:

- [mise](https://mise.jdx.dev) — provisions every pinned tool from `mise.toml` +
  `mise.lock`: Go, Moon, Python + uv (for the MkDocs docs project), the
  `golangci-lint` CLI, and `melange`/`apko`/`cosign` for releases. Run
  `mise install` once; there is nothing else to install by hand.

Tool versions live in `mise.toml`; `mise.lock` records a per-platform download URL
and checksum for each (and, for the aqua-backed CLIs, cosign/SLSA/GitHub-attestation
verification). `mise install` runs with `locked = true`, so it **fails closed** if a
tool lacks a pre-resolved, checksummed entry for the current platform. Moon runs every
task against these tools as `system` binaries on PATH and manages no toolchain itself.
To bump a tool, edit its version in `mise.toml`, run
`mise lock --platform linux-x64,linux-arm64,macos-x64,macos-arm64`, and commit
`mise.toml` + `mise.lock`.

## Common tasks

Moon is the standard task front door:

```sh
moon run root:format
moon run root:lint
moon run root:build
moon run root:test
moon run root:check
```

CI runs the same aggregate check:

```sh
moon ci --summary minimal
```

The `cuefn` binary today is a thin Cobra/Viper scaffold:

```sh
go run ./cmd/cuefn --version
go test ./...
```

`cmd/cuefn` stays thin; `internal/cli` owns command construction; Viper-backed
flags can also be supplied through `CUEFN_*` environment variables.

## Container image

The function runtime image is built **without a Dockerfile**:
[melange](https://github.com/chainguard-dev/melange) compiles the binary into a
signed [Wolfi](https://github.com/wolfi-dev) apk (`melange.yaml`), and
[apko](https://github.com/chainguard-dev/apko) assembles it into a minimal,
multi-arch, non-root runtime image (`apko.yaml`) — uid 65532, ca-certificates,
tzdata, no shell. Each architecture builds natively (no QEMU). Build and run it
locally with the bundled mise task (it uses melange's Docker runner, so Docker
must be running):

```sh
mise run image-local              # build the host-arch image, load as crossplane-cuefn:dev
docker run --rm crossplane-cuefn:dev --version
```

The Wolfi base intentionally floats to the latest packages (fresh CA bundle and
timezones, low CVE surface); the exact resolved versions are recorded in the
per-build SBOM and provenance attestation rather than pinned. `version`, `commit`,
and `date` are stamped into the binary via melange `--vars-file`.

> Note: a Crossplane composition function ships as an `xpkg` (an OCI image
> wrapping an embedded runtime image). The current release pipeline produces a
> standard runtime image; xpkg packaging is a planned follow-up.

## CI and security

The default CI workflow keeps permissions minimal, pins external actions, disables checkout credential persistence, and delegates checks to Moon.
It uses GitHub-hosted dependency caches for Go, golangci-lint, and uv download artifacts.
The docs workflow builds the MkDocs site on pull requests and deploys `docs/build` to GitHub Pages from the default branch.
The scheduled security scan workflow builds the local container image weekly, scans it for high/critical fixed vulnerabilities, and uploads SARIF results to GitHub code scanning.
Dependabot covers GitHub Actions, the root Go module, and the docs uv project.

Repository settings live in `.github/repository-settings.toml`: immutable
releases, private vulnerability reporting, signed commits, squash-only merges,
GitHub Pages workflow publishing, and protected tags.

## Release layer

Releases are automated through Release Please, GoReleaser (binaries, checksums,
SBOMs), and the native-runner melange/apko container build, with keyless cosign
signing and SLSA Build L3 provenance attestations generated in an isolated
`attest.yml` reusable workflow. Binaries are installable with `ghd` per the root
`ghd.toml`.

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) for local setup and the pull request workflow.

## Security

See [SECURITY.md](SECURITY.md) for supported versions and the private vulnerability reporting path.

## License

Licensed under either of [Apache License, Version 2.0](LICENSE-APACHE) or
[MIT license](LICENSE-MIT) at your option (SPDX: `Apache-2.0 OR MIT`). Unless you
explicitly state otherwise, any contribution intentionally submitted for
inclusion in this project shall be dual licensed as above, without any additional
terms or conditions.
