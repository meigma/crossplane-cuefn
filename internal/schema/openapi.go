package schema

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"cuelang.org/go/cue"
	"cuelang.org/go/encoding/openapi"
	extv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
)

// componentSchemas is the set of OpenAPI component schemas generated from a
// module's definitions, keyed by the OpenAPI component name (a definition's
// label without its leading "#"). References between them are kept as $ref by
// generating with ExpandReferences:false; the inliner resolves them later.
type componentSchemas map[string]*extv1.JSONSchemaProps

// generateComponents reduces module to its definitions, runs OpenAPI generation
// with ExpandReferences:false (the only bug-free path for bounded numbers), and
// decodes the resulting component schemas into apiextensions/v1 JSONSchemaProps.
func generateComponents(module cue.Value) (componentSchemas, error) {
	defs, err := definitionsOnly(module)
	if err != nil {
		return nil, err
	}

	file, err := openapi.Generate(defs, &openapi.Config{
		// ExpandReferences:true is buggy with bounded numbers ("unsupported op
		// for number &"); generate with references intact and inline them later.
		ExpandReferences: false,
		Info: map[string]string{
			"title":   "cuefn generated schema",
			"version": "v0",
		},
	})
	if err != nil {
		return nil, fmt.Errorf("cannot generate OpenAPI schema: %w", err)
	}

	docValue := module.Context().BuildFile(file)
	if err = docValue.Err(); err != nil {
		return nil, fmt.Errorf("cannot build generated OpenAPI document: %w", err)
	}
	raw, err := docValue.MarshalJSON()
	if err != nil {
		return nil, fmt.Errorf("cannot marshal generated OpenAPI document: %w", err)
	}

	var doc struct {
		Components struct {
			Schemas map[string]json.RawMessage `json:"schemas"`
		} `json:"components"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, fmt.Errorf("cannot parse generated OpenAPI document: %w", err)
	}

	out := make(componentSchemas, len(doc.Components.Schemas))
	for name, rawSchema := range doc.Components.Schemas {
		var props extv1.JSONSchemaProps
		if err := json.Unmarshal(rawSchema, &props); err != nil {
			return nil, fmt.Errorf("cannot decode generated schema %q: %w", name, err)
		}
		out[name] = &props
	}
	return out, nil
}

// definitionsOnly returns a fresh value containing only module's top-level
// definitions (#Foo). OpenAPI generation rejects regular top-level fields such
// as the module's input/resources, so the transform fields must be dropped
// before generating.
func definitionsOnly(module cue.Value) (cue.Value, error) {
	ctx := module.Context()
	out := ctx.CompileString("{}")

	iter, err := module.Fields(cue.Definitions(true))
	if err != nil {
		return cue.Value{}, fmt.Errorf("cannot iterate module fields: %w", err)
	}
	for iter.Next() {
		sel := iter.Selector()
		if !sel.IsDefinition() {
			continue
		}
		out = out.FillPath(cue.MakePath(sel), iter.Value())
	}
	if err := out.Err(); err != nil {
		return cue.Value{}, fmt.Errorf("cannot reduce module to definitions: %w", err)
	}
	return out, nil
}

// checkDisjunctions walks a generated schema and returns a DisjunctionError for
// the first type-crossing disjunction it finds (rendered by OpenAPI as a
// non-empty oneOf). Same-type disjunctions render as enum, not oneOf, so this
// rejects only the constructs a structural schema cannot express. root names the
// component the schema came from so the reported field path is unambiguous.
func checkDisjunctions(root string, s *extv1.JSONSchemaProps) error {
	return walkForOneOf(root, s)
}

func walkForOneOf(path string, s *extv1.JSONSchemaProps) error {
	if s == nil {
		return nil
	}
	if len(s.OneOf) > 0 {
		return &DisjunctionError{
			Field:  path,
			Detail: "a type-crossing disjunction (e.g. string|int or a struct union) cannot be a Kubernetes structural schema",
		}
	}
	if err := walkMapsForOneOf(path, s); err != nil {
		return err
	}
	return walkListsForOneOf(path, s)
}

// walkMapsForOneOf scans the property, pattern-property, and additional-property
// sub-schemas. Properties are visited in sorted order for deterministic
// reporting.
func walkMapsForOneOf(path string, s *extv1.JSONSchemaProps) error {
	for _, name := range sortedKeys(s.Properties) {
		child := s.Properties[name]
		if err := walkForOneOf(path+"."+name, &child); err != nil {
			return err
		}
	}
	for _, name := range sortedKeys(s.PatternProperties) {
		child := s.PatternProperties[name]
		if err := walkForOneOf(path+".["+name+"]", &child); err != nil {
			return err
		}
	}
	if ap := s.AdditionalProperties; ap != nil && ap.Schema != nil {
		if err := walkForOneOf(path+".*", ap.Schema); err != nil {
			return err
		}
	}
	return nil
}

// walkListsForOneOf scans the list-item and allOf/anyOf/not sub-schemas.
func walkListsForOneOf(path string, s *extv1.JSONSchemaProps) error {
	if s.Items != nil {
		if s.Items.Schema != nil {
			if err := walkForOneOf(path+"[]", s.Items.Schema); err != nil {
				return err
			}
		}
		for i := range s.Items.JSONSchemas {
			if err := walkForOneOf(fmt.Sprintf("%s[%d]", path, i), &s.Items.JSONSchemas[i]); err != nil {
				return err
			}
		}
	}
	for _, group := range [][]extv1.JSONSchemaProps{s.AllOf, s.AnyOf} {
		for i := range group {
			if err := walkForOneOf(path, &group[i]); err != nil {
				return err
			}
		}
	}
	if s.Not != nil {
		return walkForOneOf(path, s.Not)
	}
	return nil
}

func sortedKeys(m map[string]extv1.JSONSchemaProps) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// refName extracts the component name from an OpenAPI local reference such as
// "#/components/schemas/Spec".
func refName(ref string) string {
	if i := strings.LastIndex(ref, "/"); i >= 0 {
		return ref[i+1:]
	}
	return ref
}
