package function_test

import (
	"bytes"
	"context"
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
// fixtures are reproducible across machines and CI. It mirrors the pin used by
// the render package's OCI integration tests.
const registryImage = "registry:2.8.3"

// requireDocker skips the calling test when no usable Docker daemon is present,
// so `go test ./...` stays green on a developer machine without Docker.
func requireDocker(t *testing.T) {
	t.Helper()
	testcontainers.SkipIfProviderIsNotHealthy(t)
}

// testRegistry is a throwaway local OCI registry for publishing the example CUE
// module. host is the "host:port" address; cueRegistry is the matching
// CUE_REGISTRY value (plain HTTP via the "+insecure" suffix).
type testRegistry struct {
	host        string
	cueRegistry string
	container   *registry.RegistryContainer
}

// startRegistry runs a registry:2 container and returns a handle to it.
func startRegistry(t *testing.T) *testRegistry {
	t.Helper()
	requireDocker(t)

	ctx := context.Background()
	c, err := registry.Run(ctx, registryImage)
	require.NoError(t, err, "start registry container")

	host, err := c.HostAddress(ctx)
	require.NoError(t, err, "registry host address")

	reg := &testRegistry{host: host, cueRegistry: host + "+insecure", container: c}
	t.Cleanup(func() {
		if reg.container != nil {
			require.NoError(t, testcontainers.TerminateContainer(reg.container))
		}
	})
	return reg
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
