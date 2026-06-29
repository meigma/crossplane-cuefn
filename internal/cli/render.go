package cli

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"sigs.k8s.io/yaml"

	"github.com/meigma/crossplane-cuefn/internal/render"
)

// renderFlags holds the flags for the render subcommand.
type renderFlags struct {
	dir      string
	cacheDir string
	xr       string
	env      string
}

// newRenderCommand builds the `cuefn render` subcommand: a cluster-free,
// crossplane-CLI-free local evaluation of a module against an XR (and optional
// environment) that prints the rendered resources and status to stdout. It
// reuses the same engine and loaders the served function uses, so what an author
// sees locally matches what Crossplane renders.
func newRenderCommand(options Options) *cobra.Command {
	f := renderFlags{}

	cmd := &cobra.Command{
		Use:   "render <module-ref>",
		Short: "Render a CUE module against an XR locally and print the result",
		Long: "Evaluate a CUE module against an observed XR and optional environment, " +
			"printing the rendered composed resources and composite status as YAML. " +
			"With --dir the module is served from a local directory offline; otherwise " +
			"it is fetched from the OCI registry configured via CUE_REGISTRY.",
		Args:          cobra.ExactArgs(1),
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runRender(cmd.Context(), options, f, args[0])
		},
	}

	cmd.Flags().StringVar(&f.dir, "dir", "",
		"serve the module from this local directory instead of fetching it over OCI")
	cmd.Flags().StringVar(&f.cacheDir, "cache-dir", "",
		"directory for the CUE module cache and dependency downloads (overrides CUE_CACHE_DIR)")
	cmd.Flags().StringVar(&f.xr, "xr", "", "path to the observed XR YAML (required)")
	cmd.Flags().StringVar(&f.env, "env", "", "path to a merged environment YAML (optional)")
	_ = cmd.MarkFlagRequired("xr")

	return cmd
}

// runRender reads the inputs, selects a loader, renders, and prints the result.
func runRender(ctx context.Context, options Options, f renderFlags, ref string) error {
	inputs, err := readRenderInputs(f)
	if err != nil {
		return err
	}

	loader, err := renderLoader(f)
	if err != nil {
		return err
	}

	result, err := render.New(loader).Render(ctx, ref, inputs)
	if err != nil {
		return fmt.Errorf("cannot render module %q: %w", ref, err)
	}

	return printRenderResult(options, result)
}

// renderLoader returns a dependency-aware LocalLoader when --dir is set and an
// OCILoader (honoring CUE_REGISTRY in the process environment) otherwise.
func renderLoader(f renderFlags) (render.ModuleLoader, error) {
	return moduleLoader(f.dir, f.cacheDir)
}

// readRenderInputs reads the XR (required) and environment (optional) YAML files
// into the curated engine inputs.
func readRenderInputs(f renderFlags) (render.Inputs, error) {
	xr, err := readYAMLObject(f.xr)
	if err != nil {
		return render.Inputs{}, fmt.Errorf("cannot read XR %q: %w", f.xr, err)
	}

	spec, _ := xr["spec"].(map[string]any)
	meta, _ := xr["metadata"].(map[string]any)
	name, _ := meta["name"].(string)
	namespace, _ := meta["namespace"].(string)

	inputs := render.Inputs{
		Spec:     spec,
		Metadata: render.Metadata{Name: name, Namespace: namespace},
	}

	if f.env != "" {
		env, err := readYAMLObject(f.env)
		if err != nil {
			return render.Inputs{}, fmt.Errorf("cannot read environment %q: %w", f.env, err)
		}
		inputs.Environment = env
	}

	return inputs, nil
}

// readYAMLObject reads a YAML file into a map.
func readYAMLObject(path string) (map[string]any, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var out map[string]any
	if err := yaml.Unmarshal(data, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// renderResource is the printed shape of one rendered resource: its readiness
// and the rendered object.
type renderResource struct {
	Ready  string         `json:"ready,omitempty"`
	Object map[string]any `json:"object"`
}

// renderOutput is the printed shape of a render: the author-keyed resources and
// the optional composite status.
type renderOutput struct {
	Resources map[string]renderResource `json:"resources"`
	Status    map[string]any            `json:"status,omitempty"`
}

// printRenderResult marshals the render result to deterministic YAML and writes
// it to the command's output.
func printRenderResult(options Options, result render.Result) error {
	out := renderOutput{Resources: make(map[string]renderResource, len(result.Resources)), Status: result.Status}
	for name, r := range result.Resources {
		out.Resources[name] = renderResource{Ready: string(r.Ready), Object: r.Object}
	}

	data, err := yaml.Marshal(out)
	if err != nil {
		return fmt.Errorf("cannot marshal render result: %w", err)
	}
	if _, err := options.Out.Write(data); err != nil {
		return fmt.Errorf("cannot write render result: %w", err)
	}
	return nil
}
