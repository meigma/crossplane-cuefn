package integration_test

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/meigma/crossplane-cuefn/internal/test/common"
)

// TestObservedResources_CrossplaneRender proves Crossplane's raw observed
// object flag reaches cuefn under the stable composition-resource annotation
// and changes the module's readiness output when the observation changes.
func TestObservedResources_CrossplaneRender(t *testing.T) {
	reg := common.StartRegistry(t)
	crossplane := common.RequireCrossplane(t)
	reg.Publish(t, common.ObservedModuleRef, common.HermeticObservedModuleDir(t))

	bin := common.BuildBinary(t)
	port := common.FreePort(t)
	bindAddr := fmt.Sprintf("0.0.0.0:%d", port)
	dialAddr := fmt.Sprintf("127.0.0.1:%d", port)
	common.ServeFunction(t, bin, bindAddr, dialAddr, reg.CUERegistry(), t.TempDir())

	functions := common.WriteFunctions(t, t.TempDir(), dialAddr)
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
