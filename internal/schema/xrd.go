// Package schema is the author-time codegen core: it turns an already-built CUE
// module value into a Kubernetes-structural Crossplane v2 XRD, and validates a
// populated XR spec against the module's #Spec.
//
// The package is pure. It takes a cue.Value the caller has already loaded (the
// render package owns the loader, OCI, and filesystem seams) and has no
// knowledge of cobra, files, or the network, preserving the hexagonal boundary.
//
// The codegen pipeline honors two proven OpenAPI gotchas: generation runs with
// ExpandReferences:false (the only bug-free path for bounded numbers) and the
// resulting $refs are inlined here, because Kubernetes structural schemas forbid
// $ref. The one author guardrail is that schema definitions may not contain
// type-crossing disjunctions (string|int, struct unions); those surface as a
// [DisjunctionError] naming the field.
package schema

import (
	"encoding/json"
	"errors"
	"fmt"

	"cuelang.org/go/cue"
	xv2 "github.com/crossplane/crossplane/apis/v2/apiextensions/v2"
	extv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/yaml"
)

const (
	// xrdAPIVersion is the apiVersion of a Crossplane v2 XRD.
	xrdAPIVersion = "apiextensions.crossplane.io/v2"
	// xrdKind is the kind of a Crossplane v2 XRD.
	xrdKind = "CompositeResourceDefinition"

	specComponent   = "Spec"
	statusComponent = "Status"

	// objectType is the OpenAPI/JSONSchema type for a struct-shaped schema.
	objectType = "object"
)

// printerColumn mirrors the optional additionalPrinterColumns entries an author
// may declare under #API.
type printerColumn struct {
	Name     string `json:"name"`
	Type     string `json:"type"`
	JSONPath string `json:"jsonPath"`
}

// apiEnvelope is the decode target for a module's concrete #API definition.
type apiEnvelope struct {
	Group          string          `json:"group"`
	Version        string          `json:"version"`
	Kind           string          `json:"kind"`
	Plural         string          `json:"plural"`
	Scope          string          `json:"scope"`
	ShortNames     []string        `json:"shortNames,omitempty"`
	Categories     []string        `json:"categories,omitempty"`
	PrinterColumns []printerColumn `json:"printerColumns,omitempty"`
}

// GenerateXRD builds a Crossplane v2 CompositeResourceDefinition from an
// already-loaded module value. It decodes #API into the XRD envelope, generates
// a structural openAPIV3Schema whose spec comes from #Spec and whose status (and
// the status subresource) come from #Status when present, and self-checks the
// result with the API server's structural validator. It returns a
// [DisjunctionError] when a schema definition contains a type-crossing
// disjunction.
func GenerateXRD(module cue.Value) (*xv2.CompositeResourceDefinition, error) {
	api, err := decodeAPI(module)
	if err != nil {
		return nil, err
	}

	components, err := generateComponents(module)
	if err != nil {
		return nil, err
	}

	spec, ok := components[specComponent]
	if !ok {
		return nil, errors.New("module declares no #Spec definition")
	}
	specSchema, err := resolveComponent(specComponent, spec, components)
	if err != nil {
		return nil, err
	}

	top := &extv1.JSONSchemaProps{
		Type: objectType,
		Properties: map[string]extv1.JSONSchemaProps{
			"spec": *specSchema,
		},
	}

	if status, ok := components[statusComponent]; ok {
		statusSchema, err := resolveComponent(statusComponent, status, components)
		if err != nil {
			return nil, err
		}
		top.Properties["status"] = *statusSchema
	}

	// Give required-but-fully-defaultable fields an explicit empty default so the
	// API server materializes the same value CUE fills from an empty spec. Must
	// run after inlining so nested ($ref) structs are visible, and before
	// selfCheck so the materialized schema is what we structurally validate.
	materializeDefaults(top)

	if err := selfCheck(top); err != nil {
		return nil, err
	}

	return assembleXRD(api, top)
}

