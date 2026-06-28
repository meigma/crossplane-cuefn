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

// TestFunctionXpkgValidate proves the shipped embed-runtime Function xpkg: it is
// assembled over a real runtime image base (the apko-built crossplane-cuefn:dev
// when present, else a synthetic runtime stand-in), the package layer rides on
// top of the base without disturbing its runtime layers/entrypoint, and
// `crossplane xpkg extract` accepts the package — its stream naming both the
// Function and the embedded Input CRD (criterion 1). crossplane-gated.
func TestFunctionXpkgValidate(t *testing.T) {
	bin := requireCrossplane(t)

	base := runtimeBaseImage(t)
	baseLayers, err := base.Layers()
	require.NoError(t, err)

	img, err := pkg.BuildFunctionImage(base, fixtureFunction(t))
	require.NoError(t, err)

	// The runtime layers survive under the package layer (embed-runtime).
	imgLayers, err := img.Layers()
	require.NoError(t, err)
	assert.Len(t, imgLayers, len(baseLayers)+1, "package layer must ride on top of the runtime layers")

	cfg, err := img.ConfigFile()
	require.NoError(t, err)
	assert.Equal(t, []string{"/usr/bin/cuefn"}, cfg.Config.Entrypoint, "runtime entrypoint must be preserved")

	kinds := extractKinds(t, bin, img, "function")
	assert.Contains(t, kinds, "Function")
	assert.Contains(t, kinds, "CustomResourceDefinition")
}

