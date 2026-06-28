// Package dep is a dependency-only module published to the test registry. It is
// never loaded directly; the consumer module imports the values it exports to
// exercise transitive OCI dependency resolution.
package dep

// Image is the container image the consumer renders into its Deployment. Its
// value living in a separate OCI module is the whole point: a successful render
// proves the loader resolved the dependency from the registry.
Image: "ghcr.io/cuefn/dep:1.2.3"

// Port is a numeric value the consumer reads, proving non-string values cross
// the module boundary intact.
Port: 8421
