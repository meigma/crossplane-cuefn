package testharness

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/meigma/crossplane-cuefn/internal/render"
	"github.com/meigma/crossplane-cuefn/internal/test/common"
)

const hermeticXR = `apiVersion: platform.meigma.io/v1alpha1
kind: XApp
metadata:
  name: demo
  namespace: default
spec:
  replicas: 3
`

// runCase parses content as a txtar case and runs it against the module in dir
// through the real engine (offline local loader).
func runCase(t *testing.T, dir, content string) (*CaseResult, error) {
	t.Helper()
	c, err := ParseCase("case.txtar", []byte(content))
	require.NoError(t, err)
	runner := &Runner{Loader: render.LocalLoader{Dir: dir}, Ref: "test-module"}
	return runner.Run(t.Context(), c)
}

func TestRunWantCUEPass(t *testing.T) {
	t.Parallel()

	res, err := runCase(t, common.HermeticModuleDir(t), "-- xr.yaml --\n"+hermeticXR+
		`-- environment.yaml --
tier: production
-- want.cue --
resources: {
	deployment: {
		ready: "Ready"
		object: spec: {
			replicas: 3
			template: metadata: labels: tier: "production"
		}
	}
	service: ready: "NotReady"
	config: ready:  "Unspecified"
}
requirements: close({})
status: url: "http://demo.svc"
`)
	require.NoError(t, err)
	assert.True(t, res.Passed(), "failures: %v", res.Failures())
}

func TestRunWantCUEConstraints(t *testing.T) {
	t.Parallel()

	res, err := runCase(t, common.HermeticModuleDir(t), "-- xr.yaml --\n"+hermeticXR+
		"-- want.cue --\nresources: deployment: object: spec: replicas: >=1 & <=10\n")
	require.NoError(t, err)
	assert.True(t, res.Passed(), "failures: %v", res.Failures())

	res, err = runCase(t, common.HermeticModuleDir(t), "-- xr.yaml --\n"+hermeticXR+
		"-- want.cue --\nresources: deployment: object: spec: replicas: >5\n")
	require.NoError(t, err)
	require.False(t, res.Passed())
	assert.Contains(t, res.Failures()[0].Message, "replicas")
}

func TestRunWantCUEFailureMessage(t *testing.T) {
	t.Parallel()

	res, err := runCase(t, common.HermeticModuleDir(t), "-- xr.yaml --\n"+hermeticXR+
		"-- want.cue --\nresources: deployment: object: spec: replicas: 5\n")
	require.NoError(t, err)

	failures := res.Failures()
	require.Len(t, failures, 1)
	assert.Equal(t, "want.cue", failures[0].Kind)
	assert.Contains(t, failures[0].Message, "resources.deployment.object.spec.replicas",
		"the failure must be path-qualified")
	assert.Contains(t, failures[0].Message, "conflicting values")
	// Positions must cite the txtar file's own coordinates: the want.cue
	// content starts on line 10 (marker on line 9 after the 8-line XR).
	assert.Contains(t, failures[0].Message, "case.txtar/want.cue:10",
		"CUE positions must map to the txtar file's line numbers")
}

func TestRunErrorExpectation(t *testing.T) {
	t.Parallel()

	badXR := strings.ReplaceAll(hermeticXR, "replicas: 3", "replicas: 99")

	t.Run("matching substring passes", func(t *testing.T) {
		t.Parallel()
		res, err := runCase(t, common.HermeticModuleDir(t),
			"-- xr.yaml --\n"+badXR+"-- error.txt --\nreplicas\n")
		require.NoError(t, err)
		assert.True(t, res.Passed(), "failures: %v", res.Failures())
	})

	t.Run("missing substring fails", func(t *testing.T) {
		t.Parallel()
		res, err := runCase(t, common.HermeticModuleDir(t),
			"-- xr.yaml --\n"+badXR+"-- error.txt --\nsome-other-field\n")
		require.NoError(t, err)
		require.False(t, res.Passed())
		assert.Contains(t, res.Failures()[0].Message, "does not contain")
	})

	t.Run("successful render fails the expectation", func(t *testing.T) {
		t.Parallel()
		res, err := runCase(t, common.HermeticModuleDir(t),
			"-- xr.yaml --\n"+hermeticXR+"-- error.txt --\nreplicas\n")
		require.NoError(t, err)
		require.NotEmpty(t, res.Failures())
		assert.Contains(t, res.Failures()[0].Message, "expected the render to fail")
		assert.Contains(t, res.Failures()[0].Message, "deployment")
	})
}

