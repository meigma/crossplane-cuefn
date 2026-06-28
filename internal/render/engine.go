// Package render evaluates a CUE module from a local directory against a curated
// set of inputs and returns the Kubernetes resources, readiness, and status it
// produces.
//
// The contract with a module is intentionally narrow. The engine fills a
// top-level input field with the observed XR's spec, metadata, and environment,
// then reads an author-keyed resources map (each entry an object plus an
// optional readiness hint) and an optional top-level status. Module authors
// never see Crossplane's request/response internals.
//
// This package is pure and offline: a [ModuleLoader] port abstracts where the
// module bytes come from, and the only adapter shipped here is [LocalLoader],
// which serves a fixed directory. OCI loading, the gRPC function, and codegen
// live in other packages.
package render

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
	"cuelang.org/go/cue/errors"
	"cuelang.org/go/cue/load"

	"github.com/crossplane/function-sdk-go/resource"
)

// Metadata is the subset of an XR's metadata exposed to a module.
type Metadata struct {
	// Name is the composite resource's name.
	Name string `json:"name,omitempty"`

	// Namespace is the composite resource's namespace.
	Namespace string `json:"namespace,omitempty"`
}

// Inputs are the curated values a module sees under its top-level input field.
type Inputs struct {
	// Spec is the observed composite resource's user spec. The engine projects
	// out Crossplane-reserved keys before unifying it against the module #Spec.
	Spec map[string]any `json:"spec,omitempty"`

	// Metadata is the observed composite resource's identifying metadata.
	Metadata Metadata `json:"metadata"`

	// Environment is the merged EnvironmentConfig data from the pipeline context,
	// empty when no environment was supplied.
	Environment map[string]any `json:"environment,omitempty"`
}

// Resource is a single composed resource produced by a module: a finished
// Kubernetes object plus the readiness the module assigned it.
type Resource struct {
	// Object is the rendered Kubernetes object as an unstructured map.
	Object map[string]any

	// Ready is the readiness the module assigned. An absent module hint maps to
	// [resource.ReadyUnspecified].
	Ready resource.Ready
}

// Result is the decoded output of a render: the composed resources keyed by the
// author's stable name, and the optional status the module returned.
type Result struct {
	// Resources holds the composed resources keyed by the author's map key
	// verbatim (the Crossplane composed-resource name).
	Resources map[string]Resource

	// Status is the status the module returned, or nil when it returned none.
	Status map[string]any
}

// Engine renders CUE modules into composed resources, readiness, and status.
type Engine struct {
	loader ModuleLoader
}

// New returns an Engine that loads modules with the given loader.
func New(loader ModuleLoader) *Engine {
	return &Engine{loader: loader}
}

// Render loads the module at ref, fills its input field with in (projecting
// Crossplane-reserved keys out of the spec), and returns the composed resources,
// readiness, and status it produces. It errors if the module is missing, fails
// to evaluate, violates its #Spec, or leaves resources or status non-concrete.
func (e *Engine) Render(ctx context.Context, ref string, in Inputs) (Result, error) {
	dir, cleanup, err := e.loader.Load(ctx, ref)
	if err != nil {
		return Result{}, fmt.Errorf("cannot load module %q: %w", ref, err)
	}
	defer cleanup()

	cctx := cuecontext.New()

	insts := load.Instances([]string{"."}, &load.Config{Dir: dir})
	if len(insts) == 0 {
		return Result{}, fmt.Errorf("module %q contains no CUE instances", ref)
	}
	if err = insts[0].Err; err != nil {
		return Result{}, wrapCUE(err, "cannot load module %q", ref)
	}

	v := cctx.BuildInstance(insts[0])
	if err = v.Err(); err != nil {
		return Result{}, wrapCUE(err, "cannot build module %q", ref)
	}

	v, err = fillInput(cctx, v, in)
	if err != nil {
		return Result{}, err
	}

	resources, err := readResources(v)
	if err != nil {
		return Result{}, err
	}

	status, err := readStatus(v)
	if err != nil {
		return Result{}, err
	}

	return Result{Resources: resources, Status: status}, nil
}

