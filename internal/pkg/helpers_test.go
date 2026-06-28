package pkg_test

import (
	"strings"
	"testing"

	xv2 "github.com/crossplane/crossplane/apis/v2/apiextensions/v2"
	extv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/yaml"

	"github.com/stretchr/testify/require"
)

// fixtureXRD builds a minimal but valid Crossplane v2 XRD for the example
// platform API, mirroring what internal/schema.GenerateXRD emits. The pure
// packaging tests use this typed fixture so they need neither CUE nor a registry.
func fixtureXRD(t *testing.T) *xv2.CompositeResourceDefinition {
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

// yamlDoc is the decode target for one document's identity fields.
type yamlDoc struct {
	APIVersion string         `json:"apiVersion"`
	Kind       string         `json:"kind"`
	Metadata   map[string]any `json:"metadata"`
	Spec       map[string]any `json:"spec"`
}

// splitStream splits a multi-document YAML stream into its non-empty documents.
func splitStream(t *testing.T, stream []byte) []yamlDoc {
	t.Helper()

	var docs []yamlDoc
	for chunk := range strings.SplitSeq(string(stream), "\n---\n") {
		if strings.TrimSpace(chunk) == "" {
			continue
		}
		var d yamlDoc
		require.NoError(t, yaml.Unmarshal([]byte(chunk), &d), "unmarshal stream document:\n%s", chunk)
		docs = append(docs, d)
	}
	return docs
}
