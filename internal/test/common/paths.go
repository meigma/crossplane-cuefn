package common

import (
	"io/fs"
	"net"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// RepoRoot walks up from the test's working directory to the module root (the
// directory holding go.mod) and returns it. It is depth-independent, so callers
// build asset paths as filepath.Join(common.RepoRoot(t), "...") rather than
// fragile "../../" relatives that break when a test file moves.
func RepoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	require.NoError(t, err)
	start := dir
	for {
		if _, statErr := os.Stat(filepath.Join(dir, "go.mod")); statErr == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("no go.mod found walking up from %s", start)
			return ""
		}
		dir = parent
	}
}

// HermeticModuleDir returns the path to the shared self-contained test-fixture
// module (internal/test/common/testdata/module). It is the one module the unit
// and integration suites load, so the tests never depend on the user-facing
// example/ module (which may import external schemas). It is published under
// [ExampleModuleRef] by the integration tests.
func HermeticModuleDir(t *testing.T) string {
	t.Helper()
	return filepath.Join(RepoRoot(t), "internal/test/common/testdata/module")
}

// HermeticRenderloopDir returns the path to the self-contained crossplane
// render-loop assets (composition.yaml, xr.yaml, environmentconfig.yaml) the
// render-loop integration test drives, so that test does not depend on the
// example/ assets either.
func HermeticRenderloopDir(t *testing.T) string {
	t.Helper()
	return filepath.Join(RepoRoot(t), "internal/test/common/testdata/renderloop")
}

// FreePort reserves an ephemeral TCP port, closes the listener, and returns the
// freed port so a server can bind it. There is an inherent race between the close
// and the rebind, but it is small enough for a single-process test (the chosen
// close-then-return semantics, reconciled from the two divergent copies).
func FreePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	port := l.Addr().(*net.TCPAddr).Port
	require.NoError(t, l.Close())
	return port
}

// CacheDir returns a fresh temp directory outside $HOME for use as CUE_CACHE_DIR.
// It is not t.TempDir because CUE's module cache marks extracted dependency files
// read-only, which makes the automatic t.TempDir cleanup fail with EPERM; this
// helper makes the tree writable before removing it.
func CacheDir(t *testing.T) string {
	t.Helper()
	// Not t.TempDir: CUE's modcache marks extracted files read-only, so the
	// automatic t.TempDir cleanup fails with EPERM; this helper chmods first.
	dir, err := os.MkdirTemp("", "cuefn-cache-") //nolint:usetesting // see comment
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = filepath.WalkDir(dir, func(p string, _ fs.DirEntry, err error) error {
			if err == nil {
				_ = os.Chmod(p, 0o700)
			}
			return nil
		})
		_ = os.RemoveAll(dir)
	})
	return dir
}
