// Package testharness is the core of `cuefn test`: it parses txtar test cases
// for cuefn modules, renders them through the real engine, and evaluates the
// declared expectations against a normalized result document.
//
// One case is one txtar file. Its named sections mirror the render command's
// inputs (xr.yaml, environment.yaml, required.yaml, observed.yaml) and declare
// expectations: want.cue (partial CUE unified against the result), want.yaml
// (an exact machine-maintained golden of the full normalized result), or
// error.txt (substrings a failing render must report). Numbered step sections
// (1/observed.yaml, 1/want.cue, ...) run readiness sequences: the same module
// and XR rendered against successive observed snapshots.
//
// The section vocabulary is closed and every authoring mistake is a loud
// error: unknown section names, observed fixtures against modules that never
// opted into observedResources, and unsatisfiable expectation combinations are
// all rejected rather than silently passing.
//
// The package is pure logic over bytes: callers own file discovery and IO.
// SeedGoldens and UpdateGoldens return rewritten txtar contents; they never
// touch hand-written want.cue or error.txt sections.
package testharness
