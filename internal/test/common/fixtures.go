package common

import (
	"testing"

	xv2 "github.com/crossplane/crossplane/apis/v2/apiextensions/v2"
	extv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/stretchr/testify/require"

	"github.com/meigma/crossplane-cuefn/internal/pkg"
)

// FixtureXRD builds a minimal but valid Crossplane v2 XRD for the example platform
// API, mirroring what internal/schema.GenerateXRD emits. The pure packaging tests
// use this typed fixture so they need neither CUE nor a registry.
func FixtureXRD(t *testing.T) *xv2.CompositeResourceDefinition {
	t.Helper()

	rawSchema := []byte(`{"type":"object"}`)
	return &xv2.CompositeResourceDefinition{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "apiextensions.crossplane.io/v2",
			Kind:       "CompositeResourceDefinition",
		},
		ObjectMeta: metav1.ObjectMeta{Name: "xapps.platform.meigma.io"},
		Spec: xv2.CompositeResourceDefinitionSpec{
			Group: "platform.meigma.io",
			Scope: xv2.CompositeResourceScopeNamespaced,
			Names: extv1.CustomResourceDefinitionNames{
				Kind:   "XApp",
				Plural: "xapps",
			},
			Versions: []xv2.CompositeResourceDefinitionVersion{{
				Name:          "v1alpha1",
				Served:        true,
				Referenceable: true,
				Schema: &xv2.CompositeResourceValidation{
					OpenAPIV3Schema: runtime.RawExtension{Raw: rawSchema},
				},
			}},
		},
	}
}

// BuildFixtureConfiguration assembles a Configuration from the fixture XRD plus a
// generated Composition and metadata. It is the shared input for the image tests.
func BuildFixtureConfiguration(t *testing.T) pkg.Configuration {
	t.Helper()

	xrd := FixtureXRD(t)
	comp, err := pkg.GenerateComposition(xrd, pkg.CompositionInput{
		Module:         "cuefn.example/app@v0.1.0",
		ExpectedDigest: "sha256:" + Zeros(64),
	})
	require.NoError(t, err)

	meta, err := pkg.GenerateConfigurationMeta(pkg.ConfigurationMeta{
		Name:            "xapps-configuration",
		FunctionPackage: "xpkg.meigma.io/cuefn",
		FunctionVersion: ">=v0.1.0",
	})
	require.NoError(t, err)

	return pkg.Configuration{Meta: meta, XRD: xrd, Composition: comp}
}

// FixtureFunction builds a Function from the embedded Input CRD plus a generated
// meta. It is the shared input for the function packaging tests.
func FixtureFunction(t *testing.T) pkg.Function {
	t.Helper()

	meta, err := pkg.GenerateFunctionMeta(pkg.FunctionMeta{Name: "function-cuefn"})
	require.NoError(t, err)

	fn, err := pkg.DefaultFunction(meta)
	require.NoError(t, err)
	return fn
}

// StepName extracts the "step" field of a pipeline step map.
func StepName(t *testing.T, step any) string {
	t.Helper()
	m, ok := step.(map[string]any)
	require.True(t, ok)
	name, ok := m["step"].(string)
	require.True(t, ok)
	return name
}
