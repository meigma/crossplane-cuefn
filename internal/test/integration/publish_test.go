//go:build !noxpkg

package integration_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"strings"
	"testing"

	"cuelabs.dev/go/oci/ociregistry/ociclient"
	"cuelang.org/go/mod/modregistry"
	"cuelang.org/go/mod/module"
	xpkg "github.com/crossplane/crossplane-runtime/v2/pkg/xpkg"
	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"sigs.k8s.io/yaml"

	inputv1beta1 "github.com/meigma/crossplane-cuefn/input/v1beta1"
	"github.com/meigma/crossplane-cuefn/internal/cli"
	"github.com/meigma/crossplane-cuefn/internal/render"
	"github.com/meigma/crossplane-cuefn/internal/test/common"
)

// TestPublish_EndToEnd proves `cuefn publish` runs the whole flow as one command:
// it generates the XRD/Composition, resolves the module's real manifest digest,
// assembles the Configuration xpkg, and pushes it with correct xpkg layer
// annotations (criteria 2 and 3). It then pulls the package back and asserts the
// recorded digest is the registry's actual digest, and that the runtime loader
// would accept that digest (and reject a drifted one).
func TestPublish_EndToEnd(t *testing.T) {
	reg := common.StartRegistry(t)
	reg.Publish(t, common.ExampleModuleRef, common.HermeticModuleDir(t))

	cache := t.TempDir()
	t.Setenv("CUE_REGISTRY", reg.CUERegistry())
	t.Setenv("CUE_CACHE_DIR", cache)

	pkgRef := reg.Host() + "/xapps-configuration:v0.1.0"

	var stdout bytes.Buffer
	root := cli.NewRootCommand(cli.Options{Out: &stdout})
	root.SetArgs([]string{
		"publish", common.ExampleModuleRef,
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

	// Round-trip digest stability (folds TestConfigurationRoundTrip): the pulled
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

	// Criterion 3: the package layer carries the xpkg base annotation.
	assertBaseLayerAnnotation(t, img)

	// Criterion 2: the Composition's cuefn input records the REAL resolved digest.
	in := compositionInput(t, img)
	wantDigest := reg.ManifestDigest(t, common.ExampleModuleRef)
	assert.Equal(t, common.ExampleModuleRef, in.Module)
	assert.Equal(t, wantDigest, in.ExpectedDigest, "publish must record the real resolved manifest digest")

	// The runtime verifier (OCIConfig.Expect) accepts the recorded digest...
	okLoader, err := render.NewOCILoader(render.OCIConfig{
		Env:    []string{"CUE_REGISTRY=" + reg.CUERegistry(), "CUE_CACHE_DIR=" + t.TempDir()},
		Expect: map[string]string{common.ExampleModuleRef: in.ExpectedDigest},
	})
	require.NoError(t, err)
	_, err = okLoader.Load(context.Background(), common.ExampleModuleRef)
	require.NoError(t, err, "runtime must accept the published digest for the unchanged module")

	// ...and rejects a drifted digest.
	badLoader, err := render.NewOCILoader(render.OCIConfig{
		Env:    []string{"CUE_REGISTRY=" + reg.CUERegistry(), "CUE_CACHE_DIR=" + t.TempDir()},
		Expect: map[string]string{common.ExampleModuleRef: "sha256:" + common.Zeros(64)},
	})
	require.NoError(t, err)
	_, err = badLoader.Load(context.Background(), common.ExampleModuleRef)
	require.Error(t, err, "runtime must reject a digest that does not match the module")
}

func TestPublish_ModuleAndConfigurationEndToEnd(t *testing.T) {
	reg := common.StartRegistry(t)
	cache := t.TempDir()
	t.Setenv("CUE_REGISTRY", reg.CUERegistry())
	t.Setenv("CUE_CACHE_DIR", cache)

	pkgRef := reg.Host() + "/combined-configuration:v0.1.0"
	metadata := []string{
		"org.opencontainers.image.source=https://github.com/meigma/example?ref=v0.1.0",
		"dev.meigma.owner=platform=team",
	}
	stdout := executeCombinedPublish(t, common.ExampleModuleRef, pkgRef, metadata)
	assert.Contains(t, stdout, "published module "+common.ExampleModuleRef+"@sha256:")
	assert.Contains(t, stdout, "pushed "+reg.Host()+"/combined-configuration@sha256:")

	manifest, moduleDigest := pullModuleManifest(t, reg, common.ExampleModuleRef)
	assert.Equal(t, "https://github.com/meigma/example?ref=v0.1.0",
		manifest.Annotations["org.opencontainers.image.source"])
	assert.Equal(t, "platform=team", manifest.Annotations["dev.meigma.owner"])
	require.Len(t, manifest.Layers, 2)
	assert.Equal(t, "application/zip", string(manifest.Layers[0].MediaType))
	assert.Equal(t, "application/vnd.cue.modulefile.v1", string(manifest.Layers[1].MediaType))

	img := pullConfiguration(t, pkgRef)
	config, err := img.ConfigFile()
	require.NoError(t, err)
	assert.Equal(t, "https://github.com/meigma/example?ref=v0.1.0",
		config.Config.Labels["org.opencontainers.image.source"])
	assert.Equal(t, "platform=team", config.Config.Labels["dev.meigma.owner"])
	assert.Equal(t, moduleDigest, compositionInput(t, img).ExpectedDigest)
	loader, err := render.NewOCILoader(render.OCIConfig{
		Env:    reg.Env(t.TempDir()),
		Expect: map[string]string{common.ExampleModuleRef: moduleDigest},
	})
	require.NoError(t, err)
	_, err = loader.Load(context.Background(), common.ExampleModuleRef)
	require.NoError(t, err, "runtime loader should accept the digest from combined publication")

	retryStdout := executeCombinedPublish(t, common.ExampleModuleRef, pkgRef, metadata)
	assert.Contains(t, retryStdout, "reused module "+common.ExampleModuleRef+"@"+moduleDigest)
	_, retryDigest := pullModuleManifest(t, reg, common.ExampleModuleRef)
	assert.Equal(t, moduleDigest, retryDigest)

	root := cli.NewRootCommand(cli.Options{Out: &bytes.Buffer{}})
	root.SetArgs([]string{
		"publish", common.ExampleModuleRef,
		"--dir", common.HermeticModuleDir(t),
		"--package", pkgRef,
		"--publish-module",
		"--metadata", "org.opencontainers.image.source=https://github.com/meigma/different",
		"--insecure",
	})
	err = root.ExecuteContext(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "immutable")
	_, unchangedDigest := pullModuleManifest(t, reg, common.ExampleModuleRef)
	assert.Equal(t, moduleDigest, unchangedDigest)

	metadataOnlyRef := reg.Host() + "/metadata-only-configuration:v0.1.0"
	metadataOnlyRoot := cli.NewRootCommand(cli.Options{Out: &bytes.Buffer{}})
	metadataOnlyRoot.SetArgs([]string{
		"publish", common.ExampleModuleRef,
		"--package", metadataOnlyRef,
		"--metadata", "dev.meigma.configuration=only",
		"--insecure",
	})
	require.NoError(t, metadataOnlyRoot.ExecuteContext(context.Background()))
	metadataOnlyConfig, err := pullConfiguration(t, metadataOnlyRef).ConfigFile()
	require.NoError(t, err)
	assert.Equal(t, "only", metadataOnlyConfig.Config.Labels["dev.meigma.configuration"])
	moduleAfterMetadataOnly, metadataOnlyDigest := pullModuleManifest(t, reg, common.ExampleModuleRef)
	assert.Equal(t, moduleDigest, metadataOnlyDigest)
	assert.NotContains(t, moduleAfterMetadataOnly.Annotations, "dev.meigma.configuration")
}

func TestPublish_ModuleSuccessConfigurationFailureIsRetrySafe(t *testing.T) {
	reg := common.StartRegistry(t)
	t.Setenv("CUE_REGISTRY", reg.CUERegistry())
	t.Setenv("CUE_CACHE_DIR", t.TempDir())

	ref := strings.Replace(common.ExampleModuleRef, "v0.1.0", "v0.1.1", 1)
	pkgRef := "localhost:1/unreachable-configuration:v0.1.1"
	root := cli.NewRootCommand(cli.Options{Out: &bytes.Buffer{}})
	root.SetArgs([]string{
		"publish", ref,
		"--dir", common.HermeticModuleDir(t),
		"--package", pkgRef,
		"--publish-module",
		"--insecure",
	})

	err := root.ExecuteContext(context.Background())
	require.Error(t, err)
	_, digest := pullModuleManifest(t, reg, ref)
	assert.Contains(t, err.Error(), ref+"@"+digest)
	assert.Contains(t, err.Error(), pkgRef)
	assert.Contains(t, err.Error(), "retrying the same command is safe")
}

func executeCombinedPublish(
	t *testing.T,
	ref string,
	pkgRef string,
	metadata []string,
) string {
	t.Helper()
	var stdout bytes.Buffer
	root := cli.NewRootCommand(cli.Options{Out: &stdout})
	args := []string{
		"publish", ref,
		"--dir", common.HermeticModuleDir(t),
		"--package", pkgRef,
		"--publish-module",
		"--insecure",
	}
	for _, value := range metadata {
		args = append(args, "--metadata", value)
	}
	root.SetArgs(args)
	require.NoError(t, root.ExecuteContext(context.Background()))
	return stdout.String()
}

func pullModuleManifest(t *testing.T, reg *common.Registry, ref string) (v1.Manifest, string) {
	t.Helper()
	mv, err := module.ParseVersion(ref)
	require.NoError(t, err)
	client, err := ociclient.New(reg.Host(), &ociclient.Options{Insecure: true})
	require.NoError(t, err)
	reader, err := client.GetTag(context.Background(), mv.BasePath(), mv.Version())
	require.NoError(t, err)
	digest := reader.Descriptor().Digest.String()
	defer reader.Close()
	var manifest v1.Manifest
	require.NoError(t, json.NewDecoder(reader).Decode(&manifest))

	loaded, err := modregistry.NewClient(client).GetModule(context.Background(), mv)
	require.NoError(t, err)
	assert.Equal(t, digest, loaded.ManifestDigest().String())
	return manifest, digest
}

func pullConfiguration(t *testing.T, ref string) v1.Image {
	t.Helper()
	parsed, err := name.ParseReference(ref, name.Insecure)
	require.NoError(t, err)
	img, err := remote.Image(parsed)
	require.NoError(t, err)
	return img
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
