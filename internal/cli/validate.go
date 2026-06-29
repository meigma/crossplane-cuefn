package cli

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/meigma/crossplane-cuefn/internal/render"
	"github.com/meigma/crossplane-cuefn/internal/schema"
)

// validateFlags holds the flags for the validate subcommand.
type validateFlags struct {
	module   string
	dir      string
	cacheDir string
}

// newValidateCommand builds the `cuefn validate` subcommand: it checks a
// populated XR's spec against a target module's #Spec via the CUE Go API,
// applying #Spec defaults and reporting violations (out-of-bounds, wrong enum,
// missing required, pattern) with the offending field path. A violation returns
// the error so cobra exits non-zero; a valid (or defaulted-but-omitted) XR
// returns nil and exits zero.
func newValidateCommand(options Options) *cobra.Command {
	f := validateFlags{}

	cmd := &cobra.Command{
		Use:   "validate <xr>",
		Short: "Validate a populated XR against a module's #Spec",
		Long: "Read an XR YAML file and check its spec against the target module's #Spec " +
			"using the same CUE evaluation the runtime engine uses. With --dir the module " +
			"is served from a local directory; otherwise it is fetched from the OCI " +
			"registry configured via CUE_REGISTRY (the central registry by default). Exits non-zero on the first violation.",
		Args:          cobra.ExactArgs(1),
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runValidate(cmd.Context(), options, f, args[0])
		},
	}

	cmd.Flags().StringVar(&f.module, "module", "",
		"module reference (path@version) to validate against when fetching over OCI")
	cmd.Flags().StringVar(&f.dir, "dir", "",
		"serve the module from this local directory instead of fetching it over OCI")
	cmd.Flags().StringVar(&f.cacheDir, "cache-dir", "",
		"directory for the CUE module cache and dependency downloads (overrides CUE_CACHE_DIR)")

	return cmd
}

// runValidate reads the XR, loads the module, and validates the XR spec. On a
// valid (or defaulted-but-omitted) XR it prints a one-line confirmation to the
// diagnostics stream and returns nil (exit zero); a violation is returned so
// cobra exits non-zero with the field-located message.
func runValidate(ctx context.Context, options Options, f validateFlags, xrPath string) error {
	xr, err := readYAMLObject(xrPath)
	if err != nil {
		return fmt.Errorf("cannot read XR %q: %w", xrPath, err)
	}
	spec, _ := xr["spec"].(map[string]any)

	loader, err := moduleLoader(f.dir, f.cacheDir)
	if err != nil {
		return err
	}

	module, cleanup, err := render.LoadModule(ctx, loader, f.module)
	if err != nil {
		return err
	}
	defer cleanup()

	if err := schema.Validate(module, spec); err != nil {
		return err
	}

	if _, err := fmt.Fprintf(options.Err, "%s: valid\n", xrPath); err != nil {
		return fmt.Errorf("cannot write confirmation: %w", err)
	}
	return nil
}
