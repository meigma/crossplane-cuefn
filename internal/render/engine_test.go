package render_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/crossplane/function-sdk-go/resource"

	"github.com/meigma/crossplane-cuefn/internal/render"
	"github.com/meigma/crossplane-cuefn/internal/test/common"
)

// moduleDir is the shared hermetic test-fixture module, served offline via
// LocalLoader (the tests do not depend on the user-facing example/ module).
const moduleDir = "../test/common/testdata/module"

// renderExample renders the canonical example module with the given inputs.
func renderExample(t *testing.T, in render.Inputs) render.Result {
	t.Helper()
	e := render.New(render.LocalLoader{Dir: moduleDir})
	res, err := e.Render(context.Background(), "ignored-by-local-loader", in)
	require.NoError(t, err)
	return res
}

func TestRenderKeyedResources(t *testing.T) {
	res := renderExample(t, render.Inputs{Metadata: render.Metadata{Name: "demo"}})

	keys := make([]string, 0, len(res.Resources))
	for k := range res.Resources {
		keys = append(keys, k)
	}
	assert.ElementsMatch(t, []string{"deployment", "service", "config"}, keys)

	// Keys are the author's map keys verbatim, not a derived <kind>-<name> form.
	assert.NotContains(t, res.Resources, "Deployment-demo")
	assert.NotContains(t, res.Resources, "deployment-demo")

	assert.Equal(t, "Deployment", common.Object(t, res, "deployment")["kind"])
	assert.Equal(t, "Service", common.Object(t, res, "service")["kind"])
	assert.Equal(t, "ConfigMap", common.Object(t, res, "config")["kind"])
}

func TestRenderReadiness(t *testing.T) {
	res := renderExample(t, render.Inputs{Metadata: render.Metadata{Name: "demo"}})

	assert.Equal(t, resource.ReadyTrue, res.Resources["deployment"].Ready)
	assert.Equal(t, resource.ReadyFalse, res.Resources["service"].Ready)
	// An absent ready hint must not silently default to True.
	assert.Equal(t, resource.ReadyUnspecified, res.Resources["config"].Ready)
}

func TestRenderStatusReturned(t *testing.T) {
	res := renderExample(t, render.Inputs{Metadata: render.Metadata{Name: "demo"}})

	require.NotNil(t, res.Status)
	assert.Equal(t, true, res.Status["ready"])
	assert.Equal(t, "http://demo.svc", res.Status["url"])
}

func TestRenderEnvironmentDrivesTier(t *testing.T) {
	res := renderExample(t, render.Inputs{
		Metadata:    render.Metadata{Name: "demo"},
		Environment: map[string]any{"tier": "production"},
	})

	labels, ok := common.Object(t, res, "deployment")["metadata"].(map[string]any)
	require.True(t, ok)
	tierLabels, ok := labels["labels"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "production", tierLabels["tier"])
}

func TestRenderFloat64Replicas(t *testing.T) {
	// XR spec values arrive as float64 from JSON; the JSON-marshal fill must
	// render an integral float as an int so it unifies against the bounded
	// int #Spec field and the rendered replicas equals 2.
	res := renderExample(t, render.Inputs{
		Spec:     map[string]any{"replicas": float64(2)},
		Metadata: render.Metadata{Name: "demo"},
	})

	spec, ok := common.Object(t, res, "deployment")["spec"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, 2, common.ToInt(t, spec["replicas"]))
}

func TestRenderReservedKeyProjection(t *testing.T) {
	// A spec carrying Crossplane-reserved machinery renders against the closed
	// #Spec because Render projects the reserved keys out before unifying.
	res := renderExample(t, render.Inputs{
		Spec: map[string]any{
			"image":      "img:1",
			"crossplane": map[string]any{"compositionRef": map[string]any{"name": "c"}},
		},
		Metadata: render.Metadata{Name: "demo"},
	})

	spec, ok := common.Object(t, res, "deployment")["spec"].(map[string]any)
	require.True(t, ok)
	tmpl, ok := spec["template"].(map[string]any)
	require.True(t, ok)
	podSpec, ok := tmpl["spec"].(map[string]any)
	require.True(t, ok)
	containers, ok := podSpec["containers"].([]any)
	require.True(t, ok)
	require.Len(t, containers, 1)
	c, ok := containers[0].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "img:1", c["image"])
}

func TestRenderSpecViolation(t *testing.T) {
	e := render.New(render.LocalLoader{Dir: moduleDir})

	_, err := e.Render(context.Background(), "ignored", render.Inputs{
		Spec:     map[string]any{"replicas": float64(99)},
		Metadata: render.Metadata{Name: "demo"},
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "replicas")
}

func TestRenderNoStatus(t *testing.T) {
	e := render.New(render.LocalLoader{Dir: "testdata/nostatus"})

	res, err := e.Render(context.Background(), "ignored", render.Inputs{})

	require.NoError(t, err)
	assert.Nil(t, res.Status)
	assert.Contains(t, res.Resources, "only")
}

func TestRenderNonConcreteStatus(t *testing.T) {
	e := render.New(render.LocalLoader{Dir: "testdata/badstatus"})

	_, err := e.Render(context.Background(), "ignored", render.Inputs{})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "url")
}

func TestRenderNonConcreteResources(t *testing.T) {
	e := render.New(render.LocalLoader{Dir: "testdata/nonconcrete"})

	_, err := e.Render(context.Background(), "ignored", render.Inputs{})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "name")
}
