package render

import (
	"context"
	"fmt"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
	"cuelang.org/go/cue/load"
)

// LoadModule resolves ref through loader, builds the module's CUE value, and
// returns it alongside a cleanup func the caller must invoke when done. It is the
// single load path shared by the render engine and the author-time schema
// codegen, so both consume an identically-built module value: bytes come only
// from the [ModuleLoader] port, and a loader-supplied dependency registry (set
// by OCILoader, nil for LocalLoader) resolves transitive CUE dependencies.
//
// The returned value is the raw built module (no inputs filled). On any error
// the cleanup has already run and the caller must not call it again.
func LoadModule(ctx context.Context, loader ModuleLoader, ref string) (cue.Value, func(), error) {
	ld, err := loader.Load(ctx, ref)
	if err != nil {
		return cue.Value{}, nil, fmt.Errorf("cannot load module %q: %w", ref, err)
	}

	cctx := cuecontext.New()

	cfg := &load.Config{Dir: ld.Dir}
	// A registry is supplied only by loaders that fetch from OCI; it resolves the
	// module's transitive CUE dependencies at load time. LocalLoader leaves it nil
	// so a self-contained module loads offline (see Engine.Render for the full
	// rationale on why the registry is injected explicitly rather than left to
	// CUE's nil-auto path).
	if ld.Registry != nil {
		cfg.Registry = ld.Registry
	}

	insts := load.Instances([]string{"."}, cfg)
	if len(insts) == 0 {
		ld.Cleanup()
		return cue.Value{}, nil, fmt.Errorf("module %q contains no CUE instances", ref)
	}
	if err = insts[0].Err; err != nil {
		ld.Cleanup()
		return cue.Value{}, nil, wrapCUE(err, "cannot load module %q", ref)
	}

	v := cctx.BuildInstance(insts[0])
	if err = v.Err(); err != nil {
		ld.Cleanup()
		return cue.Value{}, nil, wrapCUE(err, "cannot build module %q", ref)
	}

	return v, ld.Cleanup, nil
}
