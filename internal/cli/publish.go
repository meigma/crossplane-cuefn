//go:build !noxpkg

package cli

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"

	xv2 "github.com/crossplane/crossplane/apis/v2/apiextensions/v2"
	"github.com/google/go-containerregistry/pkg/authn"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/spf13/cobra"

	"github.com/meigma/crossplane-cuefn/internal/modulepublish"
	"github.com/meigma/crossplane-cuefn/internal/pkg"
	"github.com/meigma/crossplane-cuefn/internal/render"
	"github.com/meigma/crossplane-cuefn/internal/schema"
)

// publishFlags holds the flags for the publish subcommand.
type publishFlags struct {
	dir                string
	cacheDir           string
	pkgRef             string
	functionRef        string
	functionName       string
	functionVersion    string
	name               string
	crossplane         string
	environmentRefs    []string
	metadata           []string
	envFunctionRef     string
	envFunctionVersion string
	publishModule      bool
	insecure           bool
}

const (
	// defaultFunctionRef is the cuefn Function package repo (Crossplane's
	// function-* convention; its own repo, distinct from the runtime image repo)
	// recorded in a published Configuration's dependsOn. It is where
	// `cuefn publish-function` ships the Function xpkg.
	defaultFunctionRef     = "ghcr.io/meigma/function-cuefn"
	defaultFunctionVersion = ">=v0.0.0"

	// defaultEnvConfigFunctionRef / Version are the well-known crossplane-contrib
	// function-environment-configs package, recorded in dependsOn when a published
	// Configuration uses EnvironmentConfigs (--environment-config).
	defaultEnvConfigFunctionRef     = "xpkg.crossplane.io/crossplane-contrib/function-environment-configs"
	defaultEnvConfigFunctionVersion = ">=v0.7.2"
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
			"verifies the module has not drifted. With --publish-module, the local --dir module " +
			"is prepared and published first and its exact digest is recorded. Otherwise, with " +
			"--dir the XRD/Composition are local but the manifest digest is resolved from the " +
			"registry the module was published to.",
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
		"in-cluster Function resource name the Composition references "+
			"(defaults to the name Crossplane derives for the function-ref dependency)")
	cmd.Flags().StringVar(&f.functionVersion, "function-version", defaultFunctionVersion,
		"semver constraint for the cuefn Function dependency")
	cmd.Flags().StringVar(&f.envFunctionRef, "environment-config-function-ref", defaultEnvConfigFunctionRef,
		"function-environment-configs package recorded in dependsOn when --environment-config is used")
	cmd.Flags().StringVar(&f.envFunctionVersion, "environment-config-function-version", defaultEnvConfigFunctionVersion,
		"semver constraint for the function-environment-configs dependency")
	cmd.Flags().StringVar(&f.name, "name", "",
		"Configuration package metadata.name (defaults to <xrd-plural>-configuration)")
	cmd.Flags().StringVar(&f.crossplane, "crossplane-constraint", "",
		"optional semver constraint on the Crossplane version the package supports")
	cmd.Flags().StringArrayVar(&f.environmentRefs, "environment-config", nil,
		"name of an EnvironmentConfig the Composition merges into the pipeline context (repeatable); "+
			"each is referenced by name so its values reach the module under input.environment")
	cmd.Flags().StringArrayVar(&f.metadata, "metadata", nil,
		"OCI metadata key=value applied as Configuration labels and, with --publish-module, module annotations (repeatable)")
	cmd.Flags().BoolVar(&f.publishModule, "publish-module", false,
		"publish the local --dir module version before pushing the Configuration")
	cmd.Flags().BoolVar(&f.insecure, "insecure", false,
		"push over plain HTTP (development only; for a non-loopback throwaway registry)")
	_ = cmd.MarkFlagRequired("package")

	return cmd
}

