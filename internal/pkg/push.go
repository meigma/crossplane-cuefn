package pkg

import (
	"context"
	"fmt"

	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/remote"

	"github.com/google/go-containerregistry/pkg/name"
)

// Push writes img to the OCI reference ref and returns the pushed image's
// digest. It is the only network seam in the package: callers assemble the image
// purely, then push it here.
//
// When insecure is true the reference is parsed with name.Insecure so the push
// uses plain HTTP (needed for a non-loopback throwaway registry; loopback
// registries already resolve to HTTP automatically). opts carry auth and
// transport. A malformed ref or an unreachable/refusing destination surfaces as
// a clear error naming ref, never a panic.
func Push(ctx context.Context, ref string, img v1.Image, insecure bool, opts ...remote.Option) (name.Digest, error) {
	var nameOpts []name.Option
	if insecure {
		nameOpts = append(nameOpts, name.Insecure)
	}

	dst, err := name.ParseReference(ref, nameOpts...)
	if err != nil {
		return name.Digest{}, fmt.Errorf("invalid package reference %q: %w", ref, err)
	}

	writeOpts := append([]remote.Option{remote.WithContext(ctx)}, opts...)
	if err = remote.Write(dst, img, writeOpts...); err != nil {
		return name.Digest{}, fmt.Errorf("cannot push package to %q: %w", ref, err)
	}

	dg, err := img.Digest()
	if err != nil {
		return name.Digest{}, fmt.Errorf("cannot compute pushed image digest for %q: %w", ref, err)
	}

	return dst.Context().Digest(dg.String()), nil
}
