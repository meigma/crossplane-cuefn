//go:build !noxpkg

package modulepublish

import (
	"context"
	"encoding/json"
	"testing"

	"cuelabs.dev/go/oci/ociregistry"
	"cuelabs.dev/go/oci/ociregistry/ocimem"
	"cuelang.org/go/mod/modregistry"
	v1 "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/meigma/crossplane-cuefn/internal/test/common"
)

const sourceMetadataKey = "org.opencontainers.image.source"

func TestPrepareBuildsCanonicalAnnotatedModule(t *testing.T) {
	t.Parallel()

	metadata := map[string]string{
		sourceMetadataKey:  "https://github.com/meigma/example",
		"dev.meigma.owner": "platform=team",
	}
	artifact, err := Prepare(
		context.Background(),
		common.ExampleModuleRef,
		common.HermeticModuleDir(t),
		metadata,
	)
	require.NoError(t, err)
	require.NotEmpty(t, artifact.Digest())

	var manifest v1.Manifest
	require.NoError(t, json.Unmarshal(artifact.manifest, &manifest))
	assert.Equal(t, v1.MediaTypeImageManifest, manifest.MediaType)
	assert.Equal(t, "application/vnd.cue.module.v1+json", manifest.Config.MediaType)
	require.Len(t, manifest.Layers, 2)
	assert.Equal(t, "application/zip", manifest.Layers[0].MediaType)
	assert.Equal(t, "application/vnd.cue.modulefile.v1", manifest.Layers[1].MediaType)
	assert.Equal(t, metadata, manifest.Annotations)
}

func TestPrepareMetadataOrderDoesNotChangeDigest(t *testing.T) {
	t.Parallel()

	dir := common.HermeticModuleDir(t)
	first, err := Prepare(context.Background(), common.ExampleModuleRef, dir, map[string]string{
		"dev.meigma.b": "two",
		"dev.meigma.a": "one",
	})
	require.NoError(t, err)
	second, err := Prepare(context.Background(), common.ExampleModuleRef, dir, map[string]string{
		"dev.meigma.a": "one",
		"dev.meigma.b": "two",
	})
	require.NoError(t, err)

	assert.Equal(t, first.Digest(), second.Digest())
	assert.Equal(t, first.manifest, second.manifest)
}

func TestArtifactPublishIsImmutableAndRetrySafe(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	dir := common.HermeticModuleDir(t)
	artifact, err := Prepare(ctx, common.ExampleModuleRef, dir, map[string]string{
		"dev.meigma.release": "stable",
	})
	require.NoError(t, err)
	conflict, err := Prepare(ctx, common.ExampleModuleRef, dir, map[string]string{
		"dev.meigma.release": "changed",
	})
	require.NoError(t, err)

	registry := ocimem.New()
	resolver := staticResolver{registry: registry}

	first, err := artifact.Publish(ctx, resolver)
	require.NoError(t, err)
	assert.False(t, first.Reused)
	assert.Equal(t, artifact.Digest(), first.Digest)

	retry, err := artifact.Publish(ctx, resolver)
	require.NoError(t, err)
	assert.True(t, retry.Reused)
	assert.Equal(t, first.Digest, retry.Digest)

	_, err = conflict.Publish(ctx, resolver)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "immutable")

	mv := artifact.version
	moduleArtifact, err := modregistry.NewClient(registry).GetModule(ctx, mv)
	require.NoError(t, err)
	assert.Equal(t, artifact.Digest(), moduleArtifact.ManifestDigest().String())
}

func TestPrepareRejectsEmptyMetadata(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		metadata map[string]string
		want     string
	}{
		{name: "empty key", metadata: map[string]string{"": "value"}, want: "key"},
		{name: "empty value", metadata: map[string]string{"dev.meigma.key": ""}, want: "value"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, err := Prepare(
				context.Background(),
				common.ExampleModuleRef,
				common.HermeticModuleDir(t),
				tt.metadata,
			)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.want)
		})
	}
}

type staticResolver struct {
	registry ociregistry.Interface
}

func (r staticResolver) ResolveToRegistry(path, version string) (modregistry.RegistryLocation, error) {
	return modregistry.RegistryLocation{
		Registry:   r.registry,
		Repository: path,
		Tag:        version,
	}, nil
}