// runPublish executes the generate -> package -> push flow for the module ref.
func runPublish(ctx context.Context, options Options, f publishFlags, ref string) error {
	metadata, moduleArtifact, err := preparePublishInputs(ctx, f, ref)
	if err != nil {
		return err
	}
	xrd, cleanup, err := loadPublishXRD(ctx, f, ref)
	if err != nil {
		return err
	}
	defer cleanup()
	digest, err := publishModuleDigest(ctx, moduleArtifact, ref, f.cacheDir)
	if err != nil {
		return err
	}
	img, err := buildPublishImage(f, ref, digest, metadata, xrd)
	if err != nil {
		return err
	}
	moduleResult, err := publishPreparedModule(ctx, moduleArtifact)
	if err != nil {
		return err
	}

	dst, err := pkg.Push(ctx, f.pkgRef, img, f.insecure, remotePushOptions()...)
	if err != nil {
		return configurationPushError(err, f.pkgRef, moduleArtifact, moduleResult)
	}
	if err := printModuleResult(options, moduleArtifact, moduleResult); err != nil {
		return err
	}
	return printLine(options.Out, "pushed "+dst.String())
}

func preparePublishInputs(
	ctx context.Context,
	f publishFlags,
	ref string,
) (map[string]string, *modulepublish.Artifact, error) {
	if strings.TrimSpace(f.pkgRef) == "" {
		return nil, nil, errors.New("a destination --package reference is required")
	}
	metadata, err := parseMetadata(f.metadata)
	if err != nil {
		return nil, nil, err
	}
	if !f.publishModule {
		return metadata, nil, nil
	}
	if strings.TrimSpace(f.dir) == "" {
		return nil, nil, errors.New("--publish-module requires a local module --dir")
	}
	if validationErr := pkg.ValidateDestination(f.pkgRef, f.insecure); validationErr != nil {
		return nil, nil, validationErr
	}
	artifact, err := modulepublish.Prepare(ctx, ref, f.dir, metadata)
	if err != nil {
		return nil, nil, err
	}
	return metadata, artifact, nil
}

func loadPublishXRD(
	ctx context.Context,
	f publishFlags,
	ref string,
) (*xv2.CompositeResourceDefinition, func(), error) {
	loader, err := moduleLoader(f.dir, f.cacheDir)
	if err != nil {
		return nil, nil, err
	}
	loaded, cleanup, err := render.LoadModule(ctx, loader, ref)
	if err != nil {
		return nil, nil, err
	}
	xrd, err := schema.GenerateXRD(loaded)
	if err != nil {
		cleanup()
		return nil, nil, fmt.Errorf("cannot generate XRD for module %q: %w", ref, err)
	}
	return xrd, cleanup, nil
}

func publishModuleDigest(
	ctx context.Context,
	artifact *modulepublish.Artifact,
	ref string,
	cacheDir string,
) (string, error) {
	if artifact != nil {
		return artifact.Digest(), nil
	}
	return resolveModuleDigest(ctx, ref, cacheDir)
}

func buildPublishImage(
	f publishFlags,
	ref string,
	digest string,
	metadata map[string]string,
	xrd *xv2.CompositeResourceDefinition,
) (v1.Image, error) {
	fnName, err := functionName(f)
	if err != nil {
		return nil, err
	}
	compInput := pkg.CompositionInput{
		Module:                ref,
		ExpectedDigest:        digest,
		FunctionName:          fnName,
		EnvironmentConfigRefs: f.environmentRefs,
	}
	metaInput := pkg.ConfigurationMeta{
		Name:                 configurationName(f, xrd.Spec.Names.Plural),
		CrossplaneConstraint: f.crossplane,
		FunctionPackage:      f.functionRef,
		FunctionVersion:      f.functionVersion,
	}
	if hasEnvironmentConfigs(f.environmentRefs) {
		envName, envErr := pkg.DerivedFunctionName(f.envFunctionRef)
		if envErr != nil {
			return nil, envErr
		}
		compInput.EnvironmentConfigFunctionName = envName
		metaInput.EnvironmentConfigFunctionPackage = f.envFunctionRef
		metaInput.EnvironmentConfigFunctionVersion = f.envFunctionVersion
	}
	comp, err := pkg.GenerateComposition(xrd, compInput)
	if err != nil {
		return nil, fmt.Errorf("cannot build composition for module %q: %w", ref, err)
	}
	meta, err := pkg.GenerateConfigurationMeta(metaInput)
	if err != nil {
		return nil, fmt.Errorf("cannot build configuration metadata: %w", err)
	}
	var imageOptions []pkg.ConfigurationImageOption
	if len(metadata) > 0 {
		imageOptions = append(imageOptions, pkg.WithConfigurationLabels(metadata))
	}
	img, err := pkg.BuildConfigurationImage(
		pkg.Configuration{Meta: meta, XRD: xrd, Composition: comp},
		imageOptions...,
	)
	if err != nil {
		return nil, fmt.Errorf("cannot build configuration image: %w", err)
	}
	return img, nil
}

