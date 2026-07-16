package common

import (
	"os"
	"os/exec"
	"testing"

	"github.com/stretchr/testify/require"
)

// RequireDocker skips the calling test when integration mode is off. Once a
// caller opts in, a missing or unusable Docker daemon is a setup failure rather
// than a skipped green integration run.
func RequireDocker(t *testing.T) {
	t.Helper()
	if os.Getenv("CUEFN_INTEGRATION") == "" {
		t.Skip("integration test: set CUEFN_INTEGRATION=1 to run (via the integration moon tasks/workflow)")
	}
	docker, err := exec.LookPath("docker")
	require.NoError(t, err, "docker must be on PATH when CUEFN_INTEGRATION is set")
	out, err := exec.Command(docker, "info").CombinedOutput()
	require.NoErrorf(t, err, "docker must be usable when CUEFN_INTEGRATION is set: %s", out)
}

// RequireCrossplane skips the test when integration mode is off and otherwise
// requires a working crossplane CLI, returning its resolved path.
//
// LookPath alone is insufficient: under the moon task runner the proto-managed
// PATH places a generic `crossplane` shim ahead of the real, mise-pinned binary.
// That shim is not a crossplane and exits non-zero for every invocation. We
// therefore probe the resolved binary with the purely-local `render --help`
// (no cluster contact) and fail an opted-in integration run when it does not
// behave like crossplane rather than passing against a fake binary.
func RequireCrossplane(t *testing.T) string {
	t.Helper()
	if os.Getenv("CUEFN_INTEGRATION") == "" {
		t.Skip("integration test: set CUEFN_INTEGRATION=1 to run (via the integration moon tasks/workflow)")
	}
	path, err := exec.LookPath("crossplane")
	require.NoError(t, err, "crossplane CLI must be on PATH when CUEFN_INTEGRATION is set")
	out, err := exec.Command(path, "render", "--help").CombinedOutput()
	require.NoErrorf(t, err, "%q must be a working crossplane CLI when CUEFN_INTEGRATION is set: %s", path, out)
	return path
}

// RequireBinary skips the calling test unless bin is on PATH and returns its
// resolved path. The gated moon tasks run under `mise exec` so the pinned binary
// wins; `go test ./...` self-skips when it is absent.
func RequireBinary(t *testing.T, bin string) string {
	t.Helper()
	if os.Getenv("CUEFN_INTEGRATION") == "" {
		t.Skip("integration test: set CUEFN_INTEGRATION=1 to run (via the integration moon tasks/workflow)")
	}
	path, err := exec.LookPath(bin)
	if err != nil {
		t.Skipf("%s not on PATH; skipping (run via `mise exec`)", bin)
	}
	return path
}

// RequireDevImage skips the test unless Docker is usable and the dev image
// (DevImage) has been built locally (via `mise run image-local`). It returns the
// resolved docker path and the dev image tag.
func RequireDevImage(t *testing.T) (dockerPath, image string) {
	t.Helper()
	if os.Getenv("CUEFN_INTEGRATION") == "" {
		t.Skip("integration test: set CUEFN_INTEGRATION=1 to run (via the integration moon tasks/workflow)")
	}
	docker, err := exec.LookPath("docker")
	if err != nil {
		t.Skip("docker not on PATH; skipping image smoke test")
	}
	if out, err := exec.Command(docker, "image", "inspect", DevImage).CombinedOutput(); err != nil {
		t.Skipf("image %s not present (run `mise run image-local`): %s", DevImage, out)
	}
	return docker, DevImage
}
