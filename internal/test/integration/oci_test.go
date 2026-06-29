package integration_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/meigma/crossplane-cuefn/internal/render"
	"github.com/meigma/crossplane-cuefn/internal/test/common"
)

// Module refs published to the throwaway registry by the integration tests.
const (
	consumerRef = "cuefn.test/consumer@v0.1.0"
	depRef      = "cuefn.test/dep@v0.1.0"
	mutableRef  = "cuefn.test/mutable@v0.1.0"
)

// newOCIEngine builds an Engine backed by a fresh OCILoader for cfg.
func newOCIEngine(t *testing.T, cfg render.OCIConfig) *render.Engine {
	t.Helper()
	loader, err := render.NewOCILoader(cfg)
	require.NoError(t, err)
	return render.New(loader)
}

// TestOCI_EquivalentToLocal proves the OCILoader is a drop-in for LocalLoader: a
// dep-free module published to the registry and loaded over OCI renders
// identically to the same module loaded from disk (criterion C1).
func TestOCI_EquivalentToLocal(t *testing.T) {
	reg := common.StartRegistry(t)
	modulePath := common.HermeticModuleDir(t)
	reg.Publish(t, common.ExampleModuleRef, modulePath)

	in := render.Inputs{
		Spec:        map[string]any{"replicas": float64(3)},
		Metadata:    render.Metadata{Name: "demo"},
		Environment: map[string]any{"tier": "production"},
	}

	oci := newOCIEngine(t, render.OCIConfig{Env: reg.Env(common.CacheDir(t))})
	ociRes, err := oci.Render(context.Background(), common.ExampleModuleRef, in)
	require.NoError(t, err)

	local := render.New(render.LocalLoader{Dir: modulePath})
	localRes, err := local.Render(context.Background(), "ignored", in)
	require.NoError(t, err)

	// The two Results must be structurally identical: same resources, objects,
	// readiness, and status, regardless of which loader produced them.
	assert.Equal(t, localRes, ociRes)
}

// TestOCI_TransitiveDependency proves the injected CUE registry resolves a
// module's transitive OCI dependency at load time: the consumer imports values
// from a separately published dep module and renders them into its output
// (criterion C2).
func TestOCI_TransitiveDependency(t *testing.T) {
	reg := common.StartRegistry(t)
	ociData := filepath.Join(common.RepoRoot(t), "internal/render/testdata/oci")
	reg.Publish(t, depRef, filepath.Join(ociData, "dep"))
	reg.Publish(t, consumerRef, filepath.Join(ociData, "consumer"))

	e := newOCIEngine(t, render.OCIConfig{Env: reg.Env(common.CacheDir(t))})
	res, err := e.Render(context.Background(), consumerRef, render.Inputs{
		Metadata: render.Metadata{Name: "c1"},
	})
	require.NoError(t, err)

	// The image and port come from the imported dep module; resolving them proves
	// the transitive dependency was fetched from the registry.
	spec := containerSpec(t, res, "deployment")
	assert.Equal(t, "ghcr.io/cuefn/dep:1.2.3", spec["image"])
	ports, ok := spec["ports"].([]any)
	require.True(t, ok)
	require.Len(t, ports, 1)
	port, ok := ports[0].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, 8421, common.ToInt(t, port["containerPort"]))
}

// TestOCI_RepublishedTagRefetched proves the loader keys its cache on the
// manifest digest, not the version tag: republishing different content under the
// same version yields a new digest, a cache miss, and a re-render — something
// CUE's version-keyed modcache cannot do (criterion C3).
func TestOCI_RepublishedTagRefetched(t *testing.T) {
	reg := common.StartRegistry(t)
	mutableData := filepath.Join(common.RepoRoot(t), "internal/render/testdata/oci/mutable")

	reg.Publish(t, mutableRef, filepath.Join(mutableData, "v1"))
	digest1 := reg.ManifestDigest(t, mutableRef)

	e := newOCIEngine(t, render.OCIConfig{Env: reg.Env(common.CacheDir(t))})
	res, err := e.Render(context.Background(), mutableRef, render.Inputs{})
	require.NoError(t, err)
	assert.Equal(t, "A", variant(t, res))

	// Republish DIFFERENT content under the SAME version.
	reg.Publish(t, mutableRef, filepath.Join(mutableData, "v2"))
	digest2 := reg.ManifestDigest(t, mutableRef)
	require.NotEqual(t, digest1, digest2, "republished content must change the manifest digest")

	res, err = e.Render(context.Background(), mutableRef, render.Inputs{})
	require.NoError(t, err)
	assert.Equal(t, "B", variant(t, res), "loader must re-fetch on digest change, not serve the cached version")
}

