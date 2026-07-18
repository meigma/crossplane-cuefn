package check

import (
	"bytes"
	"fmt"
	"maps"
	"slices"

	"cuelang.org/go/cue/format"
)

// Fmt reports which of the given CUE files are not in canonical form,
// mirroring bare `cue fmt` (no simplification). Keys name the files in
// reports; values are their contents. The returned names are sorted. A file
// that fails to parse is an error, not an unformatted file: the caller must
// report it loudly, never skip it.
func Fmt(files map[string][]byte) ([]string, error) {
	var unformatted []string
	for _, name := range slices.Sorted(maps.Keys(files)) {
		canonical, err := format.Source(files[name])
		if err != nil {
			return nil, fmt.Errorf("cannot format %s: %w", name, err)
		}
		if !bytes.Equal(canonical, files[name]) {
			unformatted = append(unformatted, name)
		}
	}
	return unformatted, nil
}
