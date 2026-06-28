package schema

import (
	"context"
	"encoding/json"
	"testing"

	"cuelang.org/go/cue"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	extv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"

	"github.com/meigma/crossplane-cuefn/internal/render"
)

// loadModule builds a testdata module value for codegen tests, registering its
// cleanup with the test.
func loadModule(t *testing.T, dir string) cue.Value {
	t.Helper()
	val, cleanup, err := render.LoadModule(context.Background(), render.LocalLoader{Dir: dir}, "ignored")
	require.NoError(t, err)
	t.Cleanup(cleanup)
	return val
}

// schemaProps extracts the decoded openAPIV3Schema from a generated XRD's single
// version.
func schemaProps(t *testing.T, dir string) *extv1.JSONSchemaProps {
	t.Helper()
	xrd, err := GenerateXRD(loadModule(t, dir))
	require.NoError(t, err)
	require.Len(t, xrd.Spec.Versions, 1)

	var props extv1.JSONSchemaProps
	require.NoError(t, json.Unmarshal(xrd.Spec.Versions[0].Schema.OpenAPIV3Schema.Raw, &props))
	return &props
}

// TestGenerateXRD_DeRisked_PassesStructural proves the full codegen pipeline
// (definitions-only reduction, ExpandReferences:false generation, $ref inlining)
// yields an XRD whose openAPIV3Schema the API server's own structural validator
// accepts — the spec includes the bounded-number-with-default ExpandReferences
// bug case, a nested $ref, an enum, a regex, a bool default, a list-of-objects,
// and a string map.
func TestGenerateXRD_DeRisked_PassesStructural(t *testing.T) {
	// GenerateXRD runs selfCheck (NewStructural + ValidateStructural) internally,
	// so a non-error return is the structural pass; assert no error and a schema.
	xrd, err := GenerateXRD(loadModule(t, "testdata/derisked"))
	require.NoError(t, err)
	require.NotNil(t, xrd.Spec.Versions[0].Schema)
	require.NotEmpty(t, xrd.Spec.Versions[0].Schema.OpenAPIV3Schema.Raw)
}

// TestGenerateXRD_Fidelity asserts the envelope mapping and that required fields,
// defaults, numeric bounds, the enum, the regex, the inlined nested object, and
// printerColumns/categories/shortNames all carry through.
func TestGenerateXRD_Fidelity(t *testing.T) {
	xrd, err := GenerateXRD(loadModule(t, "testdata/derisked"))
	require.NoError(t, err)

	// Envelope.
	assert.Equal(t, "apiextensions.crossplane.io/v2", xrd.APIVersion)
	assert.Equal(t, "CompositeResourceDefinition", xrd.Kind)
	assert.Equal(t, "xwidgets.platform.example.com", xrd.Name)
	assert.Equal(t, "platform.example.com", xrd.Spec.Group)
	assert.Equal(t, xv2Scope("Namespaced"), string(xrd.Spec.Scope))
	assert.Equal(t, "XWidget", xrd.Spec.Names.Kind)
	assert.Equal(t, "xwidgets", xrd.Spec.Names.Plural)
	assert.Equal(t, []string{"wdg"}, xrd.Spec.Names.ShortNames)
	assert.Equal(t, []string{"platform"}, xrd.Spec.Names.Categories)

	require.Len(t, xrd.Spec.Versions, 1)
	v := xrd.Spec.Versions[0]
	assert.Equal(t, "v1alpha1", v.Name)
	assert.True(t, v.Served)
	assert.True(t, v.Referenceable)
	require.Len(t, v.AdditionalPrinterColumns, 2)
	assert.Equal(t, "Replicas", v.AdditionalPrinterColumns[0].Name)
	assert.Equal(t, ".spec.replicas", v.AdditionalPrinterColumns[0].JSONPath)

	spec := schemaProps(t, "testdata/derisked").Properties["spec"]

	// Required (!) and required-with-default fields.
	assert.Contains(t, spec.Required, "image")

	// Bounded number with default.
	replicas := spec.Properties["replicas"]
	assert.Equal(t, "integer", replicas.Type)
	require.NotNil(t, replicas.Minimum)
	require.NotNil(t, replicas.Maximum)
	assert.InDelta(t, 1, *replicas.Minimum, 0)
	assert.InDelta(t, 10, *replicas.Maximum, 0)
	assert.JSONEq(t, "3", rawDefault(t, replicas.Default))

	// Enum.
	tier := spec.Properties["tier"]
	assert.Len(t, tier.Enum, 3)

	// Regex pattern.
	assert.Equal(t, "^[a-z0-9./:@-]+$", spec.Properties["image"].Pattern)

	// Bool default.
	assert.JSONEq(t, "false", rawDefault(t, spec.Properties["exposed"].Default))

	// Inlined nested object (no $ref survives).
	ports := spec.Properties["ports"]
	require.NotNil(t, ports.Items)
	require.NotNil(t, ports.Items.Schema)
	assert.Nil(t, ports.Items.Schema.Ref)
	assert.Equal(t, "object", ports.Items.Schema.Type)
	assert.Contains(t, ports.Items.Schema.Properties, "port")

	// String map.
	labels := spec.Properties["labels"]
	require.NotNil(t, labels.AdditionalProperties)
	require.NotNil(t, labels.AdditionalProperties.Schema)
	assert.Equal(t, "string", labels.AdditionalProperties.Schema.Type)

	// No $ref anywhere in the generated schema.
	assertNoRefs(t, schemaProps(t, "testdata/derisked"))
}

