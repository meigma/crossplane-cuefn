// Package pkg assembles an installable Crossplane Configuration xpkg image from
// the YAML a single CUE module generates, and pushes it to an OCI registry.
//
// It is the author-time packaging core. The pure half builds the three packaged
// objects — the XRD (reused from internal/schema), a pipeline-mode Composition
// wired to the cuefn function, and the meta.pkg.crossplane.io Configuration —
// marshals them into the package.yaml YAML stream, and tars that stream into an
// xpkg image layer with the correct media types and the io.crossplane.xpkg=base
// annotation. All registry/network IO lives at the edge in push.go, preserving
// the hexagonal boundary the rest of the codebase follows.
//
// # xpkg packaging spike (P5/P6 de-risk)
//
// crossplane's own builder, github.com/crossplane/crossplane/internal/xpkg, is
// under internal/ and therefore NOT importable from this module. The escape
// hatch is github.com/crossplane/crossplane-runtime/v2/pkg/xpkg, which IS public
// (and already in the module graph via the v2 runtime dependency). It exports
// exactly the contract primitives needed: the StreamFile ("package.yaml"),
// PackageAnnotation ("base") and AnnotationKey ("io.crossplane.xpkg") constants,
// Layer (tar a YAML stream into a layer and record its annotation as a config
// label), and AnnotateLayers (propagate those labels to real OCI layer
// annotations). Building on those plus go-containerregistry — empty.Image as the
// base, mutate to append the layer and set the config, then a remote.Write push
// — yields a package that `crossplane xpkg inspect` accepts, without any
// dependency on the crossplane CLI for building.
//
// The same BuildXpkgImage shape generalizes to a Function xpkg (P6): a Function
// meta document as the package layer over a runtime image base, rather than an
// empty base. The Configuration path here proves the layout for both.
package pkg
