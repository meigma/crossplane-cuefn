package schema

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestGenerateXRD_MaterializesDefaultsForRequiredContainers proves a required,
// fully-defaultable container (a nested struct, a keyless map, a zero-min list)
// gets an explicit empty default, while a required scalar keeps its own default
// and gains no spurious container default (findings H3/M10).
//
// The materialized schema still passes selfCheck (NewStructural +
// ValidateStructural, run inside GenerateXRD), so these defaults are the exact
// values the API server fills from `spec: {}` — closing the validate/render vs
// in-cluster drift where `spec: {}` was accepted locally but rejected with
// "Required value" server-side.
func TestGenerateXRD_MaterializesDefaultsForRequiredContainers(t *testing.T) {
	spec := schemaProps(t, "testdata/nesteddefault").Properties["spec"]

	// Every #Spec field is non-optional in CUE, so all are required.
	assert.ElementsMatch(t, []string{"resources", "labels", "tags", "replicas"}, spec.Required)

	resources := spec.Properties["resources"]
	require.NotNil(t, resources.Default, "required fully-defaultable struct needs an object default")
	assert.JSONEq(t, "{}", string(resources.Default.Raw))
	// Nested scalar defaults survive the pass.
	require.NotNil(t, resources.Properties["cpu"].Default)
	assert.JSONEq(t, `"250m"`, string(resources.Properties["cpu"].Default.Raw))
	require.NotNil(t, resources.Properties["memoryMi"].Default)
	assert.JSONEq(t, "256", string(resources.Properties["memoryMi"].Default.Raw))

	labels := spec.Properties["labels"]
	require.NotNil(t, labels.Default, "required keyless map needs an object default")
	assert.JSONEq(t, "{}", string(labels.Default.Raw))

	tags := spec.Properties["tags"]
	require.NotNil(t, tags.Default, "required zero-min list needs an array default")
	assert.JSONEq(t, "[]", string(tags.Default.Raw))

	// A required scalar keeps its own default and gains no container default.
	replicas := spec.Properties["replicas"]
	require.NotNil(t, replicas.Default)
	assert.JSONEq(t, "2", string(replicas.Default.Raw))
}
