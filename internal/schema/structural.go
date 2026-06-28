package schema

import (
	"fmt"

	"k8s.io/apiextensions-apiserver/pkg/apis/apiextensions"
	extv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	structuralschema "k8s.io/apiextensions-apiserver/pkg/apiserver/schema"
	"k8s.io/apimachinery/pkg/util/validation/field"
)

// selfCheck runs the API server's own structural-schema validation over a
// generated openAPIV3Schema, so codegen fails fast with a field-located error
// instead of producing an XRD a real cluster would reject. It mirrors exactly
// what the apiserver does on CRD admission: convert the v1 schema to the
// internal representation, build the structural skeleton, then validate it.
func selfCheck(s *extv1.JSONSchemaProps) error {
	internal := &apiextensions.JSONSchemaProps{}
	if err := extv1.Convert_v1_JSONSchemaProps_To_apiextensions_JSONSchemaProps(s, internal, nil); err != nil {
		return fmt.Errorf("cannot convert generated schema for structural check: %w", err)
	}

	structural, err := structuralschema.NewStructural(internal)
	if err != nil {
		return fmt.Errorf("generated schema is not structural: %w", err)
	}

	if errs := structuralschema.ValidateStructural(field.NewPath("openAPIV3Schema"), structural); len(errs) > 0 {
		return fmt.Errorf("generated schema is not structural: %w", errs.ToAggregate())
	}
	return nil
}
