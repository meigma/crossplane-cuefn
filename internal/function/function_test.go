package function_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/structpb"

	sdkcontext "github.com/crossplane/function-sdk-go/context"
	"github.com/crossplane/function-sdk-go/logging"
	fnv1 "github.com/crossplane/function-sdk-go/proto/v1"
	"github.com/crossplane/function-sdk-go/resource"

	"github.com/meigma/crossplane-cuefn/input/v1beta1"
	"github.com/meigma/crossplane-cuefn/internal/function"
	"github.com/meigma/crossplane-cuefn/internal/render"
)

// moduleDir is the shared hermetic test-fixture module, served offline by the
// local loader (the tests do not depend on the user-facing example/ module).
const moduleDir = "../test/common/testdata/module"

// requiredDir is the shared hermetic required-resources fixture: it emits one
// matchName ConfigMap requirement (cfg) and guards a Deployment on the fetched
// objects. matchLabelsDir and invalidReqDir are package-local variants
// exercising the matchLabels oneof arm and the exactly-one violation.
const (
	requiredDir    = "../test/common/testdata/required"
	matchLabelsDir = "testdata/matchlabels"
	invalidReqDir  = "testdata/invalidreq"
	observedDir    = "../test/common/testdata/observed"
)

// localFactory returns a LoaderFactory serving the example module from disk,
// ignoring the Input's module ref (the LocalLoader is fixed to one directory).
func localFactory() function.LoaderFactory {
	return factoryFor(moduleDir)
}

// factoryFor returns a LoaderFactory serving the module at dir offline, ignoring
// the Input's module ref (the LocalLoader is fixed to one directory).
func factoryFor(dir string) function.LoaderFactory {
	return func(_ *v1beta1.Input) (render.ModuleLoader, error) {
		return render.LocalLoader{Dir: dir}, nil
	}
}

// newFunction wires a Function whose loader serves the example module offline.
func newFunction() *function.Function {
	return function.New(localFactory(), logging.NewNopLogger())
}

// envContext builds a pipeline context carrying the merged environment under the
// well-known environment context key, as function-environment-configs would.
func envContext(t *testing.T, env map[string]any) *structpb.Struct {
	t.Helper()
	s, err := structpb.NewStruct(map[string]any{sdkcontext.KeyEnvironment: env})
	require.NoError(t, err)
	return s
}

// baseRequest is a well-formed request rendering the example module for an XApp
// named demo, with tier=production supplied via the environment context.
func baseRequest(t *testing.T) *fnv1.RunFunctionRequest {
	t.Helper()
	return &fnv1.RunFunctionRequest{
		Meta: &fnv1.RequestMeta{Tag: "t"},
		Input: resource.MustStructJSON(`{
			"apiVersion": "cuefn.meigma.io/v1beta1",
			"kind": "Input",
			"module": "cuefn.example/app@v0.1.0"
		}`),
		Observed: &fnv1.State{
			Composite: &fnv1.Resource{
				Resource: resource.MustStructJSON(`{
					"apiVersion": "platform.meigma.io/v1alpha1",
					"kind": "XApp",
					"metadata": {"name": "demo", "namespace": "default"},
					"spec": {"image": "img:1", "replicas": 2}
				}`),
			},
		},
		Context: envContext(t, map[string]any{"tier": "production"}),
	}
}

// run executes RunFunction and asserts it neither panics nor returns a transport
// error, returning the response for further assertions.
func run(t *testing.T, fn *function.Function, req *fnv1.RunFunctionRequest) *fnv1.RunFunctionResponse {
	t.Helper()
	var (
		rsp *fnv1.RunFunctionResponse
		err error
	)
	require.NotPanics(t, func() {
		rsp, err = fn.RunFunction(context.Background(), req)
	})
	require.NoError(t, err)
	require.NotNil(t, rsp)
	return rsp
}

// TestRunFunction_DesiredKeyedByAuthorNames asserts the desired composed
// resources are keyed by the module's author map keys verbatim (criterion C1).
func TestRunFunction_DesiredKeyedByAuthorNames(t *testing.T) {
	t.Parallel()

	rsp := run(t, newFunction(), baseRequest(t))
	desired := rsp.GetDesired().GetResources()

	keys := make([]string, 0, len(desired))
	for k := range desired {
		keys = append(keys, k)
	}
	assert.ElementsMatch(t, []string{"deployment", "service", "config"}, keys)

	// The env-driven tier reached the rendered object, not the "unset" default.
	dep := desired["deployment"].GetResource().AsMap()
	meta, ok := dep["metadata"].(map[string]any)
	require.True(t, ok)
	labels, ok := meta["labels"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "production", labels["tier"])
}

