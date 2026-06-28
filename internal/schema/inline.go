package schema

import (
	"fmt"

	extv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
)

// inlineRefs returns a deep copy of s with every $ref replaced by the schema it
// points at, recursively, so the result is self-contained. Kubernetes structural
// schemas forbid $ref, and OpenAPI generation with ExpandReferences:false leaves
// references intact, so inlining is mandatory.
//
// The walk is cycle-detecting: a definition that (transitively) references
// itself cannot be expressed as a finite structural schema, so inlineRefs
// returns an error naming the cycle instead of recursing forever.
func inlineRefs(s *extv1.JSONSchemaProps, components componentSchemas) (*extv1.JSONSchemaProps, error) {
	in := &inliner{components: components, active: map[string]bool{}}
	return in.inline(s)
}

// inliner carries the component table and the active-reference set used for
// cycle detection across the recursive walk.
type inliner struct {
	components componentSchemas
	active     map[string]bool
}

func (in *inliner) inline(s *extv1.JSONSchemaProps) (*extv1.JSONSchemaProps, error) {
	if s == nil {
		return nil, nil //nolint:nilnil // a nil sub-schema inlines to nil.
	}
	if s.Ref != nil && *s.Ref != "" {
		return in.resolveRef(*s.Ref)
	}

	out := s.DeepCopy()
	if err := in.inlineMaps(s, out); err != nil {
		return nil, err
	}
	if err := in.inlineItems(s, out); err != nil {
		return nil, err
	}
	if err := in.inlineCombinators(s, out); err != nil {
		return nil, err
	}
	return out, nil
}

// resolveRef replaces a reference with the (recursively inlined) schema it names,
// tracking the active path to break cycles.
func (in *inliner) resolveRef(ref string) (*extv1.JSONSchemaProps, error) {
	name := refName(ref)
	if in.active[name] {
		return nil, fmt.Errorf(
			"schema definition %q is recursive and cannot be expressed as a structural schema",
			name,
		)
	}
	target, ok := in.components[name]
	if !ok {
		return nil, fmt.Errorf("schema reference %q has no matching definition", ref)
	}
	in.active[name] = true
	defer func() { in.active[name] = false }()
	return in.inline(target)
}

// inlineMaps inlines the property, pattern-property, and additional-property
// sub-schemas.
func (in *inliner) inlineMaps(s, out *extv1.JSONSchemaProps) error {
	if err := in.inlineMap(s.Properties, out.Properties); err != nil {
		return err
	}
	if err := in.inlineMap(s.PatternProperties, out.PatternProperties); err != nil {
		return err
	}
	if s.AdditionalProperties != nil && s.AdditionalProperties.Schema != nil {
		resolved, err := in.inline(s.AdditionalProperties.Schema)
		if err != nil {
			return err
		}
		out.AdditionalProperties.Schema = resolved
	}
	return nil
}

// inlineItems inlines the single and tuple list item sub-schemas.
func (in *inliner) inlineItems(s, out *extv1.JSONSchemaProps) error {
	if s.Items == nil {
		return nil
	}
	if s.Items.Schema != nil {
		resolved, err := in.inline(s.Items.Schema)
		if err != nil {
			return err
		}
		out.Items.Schema = resolved
	}
	return in.inlineSlice(s.Items.JSONSchemas, out.Items.JSONSchemas)
}

// inlineCombinators inlines the allOf/anyOf/oneOf/not sub-schemas.
func (in *inliner) inlineCombinators(s, out *extv1.JSONSchemaProps) error {
	if err := in.inlineSlice(s.AllOf, out.AllOf); err != nil {
		return err
	}
	if err := in.inlineSlice(s.AnyOf, out.AnyOf); err != nil {
		return err
	}
	if err := in.inlineSlice(s.OneOf, out.OneOf); err != nil {
		return err
	}
	if s.Not != nil {
		resolved, err := in.inline(s.Not)
		if err != nil {
			return err
		}
		out.Not = resolved
	}
	return nil
}

func (in *inliner) inlineMap(from, to map[string]extv1.JSONSchemaProps) error {
	for name, child := range from {
		resolved, err := in.inline(&child)
		if err != nil {
			return err
		}
		to[name] = *resolved
	}
	return nil
}

func (in *inliner) inlineSlice(from, to []extv1.JSONSchemaProps) error {
	for i := range from {
		resolved, err := in.inline(&from[i])
		if err != nil {
			return err
		}
		to[i] = *resolved
	}
	return nil
}