func TestRunRenderFailureWithoutExpectation(t *testing.T) {
	t.Parallel()

	badXR := strings.ReplaceAll(hermeticXR, "replicas: 3", "replicas: 99")
	res, err := runCase(t, common.HermeticModuleDir(t),
		"-- xr.yaml --\n"+badXR+"-- want.cue --\nresources: {}\n")
	require.NoError(t, err)
	require.False(t, res.Passed())
	assert.Equal(t, "render", res.Failures()[0].Kind)
	assert.Contains(t, res.Failures()[0].Message, "replicas")
}

func TestRunGoldenSeedDriftUpdate(t *testing.T) {
	t.Parallel()

	seedSource := "-- xr.yaml --\n" + hermeticXR
	res, err := runCase(t, common.HermeticModuleDir(t), seedSource)
	require.NoError(t, err)
	require.True(t, res.NeedsSeed())
	require.NotNil(t, res.Units[0].Golden)
	assert.False(t, res.Passed(), "a case needing a seed has not passed")

	// Seed: the golden lands in a want.yaml section and the case passes.
	seeded, changed := SeedGoldens([]byte(seedSource), res)
	require.True(t, changed)
	assert.Contains(t, string(seeded), "-- want.yaml --")

	res, err = runCase(t, common.HermeticModuleDir(t), string(seeded))
	require.NoError(t, err)
	assert.True(t, res.Passed(), "failures: %v", res.Failures())

	// Drift: change the XR (and only the XR — the seeded golden also contains
	// "replicas: 3", anchored differently) so the render no longer matches.
	drifted := strings.Replace(string(seeded), "spec:\n  replicas: 3", "spec:\n  replicas: 4", 1)
	require.NotEqual(t, string(seeded), drifted)
	res, err = runCase(t, common.HermeticModuleDir(t), drifted)
	require.NoError(t, err)
	require.False(t, res.Passed())
	failure := res.Failures()[0]
	assert.Equal(t, "want.yaml", failure.Kind)
	assert.Contains(t, failure.Message, "- ", "the failure must carry a line diff")
	assert.Contains(t, failure.Message, "+ ")
	assert.Contains(t, failure.Message, "replicas")

	// Update: rewrite the drifted golden and the case passes again.
	updated, changed := UpdateGoldens([]byte(drifted), res)
	require.True(t, changed)
	res, err = runCase(t, common.HermeticModuleDir(t), string(updated))
	require.NoError(t, err)
	assert.True(t, res.Passed(), "failures: %v", res.Failures())

	// A second update is a no-op.
	_, changed = UpdateGoldens(updated, res)
	assert.False(t, changed)
}

func TestRunRequiredResources(t *testing.T) {
	t.Parallel()

	res, err := runCase(t, common.HermeticRequiredModuleDir(t), `-- xr.yaml --
apiVersion: platform.meigma.io/v1alpha1
kind: XApp
metadata:
  name: demo
  namespace: default
spec:
  configName: app-cfg
-- required.yaml --
apiVersion: v1
kind: ConfigMap
metadata:
  name: app-cfg
  namespace: default
data:
  image: img:1
-- want.cue --
resources: "deployment-0": {
	ready: "Ready"
	object: spec: image: "img:1"
}
requirements: cfg: {
	apiVersion: "v1"
	kind:       "ConfigMap"
	matchName:  "app-cfg"
	namespace:  "default"
}
`)
	require.NoError(t, err)
	assert.True(t, res.Passed(), "failures: %v", res.Failures())
}

