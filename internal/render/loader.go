package render

import "context"

// ModuleLoader fetches a CUE module by reference and exposes it as a local
// directory tree (rooted at the module's cue.mod) that the engine can load.
type ModuleLoader interface {
	// Load fetches the module identified by ref ("path@version") and returns the
	// path of a local directory containing its CUE files, plus a cleanup func the
	// caller must invoke when done.
	Load(ctx context.Context, ref string) (dir string, cleanup func(), err error)
}

// LocalLoader serves a module from a fixed local directory, ignoring ref. It is
// used for tests and offline development.
type LocalLoader struct {
	// Dir is the module root directory (containing cue.mod) to serve.
	Dir string
}

// Load returns the configured directory and a no-op cleanup. The ref is ignored
// because the directory is fixed.
func (l LocalLoader) Load(_ context.Context, _ string) (string, func(), error) {
	return l.Dir, func() {}, nil
}
