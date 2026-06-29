package function_test

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	fnv1 "github.com/crossplane/function-sdk-go/proto/v1"
)

// devImage is the local image tag produced by `mise run image-local`.
const devImage = "crossplane-cuefn:dev"

// requireDevImage skips the test unless Docker is usable and the dev image has
// been built locally (via `mise run image-local`).
func requireDevImage(t *testing.T) string {
	t.Helper()
	if os.Getenv("CUEFN_INTEGRATION") == "" {
		t.Skip("integration test: set CUEFN_INTEGRATION=1 to run (via the integration moon tasks/workflow)")
	}
	docker, err := exec.LookPath("docker")
	if err != nil {
		t.Skip("docker not on PATH; skipping image smoke test")
	}
	if out, err := exec.Command(docker, "image", "inspect", devImage).
		CombinedOutput(); err != nil {
		t.Skipf("image %s not present (run `mise run image-local`): %s", devImage, out)
	}
	return docker
}

// TestImageServesFunction proves the apko image runs the function as its default
// command: launched as `function --insecure`, it starts the gRPC
// FunctionRunnerService and answers a RunFunction rather than printing help
// (criterion C4). It self-skips without Docker or the dev image.
func TestImageServesFunction(t *testing.T) {
	docker := requireDevImage(t)

	const port = "29443"
	run := exec.Command(docker, "run", "--rm", "-d",
		"-p", port+":9443",
		devImage,
		"function", "--insecure", "--address", ":9443",
	)
	idOut, err := run.CombinedOutput()
	require.NoError(t, err, "docker run: %s", idOut)
	id := strings.TrimSpace(string(idOut))
	t.Cleanup(func() { _ = exec.Command(docker, "rm", "-f", id).Run() })

	addr := "127.0.0.1:" + port
	deadline := time.Now().Add(30 * time.Second)
	for {
		conn, dialErr := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
		if dialErr == nil {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			// An empty request is enough: the server answering at all (even with a
			// fatal result for the missing input) proves it serves gRPC, not help.
			_, rpcErr := fnv1.NewFunctionRunnerServiceClient(conn).RunFunction(ctx, &fnv1.RunFunctionRequest{
				Meta: &fnv1.RequestMeta{Tag: "probe"},
			})
			cancel()
			_ = conn.Close()
			if rpcErr == nil {
				return
			}
		}
		if time.Now().After(deadline) {
			logs, _ := exec.Command(docker, "logs", id).CombinedOutput()
			assert.Failf(t, "image did not serve the function", "logs:\n%s", logs)
			return
		}
		time.Sleep(250 * time.Millisecond)
	}
}
