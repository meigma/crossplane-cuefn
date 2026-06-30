package schema

import (
	"encoding/json"
	"fmt"

	"cuelang.org/go/cue"

	"github.com/meigma/crossplane-cuefn/internal/cueerr"
	"github.com/meigma/crossplane-cuefn/internal/render"
)

// Validate checks a populated XR spec against the module's #Spec using the same
// CUE evaluation the runtime engine uses to fill inputs. It projects the
// Crossplane-reserved keys out of spec, unifies the remainder with #Spec, and
// validates the result concretely — which applies #Spec defaults, enforces
// numeric bounds and enum membership, and reports missing required (!) fields.
//
// A valid spec (and a spec that omits a defaulted field) returns nil; a
// violation returns an error whose message includes the offending field path.
func Validate(module cue.Value, spec map[string]any) error {
	specSchema := module.LookupPath(cue.ParsePath("#Spec"))
	if err := specSchema.Err(); err != nil {
		return fmt.Errorf("module declares no usable #Spec: %w", err)
	}

	projected := render.ProjectSpec(spec)
	// Marshal via JSON so an integral value such as a replicas count renders as
	// an integer and unifies against a bounded int #Spec field (matching the
	// engine's fillInput).
	raw, err := json.Marshal(projected)
	if err != nil {
		return fmt.Errorf("cannot marshal XR spec: %w", err)
	}

	specVal := module.Context().CompileBytes(raw)
	if err := specVal.Err(); err != nil {
		return cueerr.Wrap(err, "cannot compile XR spec")
	}

	unified := specSchema.Unify(specVal)
	if err := unified.Validate(cue.Concrete(true)); err != nil {
		return cueerr.Wrap(err, "XR spec does not satisfy #Spec")
	}
	return nil
}