// GenerateXRDYAML renders the generated XRD as deterministic YAML suitable for
// writing to stdout or a file.
func GenerateXRDYAML(module cue.Value) ([]byte, error) {
	xrd, err := GenerateXRD(module)
	if err != nil {
		return nil, err
	}

	// Round-trip through a generic map to drop the server-populated status block
	// (its struct fields have no omitempty), leaving only the authored XRD.
	raw, err := json.Marshal(xrd)
	if err != nil {
		return nil, fmt.Errorf("cannot marshal XRD: %w", err)
	}
	var doc map[string]any
	if err = json.Unmarshal(raw, &doc); err != nil {
		return nil, fmt.Errorf("cannot normalize XRD: %w", err)
	}
	delete(doc, "status")

	out, err := yaml.Marshal(doc)
	if err != nil {
		return nil, fmt.Errorf("cannot marshal XRD to YAML: %w", err)
	}
	return out, nil
}

// resolveComponent inlines the $refs in a generated component schema and rejects
// type-crossing disjunctions, returning the self-contained structural schema.
func resolveComponent(
	name string,
	raw *extv1.JSONSchemaProps,
	components componentSchemas,
) (*extv1.JSONSchemaProps, error) {
	if err := checkDisjunctions(name, raw); err != nil {
		return nil, err
	}
	resolved, err := inlineRefs(raw, components)
	if err != nil {
		return nil, fmt.Errorf("cannot inline references in #%s: %w", name, err)
	}
	// Disjunctions may also be hidden behind a $ref, so re-scan after inlining.
	if err := checkDisjunctions(name, resolved); err != nil {
		return nil, err
	}
	return resolved, nil
}

// decodeAPI decodes the module's concrete #API into the XRD envelope, applying
// any defaults (such as scope) declared on the definition.
func decodeAPI(module cue.Value) (apiEnvelope, error) {
	api := module.LookupPath(cue.ParsePath("#API"))
	if err := api.Err(); err != nil {
		return apiEnvelope{}, fmt.Errorf("module declares no usable #API: %w", err)
	}

	var out apiEnvelope
	if err := api.Decode(&out); err != nil {
		return apiEnvelope{}, fmt.Errorf("cannot decode #API: %w", err)
	}
	if out.Group == "" || out.Version == "" || out.Kind == "" || out.Plural == "" {
		return apiEnvelope{}, errors.New("#API must set group, version, kind, and plural")
	}
	if out.Scope == "" {
		out.Scope = string(xv2.CompositeResourceScopeNamespaced)
	}
	return out, nil
}

// assembleXRD builds the typed XRD from the decoded envelope and the structural
// schema. The single served version is also the referenceable one (v1 supports
// exactly one version).
func assembleXRD(api apiEnvelope, top *extv1.JSONSchemaProps) (*xv2.CompositeResourceDefinition, error) {
	rawSchema, err := json.Marshal(top)
	if err != nil {
		return nil, fmt.Errorf("cannot marshal openAPIV3Schema: %w", err)
	}

	version := xv2.CompositeResourceDefinitionVersion{
		Name:          api.Version,
		Served:        true,
		Referenceable: true,
		Schema: &xv2.CompositeResourceValidation{
			OpenAPIV3Schema: runtime.RawExtension{Raw: rawSchema},
		},
	}
	for _, c := range api.PrinterColumns {
		version.AdditionalPrinterColumns = append(version.AdditionalPrinterColumns,
			extv1.CustomResourceColumnDefinition{
				Name:     c.Name,
				Type:     c.Type,
				JSONPath: c.JSONPath,
			})
	}

	xrd := &xv2.CompositeResourceDefinition{
		TypeMeta: metav1.TypeMeta{
			APIVersion: xrdAPIVersion,
			Kind:       xrdKind,
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: api.Plural + "." + api.Group,
		},
		Spec: xv2.CompositeResourceDefinitionSpec{
			Group: api.Group,
			Scope: xv2.CompositeResourceScope(api.Scope),
			Names: extv1.CustomResourceDefinitionNames{
				Kind:       api.Kind,
				Plural:     api.Plural,
				ShortNames: api.ShortNames,
				Categories: api.Categories,
			},
			Versions: []xv2.CompositeResourceDefinitionVersion{version},
		},
	}
	return xrd, nil
}
