package testharness

import (
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/rogpeppe/go-internal/txtar"
)

// The closed section vocabulary. Unknown names are rejected so a typo like
// "enviroment.yaml" fails loudly instead of being silently ignored.
const (
	sectionXR          = "xr.yaml"
	sectionEnvironment = "environment.yaml"
	sectionRequired    = "required.yaml"
	sectionObserved    = "observed.yaml"
	sectionWantCUE     = "want.cue"
	sectionWantYAML    = "want.yaml"
	sectionError       = "error.txt"
)

// validSections is the error-message listing of the base vocabulary.
const validSections = "xr.yaml, environment.yaml, required.yaml, observed.yaml, " +
	"want.cue, want.yaml, error.txt, and numbered step sections such as 1/observed.yaml"

// Case is one parsed test case: the shared fixtures plus one or more units to
// execute. A case without steps has exactly one unit (the base case); a case
// with steps has one unit per step, sharing the base XR/environment/required
// fixtures.
type Case struct {
	// Name is the txtar filename without its extension.
	Name string
	// Path is the source file path, used in messages and by golden rewrites.
	Path string
	// Description is the txtar comment block, echoed on failure.
	Description string

	// XR is the observed XR manifest (required).
	XR []byte
	// Environment is the merged EnvironmentConfig data (optional).
	Environment []byte
	// Required is the flat multi-document bag of cluster objects matched
	// against the module's emitted requirements (optional).
	Required []byte

	// Units are the executions this case performs, in order.
	Units []Unit
}

// Unit is one render-and-assert execution: the base case or one numbered step.
type Unit struct {
	// Label is "" for the base unit and the step number ("1", "2", ...) for
	// step units.
	Label string
	// Observed is the observed composed-resource snapshot for this unit
	// (optional for the base unit, required for steps).
	Observed []byte
	// WantCUE is a partial CUE expectation unified against the normalized
	// result. WantCUELine is the 1-based line its content starts on in the
	// txtar file, so CUE error positions cite real file coordinates.
	WantCUE     []byte
	WantCUELine int
	// WantYAML is the exact golden of the full normalized result.
	WantYAML []byte
	// ErrorSubstrings are the non-empty lines of error.txt: each must appear
	// in the render error. Non-nil only on the base unit.
	ErrorSubstrings []string
}

// NeedsSeed reports whether the unit declares no expectation at all, which
// makes it a candidate for golden seeding.
func (u Unit) NeedsSeed() bool {
	return u.WantCUE == nil && u.WantYAML == nil && u.ErrorSubstrings == nil
}

// SectionPrefix is the txtar section-name prefix for this unit: "" for the
// base unit, "N/" for steps.
func (u Unit) SectionPrefix() string {
	if u.Label == "" {
		return ""
	}
	return u.Label + "/"
}

// ParseCase parses and validates one txtar test case. The case name derives
// from the file name; path is retained for messages and golden rewrites.
func ParseCase(path string, data []byte) (*Case, error) {
	name := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	fail := func(format string, args ...any) error {
		return fmt.Errorf("test case %q: %s", name, fmt.Sprintf(format, args...))
	}

	arch := txtar.Parse(data)
	c := &Case{
		Name:        name,
		Path:        path,
		Description: strings.TrimSpace(string(arch.Comment)),
	}

	base := &Unit{}
	steps := map[int]*Unit{}
	seen := map[string]bool{}

	for _, file := range arch.Files {
		if seen[file.Name] {
			return nil, fail("duplicate section %q", file.Name)
		}
		seen[file.Name] = true

		label, section, isStep := strings.Cut(file.Name, "/")
		var err error
		if isStep {
			err = collectStepSection(steps, label, section, data, file)
		} else {
			err = collectBaseSection(c, base, data, file)
		}
		if err != nil {
			return nil, fail("%s", err)
		}
	}

	if c.XR == nil {
		return nil, fail("missing required section %q", sectionXR)
	}

	units, err := assembleUnits(*base, steps)
	if err != nil {
		return nil, fail("%s", err)
	}
	c.Units = units
	return c, nil
}

