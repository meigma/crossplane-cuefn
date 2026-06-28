package schema

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	extv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
)

func ref(s string) *extv1.JSONSchemaProps {
	return &extv1.JSONSchemaProps{Ref: &s}
}

// TestInlineRefs_Diamond proves a non-cyclic schema shared by two fields inlines
// into both without error (a diamond, not a cycle).
func TestInlineRefs_Diamond(t *testing.T) {
	components := componentSchemas{
		"Leaf": {Type: "string"},
	}
	root := &extv1.JSONSchemaProps{
		Type: "object",
		Properties: map[string]extv1.JSONSchemaProps{
			"a": *ref("#/components/schemas/Leaf"),
			"b": *ref("#/components/schemas/Leaf"),
		},
	}

	out, err := inlineRefs(root, components)
	require.NoError(t, err)
	assert.Equal(t, "string", out.Properties["a"].Type)
	assert.Equal(t, "string", out.Properties["b"].Type)
	assert.Nil(t, out.Properties["a"].Ref)
}

// TestInlineRefs_Cycle proves a self-referential definition terminates with an
// error rather than recursing forever — a recursive type cannot be expressed as
// a finite structural schema.
func TestInlineRefs_Cycle(t *testing.T) {
	components := componentSchemas{
		"Node": {
			Type: "object",
			Properties: map[string]extv1.JSONSchemaProps{
				"child": *ref("#/components/schemas/Node"),
			},
		},
	}
	root := &extv1.JSONSchemaProps{
		Type: "object",
		Properties: map[string]extv1.JSONSchemaProps{
			"root": *ref("#/components/schemas/Node"),
		},
	}

	_, err := inlineRefs(root, components)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "recursive")
	assert.Contains(t, err.Error(), "Node")
}

// TestInlineRefs_Dangling proves a reference with no matching definition is a
// clear error, not a nil-deref.
func TestInlineRefs_Dangling(t *testing.T) {
	_, err := inlineRefs(ref("#/components/schemas/Missing"), componentSchemas{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Missing")
}
