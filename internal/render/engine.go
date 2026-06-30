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
// The engine itself is pure: a [ModuleLoader] port abstracts where the module
// bytes come from. Two adapters ship here — [LocalLoader] serves a fixed
// directory (offline for a self-contained module, or resolving dependencies
// through a registry via [NewLocalLoader]), and [OCILoader] fetches a module
// (and its transitive CUE dependencies) from an OCI registry per CUE_REGISTRY.
// The gRPC function and codegen live in other packages.
package render

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/errors"

	"github.com/crossplane/function-sdk-go/resource"

	"github.com/meigma/crossplane-cuefn/internal/cueerr"
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

	// RequiredResources holds the cluster objects Crossplane fetched for the
	// requirements this module emitted on a previous pass, keyed by the author's
	// requirement name. An empty list means "requested but none found". omitempty
	// keeps it off out.input before any requirement is delivered.
	RequiredResources map[string][]map[string]any `json:"requiredResources,omitempty"`
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

	// Requirements holds the selectors the module emitted under out.requirements,
	// keyed by requirement name. nil when the module declares none.
	Requirements map[string]Requirement
}

// Requirement is one entry of out.requirements: a selector the engine returns
// for Crossplane to fetch. Exactly one of MatchName/MatchLabels is set, enforced
// at render time by [readRequirements].
type Requirement struct {
	APIVersion  string            `json:"apiVersion"`
	Kind        string            `json:"kind"`
	MatchName   string            `json:"matchName,omitempty"`
	MatchLabels map[string]string `json:"matchLabels,omitempty"`
	Namespace   string            `json:"namespace,omitempty"`
}

// Engine renders CUE modules into composed resources, readiness, and status.
type Engine struct {
	loader ModuleLoader
}

// New returns an Engine that loads modules with the given loader.
func New(loader ModuleLoader) *Engine {
	return &Engine{loader: loader}
}

// Render loads the module at ref, fills its out.input field with in (projecting
// Crossplane-reserved keys out of the spec), and returns the composed resources,
// readiness, and status it produces. It errors if the module is missing, fails
// to evaluate, violates its #Spec, or leaves resources or status non-concrete.
//
// A module nests its transform under a single top-level `out` field
// (out.input/out.resources/out.status) — the module contract. The schema
// definitions (#API/#Spec/#Status) stay top-level and are not read here.
func (e *Engine) Render(ctx context.Context, ref string, in Inputs) (Result, error) {
	v, cleanup, err := LoadModule(ctx, e.loader, ref)
	if err != nil {
		return Result{}, err
	}
	defer cleanup()

	// Catch a module that is missing the `out` wrapper with a clear message
	// rather than a downstream "no usable resources" error.
	if !v.LookupPath(cue.ParsePath("out")).Exists() {
		return Result{}, errors.New(
			"module has no `out` field: nest the transform (input, resources, status) " +
				"under a top-level `out` field")
	}

	v, err = fillInput(v.Context(), v, in)
	if err != nil {
		return Result{}, err
	}

	// Read the emitted requirements (a pure function of stable inputs) before
	// resources, then seed an empty bucket for every declared requirement name so
	// a data-dependent guard on input.requiredResources[name] is concrete on the
	// first pass. Re-fill only when seeding actually added keys.
	requirements, err := readRequirements(v)
	if err != nil {
		return Result{}, err
	}

	if seeded := seedRequiredResources(in.RequiredResources, requirements); seeded != nil {
		in.RequiredResources = seeded
		v, err = fillInput(v.Context(), v, in)
		if err != nil {
			return Result{}, err
		}
	}

	resources, err := readResources(v)
	if err != nil {
		return Result{}, err
	}

	status, err := readStatus(v)
	if err != nil {
		return Result{}, err
	}

	return Result{Resources: resources, Status: status, Requirements: requirements}, nil
}

