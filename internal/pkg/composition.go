package pkg

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	apiextv1 "github.com/crossplane/crossplane/apis/v2/apiextensions/v1"
	xv2 "github.com/crossplane/crossplane/apis/v2/apiextensions/v2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	inputv1beta1 "github.com/meigma/crossplane-cuefn/input/v1beta1"
)

const (
	// compositionAPIVersion is the apiVersion of a Crossplane Composition.
	compositionAPIVersion = "apiextensions.crossplane.io/v1"
	// compositionKind is the kind of a Crossplane Composition.
	compositionKind = "Composition"

	// envConfigStepName is the pipeline step that merges referenced
	// EnvironmentConfigs into the pipeline context before cuefn runs.
	envConfigStepName = "function-environment-configs"
	// envConfigFunctionName is the in-cluster Function resource the env-config
	// step references.
	envConfigFunctionName = "function-environment-configs"

	// cuefnStepName is the pipeline step that fetches and evaluates the CUE
	// module.
	cuefnStepName = "cuefn"

	// cuefnInputAPIVersion / cuefnInputKind identify the cuefn step's Input
	// object. They must match the type the runtime decodes (input/v1beta1.Input).
	cuefnInputAPIVersion = "cuefn.meigma.io/v1beta1"
	cuefnInputKind       = "Input"

	// envConfigInputAPIVersion / envConfigInputKind identify the Input the
	// function-environment-configs step consumes to select EnvironmentConfigs.
	envConfigInputAPIVersion = "environmentconfigs.fn.crossplane.io/v1beta1"
	envConfigInputKind       = "Input"
)

// CompositionInput is the author half of the schema<->runtime digest lock-step:
// the semver module ref the cuefn function fetches, and the exact OCI manifest
// digest it must resolve to. Both are embedded in the cuefn pipeline step's Input
// so the runtime loader (OCIConfig.Expect) can verify the module has not drifted.
type CompositionInput struct {
	// Module is the CUE module ref in "path@version" semver form.
	Module string
	// ExpectedDigest is the resolved OCI manifest digest ("sha256:...") the
	// module must match at render time.
	ExpectedDigest string
	// FunctionName is the in-cluster Function resource name the cuefn step
	// references (the functionRef.name). It must match the Function name the
	// installed Configuration's dependsOn entry produces.
	FunctionName string
	// EnvironmentConfigRefs are the names of EnvironmentConfigs the
	// function-environment-configs step merges into the pipeline context (by
	// Reference) before cuefn evaluates the module. When empty the step is still
	// present but selects nothing, so cuefn sees an empty environment. Each name
	// becomes a `type: Reference` entry in the step's Input.
	EnvironmentConfigRefs []string
}

// GenerateComposition builds a pipeline-mode Composition for xrd. Its
// compositeTypeRef is taken from the XRD's group, referenceable version, and
// kind. The pipeline runs function-environment-configs (step 1, to surface
// EnvironmentConfigs to the module) then cuefn (step 2, carrying the module ref
// and expected digest from in). The Composition is named after the XRD's plural.
func GenerateComposition(xrd *xv2.CompositeResourceDefinition, in CompositionInput) (*apiextv1.Composition, error) {
	if xrd == nil {
		return nil, errors.New("xrd must not be nil")
	}
	if strings.TrimSpace(in.Module) == "" {
		return nil, errors.New("composition input requires a module ref")
	}

	apiVersion, kind, err := compositeTypeRef(xrd)
	if err != nil {
		return nil, err
	}

	cuefnStep, err := cuefnPipelineStep(in)
	if err != nil {
		return nil, err
	}

	envStep, err := envConfigPipelineStep(in.EnvironmentConfigRefs)
	if err != nil {
		return nil, err
	}

	comp := &apiextv1.Composition{
		TypeMeta: metav1.TypeMeta{
			APIVersion: compositionAPIVersion,
			Kind:       compositionKind,
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: xrd.Spec.Names.Plural,
		},
		Spec: apiextv1.CompositionSpec{
			CompositeTypeRef: apiextv1.TypeReference{
				APIVersion: apiVersion,
				Kind:       kind,
			},
			Mode: apiextv1.CompositionModePipeline,
			Pipeline: []apiextv1.PipelineStep{
				envStep,
				cuefnStep,
			},
		},
	}
	return comp, nil
}

