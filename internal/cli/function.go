package cli

import (
	"fmt"

	sdk "github.com/crossplane/function-sdk-go"
	"github.com/spf13/cobra"

	"github.com/meigma/crossplane-cuefn/internal/function"
	"github.com/meigma/crossplane-cuefn/internal/render"
)

// functionFlags holds the serve flags for the function subcommand.
type functionFlags struct {
	network     string
	address     string
	tlsCertsDir string
	insecure    bool
	cacheDir    string
	debug       bool
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
	cmd.Flags().StringVar(&f.tlsCertsDir, "tls-certs-dir", "",
		"directory containing server certs (tls.key, tls.crt) and the client CA (ca.crt)")
	cmd.Flags().BoolVar(&f.insecure, "insecure", false,
		"serve without mTLS credentials (development only; ignores --tls-certs-dir)")
	cmd.Flags().StringVar(&f.cacheDir, "cache-dir", "",
		"writable directory for the CUE module cache (defaults to CUE_CACHE_DIR or the OS cache)")
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

	if err := sdk.Serve(fn,
		sdk.Listen(f.network, f.address),
		sdk.MTLSCertificates(f.tlsCertsDir),
		sdk.Insecure(f.insecure),
	); err != nil {
		return fmt.Errorf("cannot serve function: %w", err)
	}
	return nil
}