// fillInput projects the observed spec, fills the module's out.input field via
// JSON marshalling, and validates that the filled input is concrete. Filling via
// JSON (rather than Go encoding) renders an integral spec value such as a float64
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
		return cue.Value{}, cueerr.Wrap(err, "cannot compile inputs")
	}

	v = v.FillPath(cue.ParsePath("out.input"), inVal)

	input := v.LookupPath(cue.ParsePath("out.input"))
	if err = input.Validate(cue.Concrete(true)); err != nil {
		return cue.Value{}, cueerr.Wrap(err, "inputs do not satisfy module #Spec")
	}
	return v, nil
}

// rawResource is the decode target for one resources map entry. The json tags
// match the module contract's object and optional ready fields.
type rawResource struct {
	Object map[string]any `json:"object"`
	Ready  string         `json:"ready"`
}

// readResources reads the author-keyed out.resources map, validates that every
// entry is concrete, and decodes it into the keyed Result resources with
// readiness mapped to the SDK enum.
func readResources(v cue.Value) (map[string]Resource, error) {
	res := v.LookupPath(cue.ParsePath("out.resources"))
	if err := res.Err(); err != nil {
		return nil, cueerr.Wrap(err, "module has no usable `out.resources` field")
	}
	if err := res.Validate(cue.Concrete(true)); err != nil {
		return nil, cueerr.Wrap(err, "`resources` did not fully evaluate")
	}

	var raw map[string]rawResource
	if err := res.Decode(&raw); err != nil {
		return nil, cueerr.Wrap(err, "cannot decode `resources`")
	}

	out := make(map[string]Resource, len(raw))
	for name, r := range raw {
		out[name] = Resource{Object: r.Object, Ready: toReady(r.Ready)}
	}
	return out, nil
}

// readStatus reads the optional out.status field. It returns nil when the
// module returns no status, and an error when status is present but non-concrete.
func readStatus(v cue.Value) (map[string]any, error) {
	status := v.LookupPath(cue.ParsePath("out.status"))
	if !status.Exists() {
		return nil, nil //nolint:nilnil // absent status is a valid, empty result.
	}
	if err := status.Validate(cue.Concrete(true)); err != nil {
		return nil, cueerr.Wrap(err, "`status` did not fully evaluate")
	}

	var out map[string]any
	if err := status.Decode(&out); err != nil {
		return nil, cueerr.Wrap(err, "cannot decode `status`")
	}
	return out, nil
}

// readRequirements reads the optional out.requirements map: the selectors the
// module emits for Crossplane to fetch. It returns nil when the module declares
// none, errors when the field is present but non-concrete, and enforces that
// each requirement sets exactly one of matchName or matchLabels (the single
// enforcement point both the function adapter and the CLI then trust).
func readRequirements(v cue.Value) (map[string]Requirement, error) {
	req := v.LookupPath(cue.ParsePath("out.requirements"))
	if !req.Exists() {
		return nil, nil //nolint:nilnil // a module that needs nothing is valid.
	}
	if err := req.Validate(cue.Concrete(true)); err != nil {
		return nil, cueerr.Wrap(err, "`requirements` did not fully evaluate")
	}

	var out map[string]Requirement
	if err := req.Decode(&out); err != nil {
		return nil, cueerr.Wrap(err, "cannot decode `requirements`")
	}

	for name, r := range out {
		if (r.MatchName != "") == (len(r.MatchLabels) > 0) { // neither or both
			return nil, fmt.Errorf(
				"requirement %q must set exactly one of matchName or matchLabels", name)
		}
	}
	return out, nil
}

// seedRequiredResources fills a non-nil empty bucket for every requirement name
// not already present in existing, so a data-dependent guard on
// input.requiredResources[name] collapses to a concrete empty list on the first
// pass. It copies existing first and returns nil when nothing was added, so the
// no-requirements path does exactly one fill and never mutates the caller's map.
func seedRequiredResources(
	existing map[string][]map[string]any,
	reqs map[string]Requirement,
) map[string][]map[string]any {
	var out map[string][]map[string]any
	for name := range reqs {
		if _, ok := existing[name]; ok {
			continue
		}
		if out == nil {
			out = make(map[string][]map[string]any, len(reqs))
			maps.Copy(out, existing)
		}
		out[name] = []map[string]any{} // non-nil empty list -> concrete cfg: []
	}
	return out
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
