// Package e2e is the edge adapter for the kind-based end-to-end harness. It boots
// a throwaway kind cluster, stands up the two OCI registries the loop needs (an
// HTTPS, CA-trusted registry for the Crossplane xpkgs and a plain-HTTP
// "+insecure" registry for the CUE modules the function fetches at render time),
// installs Crossplane, and drives the full author->publish->install->instantiate
// ->reconcile loop.
//
// Everything here shells out to Docker, kind, kubectl, helm, and chainsaw, or
// talks to live OCI registries — all external-adapter concerns. It is built only
// under the `e2e` build tag and self-skips when its tools (or Docker) are absent,
// so the core packages and the default check graph never depend on it. The render
// engine (internal/render) stays pure and is reached only through its existing
// ports.
package e2e
