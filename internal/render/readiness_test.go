package render_test

import (
	"context"
	"testing"

	"github.com/crossplane/function-sdk-go/resource"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/meigma/crossplane-cuefn/internal/render"
	"github.com/meigma/crossplane-cuefn/internal/test/common"
)

const readinessDir = "../test/common/testdata/readiness"

type readinessSnapshot struct {
	deploymentGeneration         int
	deploymentObservedGeneration int
	updatedReplicas              int
	availableReplicas            int
	jobSucceeded                 int
	jobFailed                    bool
	configUID                    string
	priorRelease                 bool
}

func currentReadinessSnapshot() readinessSnapshot {
	return readinessSnapshot{
		deploymentGeneration:         3,
		deploymentObservedGeneration: 3,
		updatedReplicas:              1,
		availableReplicas:            1,
		jobSucceeded:                 1,
		configUID:                    "config-uid",
	}
}

func observedReadinessResources(snapshot readinessSnapshot) map[string]map[string]any {
	release := "r1"
	if snapshot.priorRelease {
		release = "r0"
	}
	jobConditions := []any{map[string]any{"type": "Complete", "status": "True"}}
	if snapshot.jobFailed {
		jobConditions = append(jobConditions, map[string]any{"type": "Failed", "status": "True"})
	}

	return map[string]map[string]any{
		"migration": {
			"apiVersion": "batch/v1",
			"kind":       "Job",
			"metadata": map[string]any{
				"name": "demo-migration-" + release,
				"uid":  "migration-uid",
				"labels": map[string]any{
					"cuefn.example/release":   release,
					"cuefn.example/readiness": "demo",
				},
			},
			"status": map[string]any{
				"succeeded":  snapshot.jobSucceeded,
				"conditions": jobConditions,
			},
		},
		"workload": {
			"apiVersion": "apps/v1",
			"kind":       "Deployment",
			"metadata": map[string]any{
				"name":       "demo-workload-" + release,
				"uid":        "workload-uid",
				"generation": snapshot.deploymentGeneration,
				"labels": map[string]any{
					"cuefn.example/release":   release,
					"cuefn.example/readiness": "demo",
				},
			},
			"spec": map[string]any{"replicas": 1},
			"status": map[string]any{
				"observedGeneration": snapshot.deploymentObservedGeneration,
				"updatedReplicas":    snapshot.updatedReplicas,
				"availableReplicas":  snapshot.availableReplicas,
				"conditions": []any{
					map[string]any{"type": "Available", "status": "True"},
				},
			},
		},
		"config": {
			"apiVersion": "v1",
			"kind":       "ConfigMap",
			"metadata": map[string]any{
				"name": "demo-config-" + release,
				"uid":  snapshot.configUID,
				"labels": map[string]any{
					"cuefn.example/release":   release,
					"cuefn.example/readiness": "demo",
				},
			},
		},
	}
}

func renderReadiness(t *testing.T, observed map[string]map[string]any) render.Result {
	t.Helper()

	result, err := render.New(render.LocalLoader{Dir: readinessDir}).Render(
		context.Background(),
		"ignored-by-local-loader",
		render.Inputs{
			Metadata:          render.Metadata{Name: "demo", Namespace: "default"},
			ObservedResources: observed,
		},
	)
	require.NoError(t, err)
	return result
}

func nestedReadinessValue(t *testing.T, object map[string]any, path ...any) any {
	t.Helper()

	var value any = object
	for _, segment := range path {
		switch segment := segment.(type) {
		case string:
			fields, ok := value.(map[string]any)
			require.True(t, ok, "expected an object before field %q", segment)
			value, ok = fields[segment]
			require.True(t, ok, "expected field %q", segment)
		case int:
			items, ok := value.([]any)
			require.True(t, ok, "expected a list before index %d", segment)
			require.Greater(t, len(items), segment, "expected index %d", segment)
			value = items[segment]
		default:
			require.FailNow(t, "unsupported path segment", "%T", segment)
		}
	}
	return value
}

