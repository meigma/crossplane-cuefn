package testharness

import (
	"context"
	"fmt"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"

	"github.com/meigma/crossplane-cuefn/internal/render"
	"github.com/meigma/crossplane-cuefn/internal/snapshot"
)

// Runner executes parsed cases against one module.
type Runner struct {
	// Loader supplies the module; Ref is the module reference passed to the
	// engine (ignored by local loaders).
	Loader render.ModuleLoader
	Ref    string
}

// UnitResult is the outcome of one unit execution.
type UnitResult struct {
	// Label mirrors Unit.Label.
	Label string
	// NeedsSeed is set when the unit declared no expectation; Golden then
	// carries the normalized output a seed would write.
	NeedsSeed bool
	// Golden is the normalized result as deterministic YAML. It is nil for
	// expected-error units and units whose render failed.
	Golden []byte
	// Failures are this unit's assertion failures, empty on success.
	Failures []Failure
}

// CaseResult is the outcome of one case: its units in execution order.
type CaseResult struct {
	Case  *Case
	Units []UnitResult
}

// Passed reports whether every unit ran without failures and without needing
// a seed.
func (r *CaseResult) Passed() bool {
	for _, u := range r.Units {
		if u.NeedsSeed || len(u.Failures) > 0 {
			return false
		}
	}
	return true
}

// NeedsSeed reports whether any unit lacks an expectation.
func (r *CaseResult) NeedsSeed() bool {
	for _, u := range r.Units {
		if u.NeedsSeed {
			return true
		}
	}
	return false
}

// Failures collects every unit's failures.
func (r *CaseResult) Failures() []Failure {
	var all []Failure
	for _, u := range r.Units {
		all = append(all, u.Failures...)
	}
	return all
}

// Run renders the case's units and evaluates their expectations. It returns
// an error for authoring mistakes that make the case unrunnable (malformed
// fixtures, observed snapshots against a module that never opted in);
// assertion mismatches and render failures are reported as unit Failures.
func (r *Runner) Run(ctx context.Context, c *Case) (*CaseResult, error) {
	base, objs, err := r.caseInputs(c)
	if err != nil {
		return nil, err
	}

	if err := r.checkObservedOptIn(ctx, c); err != nil {
		return nil, err
	}

	engine := render.New(r.Loader)
	cctx := cuecontext.New()
	result := &CaseResult{Case: c}

	for _, u := range c.Units {
		ur, err := r.runUnit(ctx, engine, cctx, c, u, base, objs)
		if err != nil {
			return nil, err
		}
		result.Units = append(result.Units, ur)
	}
	return result, nil
}

// runUnit renders one unit and evaluates its expectation. Authoring mistakes
// return an error; assertion mismatches land in the result's Failures.
func (r *Runner) runUnit(
	ctx context.Context,
	engine *render.Engine,
	cctx *cue.Context,
	c *Case,
	u Unit,
	base render.Inputs,
	objs []map[string]any,
) (UnitResult, error) {
	ur := UnitResult{Label: u.Label}
	failf := func(kind, msg string) {
		ur.Failures = append(ur.Failures, Failure{Unit: u.Label, Kind: kind, Message: msg})
	}

	in := base
	if len(u.Observed) > 0 {
		observed, err := parseObserved(u.Observed)
		if err != nil {
			return ur, fmt.Errorf("test case %q: %sobserved.yaml: %w", c.Name, u.SectionPrefix(), err)
		}
		in.ObservedResources = observed
	}

	res, renderErr := snapshot.RenderWithRequiredObjects(ctx, engine, r.Ref, in, objs)

	if u.ErrorSubstrings != nil {
		if msg := evalError(u.ErrorSubstrings, renderErr, resourceNames(res)); msg != "" {
			failf("error", msg)
		}
		return ur, nil
	}

	if renderErr != nil {
		failf("render", renderErr.Error())
		return ur, nil
	}

	doc := normalizeResult(res)
	golden, err := marshalNormalized(doc)
	if err != nil {
		return ur, fmt.Errorf("test case %q: %w", c.Name, err)
	}
	ur.Golden = golden

	if u.NeedsSeed() {
		ur.NeedsSeed = true
		return ur, nil
	}

	if u.WantYAML != nil {
		if msg := evalWantYAML(u.WantYAML, golden); msg != "" {
			failf("want.yaml", msg)
		}
	}
	if u.WantCUE != nil {
		actual, err := encodeNormalized(cctx, doc)
		if err != nil {
			return ur, fmt.Errorf("test case %q: %w", c.Name, err)
		}
		if msg := evalWantCUE(cctx, c, u, actual); msg != "" {
			failf("want.cue", msg)
		}
	}
	return ur, nil
}

