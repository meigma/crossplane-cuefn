# How to set up the local toolchain

Every pinned tool comes from [mise](https://mise.jdx.dev) via `mise.toml` +
`mise.lock`: Go, Moon, Python + uv (for these docs), `golangci-lint`, the
`crossplane` and `cue` CLIs, and `melange`/`apko`/`cosign` for releases.

## Install everything

```sh
mise install
```

That provisions every tool for your platform. `mise install` runs with
`locked = true`, so it **fails closed** if a tool lacks a pre-resolved,
checksummed entry for your platform in `mise.lock` — there is nothing to install
by hand.

If mise reports the config is untrusted, run `mise trust` once in the repo.

## Run tools

Moon runs every task against these tools as `system` binaries on PATH. Run a
pinned tool directly through mise when you want to be sure the pinned version
wins over anything else on PATH:

```sh
mise exec -- cue version
mise exec -- crossplane version
```

## Common Moon tasks

```sh
moon run root:format
moon run root:lint
moon run root:build
moon run root:test
moon run root:check     # the aggregate gate, incl. docs:build --strict
```

The heavy, tool-gated suites (Docker / crossplane / chainsaw / cosign) are not
part of `root:check`; they run as dedicated tasks (`render-test`, `oci-test`,
`publish-test`, `funcpkg-test`, `schema-test`) and in the separate `integration`
workflow.

## Bump or add a tool

Edit the version in `mise.toml`, re-lock for all platforms, and commit both
files together:

```sh
mise lock --platform linux-x64,linux-arm64,macos-x64,macos-arm64
```

The `cue` CLI is pinned here for the [Quickstart](../quickstart.md)'s
`cue mod publish` step; it matches the `cuelang.org/go` version the engine
evaluates with.