// TestFunctionXpkgRoundTrip proves a push->pull round-trip of the Function xpkg
// from a throwaway registry re-yields byte-identical package contents and the
// same manifest digest (criterion 1). Docker-gated.
func TestFunctionXpkgRoundTrip(t *testing.T) {
	reg := startRegistry(t)

	img, err := pkg.BuildFunctionImage(runtimeBaseImage(t), fixtureFunction(t))
	require.NoError(t, err)

	want, err := packageYAMLBytes(img)
	require.NoError(t, err)
	wantDigest, err := img.Digest()
	require.NoError(t, err)

	ref := reg.host + "/function-cuefn:v0.1.0"
	dst, err := pkg.Push(context.Background(), ref, img, true)
	require.NoError(t, err)
	assert.Equal(t, wantDigest.String(), dst.DigestStr())

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

// TestFunctionIndexRoundTrip proves the multi-arch release path: a Function xpkg
// index over two per-arch bases pushes to a throwaway registry and pulls back as
// an index with both platform manifests (criterion 1/2 multi-arch). Docker-gated.
func TestFunctionIndexRoundTrip(t *testing.T) {
	reg := startRegistry(t)

	bases := []v1.Image{fakeRuntimeBase(t, "amd64"), fakeRuntimeBase(t, "arm64")}
	idx, err := pkg.BuildFunctionIndex(bases, fixtureFunction(t))
	require.NoError(t, err)

	ref := reg.host + "/function-cuefn:v0.1.0-index"
	_, err = pkg.PushIndex(context.Background(), ref, idx, true)
	require.NoError(t, err)

	parsed, err := name.ParseReference(ref, name.Insecure)
	require.NoError(t, err)
	pulled, err := remote.Index(parsed)
	require.NoError(t, err)

	manifest, err := pulled.IndexManifest()
	require.NoError(t, err)
	require.Len(t, manifest.Manifests, 2)

	platforms := map[string]bool{}
	for _, m := range manifest.Manifests {
		require.NotNil(t, m.Platform)
		platforms[m.Platform.OS+"/"+m.Platform.Architecture] = true
	}
	assert.True(t, platforms["linux/amd64"])
	assert.True(t, platforms["linux/arm64"])
}

// TestFunctionXpkgCosign proves the supply-chain mechanism end to end with a
// named command: push the Function xpkg to a throwaway registry, sign it with a
// locally-generated cosign key, and verify the signature (criterion 2, local).
// Docker + cosign gated; run under `mise exec` so the pinned cosign wins.
func TestFunctionXpkgCosign(t *testing.T) {
	cosign := requireBinary(t, "cosign")
	reg := startRegistry(t)

	img, err := pkg.BuildFunctionImage(runtimeBaseImage(t), fixtureFunction(t))
	require.NoError(t, err)

	ref := reg.host + "/function-cuefn:v0.1.0"
	dst, err := pkg.Push(context.Background(), ref, img, true)
	require.NoError(t, err)

	dir := t.TempDir()
	keygen := exec.Command(cosign, "generate-key-pair")
	keygen.Dir = dir
	keygen.Env = append(os.Environ(), "COSIGN_PASSWORD=")
	out, err := keygen.CombinedOutput()
	require.NoError(t, err, "cosign generate-key-pair:\n%s", out)

	// Key signing over a throwaway registry, no transparency log: --tlog-upload
	// and --use-signing-config are both disabled so cosign neither reaches the
	// public Rekor/TUF nor a default signing config (offline proof of the
	// mechanism, not a real keyless release).
	digestRef := dst.String()
	sign := exec.Command(cosign, "sign", "--key", "cosign.key", "--yes",
		"--allow-insecure-registry", "--use-signing-config=false", "--tlog-upload=false", digestRef)
	sign.Dir = dir
	sign.Env = append(os.Environ(), "COSIGN_PASSWORD=")
	out, err = sign.CombinedOutput()
	require.NoError(t, err, "cosign sign:\n%s", out)

	verify := exec.Command(cosign, "verify", "--key", "cosign.pub",
		"--allow-insecure-registry", "--insecure-ignore-tlog=true", digestRef)
	verify.Dir = dir
	out, err = verify.CombinedOutput()
	require.NoError(t, err, "cosign verify:\n%s", out)
}

// TestFunctionXpkgSBOM proves an SBOM can be generated and parsed for the pushed
// Function xpkg with a named command (criterion 2, local). syft-gated; run under
// `mise exec` so the pinned syft wins.
func TestFunctionXpkgSBOM(t *testing.T) {
	syft := requireBinary(t, "syft")
	reg := startRegistry(t)

	img, err := pkg.BuildFunctionImage(runtimeBaseImage(t), fixtureFunction(t))
	require.NoError(t, err)

	ref := reg.host + "/function-cuefn:v0.1.0"
	dst, err := pkg.Push(context.Background(), ref, img, true)
	require.NoError(t, err)

	cmd := exec.Command(syft, "scan", "registry:"+dst.String(), "-o", "spdx-json")
	cmd.Env = append(os.Environ(),
		"SYFT_REGISTRY_INSECURE_USE_HTTP=true",
		"SYFT_REGISTRY_INSECURE_SKIP_TLS_VERIFY=true",
	)
	out, err := cmd.Output()
	require.NoError(t, err, "syft scan must produce an SBOM")

	var doc struct {
		SPDXID   string `json:"SPDXID"`
		Packages []struct {
			Name string `json:"name"`
		} `json:"packages"`
	}
	require.NoError(t, yaml.Unmarshal(out, &doc))
	assert.NotEmpty(t, doc.SPDXID, "SBOM must be a parseable SPDX document")
	assert.NotEmpty(t, doc.Packages, "SBOM must list packages")
}

// runtimeBaseImage loads the real apko-built runtime image when it is present as
// a local tarball (the image.tar `mise run image-local` writes, or the path in
// CUEFN_RUNTIME_IMAGE) and otherwise falls back to a synthetic runtime stand-in,
// so the gated tests prove the embed-runtime path over the real image in the
// funcpkg-test task while staying runnable elsewhere.
func runtimeBaseImage(t *testing.T) v1.Image {
	t.Helper()

	candidates := []string{os.Getenv("CUEFN_RUNTIME_IMAGE")}
	if root := repoRoot(t); root != "" {
		candidates = append(candidates, filepath.Join(root, "image.tar"))
	}
	for _, p := range candidates {
		if p == "" {
			continue
		}
		if _, err := os.Stat(p); err != nil {
			continue
		}
		img, err := tarball.ImageFromPath(p, nil)
		require.NoError(t, err, "load runtime base tarball %q", p)
		t.Logf("using real runtime base image from %s", p)
		return img
	}

	t.Log(
		"no runtime image tarball found; using a synthetic runtime base (set CUEFN_RUNTIME_IMAGE or run `mise run image-local`)",
	)
	return fakeRuntimeBase(t, "amd64")
}

// repoRoot walks up from the test's working directory to the module root.
func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	require.NoError(t, err)
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}

// requireBinary skips the calling test unless bin is on PATH.
func requireBinary(t *testing.T, bin string) string {
	t.Helper()
	path, err := exec.LookPath(bin)
	if err != nil {
		t.Skipf("%s not on PATH; skipping (run via `mise exec`)", bin)
	}
	return path
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