func publishPreparedModule(
	ctx context.Context,
	artifact *modulepublish.Artifact,
) (modulepublish.PublishResult, error) {
	if artifact == nil {
		return modulepublish.PublishResult{}, nil
	}
	resolver, err := modulepublish.NewResolver(nil)
	if err != nil {
		return modulepublish.PublishResult{}, err
	}
	return artifact.Publish(ctx, resolver)
}

func configurationPushError(
	pushErr error,
	pkgRef string,
	artifact *modulepublish.Artifact,
	result modulepublish.PublishResult,
) error {
	if artifact == nil {
		return pushErr
	}
	return fmt.Errorf(
		"module %s@%s is published but Configuration %q failed: %w; retrying the same command is safe",
		result.Module,
		result.Digest,
		pkgRef,
		pushErr,
	)
}

func printModuleResult(
	options Options,
	artifact *modulepublish.Artifact,
	result modulepublish.PublishResult,
) error {
	if artifact == nil {
		return nil
	}
	action := "published"
	if result.Reused {
		action = "reused"
	}
	return printLine(options.Out, fmt.Sprintf(
		"%s module %s@%s",
		action,
		result.Module,
		result.Digest,
	))
}

func parseMetadata(values []string) (map[string]string, error) {
	metadata := make(map[string]string, len(values))
	for _, pair := range values {
		key, value, found := strings.Cut(pair, "=")
		if !found {
			return nil, fmt.Errorf("invalid metadata %q: expected key=value", pair)
		}
		if key == "" {
			return nil, fmt.Errorf("invalid metadata %q: key cannot be empty", pair)
		}
		if value == "" {
			return nil, fmt.Errorf("invalid metadata %q: value cannot be empty", pair)
		}
		if _, exists := metadata[key]; exists {
			return nil, fmt.Errorf("duplicate metadata key %q", key)
		}
		if key == "org.opencontainers.image.source" {
			if err := validateSourceMetadata(value); err != nil {
				return nil, err
			}
		}
		metadata[key] = value
	}
	return metadata, nil
}

func validateSourceMetadata(value string) error {
	parsed, err := url.Parse(value)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return fmt.Errorf(
			"metadata org.opencontainers.image.source must be an absolute HTTP(S) URL, got %q",
			value,
		)
	}
	return nil
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

// functionName resolves the Composition's cuefn functionRef.name: the explicit
// --function-name flag, else the name Crossplane derives for the auto-installed
// function dependency (pkg.DerivedFunctionName of --function-ref). The derived
// name is what the Configuration's dependsOn installs, so the pipeline step binds
// to it from a single Configuration install with no hand-installed Function.
func functionName(f publishFlags) (string, error) {
	if strings.TrimSpace(f.functionName) != "" {
		return f.functionName, nil
	}
	return pkg.DerivedFunctionName(f.functionRef)
}

// hasEnvironmentConfigs reports whether any non-blank EnvironmentConfig ref was
// requested.
func hasEnvironmentConfigs(refs []string) bool {
	for _, r := range refs {
		if strings.TrimSpace(r) != "" {
			return true
		}
	}
	return false
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