// TestOCI_ServesFromCacheWhenRegistryDown proves the digest-keyed cache survives
// across processes: a fresh loader pointed at the same cache dir renders the
// module after the registry is gone, via the ref->digest pointer (criterion C4).
func TestOCI_ServesFromCacheWhenRegistryDown(t *testing.T) {
	reg := common.StartRegistry(t)
	reg.Publish(t, common.ExampleModuleRef, common.HermeticModuleDir(t))

	cache := common.CacheDir(t)
	in := render.Inputs{Metadata: render.Metadata{Name: "demo"}}

	warm := newOCIEngine(t, render.OCIConfig{Env: reg.Env(cache)})
	want, err := warm.Render(context.Background(), common.ExampleModuleRef, in)
	require.NoError(t, err)

	// Take the registry down, then build a brand-new loader on the warm cache.
	reg.Stop(t)

	cold := newOCIEngine(t, render.OCIConfig{Env: reg.Env(cache)})
	got, err := cold.Render(context.Background(), common.ExampleModuleRef, in)
	require.NoError(t, err, "must serve from cache when the registry is unreachable")

	assert.Equal(t, want, got)
}

// TestOCI_ExpectedDigestMismatch proves the runtime digest lock-step: a stale
// expected digest is rejected, naming the ref and both digests, while the true
// digest renders successfully (criterion C5).
func TestOCI_ExpectedDigestMismatch(t *testing.T) {
	reg := common.StartRegistry(t)
	reg.Publish(t, common.ExampleModuleRef, common.HermeticModuleDir(t))
	in := render.Inputs{Metadata: render.Metadata{Name: "demo"}}

	t.Run("mismatch rejected", func(t *testing.T) {
		bogus := "sha256:" + strings.Repeat("0", 64)
		e := newOCIEngine(t, render.OCIConfig{
			Env:    reg.Env(common.CacheDir(t)),
			Expect: map[string]string{common.ExampleModuleRef: bogus},
		})

		var err error
		require.NotPanics(t, func() {
			_, err = e.Render(context.Background(), common.ExampleModuleRef, in)
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), common.ExampleModuleRef)
		assert.Contains(t, err.Error(), "mismatch")
		assert.Contains(t, err.Error(), bogus)
	})

	t.Run("true digest accepted", func(t *testing.T) {
		actual := reg.ManifestDigest(t, common.ExampleModuleRef)
		e := newOCIEngine(t, render.OCIConfig{
			Env:    reg.Env(common.CacheDir(t)),
			Expect: map[string]string{common.ExampleModuleRef: actual},
		})

		_, err := e.Render(context.Background(), common.ExampleModuleRef, in)
		require.NoError(t, err)
	})
}

