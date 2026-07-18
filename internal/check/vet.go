package check

import (
	"cuelang.org/go/cue"

	"github.com/meigma/crossplane-cuefn/internal/cueerr"
)

// Vet validates the loaded module without requiring concreteness — the
// programmatic equivalent of the documented author check
// `cue vet -c=false ./...` (bare `cue vet` fails on any required field
// without a default, which every useful #Spec declares). The option set
// mirrors the cue CLI's vet command: attributes, definitions, and hidden
// fields are all checked.
//
// In practice CUE's evaluator surfaces these errors eagerly, so a module that
// fails Vet usually fails render.LoadModule first (verified empirically:
// regular, definition-nested, and hidden conflicts all error at build). The
// caller must therefore report a load failure as this check's failure; Vet
// itself is the recursive backstop for anything build lets through.
func Vet(module cue.Value) error {
	err := module.Validate(
		cue.Attributes(true),
		cue.Definitions(true),
		cue.Hidden(true),
		cue.Concrete(false),
	)
	if err != nil {
		return cueerr.Wrap(err, "module failed validation")
	}
	return nil
}
