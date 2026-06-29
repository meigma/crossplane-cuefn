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
		"--dir", "../../example/module",
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
	root.SetArgs([]string{"publish", common.ExampleModuleRef, "--dir", "../../example/module"})

	err := root.ExecuteContext(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "package")
}
