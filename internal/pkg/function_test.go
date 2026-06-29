package pkg_test

import (
	"io"
	"testing"

	xpkg "github.com/crossplane/crossplane-runtime/v2/pkg/xpkg"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/meigma/crossplane-cuefn/internal/pkg"
	"github.com/meigma/crossplane-cuefn/internal/test/common"
)

// TestGenerateFunctionMeta proves the crossplane.yaml metadata is a
// meta.pkg.crossplane.io Function with the optional constraint and capabilities.
func TestGenerateFunctionMeta(t *testing.T) {
	meta, err := pkg.GenerateFunctionMeta(pkg.FunctionMeta{
		Name:                 "function-cuefn",
		CrossplaneConstraint: ">=v2.0.0-0",
		Capabilities:         []string{"composition"},
	})
	require.NoError(t, err)

	assert.Equal(t, "meta.pkg.crossplane.io/v1", meta.APIVersion)
	assert.Equal(t, "Function", meta.Kind)
	assert.Equal(t, "function-cuefn", meta.Name)
	require.NotNil(t, meta.Spec.Crossplane)
	assert.Equal(t, ">=v2.0.0-0", meta.Spec.Crossplane.Version)
	assert.Equal(t, []string{"composition"}, meta.Spec.Capabilities)
}

// TestGenerateFunctionMeta_Optional proves the optional fields are omitted when
// empty and that a missing name errors.
func TestGenerateFunctionMeta_Optional(t *testing.T) {
	meta, err := pkg.GenerateFunctionMeta(pkg.FunctionMeta{Name: "function-cuefn"})
	require.NoError(t, err)
	assert.Nil(t, meta.Spec.Crossplane)
	assert.Empty(t, meta.Spec.Capabilities)

	_, err = pkg.GenerateFunctionMeta(pkg.FunctionMeta{})
	require.Error(t, err)
}

// TestFunctionPackageYAML proves the package.yaml stream carries the Function
// meta first, then the Input CRD, with the right identities (criterion 1).
func TestFunctionPackageYAML(t *testing.T) {
	stream, err := common.FixtureFunction(t).PackageYAML()
	require.NoError(t, err)

	docs := common.SplitStream(t, stream)
	require.Len(t, docs, 2)

	assert.Equal(t, "meta.pkg.crossplane.io/v1", docs[0].APIVersion)
	assert.Equal(t, "Function", docs[0].Kind)
	assert.Equal(t, "function-cuefn", docs[0].Metadata["name"])

	assert.Equal(t, "apiextensions.k8s.io/v1", docs[1].APIVersion)
	assert.Equal(t, "CustomResourceDefinition", docs[1].Kind)
	assert.Equal(t, "inputs.cuefn.meigma.io", docs[1].Metadata["name"])

	// The CRD's group is the cuefn Input group.
	assert.Equal(t, "cuefn.meigma.io", docs[1].Spec["group"])
}

// TestDefaultFunction_Errors proves a nil meta errors rather than producing a
// half-built Function.
func TestDefaultFunction_Errors(t *testing.T) {
	_, err := pkg.DefaultFunction(nil)
	require.Error(t, err)
}

// TestBuildFunctionImage_EmbedsRuntime proves the assembled Function xpkg rides
// the package layer on top of the runtime base: the base's layers and
// entrypoint/cmd are preserved, the package layer carries the xpkg base
// annotation, and its bytes re-parse into the Function + Input CRD (criterion 1
// + criterion 3's serving invariant).
func TestBuildFunctionImage_EmbedsRuntime(t *testing.T) {
	base := common.FakeRuntimeBase(t, "amd64")
	baseLayers, err := base.Layers()
	require.NoError(t, err)

	img, err := pkg.BuildFunctionImage(base, common.FixtureFunction(t))
	require.NoError(t, err)

	// The runtime layers are preserved and exactly one package layer is appended.
	imgLayers, err := img.Layers()
	require.NoError(t, err)
	require.Len(t, imgLayers, len(baseLayers)+1)
	for i, bl := range baseLayers {
		baseDigest, bderr := bl.Digest()
		require.NoError(t, bderr)
		imgDigest, iderr := imgLayers[i].Digest()
		require.NoError(t, iderr)
		assert.Equal(t, baseDigest.String(), imgDigest.String(), "runtime layer %d must be preserved", i)
	}

	// The serving config (entrypoint + cmd) is unchanged: the package layer must
	// never alter how the image serves the gRPC function.
	cfg, err := img.ConfigFile()
	require.NoError(t, err)
	assert.Equal(t, []string{"/usr/bin/cuefn"}, cfg.Config.Entrypoint)
	assert.Equal(t, []string{"function"}, cfg.Config.Cmd)

	// The package layer carries the xpkg base annotation and re-parses into the
	// Function meta + Input CRD.
	pkgLayer := imgLayers[len(imgLayers)-1]
	mt, err := pkgLayer.MediaType()
	require.NoError(t, err)
	assert.Contains(t, string(mt), "tar")

	rc, err := xpkg.ExtractPackageYAML(img)
	require.NoError(t, err)
	defer rc.Close()
	stream, err := io.ReadAll(rc)
	require.NoError(t, err)

	docs := common.SplitStream(t, stream)
	require.Len(t, docs, 2)
	assert.Equal(t, "Function", docs[0].Kind)
	assert.Equal(t, "CustomResourceDefinition", docs[1].Kind)
}

// TestBuildFunctionImage_Errors proves a nil base errors rather than panicking.
func TestBuildFunctionImage_Errors(t *testing.T) {
	_, err := pkg.BuildFunctionImage(nil, common.FixtureFunction(t))
	require.Error(t, err)
}

// TestBuildFunctionIndex proves the multi-arch index wraps each per-arch base
// into a Function xpkg image and records its platform (release path).
func TestBuildFunctionIndex(t *testing.T) {
	bases := []v1.Image{common.FakeRuntimeBase(t, "amd64"), common.FakeRuntimeBase(t, "arm64")}

	idx, err := pkg.BuildFunctionIndex(bases, common.FixtureFunction(t))
	require.NoError(t, err)

	manifest, err := idx.IndexManifest()
	require.NoError(t, err)
	require.Len(t, manifest.Manifests, 2)

	platforms := map[string]bool{}
	for _, m := range manifest.Manifests {
		require.NotNil(t, m.Platform)
		platforms[m.Platform.OS+"/"+m.Platform.Architecture] = true
	}
	assert.True(t, platforms["linux/amd64"])
	assert.True(t, platforms["linux/arm64"])

	// Each child image must still embed the package layer and preserve serving.
	for _, m := range manifest.Manifests {
		child, err := idx.Image(m.Digest)
		require.NoError(t, err)
		cfg, err := child.ConfigFile()
		require.NoError(t, err)
		assert.Equal(t, []string{"/usr/bin/cuefn"}, cfg.Config.Entrypoint)
		rc, err := xpkg.ExtractPackageYAML(child)
		require.NoError(t, err)
		_ = rc.Close()
	}
}

// TestBuildFunctionIndex_Errors proves an empty base list errors.
func TestBuildFunctionIndex_Errors(t *testing.T) {
	_, err := pkg.BuildFunctionIndex(nil, common.FixtureFunction(t))
	require.Error(t, err)
}
