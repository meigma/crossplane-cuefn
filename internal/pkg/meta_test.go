package pkg_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/meigma/crossplane-cuefn/internal/pkg"
)

// TestGenerateConfigurationMeta proves the crossplane.yaml metadata is a
// meta.pkg.crossplane.io Configuration that depends on the cuefn Function.
func TestGenerateConfigurationMeta(t *testing.T) {
	meta, err := pkg.GenerateConfigurationMeta(pkg.ConfigurationMeta{
		Name:                 "xapps-configuration",
		CrossplaneConstraint: ">=v1.14.0-0",
		FunctionPackage:      "xpkg.meigma.io/cuefn",
		FunctionVersion:      ">=v0.1.0",
	})
	require.NoError(t, err)

	assert.Equal(t, "meta.pkg.crossplane.io/v1", meta.APIVersion)
	assert.Equal(t, "Configuration", meta.Kind)
	assert.Equal(t, "xapps-configuration", meta.Name)
	require.NotNil(t, meta.Spec.Crossplane)
	assert.Equal(t, ">=v1.14.0-0", meta.Spec.Crossplane.Version)

	require.Len(t, meta.Spec.DependsOn, 1)
	assert.Equal(t, ">=v0.1.0", meta.Spec.DependsOn[0].Version)

	// Assert the function dependency via the marshaled form (an external check
	// that also avoids reading the deprecated typed field directly).
	raw, err := json.Marshal(meta)
	require.NoError(t, err)
	var doc struct {
		Spec struct {
			DependsOn []map[string]any `json:"dependsOn"`
		} `json:"spec"`
	}
	require.NoError(t, json.Unmarshal(raw, &doc))
	require.Len(t, doc.Spec.DependsOn, 1)
	assert.Equal(t, "xpkg.meigma.io/cuefn", doc.Spec.DependsOn[0]["function"])
}

// TestGenerateConfigurationMeta_OptionalConstraint proves the Crossplane
// constraint is omitted when empty.
func TestGenerateConfigurationMeta_OptionalConstraint(t *testing.T) {
	meta, err := pkg.GenerateConfigurationMeta(pkg.ConfigurationMeta{
		Name:            "xapps-configuration",
		FunctionPackage: "xpkg.meigma.io/cuefn",
		FunctionVersion: ">=v0.1.0",
	})
	require.NoError(t, err)
	assert.Nil(t, meta.Spec.Crossplane)
}

// TestGenerateConfigurationMeta_Errors proves missing required fields error.
func TestGenerateConfigurationMeta_Errors(t *testing.T) {
	t.Run("missing name", func(t *testing.T) {
		_, err := pkg.GenerateConfigurationMeta(pkg.ConfigurationMeta{FunctionPackage: "x"})
		require.Error(t, err)
	})

	t.Run("missing function package", func(t *testing.T) {
		_, err := pkg.GenerateConfigurationMeta(pkg.ConfigurationMeta{Name: "x"})
		require.Error(t, err)
	})
}
