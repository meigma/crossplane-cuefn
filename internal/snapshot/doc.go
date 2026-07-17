// Package snapshot loads the YAML fixtures cuefn accepts as render inputs —
// XRs, environments, and required/observed cluster-object snapshots — and
// matches required resources against a module's emitted requirements the way
// Crossplane delivers them.
//
// File and directory semantics deliberately mirror Crossplane CLI's render
// flags: directories contribute only their immediate .yaml/.yml children, and
// multi-document files are split with the Kubernetes YAML reader so a "---"
// inside a value is not mistaken for a separator. Parse* functions accept raw
// bytes so callers with non-file sources (embedded fixtures, archives) share
// the exact same decoding; Load* functions wrap them with path handling.
package snapshot
