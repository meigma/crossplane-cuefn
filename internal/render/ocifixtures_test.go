package render_test

import (
	"bytes"
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"cuelang.org/go/mod/modregistry"
	"cuelang.org/go/mod/module"
	"cuelang.org/go/mod/modzip"

	"cuelabs.dev/go/oci/ociregistry/ociclient"

	"github.com/stretchr/testify/require"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/registry"
)

// registryImage pins the OCI registry image used by the integration tests so the
// fixtures are reproducible across machines and CI.
const registryImage = "registry:2.8.3"

// cacheDir returns a fresh temp directory outside $HOME for use as CUE_CACHE_DIR.
// It is not t.TempDir because CUE's module cache marks extracted dependency files
// read-only, which makes the automatic t.TempDir cleanup fail with EPERM; this
// helper makes the tree writable before removing it.
func cacheDir(t *testing.T) string {
	t.Helper()
	// Not t.TempDir: CUE's modcache marks extracted files read-only, so the
	// automatic t.TempDir cleanup fails with EPERM; this helper chmods first.
	dir, err := os.MkdirTemp("", "cuefn-cache-") //nolint:usetesting // see comment
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = filepath.WalkDir(dir, func(p string, _ fs.DirEntry, err error) error {
			if err == nil {
				_ = os.Chmod(p, 0o700)
			}
			return nil
		})
		_ = os.RemoveAll(dir)
	})
	return dir
}

// requireDocker skips the calling test when no usable Docker daemon is present,
// so `go test ./...` stays green on a developer machine without Docker while CI
// (which has a daemon) runs the suite fully.
func requireDocker(t *testing.T) {
	t.Helper()
	if os.Getenv("CUEFN_INTEGRATION") == "" {
		t.Skip("integration test: set CUEFN_INTEGRATION=1 to run (via the integration moon tasks/workflow)")
	}
	testcontainers.SkipIfProviderIsNotHealthy(t)
}

// testRegistry is a throwaway local OCI registry for publishing CUE module
// fixtures. host is the "host:port" address; cueRegistry is the matching
// CUE_REGISTRY value (plain HTTP via the "+insecure" suffix).
type testRegistry struct {
	host        string
	cueRegistry string
	container   *registry.RegistryContainer
}

// startRegistry runs a registry:2 container and returns a handle to it. The
// container is terminated at test cleanup unless it was already stopped via
// stop (used by the offline test).
func startRegistry(t *testing.T) *testRegistry {
	t.Helper()
	requireDocker(t)

	ctx := context.Background()
	c, err := registry.Run(ctx, registryImage)
	require.NoError(t, err, "start registry container")

	host, err := c.HostAddress(ctx)
	require.NoError(t, err, "registry host address")

	reg := &testRegistry{
		host:        host,
		cueRegistry: host + "+insecure",
		container:   c,
	}
	t.Cleanup(func() { reg.stop(t) })
	return reg
}

// env returns the environment a loader should use to reach this registry: a
// CUE_REGISTRY pointing at the plain-HTTP container and a CUE_CACHE_DIR rooted at
// cacheDir so the test controls (and can inspect) the cache location.
func (r *testRegistry) env(cacheDir string) []string {
	return []string{
		"CUE_REGISTRY=" + r.cueRegistry,
		"CUE_CACHE_DIR=" + cacheDir,
	}
}

// stop terminates the registry container if it is still running. It is safe to
// call more than once.
func (r *testRegistry) stop(t *testing.T) {
	t.Helper()
	if r.container == nil {
		return
	}
	require.NoError(t, testcontainers.TerminateContainer(r.container))
	r.container = nil
}

// publishModule packages the CUE module rooted at srcDir and pushes it to the
// registry at ref ("path@version") using the Go modregistry API (no cue CLI).
func (r *testRegistry) publishModule(t *testing.T, ref, srcDir string) {
	t.Helper()

	mv, err := module.ParseVersion(ref)
	require.NoError(t, err, "parse module version %q", ref)

	var buf bytes.Buffer
	require.NoError(t, modzip.CreateFromDir(&buf, mv, srcDir), "zip module %q from %s", ref, srcDir)

	reg, err := ociclient.New(r.host, &ociclient.Options{Insecure: true})
	require.NoError(t, err, "build OCI client for %s", r.host)

	client := modregistry.NewClient(reg)
	data := buf.Bytes()
	require.NoError(t,
		client.PutModule(context.Background(), mv, bytes.NewReader(data), int64(len(data))),
		"publish module %q", ref,
	)
}

// manifestDigest resolves ref's current manifest digest directly from the
// registry, used to assert the loader's digest-keyed behavior.
func (r *testRegistry) manifestDigest(t *testing.T, ref string) string {
	t.Helper()

	mv, err := module.ParseVersion(ref)
	require.NoError(t, err)

	reg, err := ociclient.New(r.host, &ociclient.Options{Insecure: true})
	require.NoError(t, err)

	m, err := modregistry.NewClient(reg).GetModule(context.Background(), mv)
	require.NoError(t, err)
	return m.ManifestDigest().String()
}
