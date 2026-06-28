package cli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apiextensions-apiserver/pkg/apis/apiextensions"
	extv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	structuralschema "k8s.io/apiextensions-apiserver/pkg/apiserver/schema"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"sigs.k8s.io/yaml"
)

const deriskedModuleDir = "../schema/testdata/derisked"

// TestGenerateCmd_EmitsStructuralXRD proves `cuefn generate --dir <derisked>`
// writes an XRD whose openAPIV3Schema the API server's structural validator
// accepts, and that the YAML is deterministic across runs (criteria 1/2 at the
// CLI level).
func TestGenerateCmd_EmitsStructuralXRD(t *testing.T) {
	t.Parallel()

	out := runGenerateCmd(t)

	var xrd struct {
		APIVersion string `json:"apiVersion"`
		Kind       string `json:"kind"`
		Spec       struct {
			Group    string `json:"group"`
			Versions []struct {
				Name   string `json:"name"`
				Schema struct {
					OpenAPIV3Schema extv1.JSONSchemaProps `json:"openAPIV3Schema"`
				} `json:"schema"`
			} `json:"versions"`
		} `json:"spec"`
	}
	require.NoError(t, yaml.Unmarshal(out, &xrd))

	assert.Equal(t, "apiextensions.crossplane.io/v2", xrd.APIVersion)
	assert.Equal(t, "CompositeResourceDefinition", xrd.Kind)
	require.Len(t, xrd.Spec.Versions, 1)

	// The emitted schema passes the same structural check the apiserver runs.
	internal := &apiextensions.JSONSchemaProps{}
	require.NoError(t, extv1.Convert_v1_JSONSchemaProps_To_apiextensions_JSONSchemaProps(
		&xrd.Spec.Versions[0].Schema.OpenAPIV3Schema, internal, nil))
	structural, err := structuralschema.NewStructural(internal)
	require.NoError(t, err)
	errs := structuralschema.ValidateStructural(field.NewPath("openAPIV3Schema"), structural)
	require.Empty(t, errs, "generated schema must be structural")

	// Deterministic across runs.
	assert.Equal(t, string(out), string(runGenerateCmd(t)))
}

// TestGenerateCmd_WritesOutputFile proves --output writes the XRD to a file
// instead of stdout.
func TestGenerateCmd_WritesOutputFile(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "xrd.yaml")
	root := NewRootCommand(Options{Out: &bytes.Buffer{}, Err: &bytes.Buffer{}})
	root.SetArgs([]string{"generate", "ignored", "--dir", deriskedModuleDir, "--output", path})
	require.NoError(t, root.ExecuteContext(context.Background()))

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Contains(t, string(data), "CompositeResourceDefinition")
}

func runGenerateCmd(t *testing.T) []byte {
	t.Helper()
	var stdout bytes.Buffer
	root := NewRootCommand(Options{Out: &stdout, Err: &bytes.Buffer{}})
	root.SetArgs([]string{"generate", "ignored", "--dir", deriskedModuleDir})
	require.NoError(t, root.ExecuteContext(context.Background()))
	return stdout.Bytes()
}
