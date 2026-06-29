//go:build !noxpkg

package cli

import (
	"bytes"
	"context"
	"io"
	"path/filepath"
	"testing"

	xpkg "github.com/crossplane/crossplane-runtime/v2/pkg/xpkg"
	"github.com/google/go-containerregistry/pkg/v1/tarball"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/meigma/crossplane-cuefn/internal/test/common"
)

// TestPublishFunction_OutputLocalXpkg proves --output assembles the Function
// package over a runtime base tarball and writes a local .xpkg (no registry)
// whose package layer re-parses into the Function + Input CRD. This is the
// no-push path the release dry run uses with `crossplane xpkg extract`.
func TestPublishFunction_OutputLocalXpkg(t *testing.T) {
	t.Parallel()

	basePath := common.WriteRuntimeBaseTarball(t, "amd64")
	out := filepath.Join(t.TempDir(), "function.xpkg")

	var stdout bytes.Buffer
	root := NewRootCommand(Options{Out: &stdout})
	root.SetArgs([]string{
		publishFunctionUse,
		"--runtime-image", basePath,
		"--output", out,
	})
	require.NoError(t, root.ExecuteContext(context.Background()))
	assert.Contains(t, stdout.String(), "wrote")

	img, err := tarball.ImageFromPath(out, nil)
	require.NoError(t, err)
	rc, err := xpkg.ExtractPackageYAML(img)
	require.NoError(t, err)
	defer rc.Close()
	stream, err := io.ReadAll(rc)
	require.NoError(t, err)

	kinds := common.StreamKinds(stream)
	assert.True(t, kinds["Function"])
	assert.True(t, kinds["CustomResourceDefinition"])
}

// TestPublishFunction_RequiresFlags proves a destination (--package or --output)
// and the --runtime-image base are required.
func TestPublishFunction_RequiresFlags(t *testing.T) {
	t.Parallel()

	t.Run("missing package", func(t *testing.T) {
		t.Parallel()
		root := NewRootCommand(Options{Out: &bytes.Buffer{}, Err: &bytes.Buffer{}})
		root.SetArgs([]string{publishFunctionUse, "--runtime-image", "image.tar"})
		require.Error(t, root.ExecuteContext(context.Background()))
	})

	t.Run("missing runtime-image", func(t *testing.T) {
		t.Parallel()
		root := NewRootCommand(Options{Out: &bytes.Buffer{}, Err: &bytes.Buffer{}})
		root.SetArgs([]string{publishFunctionUse, "--package", "localhost:5000/fn:v0"})
		require.Error(t, root.ExecuteContext(context.Background()))
	})
}
