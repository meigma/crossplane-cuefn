//go:build !noxpkg

// Package modulepublish prepares and publishes canonical CUE module OCI artifacts.
package modulepublish

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"cuelabs.dev/go/oci/ociregistry"
	"cuelabs.dev/go/oci/ociregistry/ocimem"
	"cuelang.org/go/mod/modconfig"
	"cuelang.org/go/mod/modregistry"
	"cuelang.org/go/mod/module"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
)

// Artifact is a validated CUE module represented as an exact OCI manifest and
// its referenced blobs. It can be inspected before any remote mutation.
type Artifact struct {
	version  module.Version
	manifest []byte
	desc     ociregistry.Descriptor
	blobs    []blob
}

// NewResolver builds a registry resolver using CUE's standard registry and
// authentication environment. Nil env reads the current process environment.
func NewResolver(env []string) (*modconfig.Resolver, error) {
	resolver, err := modconfig.NewResolver(&modconfig.Config{Env: env})
	if err != nil {
		return nil, fmt.Errorf("cannot build CUE registry resolver from environment: %w", err)
	}
	return resolver, nil
}

type blob struct {
	desc ociregistry.Descriptor
	data []byte
}

// PublishResult describes the immutable module version resolved after publish.
type PublishResult struct {
	// Module is the published CUE module version.
	Module string
	// Digest is the canonical OCI manifest digest.
	Digest string
	// Reused reports whether the registry already held the same artifact.
	Reused bool
}

// Prepare builds and validates the exact CUE module artifact that would be
// published for ref, adding metadata as OCI manifest annotations.
func Prepare(ctx context.Context, ref, dir string, metadata map[string]string) (*Artifact, error) {
	mv, err := module.ParseVersion(ref)
	if err != nil {
		return nil, fmt.Errorf("invalid module reference %q: %w", ref, err)
	}
	if validationErr := validateMetadata(metadata); validationErr != nil {
		return nil, validationErr
	}

	archive, vcsMeta, err := createArchive(mv, dir)
	if err != nil {
		return nil, fmt.Errorf("cannot prepare module %q: %w", ref, err)
	}

	mem := ocimem.New()
	annotated := manifestAnnotator(mem, metadata)
	client := modregistry.NewClient(annotated)
	if putErr := client.PutModuleWithMetadata(
		ctx,
		mv,
		bytes.NewReader(archive),
		int64(len(archive)),
		vcsMeta,
	); putErr != nil {
		return nil, fmt.Errorf("cannot prepare module %q: %w", ref, putErr)
	}

	manifestReader, err := mem.GetTag(ctx, mv.BasePath(), mv.Version())
	if err != nil {
		return nil, fmt.Errorf("cannot read prepared manifest for %q: %w", ref, err)
	}
	manifestDesc := manifestReader.Descriptor()
	manifestData, err := readAndClose(manifestReader)
	if err != nil {
		return nil, fmt.Errorf("cannot read prepared manifest for %q: %w", ref, err)
	}

	var manifest ocispec.Manifest
	if err := json.Unmarshal(manifestData, &manifest); err != nil {
		return nil, fmt.Errorf("cannot decode prepared manifest for %q: %w", ref, err)
	}

	refs := make([]ociregistry.Descriptor, 0, 1+len(manifest.Layers))
	refs = append(refs, manifest.Config)
	refs = append(refs, manifest.Layers...)
	blobs := make([]blob, 0, len(refs))
	for _, desc := range refs {
		reader, err := mem.GetBlob(ctx, mv.BasePath(), desc.Digest)
		if err != nil {
			return nil, fmt.Errorf("cannot read prepared blob %s for %q: %w", desc.Digest, ref, err)
		}
		data, err := readAndClose(reader)
		if err != nil {
			return nil, fmt.Errorf("cannot read prepared blob %s for %q: %w", desc.Digest, ref, err)
		}
		blobs = append(blobs, blob{desc: desc, data: data})
	}

	return &Artifact{
		version:  mv,
		manifest: manifestData,
		desc:     manifestDesc,
		blobs:    blobs,
	}, nil
}

// Digest returns the exact prepared OCI manifest digest.
func (a *Artifact) Digest() string {
	if a == nil {
		return ""
	}
	return a.desc.Digest.String()
}

