//go:build !noxpkg

package integration_test

import (
	"bytes"
	"context"
	"io"
	"testing"

	xpkg "github.com/crossplane/crossplane-runtime/v2/pkg/xpkg"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/meigma/crossplane-cuefn/internal/cli"
	"github.com/meigma/crossplane-cuefn/internal/test/common"
)

// TestPublishFunction_EndToEnd proves `cuefn publish-function` assembles the
// Function xpkg over a runtime base tarball and pushes it: the pulled package
// carries the xpkg base annotation and its stream names the Function and Input
// CRD (criterion 1). Docker-gated.
func TestPublishFunction_EndToEnd(t *testing.T) {
	reg := common.StartRegistry(t)
	basePath := common.WriteRuntimeBaseTarball(t, "amd64")
	pkgRef := reg.Host() + "/function-cuefn:v0.1.0"

	var stdout bytes.Buffer
	root := cli.NewRootCommand(cli.Options{Out: &stdout})
	root.SetArgs([]string{
		"publish-function",
		"--runtime-image", basePath,
		"--package", pkgRef,
		"--name", "function-cuefn",
		"--crossplane-constraint", ">=v2.0.0-0",
		"--insecure",
	})
	require.NoError(t, root.ExecuteContext(context.Background()))
	assert.Contains(t, stdout.String(), "pushed")

	parsed, err := name.ParseReference(pkgRef, name.Insecure)
	require.NoError(t, err)
	img, err := remote.Image(parsed)
	require.NoError(t, err)

	assertBaseLayerAnnotation(t, img)

	rc, err := xpkg.ExtractPackageYAML(img)
	require.NoError(t, err)
	defer rc.Close()
	stream, err := io.ReadAll(rc)
	require.NoError(t, err)

	kinds := common.StreamKinds(stream)
	assert.True(t, kinds["Function"], "package must contain the Function meta")
	assert.True(t, kinds["CustomResourceDefinition"], "package must embed the Input CRD")
}

// TestPublishFunction_MultiArchIndex proves several --runtime-image bases push a
// multi-arch index that pulls back with both platform manifests (release path).
// Docker-gated.
func TestPublishFunction_MultiArchIndex(t *testing.T) {
	reg := common.StartRegistry(t)
	amd := common.WriteRuntimeBaseTarball(t, "amd64")
	arm := common.WriteRuntimeBaseTarball(t, "arm64")
	pkgRef := reg.Host() + "/function-cuefn:v0.1.0-index"

	root := cli.NewRootCommand(cli.Options{Out: &bytes.Buffer{}})
	root.SetArgs([]string{
		"publish-function",
		"--runtime-image", amd,
		"--runtime-image", arm,
		"--package", pkgRef,
		"--insecure",
	})
	require.NoError(t, root.ExecuteContext(context.Background()))

	parsed, err := name.ParseReference(pkgRef, name.Insecure)
	require.NoError(t, err)
	idx, err := remote.Index(parsed)
	require.NoError(t, err)
	manifest, err := idx.IndexManifest()
	require.NoError(t, err)
	assert.Len(t, manifest.Manifests, 2)
}
