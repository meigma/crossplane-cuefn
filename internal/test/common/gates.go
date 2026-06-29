package common

import (
	"os"
	"os/exec"
	"testing"

	"github.com/testcontainers/testcontainers-go"
)

// RequireDocker skips the calling test when integration mode is off or no usable
// Docker daemon is present, so `go test ./...` stays green on a developer machine
// without Docker while CI (which has a daemon) runs the suite fully.
func RequireDocker(t *testing.T) {
	t.Helper()
	if os.Getenv("CUEFN_INTEGRATION") == "" {
		t.Skip("integration test: set CUEFN_INTEGRATION=1 to run (via the integration moon tasks/workflow)")
	}
	testcontainers.SkipIfProviderIsNotHealthy(t)
}

// RequireCrossplane skips the test unless a working crossplane CLI is on PATH and
// returns its resolved path.
//
// LookPath alone is insufficient: under the moon task runner the proto-managed
// PATH places a generic `crossplane` shim ahead of the real, mise-pinned binary.
// That shim is not a crossplane and exits non-zero for every invocation. We
// therefore probe the resolved binary with the purely-local `render --help`
// (no cluster contact) and skip when it does not behave like crossplane, so the
// test self-skips on the shim rather than failing or — worse — passing against a
// fake crossplane.
func RequireCrossplane(t *testing.T) string {
	t.Helper()
	if os.Getenv("CUEFN_INTEGRATION") == "" {
		t.Skip("integration test: set CUEFN_INTEGRATION=1 to run (via the integration moon tasks/workflow)")
	}
	path, err := exec.LookPath("crossplane")
	if err != nil {
		t.Skip("crossplane CLI not on PATH; skipping integration test")
	}
	if out, err := exec.Command(path, "render", "--help").CombinedOutput(); err != nil {
		t.Skipf("%q is not a working crossplane CLI (%v); skipping integration test: %s", path, err, out)
	}
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