func TestRenderObservedReadinessTransitions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		firstPass bool
		mutate    func(*readinessSnapshot)
		want      map[string]bool
	}{
		{
			name:      "first pass is conservatively not ready",
			firstPass: true,
			want:      map[string]bool{"migration": false, "workload": false, "config": false},
		},
		{
			name: "current observations are ready",
			want: map[string]bool{"migration": true, "workload": true, "config": true},
		},
		{
			name: "stale deployment generation is not ready",
			mutate: func(snapshot *readinessSnapshot) {
				snapshot.deploymentObservedGeneration--
			},
			want: map[string]bool{"migration": true, "workload": false, "config": true},
		},
		{
			name: "insufficient deployment replicas are not ready",
			mutate: func(snapshot *readinessSnapshot) {
				snapshot.updatedReplicas = 0
			},
			want: map[string]bool{"migration": true, "workload": false, "config": true},
		},
		{
			name: "a failed job is not ready even when complete",
			mutate: func(snapshot *readinessSnapshot) {
				snapshot.jobFailed = true
			},
			want: map[string]bool{"migration": false, "workload": true, "config": true},
		},
		{
			name: "a config map without a UID is not ready",
			mutate: func(snapshot *readinessSnapshot) {
				snapshot.configUID = ""
			},
			want: map[string]bool{"migration": true, "workload": true, "config": false},
		},
		{
			name: "healthy observations from a prior release are not ready",
			mutate: func(snapshot *readinessSnapshot) {
				snapshot.priorRelease = true
			},
			want: map[string]bool{"migration": false, "workload": false, "config": false},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			snapshot := currentReadinessSnapshot()
			if tt.mutate != nil {
				tt.mutate(&snapshot)
			}
			var observed map[string]map[string]any
			if !tt.firstPass {
				observed = observedReadinessResources(snapshot)
			}

			result := renderReadiness(t, observed)
			for name, wantReady := range tt.want {
				wantHint := resource.ReadyFalse
				if wantReady {
					wantHint = resource.ReadyTrue
				}
				assert.Equal(t, wantHint, result.Resources[name].Ready, name)
				assert.Equal(t, wantReady, result.Status[name+"Ready"], name)
			}
		})
	}
}

func TestRenderObservedReadinessDesiredResources(t *testing.T) {
	t.Parallel()

	result, err := render.New(render.LocalLoader{Dir: readinessDir}).Render(
		context.Background(),
		"ignored-by-local-loader",
		render.Inputs{
			Spec: map[string]any{
				"release":          "r2",
				"suspendMigration": false,
				"pauseWorkload":    false,
			},
			Metadata: render.Metadata{Name: "demo", Namespace: "default"},
		},
	)
	require.NoError(t, err)

	job := common.Object(t, result, "migration")
	assert.Equal(t, "demo-migration-r2", nestedReadinessValue(t, job, "metadata", "name"))
	assert.Equal(t, false, nestedReadinessValue(t, job, "spec", "suspend"))
	assert.Equal(t, "crossplane-cuefn:dev",
		nestedReadinessValue(t, job, "spec", "template", "spec", "containers", 0, "image"))

	deployment := common.Object(t, result, "workload")
	assert.Equal(t, "demo-workload-r2", nestedReadinessValue(t, deployment, "metadata", "name"))
	assert.Equal(t, false, nestedReadinessValue(t, deployment, "spec", "paused"))
	assert.Equal(t, 1, common.ToInt(t, nestedReadinessValue(t, deployment, "spec", "replicas")))
	assert.Equal(t, "crossplane-cuefn:dev",
		nestedReadinessValue(t, deployment, "spec", "template", "spec", "containers", 0, "image"))

	config := common.Object(t, result, "config")
	assert.Equal(t, "demo-config-r2", nestedReadinessValue(t, config, "metadata", "name"))
	assert.NotContains(t, config, "status", "the ConfigMap must remain conditionless")
}