// TestGenerateXRD_StatusSubresource_IffStatusPresent proves .properties.status
// exists exactly when the module declares #Status.
func TestGenerateXRD_StatusSubresource_IffStatusPresent(t *testing.T) {
	withStatus := schemaProps(t, "testdata/derisked")
	assert.Contains(t, withStatus.Properties, "status",
		"derisked module declares #Status, so .properties.status must be present")
	status := withStatus.Properties["status"]
	assert.Contains(t, status.Properties, "ready")

	noStatus := schemaProps(t, "testdata/nostatus")
	assert.NotContains(t, noStatus.Properties, "status",
		"nostatus module declares no #Status, so .properties.status must be absent")
}

// TestGenerateXRD_TypeCrossingDisjunction_NamesField proves a string|int field
// fails with a DisjunctionError that names the field, never a panic.
func TestGenerateXRD_TypeCrossingDisjunction_NamesField(t *testing.T) {
	_, err := GenerateXRD(loadModule(t, "testdata/disjunction"))
	require.Error(t, err)

	var de *DisjunctionError
	require.ErrorAs(t, err, &de)
	assert.Equal(t, "Spec.value", de.Field)
}

// TestGenerateXRDYAML_Deterministic proves the YAML output is stable across runs
// and omits the server-populated status block.
func TestGenerateXRDYAML_Deterministic(t *testing.T) {
	module := loadModule(t, "testdata/derisked")
	first, err := GenerateXRDYAML(module)
	require.NoError(t, err)
	second, err := GenerateXRDYAML(module)
	require.NoError(t, err)
	assert.Equal(t, string(first), string(second))
	assert.NotContains(t, string(first), "controllers:")
}

func assertNoRefs(t *testing.T, s *extv1.JSONSchemaProps) {
	t.Helper()
	if s == nil {
		return
	}
	assert.Nil(t, s.Ref, "no $ref may survive inlining")
	for name := range s.Properties {
		child := s.Properties[name]
		assertNoRefs(t, &child)
	}
	if s.Items != nil && s.Items.Schema != nil {
		assertNoRefs(t, s.Items.Schema)
	}
	if s.AdditionalProperties != nil {
		assertNoRefs(t, s.AdditionalProperties.Schema)
	}
}

func rawDefault(t *testing.T, d *extv1.JSON) string {
	t.Helper()
	require.NotNil(t, d)
	return string(d.Raw)
}

// xv2Scope is a tiny helper so the fidelity test reads scope as a plain string.
func xv2Scope(s string) string { return s }