// Publish promotes the prepared artifact through resolver without allowing an
// existing module version to change digest.
func (a *Artifact) Publish(ctx context.Context, resolver modregistry.Resolver) (PublishResult, error) {
	if a == nil {
		return PublishResult{}, errors.New("cannot publish a nil module artifact")
	}
	if resolver == nil {
		return PublishResult{}, errors.New("cannot publish without a registry resolver")
	}

	loc, err := resolver.ResolveToRegistry(a.version.BasePath(), a.version.Version())
	if err != nil {
		return PublishResult{}, fmt.Errorf("cannot resolve module %q: %w", a.version, err)
	}
	result := PublishResult{Module: a.version.String(), Digest: a.Digest()}

	current, err := loc.Registry.ResolveTag(ctx, loc.Repository, loc.Tag)
	switch {
	case err == nil && current.Digest == a.desc.Digest:
		result.Reused = true
		return result, nil
	case err == nil:
		return PublishResult{}, fmt.Errorf(
			"module version %q is immutable: registry has %s, prepared artifact is %s",
			a.version,
			current.Digest,
			a.desc.Digest,
		)
	case !isNotFound(err):
		return PublishResult{}, fmt.Errorf("cannot inspect module version %q: %w", a.version, err)
	}

	for _, b := range a.blobs {
		if _, pushErr := loc.Registry.PushBlob(ctx, loc.Repository, b.desc, bytes.NewReader(b.data)); pushErr != nil {
			return PublishResult{}, fmt.Errorf(
				"cannot push module blob %s for %q: %w",
				b.desc.Digest,
				a.version,
				pushErr,
			)
		}
	}
	if _, pushErr := loc.Registry.PushManifest(
		ctx,
		loc.Repository,
		loc.Tag,
		a.manifest,
		a.desc.MediaType,
	); pushErr != nil {
		return PublishResult{}, fmt.Errorf("cannot push module manifest for %q: %w", a.version, pushErr)
	}

	published, err := loc.Registry.ResolveTag(ctx, loc.Repository, loc.Tag)
	if err != nil {
		return PublishResult{}, fmt.Errorf("cannot verify published module version %q: %w", a.version, err)
	}
	if published.Digest != a.desc.Digest {
		return PublishResult{}, fmt.Errorf(
			"published module version %q resolved to %s, expected %s",
			a.version,
			published.Digest,
			a.desc.Digest,
		)
	}
	return result, nil
}

func manifestAnnotator(reg ociregistry.Interface, metadata map[string]string) ociregistry.Interface {
	return &ociregistry.Funcs{
		PushBlob_: reg.PushBlob,
		PushManifest_: func(
			ctx context.Context,
			repo string,
			tag string,
			contents []byte,
			mediaType string,
		) (ociregistry.Descriptor, error) {
			var manifest ocispec.Manifest
			if err := json.Unmarshal(contents, &manifest); err != nil {
				return ociregistry.Descriptor{}, fmt.Errorf("cannot decode CUE module manifest: %w", err)
			}
			if manifest.Annotations == nil {
				manifest.Annotations = make(map[string]string, len(metadata))
			}
			for key, value := range metadata {
				if _, exists := manifest.Annotations[key]; exists {
					return ociregistry.Descriptor{}, fmt.Errorf(
						"metadata key %q conflicts with generated CUE module metadata",
						key,
					)
				}
				manifest.Annotations[key] = value
			}
			annotated, err := json.Marshal(manifest)
			if err != nil {
				return ociregistry.Descriptor{}, fmt.Errorf("cannot encode CUE module manifest: %w", err)
			}
			return reg.PushManifest(ctx, repo, tag, annotated, mediaType)
		},
	}
}

func validateMetadata(metadata map[string]string) error {
	for key, value := range metadata {
		if key == "" {
			return errors.New("metadata key cannot be empty")
		}
		if value == "" {
			return fmt.Errorf("metadata value for %q cannot be empty", key)
		}
	}
	return nil
}

func isNotFound(err error) bool {
	return errors.Is(err, ociregistry.ErrNameUnknown) ||
		errors.Is(err, ociregistry.ErrManifestUnknown)
}

func readAndClose(reader io.ReadCloser) ([]byte, error) {
	data, readErr := io.ReadAll(reader)
	closeErr := reader.Close()
	return data, errors.Join(readErr, closeErr)
}
