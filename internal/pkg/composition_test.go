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

// TestGenerateComposition_StepOrder proves the Composition is pipeline-mode with
// the env-config step first and the cuefn step second, and that its
// compositeTypeRef is derived from the XRD's group/version/kind.
func TestGenerateComposition_StepOrder(t *testing.T) {
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

	require.Len(t, comp.Spec.Pipeline, 2)
	assert.Equal(t, "function-environment-configs", comp.Spec.Pipeline[0].Step)
	assert.Equal(t, "function-environment-configs", comp.Spec.Pipeline[0].FunctionRef.Name)
	assert.Equal(t, "cuefn", comp.Spec.Pipeline[1].Step)
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

	step := comp.Spec.Pipeline[1]
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
	assert.Equal(t, "my-cuefn", withName.Spec.Pipeline[1].FunctionRef.Name)

	defaulted, err := pkg.GenerateComposition(xrd, pkg.CompositionInput{
		Module: "cuefn.example/app@v0.1.0",
	})
	require.NoError(t, err)
	assert.Equal(t, "cuefn", defaulted.Spec.Pipeline[1].FunctionRef.Name)
}

// TestGenerateComposition_EnvironmentConfigs proves the function-environment-configs
// step carries a Reference Input for each requested EnvironmentConfig, so its
// values reach the module under input.environment, and that the step carries no
// Input when none are requested (the default, backward-compatible shape).
func TestGenerateComposition_EnvironmentConfigs(t *testing.T) {
	xrd := common.FixtureXRD(t)

	t.Run("none", func(t *testing.T) {
		comp, err := pkg.GenerateComposition(xrd, pkg.CompositionInput{Module: "cuefn.example/app@v0.1.0"})
		require.NoError(t, err)
		assert.Nil(t, comp.Spec.Pipeline[0].Input, "env-config step must carry no Input when no refs are given")
	})

	t.Run("references", func(t *testing.T) {
		comp, err := pkg.GenerateComposition(xrd, pkg.CompositionInput{
			Module:                "cuefn.example/app@v0.1.0",
			EnvironmentConfigRefs: []string{"app-environment", "shared-env"},
		})
		require.NoError(t, err)

		step := comp.Spec.Pipeline[0]
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
}
