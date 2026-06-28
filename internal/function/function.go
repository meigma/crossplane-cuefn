// Package function is the Crossplane v2 composition-function edge adapter for
// cuefn. It translates a RunFunctionRequest into the curated inputs the pure
// [render.Engine] core consumes, then translates the engine's result back into
// desired composed resources, a patched composite status, and a success
// condition on the response.
//
// All Crossplane request/response and gRPC proto types live here, at the edge.
// The [render] core stays free of them: this package depends on the core only
// through the [render.ModuleLoader] port, supplied by a [LoaderFactory] seam so
// the serve command wires an OCI loader while tests drive a local one offline.
package function

import (
	"context"
	"maps"

	"github.com/crossplane/function-sdk-go/errors"
	"github.com/crossplane/function-sdk-go/logging"
	fnv1 "github.com/crossplane/function-sdk-go/proto/v1"
	"github.com/crossplane/function-sdk-go/request"
	"github.com/crossplane/function-sdk-go/resource"
	"github.com/crossplane/function-sdk-go/resource/composed"
	"github.com/crossplane/function-sdk-go/response"

	sdkcontext "github.com/crossplane/function-sdk-go/context"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/meigma/crossplane-cuefn/input/v1beta1"
	"github.com/meigma/crossplane-cuefn/internal/render"
)

// LoaderFactory builds the [render.ModuleLoader] used to fetch the module named
// in a step's Input. It is the seam between the adapter and the core's loading
// port: the serve command supplies an OCI-backed factory ([OCILoaderFactory]),
// while tests supply one returning a [render.LocalLoader] so they run offline.
// It receives the decoded Input so a factory can fold per-step settings (such as
// ExpectedDigest) into the loader it returns.
type LoaderFactory func(in *v1beta1.Input) (render.ModuleLoader, error)

// Function is the cuefn composition function. It renders desired composed
// resources from a CUE module evaluated against the observed XR and the pipeline
// environment.
type Function struct {
	fnv1.UnimplementedFunctionRunnerServiceServer

	log       logging.Logger
	newLoader LoaderFactory
}

// New returns a Function that builds its module loader with newLoader and logs
// through log.
func New(newLoader LoaderFactory, log logging.Logger) *Function {
	return &Function{log: log, newLoader: newLoader}
}

// RunFunction decodes the step Input, builds the engine inputs from the observed
// XR and the pipeline environment, renders the named module, and writes the
// resulting composed resources, composite status, and a success condition onto
// the response.
//
// Every malformed or unreachable input path returns a single FATAL result whose
// message names the cause and returns (rsp, nil): the function never panics and
// never returns a transport error for a domain failure, so Crossplane surfaces
// the cause on the composite instead of retrying a doomed gRPC call.
func (f *Function) RunFunction(ctx context.Context, req *fnv1.RunFunctionRequest) (*fnv1.RunFunctionResponse, error) {
	f.log.Debug("Running function", "tag", req.GetMeta().GetTag())
	rsp := response.To(req, response.DefaultTTL)

	in := &v1beta1.Input{}
	if err := request.GetInput(req, in); err != nil {
		response.Fatal(rsp, errors.Wrapf(err, "cannot get Function input from %T", req))
		return rsp, nil
	}
	if in.Module == "" {
		response.Fatal(rsp, errors.New(`input field "module" is required`))
		return rsp, nil
	}

	oxr, err := request.GetObservedCompositeResource(req)
	if err != nil {
		response.Fatal(rsp, errors.Wrap(err, "cannot get observed composite resource"))
		return rsp, nil
	}

	loader, err := f.newLoader(in)
	if err != nil {
		response.Fatal(rsp, errors.Wrapf(err, "cannot build module loader for %q", in.Module))
		return rsp, nil
	}

	spec, _ := oxr.Resource.Object["spec"].(map[string]any)
	inputs := render.Inputs{
		Spec: spec,
		Metadata: render.Metadata{
			Name:      oxr.Resource.GetName(),
			Namespace: oxr.Resource.GetNamespace(),
		},
		Environment: environmentFromContext(req),
	}

	result, err := render.New(loader).Render(ctx, in.Module, inputs)
	if err != nil {
		response.Fatal(rsp, errors.Wrapf(err, "cannot render CUE module %q", in.Module))
		return rsp, nil
	}

	if err := setDesiredComposed(rsp, req, result); err != nil {
		response.Fatal(rsp, errors.Wrap(err, "cannot set desired composed resources"))
		return rsp, nil
	}

	if err := setCompositeStatus(rsp, req, oxr, result); err != nil {
		response.Fatal(rsp, errors.Wrap(err, "cannot patch desired composite status"))
		return rsp, nil
	}

	response.Normalf(rsp, "rendered %d resource(s) from module %q", len(result.Resources), in.Module)
	response.ConditionTrue(rsp, "FunctionSuccess", "Success").TargetComposite()
	f.log.Debug("Rendered resources", "module", in.Module, "count", len(result.Resources))
	return rsp, nil
}