// compositeTypeRef derives the Composition's compositeTypeRef from the XRD: its
// group plus the referenceable version form the apiVersion, and the names kind
// the kind.
func compositeTypeRef(xrd *xv2.CompositeResourceDefinition) (string, string, error) {
	group := xrd.Spec.Group
	kind := xrd.Spec.Names.Kind
	if group == "" || kind == "" {
		return "", "", errors.New("xrd is missing spec.group or spec.names.kind")
	}

	version := referenceableVersion(xrd)
	if version == "" {
		return "", "", errors.New("xrd declares no referenceable version")
	}
	return group + "/" + version, kind, nil
}

// referenceableVersion returns the name of the XRD's referenceable version,
// falling back to the first served version, then the first version.
func referenceableVersion(xrd *xv2.CompositeResourceDefinition) string {
	versions := xrd.Spec.Versions
	for _, v := range versions {
		if v.Referenceable {
			return v.Name
		}
	}
	for _, v := range versions {
		if v.Served {
			return v.Name
		}
	}
	if len(versions) > 0 {
		return versions[0].Name
	}
	return ""
}

// envConfigPipelineStep builds the function-environment-configs step. When refs
// are supplied it carries an Input selecting each EnvironmentConfig by Reference,
// so the step merges them into the pipeline context for cuefn; with no refs the
// step is present but selects nothing.
func envConfigPipelineStep(refs []string) (apiextv1.PipelineStep, error) {
	step := apiextv1.PipelineStep{
		Step:        envConfigStepName,
		FunctionRef: apiextv1.FunctionReference{Name: envConfigFunctionName},
	}
	if len(refs) == 0 {
		return step, nil
	}

	configs := make([]map[string]any, 0, len(refs))
	for _, name := range refs {
		if strings.TrimSpace(name) == "" {
			continue
		}
		configs = append(configs, map[string]any{
			"type": "Reference",
			"ref":  map[string]any{"name": name},
		})
	}
	if len(configs) == 0 {
		return step, nil
	}

	input := map[string]any{
		"apiVersion": envConfigInputAPIVersion,
		"kind":       envConfigInputKind,
		"spec":       map[string]any{"environmentConfigs": configs},
	}
	raw, err := json.Marshal(input)
	if err != nil {
		return apiextv1.PipelineStep{}, fmt.Errorf("cannot marshal environment-configs input: %w", err)
	}
	step.Input = &runtime.RawExtension{Raw: raw}
	return step, nil
}

// cuefnPipelineStep builds the cuefn pipeline step, embedding the module ref and
// expected digest as an input/v1beta1.Input. Reusing that exact type keeps the
// author and runtime halves of the digest lock-step in sync at the type level.
func cuefnPipelineStep(in CompositionInput) (apiextv1.PipelineStep, error) {
	input := inputv1beta1.Input{
		TypeMeta: metav1.TypeMeta{
			APIVersion: cuefnInputAPIVersion,
			Kind:       cuefnInputKind,
		},
		Module:         in.Module,
		ExpectedDigest: in.ExpectedDigest,
	}
	raw, err := json.Marshal(input)
	if err != nil {
		return apiextv1.PipelineStep{}, fmt.Errorf("cannot marshal cuefn input: %w", err)
	}

	name := in.FunctionName
	if name == "" {
		name = cuefnStepName
	}

	return apiextv1.PipelineStep{
		Step:        cuefnStepName,
		FunctionRef: apiextv1.FunctionReference{Name: name},
		Input:       &runtime.RawExtension{Raw: raw},
	}, nil
}