// TestRunFunction_ReadinessMapping asserts the module readiness hints map through
// to the proto Ready enum: explicit Ready/NotReady and an absent hint (criterion
// C1).
func TestRunFunction_ReadinessMapping(t *testing.T) {
	t.Parallel()

	desired := run(t, newFunction(), baseRequest(t)).GetDesired().GetResources()

	assert.Equal(t, fnv1.Ready_READY_TRUE, desired["deployment"].GetReady())
	assert.Equal(t, fnv1.Ready_READY_FALSE, desired["service"].GetReady())
	assert.Equal(t, fnv1.Ready_READY_UNSPECIFIED, desired["config"].GetReady())
}

// TestRunFunction_StatusPatchedOnComposite asserts the module status is patched
// under status on the desired composite, with the observed XR's GVK preserved
// (criterion C1).
func TestRunFunction_StatusPatchedOnComposite(t *testing.T) {
	t.Parallel()

	rsp := run(t, newFunction(), baseRequest(t))
	xr := rsp.GetDesired().GetComposite().GetResource().AsMap()

	assert.Equal(t, "platform.meigma.io/v1alpha1", xr["apiVersion"])
	assert.Equal(t, "XApp", xr["kind"])

	status, ok := xr["status"].(map[string]any)
	require.True(t, ok, "desired composite must carry a status")
	assert.Equal(t, true, status["ready"])
	assert.Equal(t, "http://demo.svc", status["url"])
}

// TestRunFunction_SuccessCondition asserts a success condition targeting the
// composite is set on the response (criterion C1).
func TestRunFunction_SuccessCondition(t *testing.T) {
	t.Parallel()

	rsp := run(t, newFunction(), baseRequest(t))

	conditions := rsp.GetConditions()
	require.Len(t, conditions, 1)
	c := conditions[0]
	assert.Equal(t, "FunctionSuccess", c.GetType())
	assert.Equal(t, "Success", c.GetReason())
	assert.Equal(t, fnv1.Status_STATUS_CONDITION_TRUE, c.GetStatus())
	assert.Equal(t, fnv1.Target_TARGET_COMPOSITE, c.GetTarget())

	// No fatal results on the happy path.
	for _, r := range rsp.GetResults() {
		assert.NotEqual(t, fnv1.Severity_SEVERITY_FATAL, r.GetSeverity())
	}
}

// TestRunFunction_FatalOnMalformedOrUnreachable asserts every malformed or
// unreachable input yields exactly one FATAL result naming the cause, leaves the
// desired state unmutated, and does not panic (criterion C1).
func TestRunFunction_FatalOnMalformedOrUnreachable(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		fn      *function.Function
		req     *fnv1.RunFunctionRequest
		wantMsg string
	}{
		{
			name: "missing module",
			fn:   newFunction(),
			req: &fnv1.RunFunctionRequest{
				Meta:  &fnv1.RequestMeta{Tag: "t"},
				Input: resource.MustStructJSON(`{"apiVersion":"cuefn.meigma.io/v1beta1","kind":"Input"}`),
			},
			wantMsg: "module",
		},
		{
			name: "spec violates module schema",
			fn:   newFunction(),
			req: &fnv1.RunFunctionRequest{
				Meta: &fnv1.RequestMeta{Tag: "t"},
				Input: resource.MustStructJSON(
					`{"apiVersion":"cuefn.meigma.io/v1beta1","kind":"Input","module":"cuefn.example/app@v0.1.0"}`,
				),
				Observed: &fnv1.State{Composite: &fnv1.Resource{
					Resource: resource.MustStructJSON(`{
						"apiVersion":"platform.meigma.io/v1alpha1","kind":"XApp",
						"metadata":{"name":"demo"},"spec":{"replicas":99}
					}`),
				}},
			},
			wantMsg: "render",
		},
		{
			name: "loader build failure",
			fn: function.New(func(_ *v1beta1.Input) (render.ModuleLoader, error) {
				return nil, errors.New("registry unreachable")
			}, logging.NewNopLogger()),
			req:     baseRequest(t),
			wantMsg: "loader",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			rsp := run(t, tt.fn, tt.req)

			results := rsp.GetResults()
			require.Len(t, results, 1)
			assert.Equal(t, fnv1.Severity_SEVERITY_FATAL, results[0].GetSeverity())
			assert.Contains(t, results[0].GetMessage(), tt.wantMsg)

			// A fatal result must not leak partial desired mutations.
			assert.Empty(t, rsp.GetDesired().GetResources())
			assert.Empty(t, rsp.GetConditions())
		})
	}
}

