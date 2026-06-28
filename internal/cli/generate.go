package cli

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/meigma/crossplane-cuefn/internal/render"
	"github.com/meigma/crossplane-cuefn/internal/schema"
)

// generateFlags holds the flags for the generate subcommand.
type generateFlags struct {
	dir    string
	output string
}

// newGenerateCommand builds the `cuefn generate` subcommand: it loads a CUE
// module (from a local directory with --dir, otherwise over OCI) and emits the
// structural Crossplane v2 XRD generated from its #API/#Spec/#Status to stdout or
// a file. It reuses the same loaders the render command uses, so the module is
// built from one load path.
func newGenerateCommand(options Options) *cobra.Command {
	f := generateFlags{}

	cmd := &cobra.Command{
		Use:   "generate <module-ref>",
		Short: "Generate a structural XRD from a CUE module's #API/#Spec/#Status",
		Long: "Load a CUE module and emit the Crossplane v2 CompositeResourceDefinition " +
			"generated from its #API envelope and #Spec/#Status schemas as YAML. With " +
			"--dir the module is served from a local directory offline; otherwise it is " +
			"fetched from the OCI registry configured via CUE_REGISTRY.",
		Args:          cobra.ExactArgs(1),
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runGenerate(cmd.Context(), options, f, args[0])
		},
	}

	cmd.Flags().StringVar(&f.dir, "dir", "",
		"serve the module from this local directory (offline) instead of fetching it over OCI")
	cmd.Flags().StringVarP(&f.output, "output", "o", "",
		"write the generated XRD to this file instead of stdout")

	return cmd
}

// runGenerate selects a loader, builds the module value, generates the XRD YAML,
// and writes it to stdout or the --output file.
func runGenerate(ctx context.Context, options Options, f generateFlags, ref string) error {
	loader, err := moduleLoader(f.dir)
	if err != nil {
		return err
	}

	module, cleanup, err := render.LoadModule(ctx, loader, ref)
	if err != nil {
		return err
	}
	defer cleanup()

	out, err := schema.GenerateXRDYAML(module)
	if err != nil {
		return fmt.Errorf("cannot generate XRD for module %q: %w", ref, err)
	}

	if f.output != "" {
		if err := os.WriteFile(f.output, out, 0o644); err != nil { //nolint:gosec // an XRD is non-secret.
			return fmt.Errorf("cannot write XRD to %q: %w", f.output, err)
		}
		return nil
	}

	if _, err := options.Out.Write(out); err != nil {
		return fmt.Errorf("cannot write XRD: %w", err)
	}
	return nil
}

// moduleLoader returns a LocalLoader when dir is set and an OCILoader (honoring
// CUE_REGISTRY) otherwise. It mirrors the render command's loader selection.
func moduleLoader(dir string) (render.ModuleLoader, error) {
	if dir != "" {
		return render.LocalLoader{Dir: dir}, nil
	}
	loader, err := render.NewOCILoader(render.OCIConfig{})
	if err != nil {
		return nil, fmt.Errorf("cannot build OCI loader: %w", err)
	}
	return loader, nil
}
