package testharness

import (
	"encoding/json"
	"fmt"

	"cuelang.org/go/cue"
	"github.com/crossplane/function-sdk-go/resource"
	"sigs.k8s.io/yaml"

	"github.com/meigma/crossplane-cuefn/internal/render"
)

// Field keys of the normalized document.
const (
	keyReady = "ready"
	keyKind  = "kind"
)

// normalizeResult flattens a render result into the one concrete document all
// expectations are evaluated against:
//
//	resources: {<name>: {ready: "Ready"|"NotReady"|"Unspecified", object: {...}}}
//	status:       {...} | null   (explicit null when the module returns none)
//	requirements: {<name>: {...}} (always present, {} when none emitted)
//
// Absences are explicit (status: null, requirements: {}) because open-struct
// unification cannot assert a missing field, but it can assert null or an
// empty struct. Readiness uses the module contract's author vocabulary
// ("Ready"/"NotReady") plus "Unspecified", not the SDK enum's wire values.
func normalizeResult(res render.Result) map[string]any {
	resources := make(map[string]any, len(res.Resources))
	for name, r := range res.Resources {
		resources[name] = map[string]any{
			keyReady: readyString(r.Ready),
			"object": r.Object,
		}
	}

	requirements := make(map[string]any, len(res.Requirements))
	for name, req := range res.Requirements {
		entry := map[string]any{
			"apiVersion": req.APIVersion,
			keyKind:      req.Kind,
		}
		if req.MatchName != "" {
			entry["matchName"] = req.MatchName
		}
		if len(req.MatchLabels) > 0 {
			entry["matchLabels"] = req.MatchLabels
		}
		if req.Namespace != "" {
			entry["namespace"] = req.Namespace
		}
		requirements[name] = entry
	}

	doc := map[string]any{
		"resources":    resources,
		"requirements": requirements,
	}
	if res.Status == nil {
		doc["status"] = nil
	} else {
		doc["status"] = res.Status
	}
	return doc
}

// readyString maps the SDK readiness enum onto the harness vocabulary.
func readyString(r resource.Ready) string {
	switch r {
	case resource.ReadyTrue:
		return "Ready"
	case resource.ReadyFalse:
		return "NotReady"
	case resource.ReadyUnspecified:
		return "Unspecified"
	default:
		return "Unspecified"
	}
}

// marshalNormalized renders the normalized document as deterministic YAML —
// the golden (want.yaml) representation.
func marshalNormalized(doc map[string]any) ([]byte, error) {
	data, err := yaml.Marshal(doc)
	if err != nil {
		return nil, fmt.Errorf("cannot marshal normalized result: %w", err)
	}
	return data, nil
}

// encodeNormalized compiles the normalized document into a CUE value for
// unification with want.cue. It goes through JSON — the same trick the engine
// uses to fill out.input — so integral numbers stay integers and nil becomes
// null.
func encodeNormalized(cctx *cue.Context, doc map[string]any) (cue.Value, error) {
	data, err := json.Marshal(doc)
	if err != nil {
		return cue.Value{}, fmt.Errorf("cannot encode normalized result: %w", err)
	}
	v := cctx.CompileBytes(data)
	if err := v.Err(); err != nil {
		return cue.Value{}, fmt.Errorf("cannot encode normalized result: %w", err)
	}
	return v, nil
}
