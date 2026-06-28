package render

import (
	"context"

	"cuelang.org/go/mod/modconfig"
)

// Loaded is the result of resolving a module reference to local bytes: the
// directory the engine loads, an optional CUE registry for resolving the
// module's transitive dependencies at load time, and a cleanup func the caller
// must invoke when done.
type Loaded struct {
	// Dir is the module root directory (containing cue.mod) to load.
	Dir string

	// Registry resolves the module's transitive CUE dependencies during load.
	// It is nil for self-contained modules served from disk (LocalLoader), in
	// which case the engine loads without a registry and dep-free modules render
	// identically.
	Registry modconfig.Registry

	// Cleanup releases any resources the loader allocated. It is always non-nil
	// and safe to call exactly once; a persistent cache returns a no-op.
	Cleanup func()
}

// ModuleLoader fetches a CUE module by reference and exposes it as a local
// directory tree (rooted at the module's cue.mod) that the engine can load.
type ModuleLoader interface {
	// Load fetches the module identified by ref ("path@version") and returns a
	// [Loaded] describing the local directory, an optional dependency registry,
	// and a cleanup func the caller must invoke when done.
	Load(ctx context.Context, ref string) (Loaded, error)
}

// LocalLoader serves a module from a fixed local directory, ignoring ref. It is
// used for tests and offline development.
type LocalLoader struct {
	// Dir is the module root directory (containing cue.mod) to serve.
	Dir string
}

// Load returns the configured directory with no dependency registry and a no-op
// cleanup. The ref is ignored because the directory is fixed.
func (l LocalLoader) Load(_ context.Context, _ string) (Loaded, error) {
	return Loaded{Dir: l.Dir, Registry: nil, Cleanup: func() {}}, nil
}
