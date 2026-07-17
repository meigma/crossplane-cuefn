package snapshot

import (
	"context"
	"fmt"
	"reflect"

	"github.com/meigma/crossplane-cuefn/internal/render"
)

// RenderWithRequiredObjects renders ref and, when the module emits requirements
// and objs supplies cluster objects, delivers the matches and re-renders.
// Requirements are by design a pure function of stable inputs, so the offline
// loop provably converges in exactly two passes: render to discover the emitted
// selectors, match the supplied objects against them, then re-render with the
// matched objects delivered. Stabilization is asserted the way Crossplane does
// rather than silently returning a bogus render.
func RenderWithRequiredObjects(
	ctx context.Context,
	engine *render.Engine,
	ref string,
	inputs render.Inputs,
	objs []map[string]any,
) (render.Result, error) {
	result, err := engine.Render(ctx, ref, inputs)
	if err != nil {
		return render.Result{}, fmt.Errorf("cannot render module %q: %w", ref, err)
	}

	if len(objs) > 0 && len(result.Requirements) > 0 {
		inputs.RequiredResources = MatchRequirements(objs, result.Requirements)
		second, err := engine.Render(ctx, ref, inputs)
		if err != nil {
			return render.Result{}, fmt.Errorf("cannot render module %q: %w", ref, err)
		}
		if !reflect.DeepEqual(second.Requirements, result.Requirements) {
			return render.Result{}, fmt.Errorf("requirements did not stabilize for module %q: "+
				"out.requirements must be a pure function of stable inputs", ref)
		}
		result = second
	}

	return result, nil
}
