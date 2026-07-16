package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/crossplane/crossplane-runtime/v2/pkg/xcrd"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const observedOne = `apiVersion: apps/v1
kind: Deployment
metadata:
  name: physical-one
  namespace: default
  annotations:
    crossplane.io/composition-resource-name: workload
status:
  observedGeneration: 7
  vendor:
    revision: abc123
`

func TestLoadObservedObjects(t *testing.T) {
	t.Parallel()

	t.Run("empty path returns empty map", func(t *testing.T) {
		t.Parallel()
		objects, err := loadObservedObjects("")
		require.NoError(t, err)
		assert.Empty(t, objects)
	})

	t.Run("file preserves full object under annotation key", func(t *testing.T) {
		t.Parallel()
		path := writeObservedFixture(t, "observed.yaml", observedOne)

		objects, err := loadObservedObjects(path)
		require.NoError(t, err)
		require.Contains(t, objects, "workload")
		assert.NotContains(t, objects, "physical-one",
			"metadata.name must not replace the stable composition-resource name")

		status, ok := objects["workload"]["status"].(map[string]any)
		require.True(t, ok)
		vendor, ok := status["vendor"].(map[string]any)
		require.True(t, ok)
		assert.Equal(t, "abc123", vendor["revision"], "kind-specific status must survive unchanged")
	})

	t.Run("multi-document file", func(t *testing.T) {
		t.Parallel()
		path := writeObservedFixture(t, "observed.yaml", observedOne+`---
apiVersion: v1
kind: ConfigMap
metadata:
  name: physical-two
  annotations:
    crossplane.io/composition-resource-name: config
data:
  value: kept
`)

		objects, err := loadObservedObjects(path)
		require.NoError(t, err)
		assert.ElementsMatch(t, []string{"workload", "config"}, mapKeys(objects))
	})

	t.Run("directory aggregates yaml files", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(dir, "a.yaml"), []byte(observedOne), 0o600))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "b.yml"), []byte(`
apiVersion: batch/v1
kind: Job
metadata:
  name: migration-physical
  annotations:
    crossplane.io/composition-resource-name: migration
`), 0o600))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "README.md"), []byte("ignored"), 0o600))

		objects, err := loadObservedObjects(dir)
		require.NoError(t, err)
		assert.ElementsMatch(t, []string{"workload", "migration"}, mapKeys(objects))
	})

	t.Run("directory ignores nested yaml files", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(dir, "observed.yaml"), []byte(observedOne), 0o600))
		nested := filepath.Join(dir, "stale")
		require.NoError(t, os.Mkdir(nested, 0o700))
		require.NoError(t, os.WriteFile(filepath.Join(nested, "observed.yaml"), []byte(`
apiVersion: v1
kind: ConfigMap
metadata:
  name: stale-nested-fixture
  annotations:
    crossplane.io/composition-resource-name: stale
`), 0o600))

		objects, err := loadObservedObjects(dir)
		require.NoError(t, err)
		assert.Equal(t, []string{"workload"}, mapKeys(objects))
		assert.NotContains(t, objects, "stale", "nested fixtures must not be observed")
	})
}

func TestLoadObservedObjectsRejectsInvalidInput(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		content string
		want    string
	}{
		{
			name: "missing stable-name annotation",
			content: `apiVersion: v1
kind: ConfigMap
metadata:
  name: missing
`,
			want: xcrd.AnnotationKeyCompositionResourceName,
		},
		{
			name: "empty stable-name annotation",
			content: `apiVersion: v1
kind: ConfigMap
metadata:
  name: empty
  annotations:
    crossplane.io/composition-resource-name: ""
`,
			want: "non-empty",
		},
		{
			name: "duplicate stable name",
			content: observedOne + `---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: physical-two
  annotations:
    crossplane.io/composition-resource-name: workload
`,
			want: `duplicates stable name "workload"`,
		},
		{
			name:    "malformed YAML",
			content: "metadata: [unterminated\n",
			want:    "cannot read observed resources",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			path := writeObservedFixture(t, "invalid.yaml", tt.content)
			_, err := loadObservedObjects(path)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.want)
		})
	}

	t.Run("missing path", func(t *testing.T) {
		t.Parallel()
		_, err := loadObservedObjects(filepath.Join(t.TempDir(), "missing.yaml"))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "cannot read observed resources")
	})

	t.Run("empty directory", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		_, err := loadObservedObjects(dir)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no YAML files found")
		assert.Contains(t, err.Error(), dir)
	})
}

func writeObservedFixture(t *testing.T, name, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
	return path
}

func mapKeys[V any](values map[string]V) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	return keys
}
