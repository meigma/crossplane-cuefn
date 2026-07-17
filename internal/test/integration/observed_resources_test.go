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

// TestObservedResources_CrossplaneRender proves Crossplane's raw observed
// object flag reaches an assembled cuefn Function xpkg through the standard
// Docker runtime and changes the module's readiness output when the observation
// changes. It uses no Development runtime, local server, named container,
// substituted image, or --crossplane-binary.
func TestObservedResources_CrossplaneRender(t *testing.T) {
	reg := common.StartRegistry(t)
	crossplane := common.RequireCrossplane(t)
	reg.Publish(t, common.ObservedModuleRef, common.HermeticObservedModuleDir(t))

	_, pkgTag := loadFunctionPackage(t, "crossplane-cuefn:observed-render")
	functions := filepath.Join(t.TempDir(), "functions.yaml")
	registryRoute := "cuefn.example=" + reg.DockerHostAddress(t) + "+insecure"
	manifest := fmt.Sprintf(`apiVersion: pkg.crossplane.io/v1
kind: Function
metadata:
  name: cuefn
  annotations:
    render.crossplane.io/runtime-docker-pull-policy: Never
    render.crossplane.io/runtime-docker-env: %q
spec:
  package: %q
`, "CUE_REGISTRY="+registryRoute, pkgTag.String())
	require.NoError(t, os.WriteFile(functions, []byte(manifest), 0o600))

	assets := common.HermeticObservedloopDir(t)
	renderObserved := func(t *testing.T, snapshot string) string {
		t.Helper()
		cmd := exec.Command(crossplane, "render",
			filepath.Join(assets, "xr.yaml"),
			filepath.Join(assets, "composition.yaml"),
			functions,
			"--observed-resources", filepath.Join(assets, snapshot),
			"--timeout", "10m",
		)
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, "crossplane render with %s: %s", snapshot, out)
		return string(out)
	}

	pending := renderObserved(t, "deployment-pending.yaml")
	assert.Contains(t, pending, "workloadReady: false")
	assert.NotContains(t, pending, "evidence: seen")

	ready := renderObserved(t, "deployment.yaml")
	assert.Contains(t, ready, "workloadReady: true")
	assert.Contains(t, ready, "evidence: seen")
	assert.Contains(t, ready, "vendorOnly")
	assert.NotEqual(t, pending, ready, "readiness output must change with the observation")
}
