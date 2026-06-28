//go:build !noxpkg

package cli

import (
	"bytes"
	"context"
	"io"
	"path/filepath"
	"testing"

	xpkg "github.com/crossplane/crossplane-runtime/v2/pkg/xpkg"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/google/go-containerregistry/pkg/v1/random"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/google/go-containerregistry/pkg/v1/tarball"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"sigs.k8s.io/yaml"
)

// writeRuntimeBaseTarball writes a synthetic runtime base image (entrypoint
// /usr/bin/cuefn, cmd function) to a docker/OCI tarball and returns its path,
// standing in for the apko image.tar the publish-function command loads.
func writeRuntimeBaseTarball(t *testing.T, arch string) string {
	t.Helper()

	img, err := random.Image(128, 1)
	require.NoError(t, err)
	cfg, err := img.ConfigFile()
	require.NoError(t, err)
	cfg = cfg.DeepCopy()
	cfg.OS = "linux"
	cfg.Architecture = arch
	cfg.Config.Entrypoint = []string{"/usr/bin/cuefn"}
	cfg.Config.Cmd = []string{"function"}
	img, err = mutate.ConfigFile(img, cfg)
	require.NoError(t, err)

	path := filepath.Join(t.TempDir(), "image-"+arch+".tar")
	tag, err := name.NewTag("crossplane-cuefn:dev-" + arch)
	require.NoError(t, err)
	require.NoError(t, tarball.WriteToFile(path, tag, img))
	return path
}

// TestPublishFunction_EndToEnd proves `cuefn publish-function` assembles the
// Function xpkg over a runtime base tarball and pushes it: the pulled package
// carries the xpkg base annotation and its stream names the Function and Input
// CRD (criterion 1). Docker-gated.
func TestPublishFunction_EndToEnd(t *testing.T) {
	reg := startRegistry(t)
	basePath := writeRuntimeBaseTarball(t, "amd64")
	pkgRef := reg.host + "/function-cuefn:v0.1.0"

	var stdout bytes.Buffer
	root := NewRootCommand(Options{Out: &stdout})
	root.SetArgs([]string{
		publishFunctionUse,
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

	kinds := streamKinds(t, stream)
	assert.True(t, kinds["Function"], "package must contain the Function meta")
	assert.True(t, kinds["CustomResourceDefinition"], "package must embed the Input CRD")
}

// TestPublishFunction_MultiArchIndex proves several --runtime-image bases push a
// multi-arch index that pulls back with both platform manifests (release path).
// Docker-gated.
func TestPublishFunction_MultiArchIndex(t *testing.T) {
	reg := startRegistry(t)
	amd := writeRuntimeBaseTarball(t, "amd64")
	arm := writeRuntimeBaseTarball(t, "arm64")
	pkgRef := reg.host + "/function-cuefn:v0.1.0-index"

	root := NewRootCommand(Options{Out: &bytes.Buffer{}})
	root.SetArgs([]string{
		publishFunctionUse,
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

// TestPublishFunction_OutputLocalXpkg proves --output assembles the Function
// package over a runtime base tarball and writes a local .xpkg (no registry)
// whose package layer re-parses into the Function + Input CRD. This is the
// no-push path the release dry run uses with `crossplane xpkg extract`.
func TestPublishFunction_OutputLocalXpkg(t *testing.T) {
	t.Parallel()

	basePath := writeRuntimeBaseTarball(t, "amd64")
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

	kinds := streamKinds(t, stream)
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

// streamKinds returns the set of Kinds present in a YAML stream.
func streamKinds(t *testing.T, stream []byte) map[string]bool {
	t.Helper()
	kinds := map[string]bool{}
	for chunk := range bytes.SplitSeq(stream, []byte("\n---\n")) {
		var doc struct {
			Kind string `json:"kind"`
		}
		if err := yaml.Unmarshal(chunk, &doc); err == nil && doc.Kind != "" {
			kinds[doc.Kind] = true
		}
	}
	return kinds
}
