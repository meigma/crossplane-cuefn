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
// hermetic test-fixture module served offline via --dir. It proves the command emits
// the author-keyed resources and the composite status, and that --env drives the
// tier to "production" rather than the module's "unset" default — all without
// Docker or the crossplane CLI (criterion C2).
func TestRenderCommand_ExampleModule(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	root := NewRootCommand(Options{Out: &stdout})
	root.SetArgs([]string{
		"render", "cuefn.example/app@v0.1.0",
		"--dir", "../test/common/testdata/module",
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

// TestRenderCommand_EmitsRequirements proves that, even with no
// --required-resources supplied, `cuefn render` prints the selectors the module
// emitted under out.requirements (so authors discover what to supply) and that
// the data-dependent resource is omitted on the first pass (the seeded empty
// bucket).
func TestRenderCommand_EmitsRequirements(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	root := NewRootCommand(Options{Out: &stdout})
	root.SetArgs([]string{
		"render", "cuefn.example/required@v0.1.0",
		"--dir", "../test/common/testdata/required",
		"--xr", "testdata/xr-required.yaml",
	})

	require.NoError(t, root.ExecuteContext(context.Background()))

	var out struct {
		Resources    map[string]any `json:"resources"`
		Requirements map[string]struct {
			APIVersion string `json:"apiVersion"`
			Kind       string `json:"kind"`
			MatchName  string `json:"matchName"`
			Namespace  string `json:"namespace"`
		} `json:"requirements"`
	}
	require.NoError(t, yaml.Unmarshal(stdout.Bytes(), &out))

	// The emitted requirement is surfaced so the author learns what to supply.
	require.Contains(t, out.Requirements, "cfg")
	cfg := out.Requirements["cfg"]
	assert.Equal(t, "v1", cfg.APIVersion)
	assert.Equal(t, "ConfigMap", cfg.Kind)
	assert.Equal(t, "app-cfg", cfg.MatchName)
	assert.Equal(t, "default", cfg.Namespace)

	// The guarded Deployment is omitted on the first pass (empty seeded bucket).
	assert.NotContains(t, out.Resources, "deployment-0")
}

// TestRenderCommand_RequiredResources proves the offline two-pass loop: with a
// matching ConfigMap supplied via --required-resources, the second render pass
// surfaces it under input.requiredResources.cfg and the guarded Deployment
// renders with the ConfigMap's data.image — all without Docker or crossplane.
func TestRenderCommand_RequiredResources(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	root := NewRootCommand(Options{Out: &stdout})
	root.SetArgs([]string{
		"render", "cuefn.example/required@v0.1.0",
		"--dir", "../test/common/testdata/required",
		"--xr", "testdata/xr-required.yaml",
		"--required-resources", "testdata/required-configmap.yaml",
	})

	require.NoError(t, root.ExecuteContext(context.Background()))

	var out struct {
		Resources map[string]struct {
			Ready  string         `json:"ready"`
			Object map[string]any `json:"object"`
		} `json:"resources"`
	}
	require.NoError(t, yaml.Unmarshal(stdout.Bytes(), &out))

	require.Contains(t, out.Resources, "deployment-0")
	dep := out.Resources["deployment-0"].Object
	assert.Equal(t, "Deployment", dep["kind"])

	spec, ok := dep["spec"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "img:test", spec["image"])
}

func TestRenderCommand_ObservedResources(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	root := NewRootCommand(Options{Out: &stdout})
	root.SetArgs([]string{
		"render", "cuefn.example/observed@v0.1.0",
		"--dir", "../test/common/testdata/observed",
		"--xr", "testdata/xr.yaml",
		"--observed-resources", "testdata/observed-deployment.yaml",
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

	probe := out.Resources["probe"]
	assert.Equal(t, "True", probe.Ready)
	data, ok := probe.Object["data"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "seen", data["evidence"])
	assert.Equal(t, true, out.Status["workloadReady"])
}

// TestRenderCommand_RequirementsDoNotStabilize proves cuefn render surfaces a
// non-converging module — one whose out.requirements depends on the fetched data
// (the anti-pattern Crossplane rejects as "requirements didn't stabilize") — as
// an error rather than printing a bogus second-pass result. The impure fixture
// emits a second requirement only after the first is delivered, so the
// requirement set differs between the two passes.
func TestRenderCommand_RequirementsDoNotStabilize(t *testing.T) {
	t.Parallel()

	root := NewRootCommand(Options{Out: &bytes.Buffer{}, Err: &bytes.Buffer{}})
	root.SetArgs([]string{
		"render", "cuefn.example/impure@v0.1.0",
		"--dir", "testdata/impurereq",
		"--xr", "testdata/xr-required.yaml",
		"--required-resources", "testdata/required-configmap.yaml",
	})

	err := root.ExecuteContext(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "did not stabilize")
}

// TestRenderCommand_RequiresXR proves the command fails clearly when the required
// --xr flag is omitted.
func TestRenderCommand_RequiresXR(t *testing.T) {
	t.Parallel()

	root := NewRootCommand(Options{Out: &bytes.Buffer{}, Err: &bytes.Buffer{}})
	root.SetArgs([]string{"render", "cuefn.example/app@v0.1.0", "--dir", "../test/common/testdata/module"})

	err := root.ExecuteContext(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "xr")
}
