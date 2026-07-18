package testharness

import (
	"testing"

	"github.com/crossplane/function-sdk-go/resource"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/meigma/crossplane-cuefn/internal/render"
)

func TestNormalizeResult(t *testing.T) {
	t.Parallel()

	res := render.Result{
		Resources: map[string]render.Resource{
			"a": {Object: map[string]any{"kind": "ConfigMap"}, Ready: resource.ReadyTrue},
			"b": {Object: map[string]any{"kind": "Secret"}, Ready: resource.ReadyFalse},
			"c": {Object: map[string]any{"kind": "Service"}, Ready: resource.ReadyUnspecified},
		},
	}

	doc := normalizeResult(res)

	resources, ok := doc["resources"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "Ready", resources["a"].(map[string]any)["ready"])
	assert.Equal(t, "NotReady", resources["b"].(map[string]any)["ready"])
	assert.Equal(t, "Unspecified", resources["c"].(map[string]any)["ready"])

	// Absences are explicit so unification can assert them.
	assert.Nil(t, doc["status"], "absent status must normalize to an explicit null")
	requirements, ok := doc["requirements"].(map[string]any)
	require.True(t, ok)
	assert.Empty(t, requirements, "absent requirements must normalize to an empty map")
}

func TestNormalizeResultRequirements(t *testing.T) {
	t.Parallel()

	res := render.Result{
		Requirements: map[string]render.Requirement{
			"cfg": {
				APIVersion: "v1",
				Kind:       "ConfigMap",
				MatchName:  "app-cfg",
				Namespace:  "default",
			},
			"secrets": {
				APIVersion:  "v1",
				Kind:        "Secret",
				MatchLabels: map[string]string{"app": "web"},
			},
		},
		Status: map[string]any{"ready": true},
	}

	doc := normalizeResult(res)

	requirements, ok := doc["requirements"].(map[string]any)
	require.True(t, ok)
	cfg, ok := requirements["cfg"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "app-cfg", cfg["matchName"])
	assert.Equal(t, "default", cfg["namespace"])
	assert.NotContains(t, cfg, "matchLabels", "empty selector fields are omitted")

	secrets, ok := requirements["secrets"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, map[string]string{"app": "web"}, secrets["matchLabels"])
	assert.NotContains(t, secrets, "matchName")
	assert.NotContains(t, secrets, "namespace")

	assert.Equal(t, map[string]any{"ready": true}, doc["status"])
}

func TestMarshalNormalizedDeterministicGolden(t *testing.T) {
	t.Parallel()

	doc := normalizeResult(render.Result{
		Resources: map[string]render.Resource{
			"config": {
				Object: map[string]any{"kind": "ConfigMap", "data": map[string]any{"k": "v"}},
				Ready:  resource.ReadyUnspecified,
			},
		},
	})

	golden, err := marshalNormalized(doc)
	require.NoError(t, err)
	assert.Equal(t, `requirements: {}
resources:
  config:
    object:
      data:
        k: v
      kind: ConfigMap
    ready: Unspecified
status: null
`, string(golden))
}

func TestDiffLines(t *testing.T) {
	t.Parallel()

	diff := diffLines("a\nb\nc\n", "a\nx\nc\n")
	assert.Contains(t, diff, "  a\n")
	assert.Contains(t, diff, "- b\n")
	assert.Contains(t, diff, "+ x\n")
	assert.Contains(t, diff, "  c\n")
}
