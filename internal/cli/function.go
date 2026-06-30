package cli

import (
	"fmt"
	"os"

	sdk "github.com/crossplane/function-sdk-go"
	"github.com/spf13/cobra"

	"github.com/meigma/crossplane-cuefn/internal/function"
	"github.com/meigma/crossplane-cuefn/internal/render"
)

// functionFlags holds the serve flags for the function subcommand.
type functionFlags struct {
	network        string
	address        string
	tlsCertsDir    string
	insecure       bool
	cacheDir       string
	metricsAddress string
	debug          bool
}

// newFunctionCommand builds the `cuefn function` subcommand, which serves the
// cuefn composition function over gRPC. It wires an OCI-backed loader factory so
// the served function fetches modules from the registry configured via
// CUE_REGISTRY in the process environment.
func newFunctionCommand(_ Options) *cobra.Command {
	f := functionFlags{}

	cmd := &cobra.Command{
		Use:   "function",
		Short: "Serve the cuefn Crossplane composition function over gRPC",
		Long: "Serve the cuefn composition function. Crossplane connects over mTLS " +
			"by default; pass --insecure for local development with `crossplane render`.",
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(_ *cobra.Command, _ []string) error {
			return serveFunction(f)
		},
	}

	cmd.Flags().StringVar(&f.network, "network", "tcp", "network on which to listen for gRPC connections")
	cmd.Flags().StringVar(&f.address, "address", ":9443", "address at which to listen for gRPC connections")
	// Default the certs dir to Crossplane's TLS_SERVER_CERTS_DIR so the packaged
	// Function serves mTLS in-cluster with no extra arguments — Crossplane mounts
	// the server certs and sets this env var on the runtime container. An explicit
	// --tls-certs-dir still overrides it for local use.
	cmd.Flags().StringVar(&f.tlsCertsDir, "tls-certs-dir", os.Getenv("TLS_SERVER_CERTS_DIR"),
		"directory containing server certs (tls.key, tls.crt) and the client CA (ca.crt); "+
			"defaults to the TLS_SERVER_CERTS_DIR environment variable")
	cmd.Flags().BoolVar(&f.insecure, "insecure", false,
		"serve without mTLS credentials (development only; ignores --tls-certs-dir)")
	cmd.Flags().StringVar(&f.cacheDir, "cache-dir", "",
		"writable directory for the CUE module cache (defaults to CUE_CACHE_DIR, the OS cache, then a temp dir)")
	cmd.Flags().StringVar(&f.metricsAddress, "metrics-address", ":8080",
		"address for the Prometheus metrics endpoint; pass an empty string to disable it")
	cmd.Flags().BoolVarP(&f.debug, "debug", "d", false, "emit debug logs in addition to info logs")

	return cmd
}

// serveFunction builds the function and serves it with the configured options.
func serveFunction(f functionFlags) error {
	log, err := sdk.NewLogger(f.debug)
	if err != nil {
		return fmt.Errorf("cannot create logger: %w", err)
	}

	fn := function.New(function.OCILoaderFactory(render.OCIConfig{CacheDir: f.cacheDir}), log)

	// WithMetricsServer wires the Prometheus endpoint address. function-sdk-go's
	// Serve only starts the :8080 HTTP listener (and registers the metrics
	// interceptor) when MetricsAddress is non-empty, so --metrics-address ""
	// genuinely disables the endpoint rather than faking it.
	if err := sdk.Serve(fn,
		sdk.Listen(f.network, f.address),
		sdk.MTLSCertificates(f.tlsCertsDir),
		sdk.Insecure(f.insecure),
		sdk.WithMetricsServer(f.metricsAddress),
	); err != nil {
		return fmt.Errorf("cannot serve function: %w", err)
	}
	return nil
}
