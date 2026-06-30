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

// requiredDir is the shared hermetic required-resources fixture: it emits one
// requirement (cfg -> a ConfigMap) and guards a Deployment on the delivered
// objects. Served offline via LocalLoader like moduleDir.
const requiredDir = "../test/common/testdata/required"

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

// renderRequired renders the hermetic required-resources fixture with the given
// inputs, requiring no error.
func renderRequired(t *testing.T, in render.Inputs) render.Result {
	t.Helper()
	e := render.New(render.LocalLoader{Dir: requiredDir})
	res, err := e.Render(context.Background(), "ignored-by-local-loader", in)
	require.NoError(t, err)
	return res
}

func TestRenderEmitsRequirements(t *testing.T) {
	// With no delivered required resources, the engine seeds an empty cfg bucket
	// so the guard collapses to a concrete empty {}: the requirement is still
	// emitted, but the guarded Deployment is omitted. This is the load-bearing
	// first-pass seed proof.
	res := renderRequired(t, render.Inputs{
		Metadata: render.Metadata{Name: "app", Namespace: "default"},
	})

	req, ok := res.Requirements["cfg"]
	require.True(t, ok, "requirements must carry the emitted cfg selector")
	assert.Equal(t, "v1", req.APIVersion)
	assert.Equal(t, "ConfigMap", req.Kind)
	assert.Equal(t, "app-cfg", req.MatchName)
	assert.Equal(t, "default", req.Namespace)
	assert.Empty(t, req.MatchLabels)

	// out.resources is concrete and omits the guarded Deployment on the seed pass.
	assert.NotContains(t, res.Resources, "deployment-0")
	assert.Empty(t, res.Resources)
}

func TestRenderRequiredResourcesSurfaced(t *testing.T) {
	// A delivered ConfigMap drives the guarded Deployment, which reads its
	// data.image off the fetched object.
	res := renderRequired(t, render.Inputs{
		Metadata: render.Metadata{Name: "app", Namespace: "default"},
		RequiredResources: map[string][]map[string]any{
			"cfg": {
				{
					"apiVersion": "v1",
					"kind":       "ConfigMap",
					"metadata":   map[string]any{"name": "app-cfg", "namespace": "default"},
					"data":       map[string]any{"image": "img:9"},
				},
			},
		},
	})

	spec, ok := common.Object(t, res, "deployment-0")["spec"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "img:9", spec["image"])
}

func TestRenderRequiredResourcesNotFound(t *testing.T) {
	// An explicit empty bucket ("requested, none found") is behaviorally
	// identical to the seed path: the guarded Deployment is omitted and the
	// result is concrete.
	res := renderRequired(t, render.Inputs{
		Metadata:          render.Metadata{Name: "app", Namespace: "default"},
		RequiredResources: map[string][]map[string]any{"cfg": {}},
	})

	assert.NotContains(t, res.Resources, "deployment-0")
	assert.Empty(t, res.Resources)
}

func TestRenderRequirementClusterScoped(t *testing.T) {
	// A cluster-scoped XR (Metadata.Namespace unset) yields a concrete selector
	// with the namespace omitted, and Render does not Fatal. Paired with
	// TestRenderEmitsRequirements (namespaced XR -> req.Namespace "default"), this
	// pins both directions of the namespace-guard idiom.
	res := renderRequired(t, render.Inputs{
		Metadata: render.Metadata{Name: "app"},
	})

	req, ok := res.Requirements["cfg"]
	require.True(t, ok)
	assert.Empty(t, req.Namespace)
	assert.Equal(t, "ConfigMap", req.Kind)
}

func TestRenderNonConcreteRequirement(t *testing.T) {
	// A requirement that is structurally valid but non-concrete (matchName left
	// as `string`) must be rejected by readRequirements' concreteness check,
	// before the exactly-one match validation is reached.
	e := render.New(render.LocalLoader{Dir: "testdata/nonconcreterequirement"})

	_, err := e.Render(context.Background(), "ignored", render.Inputs{})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "requirements")
}

func TestRenderRequirementMatchValidation(t *testing.T) {
	// A requirement must set exactly one of matchName or matchLabels. Both the
	// "neither" and "both" arms must be rejected at render time with the
	// "exactly one" error naming the requirement, so a future refactor of the
	// guard cannot let either invalid shape through.
	tests := []struct {
		name string
		dir  string
	}{
		{name: "neither", dir: "testdata/badrequirement"},
		{name: "both", dir: "testdata/bothrequirement"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := render.New(render.LocalLoader{Dir: tt.dir})

			_, err := e.Render(context.Background(), "ignored", render.Inputs{})

			require.Error(t, err)
			assert.Contains(t, err.Error(), "exactly one of matchName or matchLabels")
			assert.Contains(t, err.Error(), "cfg")
		})
	}
}
