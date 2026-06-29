package function_test

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	fnv1 "github.com/crossplane/function-sdk-go/proto/v1"
)

// exampleRef is the module ref the example Composition references.
const exampleRef = "cuefn.example/app@v0.1.0"

// requireCrossplane skips the test unless a working crossplane CLI is on PATH.
//
// LookPath alone is insufficient: under the moon task runner the proto-managed
// PATH places a generic `crossplane` shim ahead of the real, mise-pinned binary.
// That shim is not a crossplane and exits non-zero for every invocation. We
// therefore probe the resolved binary with the purely-local `render --help`
// (no cluster contact) and skip when it does not behave like crossplane, so the
// test self-skips on the shim rather than failing.
func requireCrossplane(t *testing.T) string {
	t.Helper()
	if os.Getenv("CUEFN_INTEGRATION") == "" {
		t.Skip("integration test: set CUEFN_INTEGRATION=1 to run (via the integration moon tasks/workflow)")
	}
	path, err := exec.LookPath("crossplane")
	if err != nil {
		t.Skip("crossplane CLI not on PATH; skipping render-loop integration test")
	}
	if out, err := exec.Command(path, "render", "--help").CombinedOutput(); err != nil {
		t.Skipf("%q is not a working crossplane CLI (%v); skipping render-loop integration test: %s", path, err, out)
	}
	return path
}

// freePort returns a currently-free localhost TCP port.
func freePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port
}

// buildBinary compiles the cuefn binary into a temp dir and returns its path.
func buildBinary(t *testing.T) string {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "cuefn")
	cmd := exec.Command("go", "build", "-o", bin, "../../cmd/cuefn")
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "build cuefn: %s", out)
	return bin
}

// serveFunction starts `cuefn function --insecure` listening on bindAddr with the
// given CUE_REGISTRY, waits (dialing dialAddr) until the gRPC server answers, and
// stops it at cleanup.
func serveFunction(t *testing.T, bin, bindAddr, dialAddr, cueRegistry, cacheDir string) {
	t.Helper()

	cmd := exec.Command(
		bin,
		"function",
		"--insecure",
		"--address",
		bindAddr,
		"--cache-dir",
		cacheDir,
	)
	cmd.Env = append(os.Environ(), "CUE_REGISTRY="+cueRegistry)
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	require.NoError(t, cmd.Start())
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		_, _ = cmd.Process.Wait()
	})

	waitForFunction(t, dialAddr)
}

// waitForFunction dials addr and issues a RunFunction until the server responds,
// proving it serves the gRPC FunctionRunnerService.
func waitForFunction(t *testing.T, addr string) {
	t.Helper()

	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
		if err != nil {
			time.Sleep(200 * time.Millisecond)
			continue
		}
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		_, err = fnv1.NewFunctionRunnerServiceClient(conn).RunFunction(ctx, &fnv1.RunFunctionRequest{
			Meta: &fnv1.RequestMeta{Tag: "probe"},
		})
		cancel()
		_ = conn.Close()
		if err == nil {
			return
		}
		time.Sleep(200 * time.Millisecond)
	}
	t.Fatalf("function did not become ready at %s", addr)
}

// writeFunctions writes a functions.yaml pointing the Development runtime at the
// locally-served cuefn at target, so `crossplane render` connects to it.
func writeFunctions(t *testing.T, dir, target string) string {
	t.Helper()
	path := filepath.Join(dir, "functions.yaml")
	body := fmt.Sprintf(`apiVersion: pkg.crossplane.io/v1
kind: Function
metadata:
  name: cuefn
  annotations:
    render.crossplane.io/runtime: Development
    render.crossplane.io/runtime-development-target: %q
spec:
  package: xpkg.meigma.io/cuefn:v0
---
apiVersion: pkg.crossplane.io/v1
kind: Function
metadata:
  name: function-environment-configs
spec:
  package: xpkg.crossplane.io/crossplane-contrib/function-environment-configs:v0.7.2
`, target)
	require.NoError(t, os.WriteFile(path, []byte(body), 0o600))
	return path
}

// TestRenderLoop_CrossplaneRender proves the full v2 render loop: the example
// module is served from a local OCI registry, `cuefn function --insecure`
// renders it under `crossplane render`, and the env-driven ConfigMap data.tier
// equals the EnvironmentConfig value rather than the module's "unset" default —
// proving the environment flows end to end (criterion C3). It self-skips without
// Docker and the crossplane CLI.
func TestRenderLoop_CrossplaneRender(t *testing.T) {
	reg := startRegistry(t)
	crossplane := requireCrossplane(t)
	reg.publishModule(t, exampleRef, "../../example/module")

	bin := buildBinary(t)
	// Bind the function server on all interfaces (0.0.0.0): crossplane render runs
	// the cuefn step in a Docker container and reaches the host-served function via
	// the bridge gateway (e.g. 172.17.0.1 on Linux), which a 127.0.0.1-only bind
	// refuses. functions.yaml still targets 127.0.0.1 — crossplane translates it to
	// the host's container-reachable address per platform.
	port := freePort(t)
	bindAddr := fmt.Sprintf("0.0.0.0:%d", port)
	dialAddr := fmt.Sprintf("127.0.0.1:%d", port)
	serveFunction(t, bin, bindAddr, dialAddr, reg.cueRegistry, t.TempDir())

	functions := writeFunctions(t, t.TempDir(), dialAddr)

	// crossplane render runs the function-environment-configs step as a Docker
	// container (only cuefn has the Development annotation), so a cold image pull
	// on a CI runner can blow crossplane's default 1m timeout
	// ("error waiting for container ... context deadline exceeded"). Give it room.
	cmd := exec.Command(crossplane, "render",
		"../../example/xr.yaml",
		"../../example/composition.yaml",
		functions,
		"--extra-resources", "../../example/environmentconfig.yaml",
		"--timeout", "10m",
	)
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "crossplane render: %s", out)

	rendered := string(out)
	assert.Contains(t, rendered, "kind: Deployment")
	assert.Contains(t, rendered, "kind: Service")
	assert.Contains(t, rendered, "kind: ConfigMap")

	// The env-driven tier must be the EnvironmentConfig value, not the default.
	assert.Contains(t, rendered, "tier: production")
	assert.NotContains(t, strings.ReplaceAll(rendered, "tier: production", ""), "tier: unset")
}
