//go:build !noxpkg

package cli

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/spf13/cobra"

	"github.com/meigma/crossplane-cuefn/internal/pkg"
	"github.com/meigma/crossplane-cuefn/internal/render"
	"github.com/meigma/crossplane-cuefn/internal/schema"
)

// publishFlags holds the flags for the publish subcommand.
type publishFlags struct {
	dir             string
	cacheDir        string
	pkgRef          string
	functionRef     string
	functionName    string
	functionVersion string
	name            string
	crossplane      string
	environmentRefs []string
	insecure        bool
}

const (
	// defaultFunctionRef is the cuefn Function package repo (Crossplane's
	// function-* convention; its own repo, distinct from the runtime image repo)
	// recorded in a published Configuration's dependsOn. It is where
	// `cuefn publish-function` ships the Function xpkg.
	defaultFunctionRef     = "ghcr.io/meigma/function-cuefn"
	defaultFunctionVersion = ">=v0.0.0"
)

// newPublishCommand builds the `cuefn publish` subcommand: the one-command
// generate -> package -> push flow. It loads a CUE module, generates its XRD
// (reusing internal/schema), resolves the module's live OCI manifest digest,
// builds a pipeline Composition that records the module ref and that digest (the
// author half of the runtime digest lock-step), assembles a Crossplane
// Configuration xpkg, and pushes it to the destination registry.
func newPublishCommand(options Options) *cobra.Command {
	f := publishFlags{}

	cmd := &cobra.Command{
		Use:   "publish <module-ref>",
		Short: "Build and push a Crossplane Configuration xpkg from a CUE module",
		Long: "Generate the XRD and a pipeline Composition from a CUE module, assemble a " +
			"Crossplane Configuration package (xpkg), and push it to an OCI registry. The " +
			"module's resolved manifest digest is recorded in the Composition so the runtime " +
			"verifies the module has not drifted. With --dir the XRD/Composition are built " +
			"from a local module directory, but the manifest digest is still resolved from " +
			"the registry the module was published to.",
		Args:          cobra.ExactArgs(1),
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runPublish(cmd.Context(), options, f, args[0])
		},
	}

	cmd.Flags().StringVar(&f.dir, "dir", "",
		"build the XRD/Composition from this local module directory instead of fetching it over OCI")
	cmd.Flags().StringVar(&f.cacheDir, "cache-dir", "",
		"directory for the CUE module cache and dependency downloads (overrides CUE_CACHE_DIR)")
	cmd.Flags().StringVar(&f.pkgRef, "package", "",
		"destination OCI reference for the Configuration package (required)")
	cmd.Flags().StringVar(&f.functionRef, "function-ref", defaultFunctionRef,
		"cuefn Function package OCI ref recorded in the Configuration's dependsOn")
	cmd.Flags().StringVar(&f.functionName, "function-name", "",
		"in-cluster Function resource name the Composition references (defaults to the function-ref's last path segment)")
	cmd.Flags().StringVar(&f.functionVersion, "function-version", defaultFunctionVersion,
		"semver constraint for the cuefn Function dependency")
	cmd.Flags().StringVar(&f.name, "name", "",
		"Configuration package metadata.name (defaults to <xrd-plural>-configuration)")
	cmd.Flags().StringVar(&f.crossplane, "crossplane-constraint", "",
		"optional semver constraint on the Crossplane version the package supports")
	cmd.Flags().StringArrayVar(&f.environmentRefs, "environment-config", nil,
		"name of an EnvironmentConfig the Composition merges into the pipeline context (repeatable); "+
			"each is referenced by name so its values reach the module under input.environment")
	cmd.Flags().BoolVar(&f.insecure, "insecure", false,
		"push over plain HTTP (development only; for a non-loopback throwaway registry)")
	_ = cmd.MarkFlagRequired("package")

	return cmd
}

