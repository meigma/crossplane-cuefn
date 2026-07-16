package cli

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	utilyaml "k8s.io/apimachinery/pkg/util/yaml"
	"sigs.k8s.io/yaml"

	"github.com/meigma/crossplane-cuefn/internal/render"
)

// loadRequiredObjects reads a flat bag of real cluster objects from path, which
// may be a single YAML file or a directory of them. It returns nil when path is
// empty (the flag is unset). Each file may hold multiple "---"-separated
// documents; empty documents are skipped. The objects are matched against the
// module's emitted requirements by matchRequirements — they are not filename- or
// directory-keyed.
func loadRequiredObjects(path string) ([]map[string]any, error) {
	return loadResourceObjects(path, "required resources")
}

// loadResourceObjects reads a flat bag of Kubernetes objects from a YAML file
// or the immediate YAML children of a directory, using label in contextual
// errors. It is shared by the required- and observed-resource flags so both
// match Crossplane's non-recursive directory semantics while accepting the same
// safe multi-document input conventions.
func loadResourceObjects(path, label string) ([]map[string]any, error) {
	if path == "" {
		return nil, nil
	}

	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("cannot read %s %q: %w", label, path, err)
	}

	if !info.IsDir() {
		objs, readErr := readYAMLObjects(path)
		if readErr != nil {
			return nil, fmt.Errorf("cannot read %s %q: %w", label, path, readErr)
		}
		return objs, nil
	}

	entries, err := os.ReadDir(path)
	if err != nil {
		return nil, fmt.Errorf("cannot read %s %q: cannot read directory: %w", label, path, err)
	}

	files := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if ext := filepath.Ext(entry.Name()); ext != ".yaml" && ext != ".yml" {
			continue
		}
		files = append(files, filepath.Join(path, entry.Name()))
	}
	if len(files) == 0 {
		return nil, fmt.Errorf(
			"cannot read %s %q: no YAML files found in %q (.yaml or .yml)",
			label,
			path,
			path,
		)
	}

	var objs []map[string]any
	for _, file := range files {
		fileObjs, readErr := readYAMLObjects(file)
		if readErr != nil {
			return nil, fmt.Errorf("cannot read %s %q: cannot read %q: %w", label, path, file, readErr)
		}
		objs = append(objs, fileObjs...)
	}
	return objs, nil
}

// readYAMLObjects reads a YAML file that may hold multiple documents and decodes
// each non-empty one into a map. It uses the Kubernetes multi-document reader so
// only a "---" at column 0 separates documents — a "---" line inside a value
// (e.g. embedded YAML in a ConfigMap datum) is not mistaken for a separator. The
// existing readYAMLObject reads a single object only, so this is the multi-doc
// reader the directory/file loader needs.
func readYAMLObjects(path string) ([]map[string]any, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var objs []map[string]any
	reader := utilyaml.NewYAMLReader(bufio.NewReader(bytes.NewReader(data)))
	for {
		doc, err := reader.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, err
		}
		if strings.TrimSpace(string(doc)) == "" {
			continue
		}
		var obj map[string]any
		if err := yaml.Unmarshal(doc, &obj); err != nil {
			return nil, err
		}
		if obj == nil { // a document that is only comments decodes to nil.
			continue
		}
		objs = append(objs, obj)
	}
	return objs, nil
}

// matchRequirements groups the supplied objects under each requirement name by
// filtering on apiVersion, kind, namespace (when the selector sets one), and
// either matchName (exact metadata.name) or matchLabels (a subset of
// metadata.labels). Each matched bucket is sorted by "namespace/name" for
// determinism. Every requirement name is always present as a key, with a
// non-nil empty slice when nothing matches, so the module sees a concrete
// cfg: [] rather than an absent field.
func matchRequirements(
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