// requiredRequest is a well-formed request rendering the required-resources
// fixture for an XApp named demo in namespace default, with the default
// configName ("app-cfg"). The fixture emits a ConfigMap requirement keyed "cfg".
func requiredRequest(t *testing.T) *fnv1.RunFunctionRequest {
	t.Helper()
	return &fnv1.RunFunctionRequest{
		Meta: &fnv1.RequestMeta{Tag: "t"},
		Input: resource.MustStructJSON(`{
			"apiVersion": "cuefn.meigma.io/v1beta1",
			"kind": "Input",
			"module": "cuefn.example/required@v0"
		}`),
		Observed: &fnv1.State{
			Composite: &fnv1.Resource{
				Resource: resource.MustStructJSON(`{
					"apiVersion": "platform.meigma.io/v1alpha1",
					"kind": "XApp",
					"metadata": {"name": "demo", "namespace": "default"},
					"spec": {"configName": "app-cfg"}
				}`),
			},
		},
	}
}

// hasWarning reports whether the response carries a non-fatal warning result.
func hasWarning(rsp *fnv1.RunFunctionResponse) bool {
	for _, r := range rsp.GetResults() {
		if r.GetSeverity() == fnv1.Severity_SEVERITY_WARNING {
			return true
		}
	}
	return false
}

// TestRunFunction_EmitsRequirements asserts the module's out.requirements selector
// is mapped onto rsp.Requirements.Resources, keyed by the author's name, with its
// ApiVersion/Kind/MatchName/Namespace round-tripped. The capability is advertised
// so no diagnostic warning fires.
func TestRunFunction_EmitsRequirements(t *testing.T) {
	t.Parallel()

	req := requiredRequest(t)
	req.Meta.Capabilities = []fnv1.Capability{fnv1.Capability_CAPABILITY_REQUIRED_RESOURCES}

	fn := function.New(factoryFor(requiredDir), logging.NewNopLogger())
	rsp := run(t, fn, req)

	sel := rsp.GetRequirements().GetResources()["cfg"]
	require.NotNil(t, sel, "the cfg requirement must be emitted")
	assert.Equal(t, "v1", sel.GetApiVersion())
	assert.Equal(t, "ConfigMap", sel.GetKind())
	assert.Equal(t, "app-cfg", sel.GetMatchName())
	assert.Equal(t, "default", sel.GetNamespace())

	// Capability advertised -> no diagnostic warning, no fatal.
	assert.False(t, hasWarning(rsp), "no warning when the capability is advertised")
	for _, r := range rsp.GetResults() {
		assert.NotEqual(t, fnv1.Severity_SEVERITY_FATAL, r.GetSeverity())
	}
}

// TestRunFunction_EmitsMatchLabels asserts the matchLabels arm of the selector
// oneof round-trips: a requirement set with labels (and no matchName) surfaces
// under sel.GetMatchLabels().GetLabels().
func TestRunFunction_EmitsMatchLabels(t *testing.T) {
	t.Parallel()

	req := &fnv1.RunFunctionRequest{
		Meta: &fnv1.RequestMeta{
			Tag:          "t",
			Capabilities: []fnv1.Capability{fnv1.Capability_CAPABILITY_REQUIRED_RESOURCES},
		},
		Input: resource.MustStructJSON(`{
			"apiVersion": "cuefn.meigma.io/v1beta1",
			"kind": "Input",
			"module": "cuefn.example/matchlabels@v0"
		}`),
		Observed: &fnv1.State{
			Composite: &fnv1.Resource{
				Resource: resource.MustStructJSON(`{
					"apiVersion": "platform.meigma.io/v1alpha1",
					"kind": "XApp",
					"metadata": {"name": "demo", "namespace": "default"},
					"spec": {"appName": "app"}
				}`),
			},
		},
	}

	fn := function.New(factoryFor(matchLabelsDir), logging.NewNopLogger())
	rsp := run(t, fn, req)

	sel := rsp.GetRequirements().GetResources()["cfg"]
	require.NotNil(t, sel, "the cfg requirement must be emitted")
	assert.Equal(t, "v1", sel.GetApiVersion())
	assert.Equal(t, "ConfigMap", sel.GetKind())
	assert.Empty(t, sel.GetMatchName(), "matchLabels-only selector sets no matchName")
	assert.Equal(t, map[string]string{"app": "app"}, sel.GetMatchLabels().GetLabels())
}

// TestRunFunction_ReceivesRequiredResources asserts the required resources
// delivered on the request reach the module under out.input.requiredResources:
// the guarded Deployment reads the fetched ConfigMap's data.image.
func TestRunFunction_ReceivesRequiredResources(t *testing.T) {
	t.Parallel()

	req := requiredRequest(t)
	req.RequiredResources = map[string]*fnv1.Resources{
		"cfg": {Items: []*fnv1.Resource{{Resource: resource.MustStructJSON(
			`{"apiVersion":"v1","kind":"ConfigMap","metadata":{"name":"app-cfg","namespace":"default"},"data":{"image":"img:9"}}`,
		)}}},
	}

	fn := function.New(factoryFor(requiredDir), logging.NewNopLogger())
	rsp := run(t, fn, req)

	dep := rsp.GetDesired().GetResources()["deployment-0"].GetResource().AsMap()
	require.NotEmpty(t, dep, "the guarded Deployment must render from the fetched ConfigMap")
	spec, ok := dep["spec"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "img:9", spec["image"])
}