// runPublish executes the generate -> package -> push flow for the module ref.
func runPublish(ctx context.Context, options Options, f publishFlags, ref string) error {
	if strings.TrimSpace(f.pkgRef) == "" {
		return errors.New("a destination --package reference is required")
	}

	// Load the module (local or OCI) and generate the typed XRD; the typed XRD
	// (not the YAML wrapper) supplies the Composition's compositeTypeRef.
	loader, err := moduleLoader(f.dir, f.cacheDir)
	if err != nil {
		return err
	}
	module, cleanup, err := render.LoadModule(ctx, loader, ref)
	if err != nil {
		return err
	}
	defer cleanup()

	xrd, err := schema.GenerateXRD(module)
	if err != nil {
		return fmt.Errorf("cannot generate XRD for module %q: %w", ref, err)
	}

	// Resolve the live manifest digest from the registry. This is always an OCI
	// operation: even with --dir, publish records the real published digest so the
	// runtime lock-step is meaningful.
	digest, err := resolveModuleDigest(ctx, ref, f.cacheDir)
	if err != nil {
		return err
	}

	comp, err := pkg.GenerateComposition(xrd, pkg.CompositionInput{
		Module:                ref,
		ExpectedDigest:        digest,
		FunctionName:          functionName(f),
		EnvironmentConfigRefs: f.environmentRefs,
	})
	if err != nil {
		return fmt.Errorf("cannot build composition for module %q: %w", ref, err)
	}

	meta, err := pkg.GenerateConfigurationMeta(pkg.ConfigurationMeta{
		Name:                 configurationName(f, xrd.Spec.Names.Plural),
		CrossplaneConstraint: f.crossplane,
		FunctionPackage:      f.functionRef,
		FunctionVersion:      f.functionVersion,
	})
	if err != nil {
		return fmt.Errorf("cannot build configuration metadata: %w", err)
	}

	img, err := pkg.BuildConfigurationImage(pkg.Configuration{Meta: meta, XRD: xrd, Composition: comp})
	if err != nil {
		return fmt.Errorf("cannot build configuration image: %w", err)
	}

	dst, err := pkg.Push(ctx, f.pkgRef, img, f.insecure, remotePushOptions()...)
	if err != nil {
		return err
	}

	return printLine(options.Out, "pushed "+dst.String())
}

// resolveModuleDigest builds an OCI loader honoring CUE_REGISTRY and resolves
// ref's current manifest digest. It is the publish-time half of the digest
// lock-step and reuses the same loader configuration the function serves with;
// cacheDir overrides CUE_CACHE_DIR to match the module-load path.
func resolveModuleDigest(ctx context.Context, ref, cacheDir string) (string, error) {
	loader, err := render.NewOCILoader(render.OCIConfig{CacheDir: cacheDir})
	if err != nil {
		return "", fmt.Errorf("cannot build OCI loader: %w", err)
	}
	digest, err := loader.ResolveDigest(ctx, ref)
	if err != nil {
		return "", err
	}
	return digest, nil
}

// remotePushOptions builds the go-containerregistry push options: credentials
// come from the standard Docker keychain so an authenticated registry works
// without extra wiring (a throwaway insecure registry resolves to anonymous).
func remotePushOptions() []remote.Option {
	return []remote.Option{remote.WithAuthFromKeychain(authn.DefaultKeychain)}
}

// functionName resolves the Composition's functionRef.name: the explicit flag,
// else the last path segment of the function package ref.
func functionName(f publishFlags) string {
	if strings.TrimSpace(f.functionName) != "" {
		return f.functionName
	}
	return lastPathSegment(f.functionRef)
}

// configurationName resolves the Configuration metadata.name: the explicit flag,
// else "<plural>-configuration".
func configurationName(f publishFlags, plural string) string {
	if strings.TrimSpace(f.name) != "" {
		return f.name
	}
	if plural == "" {
		return "configuration"
	}
	return plural + "-configuration"
}

// lastPathSegment returns the final path segment of an OCI ref, stripping any tag
// or digest, e.g. "xpkg.meigma.io/cuefn:v0" -> "cuefn".
func lastPathSegment(ref string) string {
	s := ref
	if i := strings.LastIndex(s, "/"); i >= 0 {
		s = s[i+1:]
	}
	if i := strings.IndexAny(s, ":@"); i >= 0 {
		s = s[:i]
	}
	// An empty result is fine: GenerateComposition defaults the functionRef name
	// when FunctionName is blank.
	return s
}
