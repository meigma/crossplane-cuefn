# Contributing

Thank you for your interest in contributing to crossplane-cuefn.
For private vulnerability reporting, use [SECURITY.md](SECURITY.md) instead of public channels.

## Reporting Bugs

Report non-security bugs through GitHub issues.
Include the following details when possible:

- version, commit, or environment details
- steps to reproduce
- expected behavior
- actual behavior
- logs, screenshots, or a minimal reproduction

If you are reporting a security issue, stop and follow [SECURITY.md](SECURITY.md) instead.

## Pull Requests

Contributors should:

1. Keep changes focused and scoped to a single problem.
2. Add or update tests when behavior changes.
3. Update documentation when user-facing behavior changes.
4. Use Conventional Commit subjects, such as `feat: add config loader` or `fix: handle empty input`.
5. Make sure `moon run root:check` passes before requesting review.

## Local Setup

The toolchain comes entirely from [mise](https://mise.jdx.dev) — Go, Moon, Python
+ uv (for the docs), `golangci-lint`, the `crossplane` and `cue` CLIs,
`melange`/`apko`/`cosign` for releases, and the test-suite tools. Versions are
pinned in `mise.toml` and locked with per-platform checksums in `mise.lock`, so
`mise install` **fails closed** if a tool lacks a verified entry for your
platform.

```sh
mise install            # provision the pinned toolchain (run mise trust once if prompted)
moon run root:check     # the aggregate gate: format, lint, build, unit tests, docs --strict
```

Moon is the task front door, running each tool as a `system` binary on PATH:

```sh
moon run root:format
moon run root:lint
moon run root:build
moon run root:test
go run ./cmd/cuefn --version
```

To bump or add a tool, edit `mise.toml`, then
`mise lock --platform linux-x64,linux-arm64,macos-x64,macos-arm64` and commit both
files. See [the local toolchain how-to](https://meigma.github.io/crossplane-cuefn/how-to/local-toolchain/)
for details.

## Tests

`root:test` (in `root:check`) runs the fast unit tests. The heavy,
infrastructure-gated suites are separate moon tasks run in their own CI
workflows: `render-test`, `oci-test`, `publish-test`, `funcpkg-test`, and
`schema-test` (Docker, the `crossplane`/`chainsaw` CLIs, `cosign`, `syft`,
`setup-envtest`) in the `integration` workflow, plus `e2e-test`
(`kind`/`kubectl`/`helm`/`chainsaw` and the `crossplane-cuefn:dev` image) in the
`e2e` workflow.

## Building the runtime image

The function runtime image is built without a Dockerfile: melange compiles the
binary into a signed Wolfi apk, and apko assembles a minimal, multi-arch, nonroot
image. Build and run it locally with the bundled mise task (Docker must be
running):

```sh
mise run image-local                       # builds crossplane-cuefn:dev and image.tar
docker run --rm crossplane-cuefn:dev --version
```

See [how to build the runtime image](https://meigma.github.io/crossplane-cuefn/how-to/build-image/)
and [the lean runtime image (noxpkg)](https://meigma.github.io/crossplane-cuefn/explanation/noxpkg-split/).

## Releases and the supply chain

Releases are automated. Release Please reads Conventional Commit subjects to cut
release PRs; merging one tags the release and runs the pipeline — GoReleaser
(binaries, checksums, SBOMs), the native-runner melange/apko image, and the
Crossplane Function package, all signed keyless with cosign and carrying SLSA
Build L3 provenance attestations.

Two components are versioned independently:

- the **product** (the `cuefn` binary, image, and Function package) on `v*` tags, and
- the **module contract** (`contract/`) on `contract/v*` tags, published to the CUE
  Central Registry via GitHub OIDC. Its major is welded to the product's major
  (both `v0`).

Keep release-impacting commits clear: routine `docs`, `ci`, `test`, and `chore`
commits cut no release.
