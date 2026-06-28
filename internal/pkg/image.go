package pkg

import (
	"bytes"
	"fmt"

	xpkg "github.com/crossplane/crossplane-runtime/v2/pkg/xpkg"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
)

// BuildXpkgImage tars packageYAML (a marshaled multi-doc YAML stream) into the
// package layer of base and returns the assembled xpkg image. The layer is named
// package.yaml and carries the io.crossplane.xpkg=base annotation Crossplane uses
// to locate a package's contents; base's existing config labels and layers are
// preserved.
//
// base is empty.Image for a Configuration (no runtime); for an embed-runtime
// Function xpkg it is the runtime image, so the package layer rides on top of the
// runtime filesystem. This single shape is the spike's shared assembler for both
// the Configuration package (this phase) and the Function package (P6).
func BuildXpkgImage(base v1.Image, packageYAML []byte) (v1.Image, error) {
	cfgFile, err := base.ConfigFile()
	if err != nil {
		return nil, fmt.Errorf("cannot read base image config: %w", err)
	}

	cfg := cfgFile.Config
	if cfg.Labels == nil {
		cfg.Labels = map[string]string{}
	}

	// Layer records the package annotation as a config label keyed by the layer's
	// digest; AnnotateLayers (below) later promotes it to a real OCI layer
	// annotation. This two-step dance matches crossplane's own builder, because
	// the OCI tarball form cannot carry layer annotations directly.
	layer, err := xpkg.Layer(
		bytes.NewReader(packageYAML),
		xpkg.StreamFile,
		xpkg.PackageAnnotation,
		int64(len(packageYAML)),
		xpkg.StreamFileMode,
		&cfg,
	)
	if err != nil {
		return nil, fmt.Errorf("cannot build package layer: %w", err)
	}

	img, err := mutate.AppendLayers(base, layer)
	if err != nil {
		return nil, fmt.Errorf("cannot append package layer: %w", err)
	}

	img, err = mutate.Config(img, cfg)
	if err != nil {
		return nil, fmt.Errorf("cannot set image config: %w", err)
	}

	img, err = xpkg.AnnotateLayers(img)
	if err != nil {
		return nil, fmt.Errorf("cannot annotate package layers: %w", err)
	}

	return img, nil
}
