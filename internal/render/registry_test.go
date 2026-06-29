package render

import (
	"testing"

	"cuelang.org/go/mod/modconfig"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestBuildRegistry_Routing proves the registry-routing contract the
// dependency-aware local loader relies on, fully offline (ResolveToLocation is
// pure config-based routing, no network): the central registry is the catch-all
// default unless CUE_REGISTRY is a bare value, so a module importing a public
// dependency resolves it from central without the author configuring anything,
// while a private prefix-scoped registry still routes its own modules locally.
func TestBuildRegistry_Routing(t *testing.T) {
	cacheDir := t.TempDir()
	const (
		k8sPath = "cue.dev/x/k8s.io@v0"
		k8sVers = "v0.7.0"
	)

	t.Run("central is the default when CUE_REGISTRY is unset", func(t *testing.T) {
		resolver, _, _, err := buildRegistry(OCIConfig{Env: []string{}, CacheDir: cacheDir})
		require.NoError(t, err)

		loc, ok := resolver.ResolveToLocation(k8sPath, k8sVers)
		require.True(t, ok)
		assert.Equal(t, modconfig.DefaultRegistry, loc.Host)
		assert.False(t, loc.Insecure)
	})

	t.Run("prefix-scoped CUE_REGISTRY keeps central as the catch-all", func(t *testing.T) {
		env := []string{"CUE_REGISTRY=cuefn.test=localhost:5000+insecure"}
		resolver, _, _, err := buildRegistry(OCIConfig{Env: env, CacheDir: cacheDir})
		require.NoError(t, err)

		// The private prefix routes to the local insecure registry...
		priv, ok := resolver.ResolveToLocation("cuefn.test/app@v0", "v0.1.0")
		require.True(t, ok)
		assert.Equal(t, "localhost:5000", priv.Host)
		assert.True(t, priv.Insecure)

		// ...while everything else (the public k8s dependency) still resolves
		// from central, with no trailing ",registry.cue.works" required.
		pub, ok := resolver.ResolveToLocation(k8sPath, k8sVers)
		require.True(t, ok)
		assert.Equal(t, modconfig.DefaultRegistry, pub.Host)
		assert.False(t, pub.Insecure)
	})

	t.Run("bare CUE_REGISTRY replaces central (the deliberate offline override)", func(t *testing.T) {
		env := []string{"CUE_REGISTRY=localhost:5000+insecure"}
		resolver, _, _, err := buildRegistry(OCIConfig{Env: env, CacheDir: cacheDir})
		require.NoError(t, err)

		loc, ok := resolver.ResolveToLocation(k8sPath, k8sVers)
		require.True(t, ok)
		assert.Equal(t, "localhost:5000", loc.Host)
		assert.True(t, loc.Insecure)
	})
}
