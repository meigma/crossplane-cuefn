package pkg_test

import (
	"bytes"
	"compress/gzip"
	"context"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	xpkg "github.com/crossplane/crossplane-runtime/v2/pkg/xpkg"
	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/empty"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/google/go-containerregistry/pkg/v1/tarball"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"sigs.k8s.io/yaml"

	"github.com/meigma/crossplane-cuefn/internal/pkg"
)

// TestConfigurationRoundTrip proves a push->pull round-trip from a throwaway
// registry re-yields byte-identical package contents and the same manifest
// digest (criterion 1b).
func TestConfigurationRoundTrip(t *testing.T) {
	reg := startRegistry(t)

	img, err := pkg.BuildConfigurationImage(buildFixtureConfiguration(t))
	require.NoError(t, err)

	want, err := packageYAMLBytes(img)
	require.NoError(t, err)
	wantDigest, err := img.Digest()
	require.NoError(t, err)

	ref := reg.host + "/xapps-configuration:v0.1.0"
	dst, err := pkg.Push(context.Background(), ref, img, true)
	require.NoError(t, err)
	assert.Equal(t, wantDigest.String(), dst.DigestStr(), "pushed digest must name the built image")

	parsed, err := name.ParseReference(ref, name.Insecure)
	require.NoError(t, err)
	pulled, err := remote.Image(parsed)
	require.NoError(t, err)

	gotDigest, err := pulled.Digest()
	require.NoError(t, err)
	assert.Equal(t, wantDigest.String(), gotDigest.String(), "round-tripped manifest digest must match")

	got, err := packageYAMLBytes(pulled)
	require.NoError(t, err)
	assert.Equal(t, want, got, "round-tripped package.yaml must be byte-identical")
}

// TestXpkgValidate proves the crossplane CLI accepts the built Configuration
// package: written as a local .xpkg tarball, `crossplane xpkg extract` parses it
// into its cache format, and the extracted stream names the Configuration, XRD,
// and Composition (criterion 1c). No registry/Docker needed; crossplane-gated.
func TestXpkgValidate(t *testing.T) {
	bin := requireCrossplane(t)

	img, err := pkg.BuildConfigurationImage(buildFixtureConfiguration(t))
	require.NoError(t, err)

	kinds := extractKinds(t, bin, img, "configuration")
	assert.Contains(t, kinds, "Configuration")
	assert.Contains(t, kinds, "CompositeResourceDefinition")
	assert.Contains(t, kinds, "Composition")
}

// TestFunctionXpkgValidate_Prototype de-risks P6: the same BuildXpkgImage shape
// assembles a Function xpkg (a meta.pkg.crossplane.io Function as the package
// layer), and `crossplane xpkg extract` accepts it too. A shipping embed-runtime
// Function would pass the runtime image as the base instead of empty.Image; the
// package layer and annotations are identical, which is what this validates
// (criterion 4).
func TestFunctionXpkgValidate_Prototype(t *testing.T) {
	bin := requireCrossplane(t)

	meta := map[string]any{
		"apiVersion": "meta.pkg.crossplane.io/v1",
		"kind":       "Function",
		"metadata":   map[string]any{"name": "cuefn"},
	}
	metaYAML, err := yaml.Marshal(meta)
	require.NoError(t, err)

	img, err := pkg.BuildXpkgImage(empty.Image, metaYAML)
	require.NoError(t, err)

	kinds := extractKinds(t, bin, img, "function")
	assert.Contains(t, kinds, "Function")
}

// TestPush_UnreachableDestination proves a push to a closed port returns a clear
// non-nil error naming the destination, never a panic (criterion 3).
func TestPush_UnreachableDestination(t *testing.T) {
	img, err := pkg.BuildConfigurationImage(buildFixtureConfiguration(t))
	require.NoError(t, err)

	const ref = "127.0.0.1:1/xapps-configuration:v0.1.0"
	var perr error
	require.NotPanics(t, func() {
		_, perr = pkg.Push(context.Background(), ref, img, true)
	})
	require.Error(t, perr)
	assert.Contains(t, perr.Error(), ref)
}

// TestPush_MalformedReference proves a malformed destination ref errors cleanly.
func TestPush_MalformedReference(t *testing.T) {
	img, err := pkg.BuildConfigurationImage(buildFixtureConfiguration(t))
	require.NoError(t, err)

	var perr error
	require.NotPanics(t, func() {
		_, perr = pkg.Push(context.Background(), "NOT A REF", img, false)
	})
	require.Error(t, perr)
}

// packageYAMLBytes reads the image's package.yaml layer back into bytes.
func packageYAMLBytes(img v1.Image) ([]byte, error) {
	rc, err := xpkg.ExtractPackageYAML(img)
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	return io.ReadAll(rc)
}

// extractKinds writes img as a local .xpkg, runs `crossplane xpkg extract` over
// it, and returns the set of Kinds present in the extracted (gzipped) stream. It
// fails the test if the CLI exits non-zero, which is the validation assertion.
func extractKinds(t *testing.T, bin string, img v1.Image, base string) map[string]bool {
	t.Helper()

	dir := t.TempDir()
	xpkgPath := filepath.Join(dir, base+".xpkg")
	tag, err := name.NewTag("local/" + base + ":test")
	require.NoError(t, err)
	require.NoError(t, tarball.WriteToFile(xpkgPath, tag, img))

	outPath := filepath.Join(dir, "out.gz")
	cmd := exec.Command(bin, "xpkg", "extract", "--from-xpkg", xpkgPath, "-o", outPath)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "crossplane xpkg extract must accept the package:\n%s", out)

	raw, err := os.ReadFile(outPath)
	require.NoError(t, err)
	gz, err := gzip.NewReader(bytes.NewReader(raw))
	require.NoError(t, err)
	defer gz.Close()
	stream, err := io.ReadAll(gz)
	require.NoError(t, err)

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
