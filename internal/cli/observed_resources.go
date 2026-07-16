package cli

import (
	"fmt"

	"github.com/crossplane/crossplane-runtime/v2/pkg/xcrd"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// loadObservedObjects reads the raw composed objects accepted by Crossplane's
// render --observed-resources flag and keys them by their standard composition-
// resource-name annotation. Missing, empty, and duplicate stable names are
// rejected rather than silently changing the observation map.
func loadObservedObjects(path string) (map[string]map[string]any, error) {
	objects, err := loadResourceObjects(path, "observed resources")
	if err != nil {
		return nil, err
	}
	if len(objects) == 0 {
		return map[string]map[string]any{}, nil
	}

	out := make(map[string]map[string]any, len(objects))
	seen := make(map[string]string, len(objects))
	for _, object := range objects {
		u := &unstructured.Unstructured{Object: object}
		identity := observedObjectIdentity(u)
		name := xcrd.GetCompositionResourceName(u)
		if name == "" {
			return nil, fmt.Errorf(
				"observed resource %s must set a non-empty %q annotation",
				identity,
				xcrd.AnnotationKeyCompositionResourceName,
			)
		}
		if previous, ok := seen[name]; ok {
			return nil, fmt.Errorf(
				"observed resource %s duplicates stable name %q from %s (%s annotation)",
				identity,
				name,
				previous,
				xcrd.AnnotationKeyCompositionResourceName,
			)
		}
		seen[name] = identity
		out[name] = object
	}
	return out, nil
}

// observedObjectIdentity returns a compact kind/namespace/name identifier for
// actionable loader errors, tolerating partially formed mock objects.
func observedObjectIdentity(object *unstructured.Unstructured) string {
	kind := object.GetKind()
	if kind == "" {
		kind = "<unknown-kind>"
	}
	name := object.GetName()
	if name == "" {
		name = "<unnamed>"
	}
	if namespace := object.GetNamespace(); namespace != "" {
		return fmt.Sprintf("%s %s/%s", kind, namespace, name)
	}
	return fmt.Sprintf("%s %s", kind, name)
}
