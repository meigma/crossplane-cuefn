package snapshot

import (
	"sort"

	"github.com/meigma/crossplane-cuefn/internal/render"
)

// LoadRequiredObjects reads a flat bag of real cluster objects from path, which
// may be a single YAML file or a directory of them. It returns nil when path is
// empty (the flag is unset). Each file may hold multiple "---"-separated
// documents; empty documents are skipped. The objects are matched against the
// module's emitted requirements by MatchRequirements — they are not filename-
// or directory-keyed.
func LoadRequiredObjects(path string) ([]map[string]any, error) {
	return loadObjects(path, "required resources")
}

// MatchRequirements groups the supplied objects under each requirement name by
// filtering on apiVersion, kind, namespace (when the selector sets one), and
// either matchName (exact metadata.name) or matchLabels (a subset of
// metadata.labels). Each matched bucket is sorted by "namespace/name" for
// determinism. Every requirement name is always present as a key, with a
// non-nil empty slice when nothing matches, so the module sees a concrete
// cfg: [] rather than an absent field.
func MatchRequirements(
	objs []map[string]any,
	reqs map[string]render.Requirement,
) map[string][]map[string]any {
	out := make(map[string][]map[string]any, len(reqs))
	for name, req := range reqs {
		matched := make([]map[string]any, 0)
		for _, obj := range objs {
			if matchesRequirement(obj, req) {
				matched = append(matched, obj)
			}
		}
		sort.SliceStable(matched, func(i, j int) bool {
			return objectKey(matched[i]) < objectKey(matched[j])
		})
		out[name] = matched
	}
	return out
}

// matchesRequirement reports whether obj satisfies the selector req. It reads
// apiVersion, kind, and metadata defensively so a malformed object simply fails
// to match rather than panicking.
func matchesRequirement(obj map[string]any, req render.Requirement) bool {
	apiVersion, _ := obj["apiVersion"].(string)
	kind, _ := obj["kind"].(string)
	if apiVersion != req.APIVersion || kind != req.Kind {
		return false
	}

	meta, _ := obj["metadata"].(map[string]any)
	if req.Namespace != "" {
		namespace, _ := meta["namespace"].(string)
		if namespace != req.Namespace {
			return false
		}
	}

	if req.MatchName != "" {
		name, _ := meta["name"].(string)
		return name == req.MatchName
	}

	// matchLabels: every selector label must be present and equal on the object.
	labels, _ := meta["labels"].(map[string]any)
	for k, v := range req.MatchLabels {
		got, ok := labels[k].(string)
		if !ok || got != v {
			return false
		}
	}
	return true
}

// objectKey is the "namespace/name" sort key for a cluster object, read
// defensively.
func objectKey(obj map[string]any) string {
	meta, _ := obj["metadata"].(map[string]any)
	namespace, _ := meta["namespace"].(string)
	name, _ := meta["name"].(string)
	return namespace + "/" + name
}
