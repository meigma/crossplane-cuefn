package snapshot

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/meigma/crossplane-cuefn/internal/render"
)

// configMap builds a minimal ConfigMap object for the match tests.
func configMap(name, namespace string, labels map[string]any) map[string]any {
	meta := map[string]any{"name": name}
	if namespace != "" {
		meta["namespace"] = namespace
	}
	if labels != nil {
		meta["labels"] = labels
	}
	return map[string]any{
		"apiVersion": "v1",
		"kind":       "ConfigMap",
		"metadata":   meta,
	}
}

func TestMatchRequirements(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		objs []map[string]any
		reqs map[string]render.Requirement
		// want maps each requirement key to the ordered metadata.name values
		// expected in its bucket. A present key with an empty slice asserts the
		// "requested, none found" empty bucket.
		want map[string][]string
	}{
		{
			name: "matchName hit",
			objs: []map[string]any{
				configMap("app-cfg", "default", nil),
				configMap("other", "default", nil),
			},
			reqs: map[string]render.Requirement{
				"cfg": {APIVersion: "v1", Kind: "ConfigMap", MatchName: "app-cfg"},
			},
			want: map[string][]string{"cfg": {"app-cfg"}},
		},
		{
			name: "matchName miss yields present empty bucket",
			objs: []map[string]any{configMap("other", "default", nil)},
			reqs: map[string]render.Requirement{
				"cfg": {APIVersion: "v1", Kind: "ConfigMap", MatchName: "app-cfg"},
			},
			want: map[string][]string{"cfg": {}},
		},
		{
			name: "matchLabels subset hit",
			objs: []map[string]any{
				configMap("a", "default", map[string]any{"app": "web", "tier": "fe"}),
				configMap("b", "default", map[string]any{"app": "web"}),
				configMap("c", "default", map[string]any{"app": "api"}),
			},
			reqs: map[string]render.Requirement{
				"cfg": {APIVersion: "v1", Kind: "ConfigMap", MatchLabels: map[string]string{"app": "web"}},
			},
			// Both a and b carry app=web; the selector is a subset, so c is excluded.
			want: map[string][]string{"cfg": {"a", "b"}},
		},
		{
			name: "multi-key matchLabels requires every selector label",
			objs: []map[string]any{
				configMap("both", "default", map[string]any{"app": "web", "tier": "fe"}),
				configMap("one", "default", map[string]any{"app": "web"}),
				configMap("missing", "default", map[string]any{"tier": "fe"}),
				configMap("super", "default", map[string]any{"app": "web", "tier": "fe", "env": "prod"}),
			},
			reqs: map[string]render.Requirement{
				"cfg": {
					APIVersion:  "v1",
					Kind:        "ConfigMap",
					MatchLabels: map[string]string{"app": "web", "tier": "fe"},
				},
			},
			// Only objects carrying BOTH app=web AND tier=fe match (a superset is
			// fine); "one" (missing tier) and "missing" (missing app) are excluded.
			want: map[string][]string{"cfg": {"both", "super"}},
		},
		{
			name: "namespace filter excludes wrong namespace",
			objs: []map[string]any{
				configMap("app-cfg", "prod", nil),
				configMap("app-cfg", "default", nil),
			},
			reqs: map[string]render.Requirement{
				"cfg": {APIVersion: "v1", Kind: "ConfigMap", MatchName: "app-cfg", Namespace: "default"},
			},
			want: map[string][]string{"cfg": {"app-cfg"}},
		},
		{
			name: "deterministic sort by namespace then name",
			objs: []map[string]any{
				configMap("z", "b", map[string]any{"app": "web"}),
				configMap("a", "b", map[string]any{"app": "web"}),
				configMap("m", "a", map[string]any{"app": "web"}),
			},
			reqs: map[string]render.Requirement{
				"cfg": {APIVersion: "v1", Kind: "ConfigMap", MatchLabels: map[string]string{"app": "web"}},
			},
			// "a/m" < "b/a" < "b/z".
			want: map[string][]string{"cfg": {"m", "a", "z"}},
		},
		{
			name: "kind mismatch excludes",
			objs: []map[string]any{
				{"apiVersion": "v1", "kind": "Secret", "metadata": map[string]any{"name": "app-cfg"}},
			},
			reqs: map[string]render.Requirement{
				"cfg": {APIVersion: "v1", Kind: "ConfigMap", MatchName: "app-cfg"},
			},
			want: map[string][]string{"cfg": {}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := MatchRequirements(tt.objs, tt.reqs)

			require.Len(t, got, len(tt.want))
			for key, names := range tt.want {
				bucket, ok := got[key]
				require.True(t, ok, "requirement %q must always be a present key", key)
				require.NotNil(t, bucket, "bucket for %q must be non-nil so the module sees a concrete list", key)

				gotNames := make([]string, 0, len(bucket))
				for _, obj := range bucket {
					meta, _ := obj["metadata"].(map[string]any)
					name, _ := meta["name"].(string)
					gotNames = append(gotNames, name)
				}
				assert.Equal(t, names, gotNames, "ordered matches for %q", key)
			}
		})
	}
}

func TestLoadRequiredObjects(t *testing.T) {
	t.Parallel()

	const single = `apiVersion: v1
kind: ConfigMap
metadata:
  name: app-cfg
data:
  image: img:1
`
	const multi = `apiVersion: v1
kind: ConfigMap
metadata:
  name: one
---
# a comment-only document is skipped
---
apiVersion: v1
kind: ConfigMap
metadata:
  name: two
`

	t.Run("empty path returns nil", func(t *testing.T) {
		t.Parallel()
		objs, err := LoadRequiredObjects("")
		require.NoError(t, err)
		assert.Nil(t, objs)
	})

	t.Run("single document file", func(t *testing.T) {
		t.Parallel()
		path := filepath.Join(t.TempDir(), "cm.yaml")
		require.NoError(t, os.WriteFile(path, []byte(single), 0o600))

		objs, err := LoadRequiredObjects(path)
		require.NoError(t, err)
		require.Len(t, objs, 1)
		assert.Equal(t, "ConfigMap", objs[0]["kind"])
	})

	t.Run("multi-document file skips empty documents", func(t *testing.T) {
		t.Parallel()
		path := filepath.Join(t.TempDir(), "cms.yaml")
		require.NoError(t, os.WriteFile(path, []byte(multi), 0o600))

		objs, err := LoadRequiredObjects(path)
		require.NoError(t, err)
		require.Len(t, objs, 2)
		assert.Equal(t, "one", objName(t, objs[0]))
		assert.Equal(t, "two", objName(t, objs[1]))
	})

	t.Run("directory aggregates yaml files and skips non-yaml", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(dir, "a.yaml"), []byte(single), 0o600))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "b.yml"), []byte(multi), 0o600))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "README.md"), []byte("ignored"), 0o600))

		objs, err := LoadRequiredObjects(dir)
		require.NoError(t, err)
		// 1 from a.yaml + 2 from b.yml; README.md ignored.
		assert.Len(t, objs, 3)
	})

	t.Run("missing path errors", func(t *testing.T) {
		t.Parallel()
		_, err := LoadRequiredObjects(filepath.Join(t.TempDir(), "does-not-exist.yaml"))
		require.Error(t, err)
	})
}

// objName extracts metadata.name from a decoded object for the assertions.
func objName(t *testing.T, obj map[string]any) string {
	t.Helper()
	meta, ok := obj["metadata"].(map[string]any)
	require.True(t, ok)
	n, ok := meta["name"].(string)
	require.True(t, ok)
	return n
}
