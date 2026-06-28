//go:build !noxpkg

package cli

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/tarball"
	"github.com/spf13/cobra"

	"github.com/meigma/crossplane-cuefn/internal/pkg"
)

// publishFunctionFlags holds the flags for the publish-function subcommand.
type publishFunctionFlags struct {
	runtimeImages []string
	pkgRef        string
	output        string
	name          string
	crossplane    string
	capabilities  []string
	insecure      bool
}

const (
	defaultFunctionName = "function-cuefn"
	publishFunctionUse  = "publish-function"
)

// newPublishFunctionCommand builds the `cuefn publish-function` subcommand: it
// assembles the Function xpkg (the meta.pkg.crossplane.io Function plus the
// embedded Input CRD) over one or more apko-built runtime image bases and pushes
// it to an OCI registry. A single --runtime-image produces a single-arch package
// image; several produce a multi-arch index (a Function package image IS the
// runtime image, so a real install needs a per-node-arch index). Signing is left
// to cosign at the edge (keyless in CI, a local key for proofs).
func newPublishFunctionCommand(options Options) *cobra.Command {
	f := publishFunctionFlags{}

	cmd := &cobra.Command{
		Use:   publishFunctionUse,
		Short: "Build and push the cuefn Crossplane Function xpkg over a runtime image base",
		Long: "Assemble a Crossplane Function package (xpkg) — the package metadata plus the " +
			"embedded Input CRD — over the apko-built cuefn runtime image, and push it to an OCI " +
			"registry. The package image is the runtime image plus the package layer, so it both " +
			"installs as a Function and serves `cuefn function`. Pass --runtime-image once for a " +
			"single-arch image or repeatedly for a multi-arch index.",
		Args:          cobra.NoArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runPublishFunction(cmd.Context(), options, f)
		},
	}

	cmd.Flags().StringArrayVar(&f.runtimeImages, "runtime-image", nil,
		"runtime image base: a local OCI/docker tarball path or a registry ref (repeat for a multi-arch index)")
	cmd.Flags().StringVar(&f.pkgRef, "package", "",
		"destination OCI reference for the Function package (required unless --output is set)")
	cmd.Flags().StringVar(&f.output, "output", "",
		"write the assembled single-arch Function package to this local .xpkg file instead of pushing (e.g. for `crossplane xpkg extract --from-xpkg`)")
	cmd.Flags().StringVar(&f.name, "name", defaultFunctionName,
		"Function package metadata.name")
	cmd.Flags().StringVar(&f.crossplane, "crossplane-constraint", "",
		"optional semver constraint on the Crossplane version the package supports")
	cmd.Flags().StringArrayVar(&f.capabilities, "capabilities", nil,
		"optional package capability strings (repeatable)")
	cmd.Flags().BoolVar(&f.insecure, "insecure", false,
		"push/pull over plain HTTP (development only; for a non-loopback throwaway registry)")
	_ = cmd.MarkFlagRequired("runtime-image")

	return cmd
}

// runPublishFunction validates the flags, builds the Function, and dispatches to
// the local-write, single-arch, or multi-arch delivery path.
func runPublishFunction(ctx context.Context, options Options, f publishFunctionFlags) error {
	if strings.TrimSpace(f.pkgRef) == "" && strings.TrimSpace(f.output) == "" {
		return errors.New("either a destination --package reference or an --output path is required")
	}
	if len(f.runtimeImages) == 0 {
		return errors.New("at least one --runtime-image base is required")
	}

	meta, err := pkg.GenerateFunctionMeta(pkg.FunctionMeta{
		Name:                 f.name,
		CrossplaneConstraint: f.crossplane,
		Capabilities:         f.capabilities,
	})
	if err != nil {
		return err
	}
	fn, err := pkg.DefaultFunction(meta)
	if err != nil {
		return err
	}

	switch {
	case strings.TrimSpace(f.output) != "":
		return writeFunctionXpkg(ctx, options, f, fn)
	case len(f.runtimeImages) == 1:
		return pushFunctionImage(ctx, options, f, fn)
	default:
		return pushFunctionIndex(ctx, options, f, fn)
	}
}

// writeFunctionXpkg assembles the single-arch package over the one runtime base
// and writes it to a local .xpkg (no registry), suitable for `crossplane xpkg
// extract --from-xpkg` in a dry run.
func writeFunctionXpkg(ctx context.Context, options Options, f publishFunctionFlags, fn pkg.Function) error {
	if len(f.runtimeImages) != 1 {
		return errors.New("--output writes a single-arch package; pass exactly one --runtime-image")
	}
	img, err := assembleFunctionImage(ctx, f, f.runtimeImages[0], fn)
	if err != nil {
		return err
	}
	if err := writeLocalXpkg(f.output, f.name, img); err != nil {
		return err
	}
	return printLine(options.Out, "wrote "+f.output)
}

// pushFunctionImage assembles the single-arch package and pushes it.
func pushFunctionImage(ctx context.Context, options Options, f publishFunctionFlags, fn pkg.Function) error {
	img, err := assembleFunctionImage(ctx, f, f.runtimeImages[0], fn)
	if err != nil {
		return err
	}
	dst, err := pkg.Push(ctx, f.pkgRef, img, f.insecure, remotePushOptions()...)
	if err != nil {
		return err
	}
	return printLine(options.Out, "pushed "+dst.String())
}

// pushFunctionIndex assembles a multi-arch package index over every runtime base
// and pushes it (the package image is the runtime, so it must run on every arch).
func pushFunctionIndex(ctx context.Context, options Options, f publishFunctionFlags, fn pkg.Function) error {
	bases := make([]v1.Image, 0, len(f.runtimeImages))
	for _, src := range f.runtimeImages {
		base, err := loadRuntimeBase(ctx, src, f.insecure)
		if err != nil {
			return err
		}
		bases = append(bases, base)
	}
	idx, err := pkg.BuildFunctionIndex(bases, fn)
	if err != nil {
		return err
	}
	dst, err := pkg.PushIndex(ctx, f.pkgRef, idx, f.insecure, remotePushOptions()...)
	if err != nil {
		return err
	}
	return printLine(options.Out, "pushed "+dst.String())
}

// assembleFunctionImage loads the runtime base at src and assembles the Function
// package image over it.
func assembleFunctionImage(ctx context.Context, f publishFunctionFlags, src string, fn pkg.Function) (v1.Image, error) {
	base, err := loadRuntimeBase(ctx, src, f.insecure)
	if err != nil {
		return nil, err
	}
	return pkg.BuildFunctionImage(base, fn)
}

// writeLocalXpkg writes img to path as a local .xpkg tarball, tagged with the
// package name so `crossplane xpkg extract --from-xpkg` can read it.
func writeLocalXpkg(path, fnName string, img v1.Image) error {
	tag, err := name.NewTag("local/" + fnName + ":packaged")
	if err != nil {
		return fmt.Errorf("cannot build local package tag: %w", err)
	}
	if err := tarball.WriteToFile(path, tag, img); err != nil {
		return fmt.Errorf("cannot write Function package to %q: %w", path, err)
	}
	return nil
}