// setDesiredComposed merges the rendered resources into the desired composed
// resources accumulated by earlier pipeline steps. Each resource is keyed by the
// module author's map key verbatim (the Crossplane composed-resource name) and
// carries the readiness the module assigned, mapped straight through the SDK
// enum by SetDesiredComposedResources.
func setDesiredComposed(rsp *fnv1.RunFunctionResponse, req *fnv1.RunFunctionRequest, result render.Result) error {
	desired, err := request.GetDesiredComposedResources(req)
	if err != nil {
		return errors.Wrap(err, "cannot get desired composed resources")
	}

	for name, r := range result.Resources {
		dcd := resource.NewDesiredComposed()
		dcd.Resource = &composed.Unstructured{Unstructured: unstructured.Unstructured{Object: r.Object}}
		dcd.Ready = r.Ready
		desired[resource.Name(name)] = dcd
	}

	return response.SetDesiredComposedResources(rsp, desired)
}

// setCompositeStatus patches the module's status under the desired composite's
// status field. It starts from the desired composite accumulated by earlier
// steps to preserve their state, then copies the observed XR's GVK so the patch
// is addressed to the right resource, and writes status only when the module
// returned one.
func setCompositeStatus(
	rsp *fnv1.RunFunctionResponse,
	req *fnv1.RunFunctionRequest,
	oxr *resource.Composite,
	result render.Result,
) error {
	if result.Status == nil {
		return nil
	}

	dxr, err := request.GetDesiredCompositeResource(req)
	if err != nil {
		return errors.Wrap(err, "cannot get desired composite resource")
	}

	dxr.Resource.SetGroupVersionKind(oxr.Resource.GroupVersionKind())
	dxr.Resource.Object["status"] = mergeStatus(dxr.Resource.Object["status"], result.Status)

	return response.SetDesiredCompositeResource(rsp, dxr)
}

// mergeStatus overlays the module status onto any status already present on the
// desired composite, so a status accumulated by an earlier step is preserved and
// the module's keys win on conflict.
func mergeStatus(existing any, status map[string]any) map[string]any {
	out, ok := existing.(map[string]any)
	if !ok {
		out = map[string]any{}
	}
	maps.Copy(out, status)
	return out
}

// environmentFromContext extracts the merged EnvironmentConfig data that
// function-environment-configs publishes under the well-known environment
// context key, returning nil when no environment is present.
func environmentFromContext(req *fnv1.RunFunctionRequest) map[string]any {
	v, ok := request.GetContextKey(req, sdkcontext.KeyEnvironment)
	if !ok {
		return nil
	}
	s := v.GetStructValue()
	if s == nil {
		return nil
	}
	return s.AsMap()
}

// OCILoaderFactory returns a [LoaderFactory] that builds a [render.OCILoader]
// from base, folding the Input's ExpectedDigest into the loader's per-ref digest
// expectation so a step can lock a module to a specific manifest. The base
// config carries process-level settings (cache dir, CUE_REGISTRY via Env); the
// returned factory clones it per request so concurrent steps never share an
// Expect map.
func OCILoaderFactory(base render.OCIConfig) LoaderFactory {
	return func(in *v1beta1.Input) (render.ModuleLoader, error) {
		cfg := base
		if in.ExpectedDigest != "" {
			cfg.Expect = map[string]string{in.Module: in.ExpectedDigest}
		}
		return render.NewOCILoader(cfg)
	}
}
