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

	// envSourceTypeField discriminates the entries in the env-config Input:
	// environmentConfigs sources (Reference/Selector) and their label matchers.
	envSourceTypeField = "type"
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
	// installed Configuration's dependsOn entry produces — see DerivedFunctionName.
	FunctionName string
	// EnvironmentConfigRefs are the names of EnvironmentConfigs the
	// function-environment-configs step merges into the pipeline context (by
	// Reference) before cuefn evaluates the module. When empty the step is omitted
	// entirely, so a default install needs only the cuefn Function. Each name
	// becomes a `type: Reference` entry in the step's Input.
	EnvironmentConfigRefs []string
	// EnvironmentConfigSelectors select additional EnvironmentConfigs by labels
	// whose values are read from the composite resource at render time. Each
	// selector becomes a `type: Selector` entry (mode Single) after the Reference
	// entries, so its data merges over theirs. Single mode fails the render when
	// zero or multiple EnvironmentConfigs match.
	EnvironmentConfigSelectors []EnvironmentConfigSelector
	// EnvironmentConfigFunctionName is the in-cluster Function resource name the
	// function-environment-configs step references. Like FunctionName it must match
	// the auto-installed Function name (DerivedFunctionName of the env-config
	// function package). Only used when EnvironmentConfigRefs or
	// EnvironmentConfigSelectors is non-empty; defaults to
	// "function-environment-configs" when blank.
	EnvironmentConfigFunctionName string
}

// EnvironmentConfigSelector selects exactly one EnvironmentConfig by labels
// (function-environment-configs Selector source, mode Single).
type EnvironmentConfigSelector struct {
	// MatchLabels are the label matchers the selected EnvironmentConfig must
	// satisfy. At least one is required.
	MatchLabels []EnvironmentConfigLabelMatch
}

// EnvironmentConfigLabelMatch matches one EnvironmentConfig label against a
// value read from the composite resource (FromCompositeFieldPath).
type EnvironmentConfigLabelMatch struct {
	// Key is the label key on the EnvironmentConfig.
	Key string
	// ValueFromFieldPath is the composite-resource field path whose value the
	// label must equal (for example "metadata.name").
	ValueFromFieldPath string
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

	// The function-environment-configs step is emitted only when EnvironmentConfigs
	// are requested. Emitting it unconditionally (as before) put a step in every
	// Composition whose Function the Configuration never declared as a dependency,
	// so the first reconcile failed "cannot find an active FunctionRevision"; and
	// when no refs were selected the step merged nothing — a silent no-op.
	var pipeline []apiextv1.PipelineStep
	if refs := nonBlank(in.EnvironmentConfigRefs); len(refs) > 0 || len(in.EnvironmentConfigSelectors) > 0 {
		envStep, err := envConfigPipelineStep(in.EnvironmentConfigFunctionName, refs, in.EnvironmentConfigSelectors)
		if err != nil {
			return nil, err
		}
		pipeline = append(pipeline, envStep)
	}
	pipeline = append(pipeline, cuefnStep)

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
			Mode:     apiextv1.CompositionModePipeline,
			Pipeline: pipeline,
		},
	}
	return comp, nil
}

// nonBlank returns the non-empty, trimmed entries of refs.
func nonBlank(refs []string) []string {
	out := make([]string, 0, len(refs))
	for _, r := range refs {
		if strings.TrimSpace(r) != "" {
			out = append(out, r)
		}
	}
	return out
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

// envConfigPipelineStep builds the function-environment-configs step, selecting
// each EnvironmentConfig in refs by Reference and each selector by labels
// (mode Single) so the step merges them into the pipeline context for cuefn.
// References come first, then selectors, so selector data merges over reference
// data. funcName is the in-cluster Function resource name the step references;
// it must match the auto-installed env-config Function name (it defaults to
// envConfigFunctionName when blank). refs and selectors must not both be empty.
func envConfigPipelineStep(
	funcName string,
	refs []string,
	selectors []EnvironmentConfigSelector,
) (apiextv1.PipelineStep, error) {
	if funcName == "" {
		funcName = envConfigFunctionName
	}
	step := apiextv1.PipelineStep{
		Step:        envConfigStepName,
		FunctionRef: apiextv1.FunctionReference{Name: funcName},
	}

	configs := make([]map[string]any, 0, len(refs)+len(selectors))
	for _, name := range refs {
		configs = append(configs, map[string]any{
			envSourceTypeField: "Reference",
			"ref":              map[string]any{"name": name},
		})
	}
	for i, sel := range selectors {
		if len(sel.MatchLabels) == 0 {
			return apiextv1.PipelineStep{}, fmt.Errorf(
				"environment-config selector %d requires at least one label matcher", i)
		}
		labels := make([]map[string]any, 0, len(sel.MatchLabels))
		for _, m := range sel.MatchLabels {
			if strings.TrimSpace(m.Key) == "" || strings.TrimSpace(m.ValueFromFieldPath) == "" {
				return apiextv1.PipelineStep{}, fmt.Errorf(
					"environment-config selector %d requires a label key and a composite field path", i)
			}
			labels = append(labels, map[string]any{
				envSourceTypeField:   "FromCompositeFieldPath",
				"key":                m.Key,
				"valueFromFieldPath": m.ValueFromFieldPath,
			})
		}
		configs = append(configs, map[string]any{
			envSourceTypeField: "Selector",
			"selector": map[string]any{
				"mode":        "Single",
				"matchLabels": labels,
			},
		})
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
