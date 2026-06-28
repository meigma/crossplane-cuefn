package render_test

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/meigma/crossplane-cuefn/internal/render"
)

func TestProjectSpecStripsReserved(t *testing.T) {
	in := map[string]any{
		"image":                       "img:1",
		"replicas":                    3,
		"crossplane":                  map[string]any{"compositionRef": map[string]any{"name": "c"}},
		"compositionRef":              map[string]any{"name": "c"},
		"compositionSelector":         map[string]any{},
		"compositionRevisionRef":      map[string]any{},
		"compositionRevisionSelector": map[string]any{},
		"compositionUpdatePolicy":     "Automatic",
		"claimRef":                    map[string]any{},
		"resourceRef":                 map[string]any{},
		"resourceRefs":                []any{},
		"writeConnectionSecretToRef":  map[string]any{},
		"publishConnectionDetailsTo":  map[string]any{},
		"environmentConfigRefs":       []any{},
	}

	got := render.ProjectSpec(in)

	want := map[string]any{"image": "img:1", "replicas": 3}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("ProjectSpec() mismatch (-want +got):\n%s", diff)
	}
}

func TestProjectSpecPreservesUserKeys(t *testing.T) {
	in := map[string]any{
		"image":    "img:1",
		"replicas": 3,
		"nested":   map[string]any{"a": 1},
	}

	got := render.ProjectSpec(in)

	assert.Equal(t, in, got)
}

func TestProjectSpecDoesNotMutateInput(t *testing.T) {
	in := map[string]any{
		"image":      "img:1",
		"crossplane": map[string]any{"compositionRef": map[string]any{"name": "c"}},
	}

	_ = render.ProjectSpec(in)

	_, stillThere := in["crossplane"]
	assert.True(t, stillThere, "ProjectSpec must not mutate its input")
	assert.Len(t, in, 2)
}

func TestProjectSpecNil(t *testing.T) {
	assert.Nil(t, render.ProjectSpec(nil))
}

func TestProjectSpecEmpty(t *testing.T) {
	got := render.ProjectSpec(map[string]any{})

	require.NotNil(t, got)
	assert.Empty(t, got)
}
