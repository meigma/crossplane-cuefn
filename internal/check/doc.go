// Package check is the static module-health core behind `cuefn check`: it
// verifies canonical CUE formatting, that the module evaluates cleanly without
// requiring concreteness (the programmatic `cue vet -c=false ./...`), and that
// the module's XRD generates — optionally comparing it against a reviewed
// golden file.
//
// The package is pure in the same sense as the schema package: bytes and
// already-loaded cue.Values in, findings out. File discovery, golden reads and
// writes, and reporting belong to the CLI adapter. `cue mod tidy` is
// deliberately absent: its implementation is not importable from
// cuelang.org/go, and shelling out to a cue binary would break the
// single-binary promise.
package check
