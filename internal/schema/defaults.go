package schema

import (
	extv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
)

// arrayType is the JSON Schema type for a list.
const arrayType = "array"

// materializeDefaults gives every required property that an empty value already
// satisfies an explicit default, so the API server fills the same value CUE
// computes from an empty spec. Without it `cuefn validate`/`render` accept a
// `spec: {}` (CUE resolves #Spec & {} recursively) that the API server rejects
// with "Required value" for a required-but-fully-defaultable field, breaking the
// documented no-drift guarantee. It recurses depth-first and reports whether
// schema itself is empty-satisfiable.
//
// Empty-satisfiable means a concrete value needs no input: a scalar with a
// default, an object whose every required property is empty-satisfiable (incl. a
// pure map with no required keys), or a list with no minimum item count. A
// genuinely-required field (a scalar with no default, a list with minItems > 0)
// is left untouched, so it still requires input — matching CUE, which also
// rejects an empty value for it.
func materializeDefaults(schema *extv1.JSONSchemaProps) bool {
	if schema == nil {
		return true
	}
	switch schema.Type {
	case objectType:
		return materializeObjectDefaults(schema)
	case arrayType:
		if schema.Items != nil && schema.Items.Schema != nil {
			materializeDefaults(schema.Items.Schema)
		}
		return schema.Default != nil || schema.MinItems == nil || *schema.MinItems == 0
	default:
		return schema.Default != nil
	}
}

// materializeObjectDefaults recurses into an object's properties, sets an empty
// default on each required property an empty value satisfies, and reports whether
// the object as a whole is empty-satisfiable.
func materializeObjectDefaults(schema *extv1.JSONSchemaProps) bool {
	required := make(map[string]bool, len(schema.Required))
	for _, name := range schema.Required {
		required[name] = true
	}

	allRequiredSatisfiable := true
	for name := range schema.Properties {
		child := schema.Properties[name]
		satisfiable := materializeDefaults(&child)
		if required[name] {
			if satisfiable && child.Default == nil {
				if def := emptyContainerDefault(&child); def != nil {
					child.Default = def
				}
			}
			if !satisfiable {
				allRequiredSatisfiable = false
			}
		}
		schema.Properties[name] = child
	}

	// Recurse into a pure map's value schema so nested defaults are set there too.
	if schema.AdditionalProperties != nil && schema.AdditionalProperties.Schema != nil {
		materializeDefaults(schema.AdditionalProperties.Schema)
	}
	return allRequiredSatisfiable
}

// emptyContainerDefault returns the empty default ({} or []) for a container
// schema, or nil for a scalar, which cannot be materialized from nothing.
func emptyContainerDefault(s *extv1.JSONSchemaProps) *extv1.JSON {
	switch s.Type {
	case objectType:
		return &extv1.JSON{Raw: []byte("{}")}
	case arrayType:
		return &extv1.JSON{Raw: []byte("[]")}
	default:
		return nil
	}
}
