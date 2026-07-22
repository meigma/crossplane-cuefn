package pkg_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	inputv1beta1 "github.com/meigma/crossplane-cuefn/input/v1beta1"
	"github.com/meigma/crossplane-cuefn/internal/pkg"
	"github.com/meigma/crossplane-cuefn/internal/test/common"
)

// TestGenerateComposition_DefaultPipeline proves the Composition is pipeline-mode
// with a single cuefn step when no EnvironmentConfigs are requested (so a default
// install needs only the cuefn Function), and that its compositeTypeRef is derived
// from the XRD's group/version/kind.
func TestGenerateComposition_DefaultPipeline(t *testing.T) {
	xrd := common.FixtureXRD(t)

	comp, err := pkg.GenerateComposition(xrd, pkg.CompositionInput{
		Module:         "cuefn.example/app@v0.1.0",
		ExpectedDigest: "sha256:" + common.Zeros(64),
	})
	require.NoError(t, err)

	assert.Equal(t, "apiextensions.crossplane.io/v1", comp.APIVersion)
	assert.Equal(t, "Composition", comp.Kind)
	assert.Equal(t, "Pipeline", string(comp.Spec.Mode))
	assert.Equal(t, "platform.meigma.io/v1alpha1", comp.Spec.CompositeTypeRef.APIVersion)
	assert.Equal(t, "XApp", comp.Spec.CompositeTypeRef.Kind)

	require.Len(t, comp.Spec.Pipeline, 1)
	assert.Equal(t, "cuefn", comp.Spec.Pipeline[0].Step)
}

// TestGenerateComposition_LockStep proves the cuefn step's Input round-trips into
// input/v1beta1.Input carrying the exact module ref and expected digest — the
// author half of the runtime digest lock-step the function later verifies.
func TestGenerateComposition_LockStep(t *testing.T) {
	xrd := common.FixtureXRD(t)
	const ref = "cuefn.example/app@v0.3.0"
	digest := "sha256:" + common.Zeros(64)

	comp, err := pkg.GenerateComposition(xrd, pkg.CompositionInput{
		Module:         ref,
		ExpectedDigest: digest,
	})
	require.NoError(t, err)

	step := comp.Spec.Pipeline[0]
	require.Equal(t, "cuefn", step.Step)
	require.NotNil(t, step.Input, "cuefn step must carry an Input")

	var in inputv1beta1.Input
	require.NoError(t, json.Unmarshal(step.Input.Raw, &in))

	assert.Equal(t, "cuefn.meigma.io/v1beta1", in.APIVersion)
	assert.Equal(t, "Input", in.Kind)
	assert.Equal(t, ref, in.Module)
	assert.Equal(t, digest, in.ExpectedDigest)
}

// TestGenerateComposition_FunctionName proves the functionRef.name follows the
// supplied FunctionName, defaulting to the cuefn step name when unset.
func TestGenerateComposition_FunctionName(t *testing.T) {
	xrd := common.FixtureXRD(t)

	withName, err := pkg.GenerateComposition(xrd, pkg.CompositionInput{
		Module:       "cuefn.example/app@v0.1.0",
		FunctionName: "my-cuefn",
	})
	require.NoError(t, err)
	assert.Equal(t, "my-cuefn", withName.Spec.Pipeline[0].FunctionRef.Name)

	defaulted, err := pkg.GenerateComposition(xrd, pkg.CompositionInput{
		Module: "cuefn.example/app@v0.1.0",
	})
	require.NoError(t, err)
	assert.Equal(t, "cuefn", defaulted.Spec.Pipeline[0].FunctionRef.Name)
}

