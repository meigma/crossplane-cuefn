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

// LocalLoader serves a module from a fixed local directory, ignoring ref.
//
// The zero value (LocalLoader{Dir: ...}) resolves no dependencies: it returns a
// nil registry, so a self-contained module loads fully offline. This is the
// primitive used by hermetic tests and offline development. Use [NewLocalLoader]
// to serve a module that imports OCI dependencies (e.g. the official
// cue.dev/x/k8s.io schema); it attaches a registry so those deps resolve at load
// time.
type LocalLoader struct {
	// Dir is the module root directory (containing cue.mod) to serve.
	Dir string

	// registry resolves the served module's transitive CUE dependencies. It is
	// nil for the zero value (offline) and set by NewLocalLoader.
	registry modconfig.Registry
}

// NewLocalLoader serves the module in dir and resolves its transitive CUE
// dependencies through a registry built from cfg (central by default;
// CUE_REGISTRY for private or override registries). Use it for a local module
// that imports OCI dependencies; the zero-value LocalLoader stays offline.
func NewLocalLoader(dir string, cfg OCIConfig) (*LocalLoader, error) {
	_, registry, _, err := buildRegistry(cfg)
	if err != nil {
		return nil, err
	}
	return &LocalLoader{Dir: dir, registry: registry}, nil
}

// Load returns the configured directory with its dependency registry (nil for a
// zero-value LocalLoader) and a no-op cleanup. The ref is ignored because the
// directory is fixed.
func (l LocalLoader) Load(_ context.Context, _ string) (Loaded, error) {
	return Loaded{Dir: l.Dir, Registry: l.registry, Cleanup: func() {}}, nil
}
