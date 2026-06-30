package integration_test

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/meigma/crossplane-cuefn/internal/test/common"
)

// TestRequiredResources_CrossplaneRender proves the full v2 required-resources
// fixpoint loop end to end: the hermetic required fixture is served from a local
// OCI registry, `cuefn function --insecure` emits a ConfigMap requirement under
// `crossplane render`, and the matching ConfigMap supplied via
// `--required-resources <dir>` is fetched and delivered on the next pass so the
// guarded Deployment renders with the ConfigMap's data.image instead of an empty
// resource set. It self-skips without Docker and the crossplane CLI.
func TestRequiredResources_CrossplaneRender(t *testing.T) {
	reg := common.StartRegistry(t)
	crossplane := common.RequireCrossplane(t)
	reg.Publish(t, common.RequiredModuleRef, common.HermeticRequiredModuleDir(t))

	bin := common.BuildBinary(t)
	// Bind the function server on all interfaces (0.0.0.0): crossplane render runs
	// the cuefn step in a Docker container and reaches the host-served function via
	// the bridge gateway, which a 127.0.0.1-only bind refuses. functions.yaml still
	// targets 127.0.0.1 — crossplane translates it to the container-reachable
	// address per platform.
	port := common.FreePort(t)
	bindAddr := fmt.Sprintf("0.0.0.0:%d", port)
	dialAddr := fmt.Sprintf("127.0.0.1:%d", port)
	common.ServeFunction(t, bin, bindAddr, dialAddr, reg.CUERegistry(), t.TempDir())

	functions := common.WriteFunctions(t, t.TempDir(), dialAddr)

	assets := common.HermeticRequiredloopDir(t)

	// Hand --required-resources a directory holding only the matching ConfigMap so
	// the composition.yaml and xr.yaml in the assets dir are not also slurped in as
	// candidate cluster objects.
	reqDir := t.TempDir()
	cm, err := os.ReadFile(filepath.Join(assets, "configmap.yaml"))
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(reqDir, "configmap.yaml"), cm, 0o600))

	// A cold function-runtime container pull on a CI runner can blow crossplane's
	// default 1m timeout, so give it room.
	cmd := exec.Command(crossplane, "render",
		filepath.Join(assets, "xr.yaml"),
		filepath.Join(assets, "composition.yaml"),
		functions,
		"--required-resources", reqDir,
		"--timeout", "10m",
	)
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "crossplane render: %s", out)

	rendered := string(out)
	// The data-dependent Deployment renders only after Crossplane fetched the
	// ConfigMap and re-invoked, and it carries the ConfigMap's data.image.
	assert.Contains(t, rendered, "kind: Deployment")
	assert.Contains(t, rendered, "image: ghcr.io/stefanprodan/podinfo:6.7.0")
}
