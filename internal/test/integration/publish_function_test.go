//go:build !noxpkg

package integration_test

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"

	xpkg "github.com/crossplane/crossplane-runtime/v2/pkg/xpkg"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/meigma/crossplane-cuefn/internal/cli"
	"github.com/meigma/crossplane-cuefn/internal/test/common"
)

// runtimeImagePath returns the path to a runtime base image tarball for the
// publish-function command: the REAL apko runtime base (via $CUEFN_RUNTIME_IMAGE
// or <repo>/image.tar) when present, else a freshly written synthetic tarball.
// Preferring the real base exercises the embed-runtime path over the actual apko
// image through the CLI command (folding TestFunctionXpkgRoundTrip's real-base
// coverage) when the funcpkg task has built it, while staying runnable elsewhere.
func runtimeImagePath(t *testing.T, arch string) string {
	t.Helper()
	for _, p := range []string{os.Getenv("CUEFN_RUNTIME_IMAGE"), filepath.Join(common.RepoRoot(t), "image.tar")} {
		if p == "" {
			continue
		}
		if _, err := os.Stat(p); err == nil {
			t.Logf("publish-function using real runtime base image %s", p)
			return p
		}
	}
	return common.WriteRuntimeBaseTarball(t, arch)
}

// TestPublishFunction_EndToEnd proves `cuefn publish-function` assembles the
// Function xpkg over a runtime base and pushes it: the pulled package carries the
// xpkg base annotation and its stream names the Function and Input CRD
// (criterion 1). It prefers the real apko runtime base when present (see
// runtimeImagePath) and asserts the pulled image's digest is a stable sha256 ref
// across re-pulls (folds TestFunctionXpkgRoundTrip). Docker-gated.
func TestPublishFunction_EndToEnd(t *testing.T) {
	reg := common.StartRegistry(t)
	basePath := runtimeImagePath(t, "amd64")
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

	// Round-trip digest stability (folds TestFunctionXpkgRoundTrip): the pulled
	// image resolves to a non-empty sha256 digest that is identical across
	// re-pulls.
	digest, err := img.Digest()
	require.NoError(t, err)
	assert.Equal(t, "sha256", digest.Algorithm, "pulled image digest must be a sha256 ref")
	assert.NotEmpty(t, digest.Hex, "pulled image digest must be non-empty")
	repulled, err := remote.Image(parsed)
	require.NoError(t, err)
	reDigest, err := repulled.Digest()
	require.NoError(t, err)
	assert.Equal(t, digest.String(), reDigest.String(), "round-tripped digest must be identical across pulls")

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
// It asserts not just that two manifests are present but that BOTH target
// platforms (linux/amd64 and linux/arm64) appear (ported from
// TestFunctionIndexRoundTrip). Docker-gated.
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

	// Both target platforms must be present (ported from TestFunctionIndexRoundTrip).
	platforms := map[string]bool{}
	for _, m := range manifest.Manifests {
		require.NotNil(t, m.Platform)
		platforms[m.Platform.OS+"/"+m.Platform.Architecture] = true
	}
	assert.True(t, platforms["linux/amd64"], "index must contain a linux/amd64 manifest")
	assert.True(t, platforms["linux/arm64"], "index must contain a linux/arm64 manifest")
}
