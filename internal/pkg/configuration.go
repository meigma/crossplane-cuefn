package pkg

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"

	apiextv1 "github.com/crossplane/crossplane/apis/v2/apiextensions/v1"
	xv2 "github.com/crossplane/crossplane/apis/v2/apiextensions/v2"
	metav1cp "github.com/crossplane/crossplane/apis/v2/pkg/meta/v1"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/empty"
	"sigs.k8s.io/yaml"
)

// yamlStreamSeparator delimits documents in the package.yaml YAML stream.
const yamlStreamSeparator = "---\n"

// Configuration is the full set of objects packaged into one Configuration xpkg:
// the package metadata (crossplane.yaml), the XRD, and the Composition.
type Configuration struct {
	// Meta is the meta.pkg.crossplane.io Configuration object.
	Meta *metav1cp.Configuration
	// XRD is the generated CompositeResourceDefinition (from internal/schema).
	XRD *xv2.CompositeResourceDefinition
	// Composition is the pipeline-mode Composition (from GenerateComposition).
	Composition *apiextv1.Composition
}

// PackageYAML marshals the Configuration's three objects into the package.yaml
// YAML stream, with the package metadata first, then the XRD, then the
// Composition. Crossplane reads every document from this single stream; the
// document order matches crossplane's own builder (meta first).
func (c Configuration) PackageYAML() ([]byte, error) {
	if c.Meta == nil {
		return nil, errors.New("configuration is missing its package metadata")
	}
	if c.XRD == nil {
		return nil, errors.New("configuration is missing its XRD")
	}
	if c.Composition == nil {
		return nil, errors.New("configuration is missing its Composition")
	}

	docs := []any{c.Meta, c.XRD, c.Composition}
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

// BuildConfigurationImage marshals c into the package.yaml stream and assembles a
// Configuration xpkg image (empty base, single annotated package layer) ready to
// push.
func BuildConfigurationImage(c Configuration) (v1.Image, error) {
	stream, err := c.PackageYAML()
	if err != nil {
		return nil, err
	}
	return BuildXpkgImage(empty.Image, stream)
}

// marshalDoc renders a packaged object as YAML. It round-trips through a generic
// map to drop the server-populated, never-authored top-level status block (the
// XRD's status struct has no omitempty), leaving only the authored document.
func marshalDoc(obj any) ([]byte, error) {
	raw, err := json.Marshal(obj)
	if err != nil {
		return nil, fmt.Errorf("cannot marshal package document: %w", err)
	}
	var doc map[string]any
	if err = json.Unmarshal(raw, &doc); err != nil {
		return nil, fmt.Errorf("cannot normalize package document: %w", err)
	}
	delete(doc, "status")

	out, err := yaml.Marshal(doc)
	if err != nil {
		return nil, fmt.Errorf("cannot marshal package document to YAML: %w", err)
	}
	return out, nil
}
