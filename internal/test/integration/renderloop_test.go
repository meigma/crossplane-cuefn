package integration_test

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/meigma/crossplane-cuefn/internal/test/common"
)

// TestRenderLoop_CrossplaneRender proves the full v2 render loop: the example
// module is served from a local OCI registry, `cuefn function --insecure`
// renders it under `crossplane render`, and the env-driven ConfigMap data.tier
// equals the EnvironmentConfig value rather than the module's "unset" default —
// proving the environment flows end to end (criterion C3). It self-skips without
// Docker and the crossplane CLI.
func TestRenderLoop_CrossplaneRender(t *testing.T) {
	reg := common.StartRegistry(t)
	crossplane := common.RequireCrossplane(t)
	root := common.RepoRoot(t)
	reg.Publish(t, common.ExampleModuleRef, filepath.Join(root, "example/module"))

	bin := common.BuildBinary(t)
	// Bind the function server on all interfaces (0.0.0.0): crossplane render runs
	// the cuefn step in a Docker container and reaches the host-served function via
	// the bridge gateway (e.g. 172.17.0.1 on Linux), which a 127.0.0.1-only bind
	// refuses. functions.yaml still targets 127.0.0.1 — crossplane translates it to
	// the host's container-reachable address per platform.
	port := common.FreePort(t)
	bindAddr := fmt.Sprintf("0.0.0.0:%d", port)
	dialAddr := fmt.Sprintf("127.0.0.1:%d", port)
	common.ServeFunction(t, bin, bindAddr, dialAddr, reg.CUERegistry(), t.TempDir())

	functions := common.WriteFunctions(t, t.TempDir(), dialAddr)

	// crossplane render runs the function-environment-configs step as a Docker
	// container (only cuefn has the Development annotation), so a cold image pull
	// on a CI runner can blow crossplane's default 1m timeout
	// ("error waiting for container ... context deadline exceeded"). Give it room.
	cmd := exec.Command(crossplane, "render",
		filepath.Join(root, "example/xr.yaml"),
		filepath.Join(root, "example/composition.yaml"),
		functions,
		"--extra-resources", filepath.Join(root, "example/environmentconfig.yaml"),
		"--timeout", "10m",
	)
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "crossplane render: %s", out)

	rendered := string(out)
	assert.Contains(t, rendered, "kind: Deployment")
	assert.Contains(t, rendered, "kind: Service")
	assert.Contains(t, rendered, "kind: ConfigMap")

	// The env-driven tier must be the EnvironmentConfig value, not the default.
	assert.Contains(t, rendered, "tier: production")
	assert.NotContains(t, strings.ReplaceAll(rendered, "tier: production", ""), "tier: unset")
}
