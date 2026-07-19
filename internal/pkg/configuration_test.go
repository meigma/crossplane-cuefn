package pkg_test

import (
	"io"
	"testing"

	xpkg "github.com/crossplane/crossplane-runtime/v2/pkg/xpkg"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/meigma/crossplane-cuefn/internal/pkg"
	"github.com/meigma/crossplane-cuefn/internal/test/common"
)

// TestPackageYAML_DocOrder proves the package.yaml stream carries the three
// expected documents in crossplane's order: meta, XRD, Composition.
func TestPackageYAML_DocOrder(t *testing.T) {
	stream, err := common.BuildFixtureConfiguration(t).PackageYAML()
	require.NoError(t, err)

	docs := common.SplitStream(t, stream)
	require.Len(t, docs, 3)

	assert.Equal(t, "meta.pkg.crossplane.io/v1", docs[0].APIVersion)
	assert.Equal(t, "Configuration", docs[0].Kind)
	assert.Equal(t, "apiextensions.crossplane.io/v2", docs[1].APIVersion)
	assert.Equal(t, "CompositeResourceDefinition", docs[1].Kind)
	assert.Equal(t, "apiextensions.crossplane.io/v1", docs[2].APIVersion)
	assert.Equal(t, "Composition", docs[2].Kind)
}

// TestBuildConfigurationImage_EmbeddedDocs proves the assembled image's package
// layer, read back and untarred, re-parses into exactly the expected Crossplane
// kinds/apiVersions with the right Composition step order and the function
// dependency — checking the bytes, not the in-memory objects (criterion 1a).
func TestBuildConfigurationImage_EmbeddedDocs(t *testing.T) {
	cfg := common.BuildFixtureConfiguration(t)
	img, err := pkg.BuildConfigurationImage(cfg)
	require.NoError(t, err)

	// The package layer must carry the xpkg base annotation; ExtractPackageYAML
	// locates it by that annotation and returns the package.yaml stream.
	rc, err := xpkg.ExtractPackageYAML(img)
	require.NoError(t, err)
	defer rc.Close()

	stream, err := io.ReadAll(rc)
	require.NoError(t, err)

	docs := common.SplitStream(t, stream)
	require.Len(t, docs, 3)

	// Meta: Configuration depending on the cuefn function.
	assert.Equal(t, "meta.pkg.crossplane.io/v1", docs[0].APIVersion)
	assert.Equal(t, "Configuration", docs[0].Kind)
	deps, ok := docs[0].Spec["dependsOn"].([]any)
	require.True(t, ok, "meta must declare dependsOn")
	require.Len(t, deps, 1)
	dep, ok := deps[0].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "xpkg.meigma.io/cuefn", dep["function"])

	// XRD.
	assert.Equal(t, "apiextensions.crossplane.io/v2", docs[1].APIVersion)
	assert.Equal(t, "CompositeResourceDefinition", docs[1].Kind)

	// Composition: pipeline mode with the single cuefn step (the fixture requests
	// no EnvironmentConfigs).
	assert.Equal(t, "apiextensions.crossplane.io/v1", docs[2].APIVersion)
	assert.Equal(t, "Composition", docs[2].Kind)
	assert.Equal(t, "Pipeline", docs[2].Spec["mode"])
	pipeline, ok := docs[2].Spec["pipeline"].([]any)
	require.True(t, ok)
	require.Len(t, pipeline, 1)
	assert.Equal(t, "cuefn", common.StepName(t, pipeline[0]))
}

// TestBuildConfigurationImage_Errors proves an incomplete Configuration errors
// rather than producing a broken image.
func TestBuildConfigurationImage_Errors(t *testing.T) {
	_, err := pkg.BuildConfigurationImage(pkg.Configuration{})
	require.Error(t, err)
}

func TestBuildConfigurationImage_Labels(t *testing.T) {
	t.Parallel()

	labels := map[string]string{
		"org.opencontainers.image.source": "https://github.com/meigma/example",
		"dev.meigma.owner":                "platform=team",
	}
	img, err := pkg.BuildConfigurationImage(
		common.BuildFixtureConfiguration(t),
		pkg.WithConfigurationLabels(labels),
	)
	require.NoError(t, err)
	config, err := img.ConfigFile()
	require.NoError(t, err)
	for key, value := range labels {
		assert.Equal(t, value, config.Config.Labels[key])
	}
}

func TestBuildConfigurationImage_RejectsLabelCollision(t *testing.T) {
	t.Parallel()

	base, err := pkg.BuildConfigurationImage(common.BuildFixtureConfiguration(t))
	require.NoError(t, err)
	config, err := base.ConfigFile()
	require.NoError(t, err)
	require.NotEmpty(t, config.Config.Labels)

	var generatedKey string
	for key := range config.Config.Labels {
		generatedKey = key
		break
	}
	_, err = pkg.BuildConfigurationImage(
		common.BuildFixtureConfiguration(t),
		pkg.WithConfigurationLabels(map[string]string{generatedKey: "override"}),
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "conflicts")
}
