package testharness

import (
	"bytes"
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"cuelang.org/go/cue"
	cueerrors "cuelang.org/go/cue/errors"
)

// Failure is one assertion failure within a unit. Kind names the failing
// expectation ("want.cue", "want.yaml", "error", "render").
type Failure struct {
	Unit    string
	Kind    string
	Message string
}

// evalWantCUE unifies the unit's want.cue expectation with the normalized
// result and validates the unification is conflict-free and concrete. The
// returned message is "" on success; on failure it is CUE's own path-qualified
// conflict report, with positions in the txtar file's coordinates (the source
// is newline-padded to the section's real starting line).
func evalWantCUE(cctx *cue.Context, c *Case, u Unit, actual cue.Value) string {
	filename := filepath.Base(c.Path) + "/" + u.SectionPrefix() + sectionWantCUE
	padded := append(bytes.Repeat([]byte("\n"), max(u.WantCUELine-1, 0)), u.WantCUE...)

	want := cctx.CompileBytes(padded, cue.Filename(filename))
	if err := want.Err(); err != nil {
		return "cannot compile want.cue:\n" + cueerrors.Details(err, nil)
	}

	unified := want.Unify(actual)
	if err := unified.Validate(cue.Concrete(true)); err != nil {
		return stripSyntheticPositions(cueerrors.Details(err, nil))
	}
	return ""
}

// syntheticPosition matches a bare "line:col" location — a position in the
// JSON-encoded render result, which has no meaningful source to cite. Real
// positions carry the txtar filename and survive.
var syntheticPosition = regexp.MustCompile(`^\d+:\d+$`)

// stripSyntheticPositions drops position lines that point into the encoded
// render result, keeping the failure focused on the author's want.cue.
func stripSyntheticPositions(details string) string {
	var out []string
	for line := range strings.SplitSeq(details, "\n") {
		if syntheticPosition.MatchString(strings.TrimSpace(line)) {
			continue
		}
		out = append(out, line)
	}
	return strings.Join(out, "\n")
}

// evalWantYAML compares the unit's want.yaml golden byte-exactly (modulo line
// endings and a trailing newline) against the normalized result's YAML. The
// returned message is "" on success and a line diff on mismatch.
func evalWantYAML(want, got []byte) string {
	w := normalizeText(want)
	g := normalizeText(got)
	if w == g {
		return ""
	}
	return "golden mismatch (-want.yaml +rendered):\n" + diffLines(w, g)
}

// evalError checks an expected-failure unit: the render must have failed and
// the error must contain every declared substring.
func evalError(substrings []string, renderErr error, resources []string) string {
	if renderErr == nil {
		sort.Strings(resources)
		return fmt.Sprintf(
			"expected the render to fail, but it succeeded with resources [%s]",
			strings.Join(resources, ", "),
		)
	}
	var missing []string
	for _, sub := range substrings {
		if !strings.Contains(renderErr.Error(), sub) {
			missing = append(missing, sub)
		}
	}
	if len(missing) > 0 {
		return fmt.Sprintf(
			"render failed as expected, but the error does not contain %q:\n%s",
			missing,
			renderErr.Error(),
		)
	}
	return ""
}

// normalizeText strips carriage returns and enforces one trailing newline so
// golden comparison is stable across platforms and editors.
func normalizeText(data []byte) string {
	s := strings.ReplaceAll(string(data), "\r\n", "\n")
	return strings.TrimRight(s, "\n") + "\n"
}