// collectBaseSection files one unprefixed section into the case fixtures or
// the base unit.
func collectBaseSection(c *Case, base *Unit, data []byte, file txtar.File) error {
	switch file.Name {
	case sectionXR:
		c.XR = file.Data
	case sectionEnvironment:
		c.Environment = file.Data
	case sectionRequired:
		c.Required = file.Data
	case sectionObserved:
		base.Observed = file.Data
	case sectionWantCUE:
		base.WantCUE = file.Data
		base.WantCUELine = sectionContentLine(data, file.Name)
	case sectionWantYAML:
		base.WantYAML = file.Data
	case sectionError:
		subs := errorSubstrings(file.Data)
		if len(subs) == 0 {
			return errors.New("error.txt must contain at least one non-empty line")
		}
		base.ErrorSubstrings = subs
	default:
		return fmt.Errorf("unknown section %q (valid sections: %s)", file.Name, validSections)
	}
	return nil
}

// collectStepSection files one "N/..." section into its numbered step.
func collectStepSection(steps map[int]*Unit, label, section string, data []byte, file txtar.File) error {
	index, err := strconv.Atoi(label)
	if err != nil || index < 1 {
		return fmt.Errorf("unknown section %q (valid sections: %s)", file.Name, validSections)
	}
	step, ok := steps[index]
	if !ok {
		step = &Unit{Label: label}
		steps[index] = step
	}
	switch section {
	case sectionObserved:
		step.Observed = file.Data
	case sectionWantCUE:
		step.WantCUE = file.Data
		step.WantCUELine = sectionContentLine(data, file.Name)
	case sectionWantYAML:
		step.WantYAML = file.Data
	default:
		return fmt.Errorf("unknown section %q (steps allow only observed.yaml, want.cue, and want.yaml)", file.Name)
	}
	return nil
}

// assembleUnits cross-validates the base/step combination and returns the
// units in execution order.
func assembleUnits(base Unit, steps map[int]*Unit) ([]Unit, error) {
	if len(steps) == 0 {
		if base.ErrorSubstrings != nil && (base.WantCUE != nil || base.WantYAML != nil) {
			return nil, errors.New("error.txt cannot be combined with want.cue or want.yaml")
		}
		return []Unit{base}, nil
	}

	// Steps present: the base may only carry shared fixtures.
	if base.Observed != nil || base.WantCUE != nil || base.WantYAML != nil {
		return nil, errors.New(
			"base observed.yaml/want.cue/want.yaml cannot be combined with step sections; move them into a step",
		)
	}
	if base.ErrorSubstrings != nil {
		return nil, errors.New("error.txt cannot be combined with step sections")
	}

	indices := make([]int, 0, len(steps))
	for index := range steps {
		indices = append(indices, index)
	}
	sort.Ints(indices)

	units := make([]Unit, 0, len(indices))
	for i, index := range indices {
		if index != i+1 {
			return nil, fmt.Errorf(
				"step sections must be numbered contiguously from 1 (found step %d without step %d)",
				index,
				i+1,
			)
		}
		step := steps[index]
		if step.Observed == nil {
			return nil, fmt.Errorf("step %d needs an observed.yaml section", index)
		}
		units = append(units, *step)
	}
	return units, nil
}

// errorSubstrings returns the trimmed non-empty lines of an error.txt section.
func errorSubstrings(data []byte) []string {
	var subs []string
	for line := range strings.SplitSeq(string(data), "\n") {
		if trimmed := strings.TrimSpace(line); trimmed != "" {
			subs = append(subs, trimmed)
		}
	}
	return subs
}

// markerToContent is the line distance from a txtar section marker to the
// section's first content line: the marker occupies line i+1 (1-based), so
// content starts on line i+2.
const markerToContent = 2

// sectionContentLine returns the 1-based line number the named section's
// content starts on, so CUE positions inside the section can be reported in
// the txtar file's own coordinates.
func sectionContentLine(data []byte, name string) int {
	marker := "-- " + name + " --"
	for i, line := range strings.Split(string(data), "\n") {
		if strings.TrimSpace(line) == marker {
			return i + markerToContent
		}
	}
	return 1
}
