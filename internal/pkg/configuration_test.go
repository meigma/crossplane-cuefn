package pkg_test

import (
	"io"
	"testing"

	xpkg "github.com/crossplane/crossplane-runtime/v2/pkg/xpkg"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/meigma/crossplane-cuefn/internal/pkg"
)

// buildFixtureConfiguration assembles a Configuration from the fixture XRD plus a
// generated Composition and metadata. It is the shared input for the image tests.
func buildFixtureConfiguration(t *testing.T) pkg.Configuration {
	t.Helper()

	xrd := fixtureXRD(t)
	comp, err := pkg.GenerateComposition(xrd, pkg.CompositionInput{
		Module:         "cuefn.example/app@v0.1.0",
		ExpectedDigest: "sha256:" + zeros(64),
	})
	require.NoError(t, err)

	meta, err := pkg.GenerateConfigurationMeta(pkg.ConfigurationMeta{
		Name:            "xapps-configuration",
		FunctionPackage: "xpkg.meigma.io/cuefn",
		FunctionVersion: ">=v0.1.0",
	})
	require.NoError(t, err)

	return pkg.Configuration{Meta: meta, XRD: xrd, Composition: comp}
}

// TestPackageYAML_DocOrder proves the package.yaml stream carries the three
// expected documents in crossplane's order: meta, XRD, Composition.
func TestPackageYAML_DocOrder(t *testing.T) {
	stream, err := buildFixtureConfiguration(t).PackageYAML()
	require.NoError(t, err)

	docs := splitStream(t, stream)
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
	cfg := buildFixtureConfiguration(t)
	img, err := pkg.BuildConfigurationImage(cfg)
	require.NoError(t, err)

	// The package layer must carry the xpkg base annotation; ExtractPackageYAML
	// locates it by that annotation and returns the package.yaml stream.
	rc, err := xpkg.ExtractPackageYAML(img)
	require.NoError(t, err)
	defer rc.Close()

	stream, err := io.ReadAll(rc)
	require.NoError(t, err)

	docs := splitStream(t, stream)
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

	// Composition: pipeline mode, env-config then cuefn.
	assert.Equal(t, "apiextensions.crossplane.io/v1", docs[2].APIVersion)
	assert.Equal(t, "Composition", docs[2].Kind)
	assert.Equal(t, "Pipeline", docs[2].Spec["mode"])
	pipeline, ok := docs[2].Spec["pipeline"].([]any)
	require.True(t, ok)
	require.Len(t, pipeline, 2)
	assert.Equal(t, "function-environment-configs", stepName(t, pipeline[0]))
	assert.Equal(t, "cuefn", stepName(t, pipeline[1]))
}

// TestBuildConfigurationImage_Errors proves an incomplete Configuration errors
// rather than producing a broken image.
func TestBuildConfigurationImage_Errors(t *testing.T) {
	_, err := pkg.BuildConfigurationImage(pkg.Configuration{})
	require.Error(t, err)
}

// stepName extracts the "step" field of a pipeline step map.
func stepName(t *testing.T, step any) string {
	t.Helper()
	m, ok := step.(map[string]any)
	require.True(t, ok)
	name, ok := m["step"].(string)
	require.True(t, ok)
	return name
}