// fillInput projects the observed spec, fills the module's input field via JSON
// marshalling, and validates that the filled input is concrete. Filling via JSON
// (rather than Go encoding) renders an integral spec value such as a float64
// replicas count as an integer, so it unifies against a bounded int #Spec field;
// validating the result surfaces #Spec bound violations as errors here.
func fillInput(cctx *cue.Context, v cue.Value, in Inputs) (cue.Value, error) {
	in.Spec = ProjectSpec(in.Spec)

	inJSON, err := json.Marshal(in)
	if err != nil {
		return cue.Value{}, fmt.Errorf("cannot marshal inputs: %w", err)
	}

	inVal := cctx.CompileBytes(inJSON)
	if err = inVal.Err(); err != nil {
		return cue.Value{}, wrapCUE(err, "cannot compile inputs")
	}

	v = v.FillPath(cue.ParsePath("input"), inVal)

	input := v.LookupPath(cue.ParsePath("input"))
	if err = input.Validate(cue.Concrete(true)); err != nil {
		return cue.Value{}, wrapCUE(err, "inputs do not satisfy module #Spec")
	}
	return v, nil
}

// rawResource is the decode target for one resources map entry. The json tags
// match the module contract's object and optional ready fields.
type rawResource struct {
	Object map[string]any `json:"object"`
	Ready  string         `json:"ready"`
}

// readResources reads the author-keyed resources map, validates that every entry
// is concrete, and decodes it into the keyed Result resources with readiness
// mapped to the SDK enum.
func readResources(v cue.Value) (map[string]Resource, error) {
	res := v.LookupPath(cue.ParsePath("resources"))
	if err := res.Err(); err != nil {
		return nil, wrapCUE(err, "module has no usable `resources` field")
	}
	if err := res.Validate(cue.Concrete(true)); err != nil {
		return nil, wrapCUE(err, "`resources` did not fully evaluate")
	}

	var raw map[string]rawResource
	if err := res.Decode(&raw); err != nil {
		return nil, wrapCUE(err, "cannot decode `resources`")
	}

	out := make(map[string]Resource, len(raw))
	for name, r := range raw {
		out[name] = Resource{Object: r.Object, Ready: toReady(r.Ready)}
	}
	return out, nil
}

// readStatus reads the optional top-level status field. It returns nil when the
// module returns no status, and an error when status is present but non-concrete.
func readStatus(v cue.Value) (map[string]any, error) {
	status := v.LookupPath(cue.ParsePath("status"))
	if !status.Exists() {
		return nil, nil //nolint:nilnil // absent status is a valid, empty result.
	}
	if err := status.Validate(cue.Concrete(true)); err != nil {
		return nil, wrapCUE(err, "`status` did not fully evaluate")
	}

	var out map[string]any
	if err := status.Decode(&out); err != nil {
		return nil, wrapCUE(err, "cannot decode `status`")
	}
	return out, nil
}

// toReady maps a module readiness hint to the SDK enum: "Ready" becomes
// [resource.ReadyTrue], "NotReady" becomes [resource.ReadyFalse], and an absent
// or empty hint becomes [resource.ReadyUnspecified].
func toReady(hint string) resource.Ready {
	switch hint {
	case "Ready":
		return resource.ReadyTrue
	case "NotReady":
		return resource.ReadyFalse
	default:
		return resource.ReadyUnspecified
	}
}

// wrapCUE wraps a CUE error with a message and appends [errors.Details] so the
// offending field path or key appears in the surfaced message.
func wrapCUE(err error, format string, args ...any) error {
	msg := fmt.Sprintf(format, args...)
	details := strings.TrimRight(errors.Details(err, nil), "\n")
	if details == "" {
		return fmt.Errorf("%s: %w", msg, err)
	}
	return fmt.Errorf("%s: %w\n%s", msg, err, details)
}
