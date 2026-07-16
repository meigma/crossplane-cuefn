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
// object flag reaches cuefn under the stable composition-resource annotation.
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
	cmd := exec.Command(crossplane, "render",
		filepath.Join(assets, "xr.yaml"),
		filepath.Join(assets, "composition.yaml"),
		functions,
		"--observed-resources", filepath.Join(assets, "deployment.yaml"),
		"--timeout", "10m",
	)
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "crossplane render: %s", out)

	rendered := string(out)
	assert.Contains(t, rendered, "workloadReady: true")
	assert.Contains(t, rendered, "evidence: seen")
	assert.Contains(t, rendered, "vendorOnly")
}
