package snapshot

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	utilyaml "k8s.io/apimachinery/pkg/util/yaml"
	"sigs.k8s.io/yaml"
)

// ParseObject decodes a single YAML document into a map.
func ParseObject(data []byte) (map[string]any, error) {
	var out map[string]any
	if err := yaml.Unmarshal(data, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// LoadObject reads a YAML file holding a single object into a map.
func LoadObject(path string) (map[string]any, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return ParseObject(data)
}

// ParseObjects decodes YAML that may hold multiple documents into one map per
// non-empty document. It uses the Kubernetes multi-document reader so only a
// "---" at column 0 separates documents — a "---" line inside a value (e.g.
// embedded YAML in a ConfigMap datum) is not mistaken for a separator.
func ParseObjects(data []byte) ([]map[string]any, error) {
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

// readObjectsFile reads a YAML file that may hold multiple documents.
func readObjectsFile(path string) ([]map[string]any, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return ParseObjects(data)
}

// loadObjects reads a flat bag of Kubernetes objects from a YAML file or the
// immediate YAML children of a directory, using label in contextual errors. It
// is shared by the required- and observed-resource loaders so both match
// Crossplane's non-recursive directory semantics while accepting the same safe
// multi-document input conventions.
func loadObjects(path, label string) ([]map[string]any, error) {
	if path == "" {
		return nil, nil
	}

	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("cannot read %s %q: %w", label, path, err)
	}

	if !info.IsDir() {
		objs, readErr := readObjectsFile(path)
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
		fileObjs, readErr := readObjectsFile(file)
		if readErr != nil {
			return nil, fmt.Errorf("cannot read %s %q: cannot read %q: %w", label, path, file, readErr)
		}
		objs = append(objs, fileObjs...)
	}
	return objs, nil
}
