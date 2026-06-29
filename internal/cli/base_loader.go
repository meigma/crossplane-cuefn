//go:build !noxpkg

package cli

import (
	"context"
	"fmt"
	"os"

	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/google/go-containerregistry/pkg/v1/tarball"
)

// loadRuntimeBase resolves a runtime image base from one of two sources, at the
// edge of the packaging core: a local OCI/docker image tarball (e.g. the
// image.tar `mise run image-local` writes) when source names an existing file,
// or a registry reference otherwise (resolved by digest for a reproducible
// release). It is the adapter that keeps function.go's BuildFunctionImage free of
// filesystem/registry IO.
func loadRuntimeBase(ctx context.Context, source string, insecure bool) (v1.Image, error) {
	if fi, err := os.Stat(source); err == nil && !fi.IsDir() {
		img, err := tarball.ImageFromPath(source, nil)
		if err != nil {
			return nil, fmt.Errorf("cannot load runtime base from tarball %q: %w", source, err)
		}
		return img, nil
	}

	var nameOpts []name.Option
	if insecure {
		nameOpts = append(nameOpts, name.Insecure)
	}
	ref, err := name.ParseReference(source, nameOpts...)
	if err != nil {
		return nil, fmt.Errorf("invalid runtime base reference %q: %w", source, err)
	}
	img, err := remote.Image(ref, append([]remote.Option{remote.WithContext(ctx)}, remotePushOptions()...)...)
	if err != nil {
		return nil, fmt.Errorf("cannot pull runtime base %q: %w", source, err)
	}
	return img, nil
}