func TestRunFunction_ReceivesObservedComposedResources(t *testing.T) {
	t.Parallel()

	req := baseRequest(t)
	req.Observed.Resources = map[string]*fnv1.Resource{
		"workload": {
			Resource: resource.MustStructJSON(`{
				"apiVersion":"apps/v1",
				"kind":"Deployment",
				"metadata":{"name":"physical-name"},
				"status":{"custom":{"nested":"seen","vendorOnly":true}}
			}`),
			ConnectionDetails: map[string][]byte{"password": []byte("super-secret")},
		},
	}

	fn := function.New(factoryFor(observedDir), logging.NewNopLogger())
	rsp := run(t, fn, req)

	probe := rsp.GetDesired().GetResources()["probe"]
	require.NotNil(t, probe)
	assert.Equal(t, fnv1.Ready_READY_TRUE, probe.GetReady())

	object := probe.GetResource().AsMap()
	data, ok := object["data"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "seen", data["evidence"], "open kind-specific status must reach CUE")
	observation, ok := data["observation"].(string)
	require.True(t, ok)
	assert.Contains(t, observation, `"vendorOnly":true`,
		"the fixture must echo the full observed object for this boundary assertion")
	assert.NotContains(t, observation, "connectionDetails")
	assert.NotContains(t, observation, "super-secret",
		"observed connection details must never enter the CUE object input")
}

func TestRunFunction_MalformedObservedResourceIsFatalWithoutMutation(t *testing.T) {
	t.Parallel()

	req := baseRequest(t)
	req.Observed.Resources = map[string]*fnv1.Resource{
		"broken": {},
	}
	req.Desired = &fnv1.State{Resources: map[string]*fnv1.Resource{
		"prior": {Resource: resource.MustStructJSON(
			`{"apiVersion":"v1","kind":"ConfigMap","metadata":{"name":"prior"}}`,
		)},
	}}

	fn := function.New(factoryFor(observedDir), logging.NewNopLogger())
	rsp := run(t, fn, req)

	results := rsp.GetResults()
	require.Len(t, results, 1)
	assert.Equal(t, fnv1.Severity_SEVERITY_FATAL, results[0].GetSeverity())
	assert.Contains(t, results[0].GetMessage(), "observed composed resource")

	desired := rsp.GetDesired().GetResources()
	require.Len(t, desired, 1, "fatal observation decoding must not add partial desired resources")
	assert.Equal(t, "prior", desired["prior"].GetResource().AsMap()["metadata"].(map[string]any)["name"])
	assert.Empty(t, rsp.GetConditions())
}

// TestRunFunction_WarnsWithoutCapability asserts that when a module emits
// requirements but Crossplane does not advertise the capability, a non-fatal
// warning is added AND the requirements are still emitted (emission is
// unconditional; the gate controls only the diagnostic).
func TestRunFunction_WarnsWithoutCapability(t *testing.T) {
	t.Parallel()

	req := requiredRequest(t) // no Meta.Capabilities

	fn := function.New(factoryFor(requiredDir), logging.NewNopLogger())
	rsp := run(t, fn, req)

	assert.True(t, hasWarning(rsp), "a warning must fire when the capability is absent")
	assert.NotNil(t, rsp.GetRequirements().GetResources()["cfg"],
		"requirements stay emitted regardless of the capability")
}

// TestRunFunction_InvalidRequirement asserts that a module whose requirement sets
// both match fields fails the engine's exactly-one check and is surfaced as a
// single Fatal result.
func TestRunFunction_InvalidRequirement(t *testing.T) {
	t.Parallel()

	req := &fnv1.RunFunctionRequest{
		Meta: &fnv1.RequestMeta{Tag: "t"},
		Input: resource.MustStructJSON(`{
			"apiVersion": "cuefn.meigma.io/v1beta1",
			"kind": "Input",
			"module": "cuefn.example/invalidreq@v0"
		}`),
		Observed: &fnv1.State{
			Composite: &fnv1.Resource{
				Resource: resource.MustStructJSON(`{
					"apiVersion": "platform.meigma.io/v1alpha1",
					"kind": "XApp",
					"metadata": {"name": "demo", "namespace": "default"},
					"spec": {"configName": "app-cfg"}
				}`),
			},
		},
	}

	fn := function.New(factoryFor(invalidReqDir), logging.NewNopLogger())
	rsp := run(t, fn, req)

	results := rsp.GetResults()
	require.Len(t, results, 1)
	assert.Equal(t, fnv1.Severity_SEVERITY_FATAL, results[0].GetSeverity())
	assert.Contains(t, results[0].GetMessage(), "exactly one")
}