// TestGenerateComposition_EnvironmentConfigs proves the function-environment-configs
// step is emitted only when EnvironmentConfigs are requested, carries a Reference
// Input for each, and references the env-config Function by the supplied
// (Crossplane-derived) name.
func TestGenerateComposition_EnvironmentConfigs(t *testing.T) {
	xrd := common.FixtureXRD(t)

	t.Run("none omits the env step", func(t *testing.T) {
		comp, err := pkg.GenerateComposition(xrd, pkg.CompositionInput{Module: "cuefn.example/app@v0.1.0"})
		require.NoError(t, err)
		require.Len(t, comp.Spec.Pipeline, 1)
		assert.Equal(t, "cuefn", comp.Spec.Pipeline[0].Step, "no env step when no EnvironmentConfigs requested")
	})

	t.Run("default env function name", func(t *testing.T) {
		comp, err := pkg.GenerateComposition(xrd, pkg.CompositionInput{
			Module:                "cuefn.example/app@v0.1.0",
			EnvironmentConfigRefs: []string{"app-environment"},
		})
		require.NoError(t, err)
		require.Len(t, comp.Spec.Pipeline, 2)
		assert.Equal(t, "function-environment-configs", comp.Spec.Pipeline[0].FunctionRef.Name)
	})

	t.Run("references", func(t *testing.T) {
		comp, err := pkg.GenerateComposition(xrd, pkg.CompositionInput{
			Module:                        "cuefn.example/app@v0.1.0",
			EnvironmentConfigRefs:         []string{"app-environment", "shared-env"},
			EnvironmentConfigFunctionName: "crossplane-contrib-function-environment-configs",
		})
		require.NoError(t, err)

		require.Len(t, comp.Spec.Pipeline, 2)
		step := comp.Spec.Pipeline[0]
		assert.Equal(t, "function-environment-configs", step.Step)
		assert.Equal(t, "crossplane-contrib-function-environment-configs", step.FunctionRef.Name)
		assert.Equal(t, "cuefn", comp.Spec.Pipeline[1].Step)
		require.NotNil(t, step.Input, "env-config step must carry an Input when refs are given")

		var in struct {
			APIVersion string `json:"apiVersion"`
			Kind       string `json:"kind"`
			Spec       struct {
				EnvironmentConfigs []struct {
					Type string `json:"type"`
					Ref  struct {
						Name string `json:"name"`
					} `json:"ref"`
				} `json:"environmentConfigs"`
			} `json:"spec"`
		}
		require.NoError(t, json.Unmarshal(step.Input.Raw, &in))
		assert.Equal(t, "environmentconfigs.fn.crossplane.io/v1beta1", in.APIVersion)
		assert.Equal(t, "Input", in.Kind)
		require.Len(t, in.Spec.EnvironmentConfigs, 2)
		assert.Equal(t, "Reference", in.Spec.EnvironmentConfigs[0].Type)
		assert.Equal(t, "app-environment", in.Spec.EnvironmentConfigs[0].Ref.Name)
		assert.Equal(t, "shared-env", in.Spec.EnvironmentConfigs[1].Ref.Name)
	})

	t.Run("selectors follow references", func(t *testing.T) {
		comp, err := pkg.GenerateComposition(xrd, pkg.CompositionInput{
			Module:                "cuefn.example/app@v0.1.0",
			EnvironmentConfigRefs: []string{"environment"},
			EnvironmentConfigSelectors: []pkg.EnvironmentConfigSelector{{
				MatchLabels: []pkg.EnvironmentConfigLabelMatch{
					{Key: "example.io/name", ValueFromFieldPath: "metadata.name"},
					{Key: "example.io/namespace", ValueFromFieldPath: "metadata.namespace"},
				},
			}},
		})
		require.NoError(t, err)

		require.Len(t, comp.Spec.Pipeline, 2)
		step := comp.Spec.Pipeline[0]
		require.NotNil(t, step.Input)

		var in struct {
			Spec struct {
				EnvironmentConfigs []struct {
					Type string `json:"type"`
					Ref  struct {
						Name string `json:"name"`
					} `json:"ref"`
					Selector struct {
						Mode        string `json:"mode"`
						MatchLabels []struct {
							Type               string `json:"type"`
							Key                string `json:"key"`
							ValueFromFieldPath string `json:"valueFromFieldPath"`
						} `json:"matchLabels"`
					} `json:"selector"`
				} `json:"environmentConfigs"`
			} `json:"spec"`
		}
		require.NoError(t, json.Unmarshal(step.Input.Raw, &in))
		require.Len(t, in.Spec.EnvironmentConfigs, 2)
		assert.Equal(
			t,
			"Reference",
			in.Spec.EnvironmentConfigs[0].Type,
			"references come first so selector data merges over them",
		)
		assert.Equal(t, "environment", in.Spec.EnvironmentConfigs[0].Ref.Name)

		sel := in.Spec.EnvironmentConfigs[1]
		assert.Equal(t, "Selector", sel.Type)
		assert.Equal(t, "Single", sel.Selector.Mode)
		require.Len(t, sel.Selector.MatchLabels, 2)
		assert.Equal(t, "FromCompositeFieldPath", sel.Selector.MatchLabels[0].Type)
		assert.Equal(t, "example.io/name", sel.Selector.MatchLabels[0].Key)
		assert.Equal(t, "metadata.name", sel.Selector.MatchLabels[0].ValueFromFieldPath)
		assert.Equal(t, "example.io/namespace", sel.Selector.MatchLabels[1].Key)
		assert.Equal(t, "metadata.namespace", sel.Selector.MatchLabels[1].ValueFromFieldPath)
	})

	t.Run("selector only emits the env step", func(t *testing.T) {
		comp, err := pkg.GenerateComposition(xrd, pkg.CompositionInput{
			Module: "cuefn.example/app@v0.1.0",
			EnvironmentConfigSelectors: []pkg.EnvironmentConfigSelector{{
				MatchLabels: []pkg.EnvironmentConfigLabelMatch{
					{Key: "example.io/name", ValueFromFieldPath: "metadata.name"},
				},
			}},
		})
		require.NoError(t, err)
		require.Len(t, comp.Spec.Pipeline, 2)
		assert.Equal(t, "function-environment-configs", comp.Spec.Pipeline[0].Step)
	})
}

// TestGenerateComposition_Errors proves malformed inputs surface clear errors
// instead of panicking.
func TestGenerateComposition_Errors(t *testing.T) {
	t.Run("nil xrd", func(t *testing.T) {
		_, err := pkg.GenerateComposition(nil, pkg.CompositionInput{Module: "x@v1"})
		require.Error(t, err)
	})

	t.Run("empty module", func(t *testing.T) {
		_, err := pkg.GenerateComposition(common.FixtureXRD(t), pkg.CompositionInput{})
		require.Error(t, err)
	})

	t.Run("selector without matchers", func(t *testing.T) {
		_, err := pkg.GenerateComposition(common.FixtureXRD(t), pkg.CompositionInput{
			Module:                     "x@v1",
			EnvironmentConfigSelectors: []pkg.EnvironmentConfigSelector{{}},
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "label matcher")
	})

	t.Run("selector with blank field path", func(t *testing.T) {
		_, err := pkg.GenerateComposition(common.FixtureXRD(t), pkg.CompositionInput{
			Module: "x@v1",
			EnvironmentConfigSelectors: []pkg.EnvironmentConfigSelector{{
				MatchLabels: []pkg.EnvironmentConfigLabelMatch{{Key: "example.io/name"}},
			}},
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "field path")
	})
}
