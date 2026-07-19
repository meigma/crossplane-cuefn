//go:build !noxpkg

package cli

import (
	"bytes"
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/meigma/crossplane-cuefn/internal/test/common"
)

// TestPublish_MalformedModuleRef proves a module ref without @version fails with
// a clear non-nil error naming the ref, and never panics (criterion 3).
func TestPublish_MalformedModuleRef(t *testing.T) {
	t.Parallel()

	root := NewRootCommand(Options{Out: &bytes.Buffer{}, Err: &bytes.Buffer{}})
	root.SetArgs([]string{
		"publish", "cuefn.example/app",
		"--dir", "../test/common/testdata/module",
		"--package", "localhost:5000/cfg:v0.1.0",
		"--insecure",
	})

	var err error
	require.NotPanics(t, func() {
		err = root.ExecuteContext(context.Background())
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cuefn.example/app")
}

// TestPublish_RequiresPackage proves the destination --package flag is required.
func TestPublish_RequiresPackage(t *testing.T) {
	t.Parallel()

	root := NewRootCommand(Options{Out: &bytes.Buffer{}, Err: &bytes.Buffer{}})
	root.SetArgs([]string{"publish", common.ExampleModuleRef, "--dir", "../test/common/testdata/module"})

	err := root.ExecuteContext(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "package")
}

func TestParseMetadata(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		values []string
		want   map[string]string
		match  string
	}{
		{
			name: "multiple values and first equals split",
			values: []string{
				"org.opencontainers.image.source=https://github.com/meigma/example?key=value",
				"dev.meigma.owner=platform=team",
			},
			want: map[string]string{
				"org.opencontainers.image.source": "https://github.com/meigma/example?key=value",
				"dev.meigma.owner":                "platform=team",
			},
		},
		{name: "missing separator", values: []string{"dev.meigma.owner"}, match: "key=value"},
		{name: "empty key", values: []string{"=platform"}, match: "key cannot be empty"},
		{name: "empty value", values: []string{"dev.meigma.owner="}, match: "value cannot be empty"},
		{
			name:   "duplicate key",
			values: []string{"dev.meigma.owner=platform", "dev.meigma.owner=runtime"},
			match:  "duplicate",
		},
		{
			name:   "source must be HTTP URL",
			values: []string{"org.opencontainers.image.source=github.com/meigma/example"},
			match:  "absolute HTTP(S) URL",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := parseMetadata(tt.values)
			if tt.match != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.match)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestPublish_PublishModuleRequiresDir(t *testing.T) {
	t.Parallel()

	root := NewRootCommand(Options{Out: &bytes.Buffer{}, Err: &bytes.Buffer{}})
	root.SetArgs([]string{
		"publish", common.ExampleModuleRef,
		"--package", "localhost:5000/cfg:v0.1.0",
		"--publish-module",
		"--insecure",
	})

	err := root.ExecuteContext(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "requires a local module --dir")
}