// TestOCI_ResolveDigest proves the publish-time digest seam returns the same
// manifest digest the registry reports, and that it errors cleanly (no panic) on
// a malformed or unpublished ref. This is the author half of the digest
// lock-step: the value recorded in a published Composition's cuefn input.
func TestOCI_ResolveDigest(t *testing.T) {
	reg := common.StartRegistry(t)
	reg.Publish(t, common.ExampleModuleRef, common.HermeticModuleDir(t))

	loader, err := render.NewOCILoader(render.OCIConfig{Env: reg.Env(common.CacheDir(t))})
	require.NoError(t, err)

	t.Run("matches registry digest", func(t *testing.T) {
		got, err := loader.ResolveDigest(context.Background(), common.ExampleModuleRef)
		require.NoError(t, err)
		assert.Equal(t, reg.ManifestDigest(t, common.ExampleModuleRef), got)
		assert.True(t, strings.HasPrefix(got, "sha256:"), "digest must be a sha256 ref, got %q", got)
	})

	t.Run("malformed ref", func(t *testing.T) {
		var err error
		require.NotPanics(t, func() {
			_, err = loader.ResolveDigest(context.Background(), "cuefn.test/noversion")
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "cuefn.test/noversion")
	})

	t.Run("unpublished ref", func(t *testing.T) {
		var err error
		require.NotPanics(t, func() {
			_, err = loader.ResolveDigest(context.Background(), "cuefn.test/missing@v0.1.0")
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "cuefn.test/missing@v0.1.0")
		assert.Contains(t, err.Error(), "not found")
	})
}

// TestOCI_ErrorPaths proves the loader classifies failures and never panics: a
// missing ref, a malformed ref, and an unreachable registry each produce a
// wrapped error naming the ref or cause (criterion C6).
func TestOCI_ErrorPaths(t *testing.T) {
	reg := common.StartRegistry(t)

	t.Run("unpublished ref", func(t *testing.T) {
		e := newOCIEngine(t, render.OCIConfig{Env: reg.Env(common.CacheDir(t))})
		var err error
		require.NotPanics(t, func() {
			_, err = e.Render(context.Background(), "cuefn.test/missing@v0.1.0", render.Inputs{})
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "cuefn.test/missing@v0.1.0")
		assert.Contains(t, err.Error(), "not found")
	})

	t.Run("malformed ref", func(t *testing.T) {
		e := newOCIEngine(t, render.OCIConfig{Env: reg.Env(common.CacheDir(t))})
		var err error
		require.NotPanics(t, func() {
			_, err = e.Render(context.Background(), "cuefn.test/noversion", render.Inputs{})
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "cuefn.test/noversion")
	})

	t.Run("unreachable registry", func(t *testing.T) {
		// A closed port with an empty cache: no online resolve and no cached copy.
		env := []string{
			"CUE_REGISTRY=localhost:1+insecure",
			"CUE_CACHE_DIR=" + common.CacheDir(t),
		}
		e := newOCIEngine(t, render.OCIConfig{Env: env})
		var err error
		require.NotPanics(t, func() {
			_, err = e.Render(context.Background(), "cuefn.test/whatever@v0.1.0", render.Inputs{})
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "cuefn.test/whatever@v0.1.0")
		assert.Contains(t, err.Error(), "reach")
	})
}

// TestOCI_NonrootCacheDir proves caching works at a writable non-$HOME location:
// rendering with an explicit CacheDir populates the digest-keyed cache under it,
// as a nonroot or read-only-root container requires (criterion C7).
func TestOCI_NonrootCacheDir(t *testing.T) {
	reg := common.StartRegistry(t)
	reg.Publish(t, common.ExampleModuleRef, common.HermeticModuleDir(t))

	cache := common.CacheDir(t)
	e := newOCIEngine(t, render.OCIConfig{
		Env:      reg.Env("/nonexistent-ignored"),
		CacheDir: cache, // explicit CacheDir wins over CUE_CACHE_DIR in Env.
	})

	_, err := e.Render(
		context.Background(),
		common.ExampleModuleRef,
		render.Inputs{Metadata: render.Metadata{Name: "demo"}},
	)
	require.NoError(t, err)

	ociCache := filepath.Join(cache, "cuefn-oci")
	require.DirExists(t, ociCache)

	// The digest-keyed extraction directory must exist and contain the module's
	// cue.mod, proving the module was cached under the writable path.
	var foundModule bool
	require.NoError(t, filepath.WalkDir(ociCache, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && filepath.Base(p) == "module.cue" {
			foundModule = true
		}
		return nil
	}))
	assert.True(t, foundModule, "expected an extracted cue.mod/module.cue under %s", ociCache)
}

// containerSpec returns the Deployment object's pod-template container map's
// enclosing spec for the named resource (the first container).
func containerSpec(t *testing.T, res render.Result, name string) map[string]any {
	t.Helper()
	obj := common.Object(t, res, name)
	spec, ok := obj["spec"].(map[string]any)
	require.True(t, ok)
	tmpl, ok := spec["template"].(map[string]any)
	require.True(t, ok)
	podSpec, ok := tmpl["spec"].(map[string]any)
	require.True(t, ok)
	containers, ok := podSpec["containers"].([]any)
	require.True(t, ok)
	require.Len(t, containers, 1)
	c, ok := containers[0].(map[string]any)
	require.True(t, ok)
	return c
}

// variant returns the "variant" data value rendered by the mutable fixture's
// ConfigMap resource.
func variant(t *testing.T, res render.Result) string {
	t.Helper()
	obj := common.Object(t, res, "config")
	data, ok := obj["data"].(map[string]any)
	require.True(t, ok)
	v, ok := data["variant"].(string)
	require.True(t, ok)
	return v
}
