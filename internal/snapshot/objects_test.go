package snapshot

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseObject(t *testing.T) {
	t.Parallel()

	obj, err := ParseObject([]byte("metadata:\n  name: demo\nspec:\n  replicas: 3\n"))
	require.NoError(t, err)
	assert.Equal(t, map[string]any{"name": "demo"}, obj["metadata"])

	_, err = ParseObject([]byte("metadata: [unterminated\n"))
	require.Error(t, err)
}

func TestParseObjectsKeepsEmbeddedSeparators(t *testing.T) {
	t.Parallel()

	// The "---" inside the ConfigMap datum is indented, so the Kubernetes
	// multi-document reader must not treat it as a document separator.
	objs, err := ParseObjects([]byte(`apiVersion: v1
kind: ConfigMap
metadata:
  name: embedded
data:
  nested.yaml: |
    ---
    key: value
---
apiVersion: v1
kind: ConfigMap
metadata:
  name: second
`))
	require.NoError(t, err)
	require.Len(t, objs, 2)
	assert.Equal(t, "embedded", objName(t, objs[0]))
	assert.Equal(t, "second", objName(t, objs[1]))
}
