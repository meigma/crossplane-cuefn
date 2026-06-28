package cli

import (
	"bytes"
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"sigs.k8s.io/yaml"
)

// TestRenderCommand_ExampleModule exercises `cuefn render` end to end against the
// canonical example module served offline via --dir. It proves the command emits
// the author-keyed resources and the composite status, and that --env drives the
// tier to "production" rather than the module's "unset" default — all without
// Docker or the crossplane CLI (criterion C2).
func TestRenderCommand_ExampleModule(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	root := NewRootCommand(Options{Out: &stdout})
	root.SetArgs([]string{
		"render", "cuefn.example/app@v0.1.0",
		"--dir", "../../example/module",
		"--xr", "testdata/xr.yaml",
		"--env", "testdata/env.yaml",
	})

	require.NoError(t, root.ExecuteContext(context.Background()))

	var out struct {
		Resources map[string]struct {
			Ready  string         `json:"ready"`
			Object map[string]any `json:"object"`
		} `json:"resources"`
		Status map[string]any `json:"status"`
	}
	require.NoError(t, yaml.Unmarshal(stdout.Bytes(), &out))

	// Resources are keyed by the module author's map keys verbatim.
	require.Contains(t, out.Resources, "deployment")
	require.Contains(t, out.Resources, "service")
	require.Contains(t, out.Resources, "config")

	assert.Equal(t, "Deployment", out.Resources["deployment"].Object["kind"])
	assert.Equal(t, "Service", out.Resources["service"].Object["kind"])
	assert.Equal(t, "ConfigMap", out.Resources["config"].Object["kind"])

	// Readiness is surfaced per resource.
	assert.Equal(t, "True", out.Resources["deployment"].Ready)
	assert.Equal(t, "False", out.Resources["service"].Ready)

	// The composite status the module returned is printed.
	require.NotNil(t, out.Status)
	assert.Equal(t, true, out.Status["ready"])
	assert.Equal(t, "http://demo.svc", out.Status["url"])

	// The env-driven tier wins over the module default "unset".
	config := out.Resources["config"].Object
	data, ok := config["data"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "production", data["tier"])
}

// TestRenderCommand_RequiresXR proves the command fails clearly when the required
// --xr flag is omitted.
func TestRenderCommand_RequiresXR(t *testing.T) {
	t.Parallel()

	root := NewRootCommand(Options{Out: &bytes.Buffer{}, Err: &bytes.Buffer{}})
	root.SetArgs([]string{"render", "cuefn.example/app@v0.1.0", "--dir", "../../example/module"})

	err := root.ExecuteContext(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "xr")
}
