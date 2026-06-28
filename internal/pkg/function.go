package pkg

import (
	"bytes"
	_ "embed"
	"errors"
	"fmt"

	metav1cp "github.com/crossplane/crossplane/apis/v2/pkg/meta/v1"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/empty"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/google/go-containerregistry/pkg/v1/types"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"sigs.k8s.io/yaml"
)

// inputCRDYAML is the controller-gen-emitted CustomResourceDefinition for the
// cuefn Input type (input/v1beta1). It travels inside every Function xpkg per
// Crossplane's function-package convention, so Crossplane can validate the
// Input a Composition pipeline step passes to the function. It is regenerated
// (and drift-checked) by the moon generate/generate-check tasks.
//
//go:embed inputcrd/cuefn.meigma.io_inputs.yaml
var inputCRDYAML []byte

// Function is the full content of a Function xpkg's package.yaml stream: the
// meta.pkg.crossplane.io Function plus the embedded Input CRD that ships inside
// every Crossplane function package.
type Function struct {
	// Meta is the meta.pkg.crossplane.io Function object (the crossplane.yaml).
	Meta *metav1cp.Function
	// InputCRD is the CustomResourceDefinition generated from input/v1beta1,
	// carried inside the package so Crossplane can validate pipeline step inputs.
	InputCRD *apiextensionsv1.CustomResourceDefinition
}

// DefaultFunction builds a Function from meta plus the embedded Input CRD. It is
// the normal constructor: callers supply only the package metadata, and the
// Input CRD is the one generated from this module's input/v1beta1 type. A parse
// failure of the embedded CRD is a build/codegen error (the file is generated
// and committed alongside this package).
func DefaultFunction(meta *metav1cp.Function) (Function, error) {
	if meta == nil {
		return Function{}, errors.New("function requires its package metadata")
	}
	crd := &apiextensionsv1.CustomResourceDefinition{}
	if err := yaml.Unmarshal(inputCRDYAML, crd); err != nil {
		return Function{}, fmt.Errorf("cannot parse embedded Input CRD: %w", err)
	}
	return Function{Meta: meta, InputCRD: crd}, nil
}

// PackageYAML marshals the Function's objects into the package.yaml YAML stream,
// with the package metadata first, then the Input CRD. The document order
// matches crossplane's own builder (meta first); the CRD reuses the same
// status-dropping marshalDoc the Configuration path uses.
func (f Function) PackageYAML() ([]byte, error) {
	if f.Meta == nil {
		return nil, errors.New("function is missing its package metadata")
	}
	if f.InputCRD == nil {
		return nil, errors.New("function is missing its Input CRD")
	}

	docs := []any{f.Meta, f.InputCRD}
	var stream bytes.Buffer
	for i, doc := range docs {
		out, err := marshalDoc(doc)
		if err != nil {
			return nil, err
		}
		if i > 0 {
			stream.WriteString(yamlStreamSeparator)
		}
		stream.Write(out)
	}
	return stream.Bytes(), nil
}

// BuildFunctionImage assembles a single-arch Function xpkg: the package.yaml
// layer (meta + Input CRD) appended over the apko runtime image base, so the
// package image IS the runtime image plus the package layer. base's entrypoint,
// config, and runtime layers are preserved — the package layer never alters
// serving. This is the first real embed-runtime use of BuildXpkgImage
// (base != empty.Image).
func BuildFunctionImage(base v1.Image, f Function) (v1.Image, error) {
	if base == nil {
		return nil, errors.New("function image requires a runtime base image")
	}
	stream, err := f.PackageYAML()
	if err != nil {
		return nil, err
	}
	return BuildXpkgImage(base, stream)
}

// BuildFunctionIndex wraps each per-arch runtime base into a Function xpkg image
// and assembles a multi-arch OCI image index (the release path). A Function
// package image is the runtime image, so a real multi-node-arch install needs an
// index over both arches; the platform of each manifest entry is taken from its
// base's config. The index media type is OCI, matching apko's published runtime
// index.
func BuildFunctionIndex(bases []v1.Image, f Function) (v1.ImageIndex, error) {
	if len(bases) == 0 {
		return nil, errors.New("function index requires at least one runtime base image")
	}

	idx := mutate.IndexMediaType(empty.Index, types.OCIImageIndex)
	for i, base := range bases {
		img, err := BuildFunctionImage(base, f)
		if err != nil {
			return nil, err
		}

		cfg, err := base.ConfigFile()
		if err != nil {
			return nil, fmt.Errorf("cannot read base image %d config: %w", i, err)
		}
		plat := &v1.Platform{OS: cfg.OS, Architecture: cfg.Architecture, Variant: cfg.Variant}
		if plat.OS == "" || plat.Architecture == "" {
			return nil, fmt.Errorf("base image %d is missing its os/architecture (cannot place it in the index)", i)
		}

		idx = mutate.AppendManifests(idx, mutate.IndexAddendum{
			Add:        img,
			Descriptor: v1.Descriptor{Platform: plat},
		})
	}
	return idx, nil
}
