package cli

import (
	"context"
	"io"
	"net"
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/meigma/crossplane-cuefn/internal/test/common"
)

// portListening reports whether something accepts a TCP connection on the
// loopback port within the timeout.
func portListening(port int, timeout time.Duration) bool {
	conn, err := net.DialTimeout("tcp", "127.0.0.1:"+strconv.Itoa(port), timeout)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}

// serveInBackground launches serveFunction with insecure gRPC on a free port and
// waits until the gRPC listener accepts connections, proving the server reached
// sdk.Serve. The server runs until the test process exits (function-sdk-go's
// Serve has no graceful-stop handle); callers pass distinct ports per server.
func serveInBackground(t *testing.T, metricsAddress string) {
	t.Helper()

	grpcPort := common.FreePort(t)
	f := functionFlags{
		network:        "tcp",
		address:        "127.0.0.1:" + strconv.Itoa(grpcPort),
		insecure:       true,
		metricsAddress: metricsAddress,
	}

	errCh := make(chan error, 1)
	go func() { errCh <- serveFunction(f) }()

	require.Eventually(t, func() bool {
		select {
		case err := <-errCh:
			t.Fatalf("serveFunction returned early: %v", err)
			return false
		default:
		}
		return portListening(grpcPort, 200*time.Millisecond)
	}, 10*time.Second, 100*time.Millisecond, "gRPC server must start listening")
}

// TestServeFunction_MetricsEnabled proves --metrics-address binds a working
// Prometheus endpoint on the chosen (non-default) port, so the flag is honored
// and the endpoint is not hardcoded to :8080.
func TestServeFunction_MetricsEnabled(t *testing.T) {
	metricsPort := common.FreePort(t)
	serveInBackground(t, "127.0.0.1:"+strconv.Itoa(metricsPort))

	url := "http://127.0.0.1:" + strconv.Itoa(metricsPort) + "/metrics"
	var body string
	require.Eventually(t, func() bool {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return false
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return false
		}
		defer func() { _ = resp.Body.Close() }()
		if resp.StatusCode != http.StatusOK {
			return false
		}
		b, err := io.ReadAll(resp.Body)
		if err != nil {
			return false
		}
		body = string(b)
		return true
	}, 10*time.Second, 200*time.Millisecond, "metrics endpoint must answer 200")

	assert.Contains(t, body, "go_", "metrics output must carry Prometheus default collectors")
}

// TestServeFunction_MetricsDisabled proves --metrics-address "" starts no HTTP
// metrics listener. The function-sdk-go default endpoint is :8080; the subtest
// self-skips if :8080 is already occupied (so an unrelated local service cannot
// cause a false failure), then asserts that serving with "" leaves :8080 free.
func TestServeFunction_MetricsDisabled(t *testing.T) {
	if portListening(8080, 200*time.Millisecond) {
		t.Skip(":8080 already in use; cannot prove the metrics endpoint stays disabled")
	}

	serveInBackground(t, "")

	// Give any (buggy) metrics server a moment to bind, then confirm :8080 — the
	// SDK default the flag overrides — never came up.
	assert.Never(t, func() bool {
		return portListening(8080, 100*time.Millisecond)
	}, 2*time.Second, 250*time.Millisecond, "metrics endpoint must stay disabled with --metrics-address \"\"")
}
