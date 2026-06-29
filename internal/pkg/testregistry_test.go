package pkg_test

import (
	"context"
	"os"
	"os/exec"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/registry"
)

// registryImage pins the OCI registry image used by the integration tests so the
// fixtures are reproducible across machines and CI. It mirrors the pin used by
// the render and function packages.
const registryImage = "registry:2.8.3"

// requireDocker skips the calling test when no usable Docker daemon is present,
// so `go test ./...` stays green on a developer machine without Docker.
func requireDocker(t *testing.T) {
	t.Helper()
	if os.Getenv("CUEFN_INTEGRATION") == "" {
		t.Skip("integration test: set CUEFN_INTEGRATION=1 to run (via the integration moon tasks/workflow)")
	}
	testcontainers.SkipIfProviderIsNotHealthy(t)
}

// requireCrossplane skips the calling test unless the crossplane CLI is on PATH.
// The publish-test moon task runs under `mise exec` so the pinned crossplane wins;
// `go test ./...` self-skips when it is absent. CI asserts the binary is present.
func requireCrossplane(t *testing.T) string {
	t.Helper()
	if os.Getenv("CUEFN_INTEGRATION") == "" {
		t.Skip("integration test: set CUEFN_INTEGRATION=1 to run (via the integration moon tasks/workflow)")
	}
	bin, err := exec.LookPath("crossplane")
	if err != nil {
		t.Skip("crossplane CLI not on PATH; skipping xpkg validation test (run via `mise exec`)")
	}
	return bin
}

// testRegistry is a throwaway local OCI registry for push/pull round-trips.
type testRegistry struct {
	host      string
	container *registry.RegistryContainer
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

	reg := &testRegistry{host: host, container: c}
	t.Cleanup(func() {
		if reg.container != nil {
			require.NoError(t, testcontainers.TerminateContainer(reg.container))
		}
	})
	return reg
}
