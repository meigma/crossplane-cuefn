// Package common holds the shared test infrastructure for the relocated
// integration and end-to-end suites: a throwaway OCI registry, environment gates,
// path/port helpers, binary/gRPC serving helpers, synthetic runtime bases, stream
// parsing, render-result accessors, and typed Crossplane fixtures.
//
// It is a non-_test.go package that imports testing on purpose so the same helpers
// are reachable from internal/test/integration, internal/test/e2e, and any staying
// unit _test.go file. It depends only on the exported APIs of internal/pkg and
// internal/render, and must never import internal/cli, internal/function,
// internal/schema, or internal/e2e (which would create an import cycle).
package common

// ExampleModuleRef is the module ref the example Composition references and the
// integration tests publish to a throwaway registry.
const ExampleModuleRef = "cuefn.example/app@v0.1.0"

// DevImage is the local image tag produced by `mise run image-local`.
const DevImage = "crossplane-cuefn:dev"

// Zeros returns a string of n '0' runes, used to build placeholder digests.
func Zeros(n int) string {
	b := make([]byte, n)
	for i := range b {
		b[i] = '0'
	}
	return string(b)
}
