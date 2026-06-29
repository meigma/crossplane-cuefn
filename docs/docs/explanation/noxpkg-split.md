# The lean runtime image (noxpkg)

The `cuefn` binary plays two roles: an **operator CLI** that packages and
publishes, and a **function runtime** that serves gRPC inside a Crossplane
cluster. These roles have very different dependency footprints. This page
explains why the build is split and how.

## The cost of packaging in the runtime

The packaging commands — `publish` and `publish-function` — import
`internal/pkg`, which uses the public `crossplane-runtime/v2/pkg/xpkg`
primitives. That import drags in a large dependency graph: sigstore/cosign, PKCS7,
and cloud-credential SDKs, none of which the runtime ever exercises. The function
never packages or signs anything; it only fetches modules and renders.

Left unchecked, that graph would be linked into the very binary the Function
package embeds and ships to every cluster node — pure dead weight in the runtime
image.

## The build-tagged seam

cuefn isolates the packaging commands behind a `//go:build` seam:

- `internal/cli/packaging.go` (`!noxpkg`) registers `publish` and
  `publish-function` and imports `internal/pkg`.
- `internal/cli/packaging_noxpkg.go` (`noxpkg`) is a no-op registration that
  imports nothing.

The `function`, `render`, `generate`, and `validate` commands are registered
unconditionally — they are the surfaces the runtime actually needs, and none of
them pull the packaging graph.

The runtime image binary is compiled with `-tags noxpkg` (in `melange.yaml`), so
it omits the two packaging commands and never links sigstore/cosign or the
cloud-credential SDKs. The full GoReleaser-built CLI keeps both commands.

## Measured impact

Dropping the packaging graph removes **12.1 MiB / 23%** of the stripped binary
(53.5 → 41.3 MiB). A `build-image-binary` moon task compiles the `noxpkg` path as
part of the default check graph, so the lean build cannot silently rot.

## What this means in practice

- The image you install as a Crossplane **Function** runs `cuefn function` from
  the lean binary. If you exec into it (you generally cannot — it has no shell),
  `publish` would not exist.
- To package a Configuration or Function, use the full `cuefn` CLI on a
  workstation or in CI, not the in-cluster runtime.
- The Function package image still serves `cuefn function` and installs as a
  Function — the split only removes commands the runtime never calls.
