package common

import (
	"bytes"
	"context"
	"net"
	"testing"

	"cuelang.org/go/mod/modregistry"
	"cuelang.org/go/mod/module"
	"cuelang.org/go/mod/modzip"

	"cuelabs.dev/go/oci/ociregistry/ociclient"

	"github.com/stretchr/testify/require"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/registry"
)

// RegistryImage pins the OCI registry image used by the integration tests so the
// fixtures are reproducible across machines and CI.
const RegistryImage = "registry:2.8.3"

// Registry is a throwaway local OCI registry for publishing CUE module fixtures
// and receiving pushed packages. host is the "host:port" address; cueRegistry is
// the matching CUE_REGISTRY value (plain HTTP via the "+insecure" suffix). It
// carries cueRegistry even for callers that do not use it, so a single type serves
// every suite.
type Registry struct {
	host        string
	cueRegistry string
	container   *registry.RegistryContainer
}

// StartRegistry runs a registry:2 container and returns a handle to it. The
// container is terminated at test cleanup unless it was already stopped via Stop
// (used by the render offline test). It gates on RequireDocker.
func StartRegistry(t *testing.T) *Registry {
	t.Helper()
	RequireDocker(t)

	ctx := context.Background()
	c, err := registry.Run(ctx, RegistryImage)
	require.NoError(t, err, "start registry container")

	host, err := c.HostAddress(ctx)
	require.NoError(t, err, "registry host address")

	reg := &Registry{
		host:        host,
		cueRegistry: host + "+insecure",
		container:   c,
	}
	t.Cleanup(func() { reg.Stop(t) })
	return reg
}

// Host returns the registry's "host:port" address reachable from the test.
func (r *Registry) Host() string { return r.host }

// CUERegistry returns the matching CUE_REGISTRY value (plain HTTP via "+insecure").
func (r *Registry) CUERegistry() string { return r.cueRegistry }

// DockerHostAddress returns the registry's host-published port through Docker's
// bridge gateway. Crossplane's render CLI places Function containers on an
// isolated bridge, where the registry container's private IP is not reachable.
func (r *Registry) DockerHostAddress(t *testing.T) string {
	t.Helper()

	provider, err := testcontainers.NewDockerProvider(
		testcontainers.WithDefaultBridgeNetwork(testcontainers.Bridge),
	)
	require.NoError(t, err, "create Docker provider")
	defer func() {
		require.NoError(t, provider.Close(), "close Docker provider")
	}()
	gateway, err := provider.GetGatewayIP(context.Background())
	require.NoError(t, err, "Docker bridge gateway IP")

	_, port, err := net.SplitHostPort(r.host)
	require.NoError(t, err, "registry host address %q", r.host)
	return net.JoinHostPort(gateway, port)
}

// Env returns the environment a loader should use to reach this registry: a
// CUE_REGISTRY pointing at the plain-HTTP container and a CUE_CACHE_DIR rooted at
// cacheDir so the test controls (and can inspect) the cache location.
func (r *Registry) Env(cacheDir string) []string {
	return []string{
		"CUE_REGISTRY=" + r.cueRegistry,
		"CUE_CACHE_DIR=" + cacheDir,
	}
}

// Publish packages the CUE module rooted at srcDir and pushes it to the registry
// at ref ("path@version") using the Go modregistry API (no cue CLI).
func (r *Registry) Publish(t *testing.T, ref, srcDir string) {
	t.Helper()
	PublishModule(t, r.host, ref, srcDir)
}

// ManifestDigest resolves ref's current manifest digest directly from the
// registry, used to assert the loader's digest-keyed behavior.
func (r *Registry) ManifestDigest(t *testing.T, ref string) string {
	t.Helper()
	return ManifestDigestAt(t, r.host, ref)
}

// Stop terminates the registry container if it is still running. It is safe to
// call more than once (the render offline test stops it mid-test and the cleanup
// stops it again).
func (r *Registry) Stop(t *testing.T) {
	t.Helper()
	if r.container == nil {
		return
	}
	require.NoError(t, testcontainers.TerminateContainer(r.container))
	r.container = nil
}

// PublishModule packages the CUE module rooted at srcDir and pushes it to host at
// ref ("path@version") using the Go modregistry API. It is the free-func mirror of
// Registry.Publish, for the e2e plain-HTTP registry (a different registry type).
func PublishModule(t *testing.T, host, ref, srcDir string) {
	t.Helper()

	mv, err := module.ParseVersion(ref)
	require.NoError(t, err, "parse module version %q", ref)

	var buf bytes.Buffer
	require.NoError(t, modzip.CreateFromDir(&buf, mv, srcDir), "zip module %q from %s", ref, srcDir)

	reg, err := ociclient.New(host, &ociclient.Options{Insecure: true})
	require.NoError(t, err, "build OCI client for %s", host)

	client := modregistry.NewClient(reg)
	data := buf.Bytes()
	require.NoError(t,
		client.PutModule(context.Background(), mv, bytes.NewReader(data), int64(len(data))),
		"publish module %q", ref,
	)
}

// ManifestDigestAt resolves ref's current manifest digest directly from host. It
// is the free-func mirror of Registry.ManifestDigest, for the e2e plain-HTTP
// registry.
func ManifestDigestAt(t *testing.T, host, ref string) string {
	t.Helper()

	mv, err := module.ParseVersion(ref)
	require.NoError(t, err)

	reg, err := ociclient.New(host, &ociclient.Options{Insecure: true})
	require.NoError(t, err)

	m, err := modregistry.NewClient(reg).GetModule(context.Background(), mv)
	require.NoError(t, err)
	return m.ManifestDigest().String()
}