// caseInputs builds the shared engine inputs (XR spec/metadata, environment)
// and the required-resource object bag from the case's base fixtures.
func (r *Runner) caseInputs(c *Case) (render.Inputs, []map[string]any, error) {
	fail := func(format string, args ...any) (render.Inputs, []map[string]any, error) {
		return render.Inputs{}, nil, fmt.Errorf("test case %q: %s", c.Name, fmt.Sprintf(format, args...))
	}

	xr, err := snapshot.ParseObject(c.XR)
	if err != nil {
		return fail("cannot parse xr.yaml: %v", err)
	}
	if len(xr) == 0 {
		return fail("xr.yaml is empty")
	}
	spec, ok := xr["spec"].(map[string]any)
	if !ok {
		return fail("xr.yaml must carry a spec object")
	}

	meta, _ := xr["metadata"].(map[string]any)
	name, _ := meta["name"].(string)
	namespace, _ := meta["namespace"].(string)

	in := render.Inputs{
		Spec:     spec,
		Metadata: render.Metadata{Name: name, Namespace: namespace},
	}

	if len(c.Environment) > 0 {
		env, envErr := snapshot.ParseObject(c.Environment)
		if envErr != nil {
			return fail("cannot parse environment.yaml: %v", envErr)
		}
		in.Environment = env
	}

	var objs []map[string]any
	if len(c.Required) > 0 {
		objs, err = snapshot.ParseObjects(c.Required)
		if err != nil {
			return fail("cannot parse required.yaml: %v", err)
		}
	}
	return in, objs, nil
}

// checkObservedOptIn rejects observed fixtures against a module that never
// materializes out.input.observedResources — the engine would silently ignore
// them, which is exactly the kind of quiet no-op the harness refuses.
func (r *Runner) checkObservedOptIn(ctx context.Context, c *Case) error {
	hasObserved := false
	for _, u := range c.Units {
		if len(u.Observed) > 0 {
			hasObserved = true
			break
		}
	}
	if !hasObserved {
		return nil
	}

	v, cleanup, err := render.LoadModule(ctx, r.Loader, r.Ref)
	if err != nil {
		return fmt.Errorf("test case %q: cannot load module: %w", c.Name, err)
	}
	defer cleanup()

	optedIn, err := render.UsesObservedResources(v)
	if err != nil {
		return fmt.Errorf("test case %q: %w", c.Name, err)
	}
	if !optedIn {
		return fmt.Errorf(
			"test case %q supplies observed.yaml, but the module does not declare "+
				"out.input.observedResources as a regular field; opt the module in or drop the section",
			c.Name,
		)
	}
	return nil
}

// parseObserved decodes an observed.yaml section into the engine's
// stable-name-keyed observation map.
func parseObserved(data []byte) (map[string]map[string]any, error) {
	objects, err := snapshot.ParseObjects(data)
	if err != nil {
		return nil, err
	}
	return snapshot.KeyObservedObjects(objects)
}

// resourceNames lists a result's resource keys for expected-failure messages.
func resourceNames(res render.Result) []string {
	names := make([]string, 0, len(res.Resources))
	for name := range res.Resources {
		names = append(names, name)
	}
	return names
}
