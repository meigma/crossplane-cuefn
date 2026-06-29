package pkg

import (
	"context"
	"fmt"

	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/remote"

	"github.com/google/go-containerregistry/pkg/name"
)

// digestable is the shared shape of an image or index that can report its own
// digest, so Push and PushIndex name the pushed artifact through one helper.
type digestable interface {
	Digest() (v1.Hash, error)
}

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
	dst, err := parseDestination(ref, insecure)
	if err != nil {
		return name.Digest{}, err
	}
	if err = remote.Write(dst, img, writeOptions(ctx, opts)...); err != nil {
		return name.Digest{}, fmt.Errorf("cannot push package to %q: %w", ref, err)
	}
	return pushedDigest(dst, ref, img)
}

// PushIndex writes a multi-arch image index idx to the OCI reference ref and
// returns the pushed index's digest. It is the multi-arch release path for a
// Function xpkg (the package image is the runtime image, so a real install needs
// a per-node-arch index). It mirrors Push: insecure selects plain HTTP, opts
// carry auth/transport, and a malformed/unreachable destination surfaces a clear
// error naming ref.
func PushIndex(
	ctx context.Context,
	ref string,
	idx v1.ImageIndex,
	insecure bool,
	opts ...remote.Option,
) (name.Digest, error) {
	dst, err := parseDestination(ref, insecure)
	if err != nil {
		return name.Digest{}, err
	}
	if err = remote.WriteIndex(dst, idx, writeOptions(ctx, opts)...); err != nil {
		return name.Digest{}, fmt.Errorf("cannot push package index to %q: %w", ref, err)
	}
	return pushedDigest(dst, ref, idx)
}

// parseDestination parses ref into a push target, selecting plain HTTP when
// insecure is set, and naming ref in any parse error.
func parseDestination(ref string, insecure bool) (name.Reference, error) {
	var nameOpts []name.Option
	if insecure {
		nameOpts = append(nameOpts, name.Insecure)
	}
	dst, err := name.ParseReference(ref, nameOpts...)
	if err != nil {
		return nil, fmt.Errorf("invalid package reference %q: %w", ref, err)
	}
	return dst, nil
}

// writeOptions prepends the context option to the caller-supplied push options.
func writeOptions(ctx context.Context, opts []remote.Option) []remote.Option {
	return append([]remote.Option{remote.WithContext(ctx)}, opts...)
}

// pushedDigest computes the artifact's digest and binds it to dst's repository,
// yielding the canonical name@digest of what was just pushed.
func pushedDigest(dst name.Reference, ref string, artifact digestable) (name.Digest, error) {
	dg, err := artifact.Digest()
	if err != nil {
		return name.Digest{}, fmt.Errorf("cannot compute pushed digest for %q: %w", ref, err)
	}
	return dst.Context().Digest(dg.String()), nil
}
