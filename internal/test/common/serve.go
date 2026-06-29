package common

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	fnv1 "github.com/crossplane/function-sdk-go/proto/v1"
)

// BuildBinary compiles the cuefn binary into a temp dir and returns its path. It
// resolves cmd/cuefn via RepoRoot so it is correct regardless of the caller's
// package depth.
func BuildBinary(t *testing.T) string {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "cuefn")
	cmd := exec.Command("go", "build", "-o", bin, filepath.Join(RepoRoot(t), "cmd/cuefn"))
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "build cuefn: %s", out)
	return bin
}

// ServeFunction starts `cuefn function --insecure` listening on bindAddr with the
// given CUE_REGISTRY, waits (dialing dialAddr) until the gRPC server answers, and
// stops it at cleanup.
func ServeFunction(t *testing.T, bin, bindAddr, dialAddr, cueRegistry, cacheDir string) {
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

	WaitForFunction(t, dialAddr)
}

// WaitForFunction dials addr and issues a RunFunction until the server responds,
// proving it serves the gRPC FunctionRunnerService.
func WaitForFunction(t *testing.T, addr string) {
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

// WriteFunctions writes a functions.yaml pointing the Development runtime at the
// locally-served cuefn at target, so `crossplane render` connects to it, and
// returns the file path.
func WriteFunctions(t *testing.T, dir, target string) string {
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