func TestRunRequiredResourcesNoneMatching(t *testing.T) {
	t.Parallel()

	// No required.yaml at all: the engine seeds an empty bucket, the guarded
	// resource is omitted, and the emitted requirement is still assertable.
	res, err := runCase(t, common.HermeticRequiredModuleDir(t), `-- xr.yaml --
apiVersion: platform.meigma.io/v1alpha1
kind: XApp
metadata:
  name: demo
  namespace: default
spec:
  configName: app-cfg
-- want.cue --
resources: close({})
requirements: cfg: matchName: "app-cfg"
`)
	require.NoError(t, err)
	assert.True(t, res.Passed(), "failures: %v", res.Failures())
}

const observedXR = `apiVersion: platform.meigma.io/v1alpha1
kind: XApp
metadata:
  name: demo
spec: {}
`

func observedSnapshot(nested string) string {
	return `apiVersion: apps/v1
kind: Deployment
metadata:
  name: workload-physical
  annotations:
    crossplane.io/composition-resource-name: workload
status:
  custom:
    nested: ` + nested + "\n"
}

func TestRunObservedSteps(t *testing.T) {
	t.Parallel()

	res, err := runCase(t, common.HermeticObservedModuleDir(t), "-- xr.yaml --\n"+observedXR+
		"-- 1/observed.yaml --\n"+observedSnapshot("pending")+
		`-- 1/want.cue --
resources: probe: ready: "NotReady"
status: workloadReady: false
-- 2/observed.yaml --
`+observedSnapshot("seen")+
		`-- 2/want.cue --
resources: probe: ready: "Ready"
status: {
	observedCount: 1
	workloadReady: true
}
`)
	require.NoError(t, err)
	require.Len(t, res.Units, 2)
	assert.True(t, res.Passed(), "failures: %v", res.Failures())
}

func TestRunObservedNotOptedIn(t *testing.T) {
	t.Parallel()

	_, err := runCase(t, common.HermeticModuleDir(t), "-- xr.yaml --\n"+hermeticXR+
		"-- observed.yaml --\n"+observedSnapshot("seen")+
		"-- want.cue --\nresources: {}\n")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "does not declare")
	assert.Contains(t, err.Error(), "observedResources")
}

func TestRunFixtureErrors(t *testing.T) {
	t.Parallel()

	t.Run("missing spec", func(t *testing.T) {
		t.Parallel()
		_, err := runCase(t, common.HermeticModuleDir(t),
			"-- xr.yaml --\nmetadata:\n  name: demo\n-- want.cue --\nresources: {}\n")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "must carry a spec object")
	})

	t.Run("malformed xr", func(t *testing.T) {
		t.Parallel()
		_, err := runCase(t, common.HermeticModuleDir(t),
			"-- xr.yaml --\nmetadata: [unterminated\n-- want.cue --\nresources: {}\n")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "cannot parse xr.yaml")
	})

	t.Run("malformed observed", func(t *testing.T) {
		t.Parallel()
		_, err := runCase(t, common.HermeticObservedModuleDir(t),
			"-- xr.yaml --\n"+observedXR+
				"-- observed.yaml --\nkind: [unterminated\n-- want.cue --\nresources: {}\n")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "observed.yaml")
	})
}

func TestSeedGoldensPlacesStepSections(t *testing.T) {
	t.Parallel()

	source := "-- xr.yaml --\n" + observedXR +
		"-- 1/observed.yaml --\n" + observedSnapshot("pending") +
		"-- 2/observed.yaml --\n" + observedSnapshot("seen")
	res, err := runCase(t, common.HermeticObservedModuleDir(t), source)
	require.NoError(t, err)
	require.True(t, res.NeedsSeed())

	seeded, changed := SeedGoldens([]byte(source), res)
	require.True(t, changed)

	// Each golden lands directly after its step's observed section.
	text := string(seeded)
	first := strings.Index(text, "-- 1/want.yaml --")
	second := strings.Index(text, "-- 2/observed.yaml --")
	require.Positive(t, first)
	require.Positive(t, second)
	assert.Less(t, first, second, "1/want.yaml must precede 2/observed.yaml")

	res, err = runCase(t, common.HermeticObservedModuleDir(t), text)
	require.NoError(t, err)
	assert.True(t, res.Passed(), "failures: %v", res.Failures())
}
