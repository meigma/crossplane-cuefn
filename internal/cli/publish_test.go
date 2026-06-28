package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"testing"

	xpkg "github.com/crossplane/crossplane-runtime/v2/pkg/xpkg"
	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"sigs.k8s.io/yaml"

	inputv1beta1 "github.com/meigma/crossplane-cuefn/input/v1beta1"
	"github.com/meigma/crossplane-cuefn/internal/render"
)

const publishExampleRef = "cuefn.example/app@v0.1.0"

// TestPublish_EndToEnd proves `cuefn publish` runs the whole flow as one command:
// it generates the XRD/Composition, resolves the module's real manifest digest,
// assembles the Configuration xpkg, and pushes it with correct xpkg layer
// annotations (criteria 2 and 3). It then pulls the package back and asserts the
// recorded digest is the registry's actual digest, and that the runtime loader
// would accept that digest (and reject a drifted one).
func TestPublish_EndToEnd(t *testing.T) {
	reg := startRegistry(t)
	reg.publishModule(t, publishExampleRef, "../../example/module")

	cache := t.TempDir()
	t.Setenv("CUE_REGISTRY", reg.cueRegistry)
	t.Setenv("CUE_CACHE_DIR", cache)

	pkgRef := reg.host + "/xapps-configuration:v0.1.0"

	var stdout bytes.Buffer
	root := NewRootCommand(Options{Out: &stdout})
	root.SetArgs([]string{
		"publish", publishExampleRef,
		"--package", pkgRef,
		"--function-ref", "xpkg.meigma.io/cuefn",
		"--function-version", ">=v0.1.0",
		"--insecure",
	})
	require.NoError(t, root.ExecuteContext(context.Background()))
	assert.Contains(t, stdout.String(), "pushed")

	// Pull the pushed Configuration package back.
	parsed, err := name.ParseReference(pkgRef, name.Insecure)
	require.NoError(t, err)
	img, err := remote.Image(parsed)
	require.NoError(t, err)

	// Criterion 3: the package layer carries the xpkg base annotation.
	assertBaseLayerAnnotation(t, img)

	// Criterion 2: the Composition's cuefn input records the REAL resolved digest.
	in := compositionInput(t, img)
	wantDigest := reg.manifestDigest(t, publishExampleRef)
	assert.Equal(t, publishExampleRef, in.Module)
	assert.Equal(t, wantDigest, in.ExpectedDigest, "publish must record the real resolved manifest digest")

	// The runtime verifier (OCIConfig.Expect) accepts the recorded digest...
	okLoader, err := render.NewOCILoader(render.OCIConfig{
		Env:    []string{"CUE_REGISTRY=" + reg.cueRegistry, "CUE_CACHE_DIR=" + t.TempDir()},
		Expect: map[string]string{publishExampleRef: in.ExpectedDigest},
	})
	require.NoError(t, err)
	_, err = okLoader.Load(context.Background(), publishExampleRef)
	require.NoError(t, err, "runtime must accept the published digest for the unchanged module")

	// ...and rejects a drifted digest.
	badLoader, err := render.NewOCILoader(render.OCIConfig{
		Env:    []string{"CUE_REGISTRY=" + reg.cueRegistry, "CUE_CACHE_DIR=" + t.TempDir()},
		Expect: map[string]string{publishExampleRef: "sha256:" + zeros(64)},
	})
	require.NoError(t, err)
	_, err = badLoader.Load(context.Background(), publishExampleRef)
	require.Error(t, err, "runtime must reject a digest that does not match the module")
}

// TestPublish_MalformedModuleRef proves a module ref without @version fails with
// a clear non-nil error naming the ref, and never panics (criterion 3).
func TestPublish_MalformedModuleRef(t *testing.T) {
	t.Parallel()

	root := NewRootCommand(Options{Out: &bytes.Buffer{}, Err: &bytes.Buffer{}})
	root.SetArgs([]string{
		"publish", "cuefn.example/app",
		"--dir", "../../example/module",
		"--package", "localhost:5000/cfg:v0.1.0",
		"--insecure",
	})

	var err error
	require.NotPanics(t, func() {
		err = root.ExecuteContext(context.Background())
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cuefn.example/app")
}

// TestPublish_RequiresPackage proves the destination --package flag is required.
func TestPublish_RequiresPackage(t *testing.T) {
	t.Parallel()

	root := NewRootCommand(Options{Out: &bytes.Buffer{}, Err: &bytes.Buffer{}})
	root.SetArgs([]string{"publish", publishExampleRef, "--dir", "../../example/module"})

	err := root.ExecuteContext(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "package")
}

// assertBaseLayerAnnotation asserts the image has a layer annotated as the xpkg
// base package layer.
func assertBaseLayerAnnotation(t *testing.T, img v1.Image) {
	t.Helper()
	manifest, err := img.Manifest()
	require.NoError(t, err)

	var found bool
	for _, l := range manifest.Layers {
		if l.Annotations[xpkg.AnnotationKey] == xpkg.PackageAnnotation {
			found = true
		}
	}
	assert.True(t, found, "pushed image must carry a layer annotated %s=%s",
		xpkg.AnnotationKey, xpkg.PackageAnnotation)
}

// compositionInput extracts the cuefn step's Input from the Composition embedded
// in the image's package.yaml stream.
func compositionInput(t *testing.T, img v1.Image) inputv1beta1.Input {
	t.Helper()

	rc, err := xpkg.ExtractPackageYAML(img)
	require.NoError(t, err)
	defer rc.Close()
	stream, err := io.ReadAll(rc)
	require.NoError(t, err)

	for chunk := range bytes.SplitSeq(stream, []byte("\n---\n")) {
		var comp struct {
			Kind string `json:"kind"`
			Spec struct {
				Pipeline []struct {
					Step  string          `json:"step"`
					Input json.RawMessage `json:"input"`
				} `json:"pipeline"`
			} `json:"spec"`
		}
		if err := yaml.Unmarshal(chunk, &comp); err != nil || comp.Kind != "Composition" {
			continue
		}
		for _, step := range comp.Spec.Pipeline {
			if step.Step != "cuefn" {
				continue
			}
			var in inputv1beta1.Input
			require.NoError(t, json.Unmarshal(step.Input, &in))
			return in
		}
	}
	t.Fatal("no cuefn pipeline step found in packaged Composition")
	return inputv1beta1.Input{}
}

// zeros returns a string of n '0' runes for building placeholder digests.
func zeros(n int) string {
	b := make([]byte, n)
	for i := range b {
		b[i] = '0'
	}
	return string(b)
}
