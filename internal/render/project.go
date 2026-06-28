package render

// reservedSpecKeys are the keys the engine strips from an observed XR spec
// before unifying it against a closed [#Spec]. The v2 reserved "crossplane"
// block nests composition machinery (compositionRef, resourceRefs, ...); the
// remaining keys are legacy v1 machinery that may still appear on an observed
// spec. Stripping them keeps a closed authoring schema from conflicting with
// Crossplane-internal fields the author never declared.
//
//nolint:gochecknoglobals // immutable lookup table for ProjectSpec.
var reservedSpecKeys = []string{
	"crossplane",
	"compositionRef",
	"compositionSelector",
	"compositionRevisionRef",
	"compositionRevisionSelector",
	"compositionUpdatePolicy",
	"claimRef",
	"resourceRef",
	"resourceRefs",
	"writeConnectionSecretToRef",
	"publishConnectionDetailsTo",
	"environmentConfigRefs",
}

// ProjectSpec returns a shallow copy of an observed XR spec with the
// Crossplane-reserved keys removed, so a closed module [#Spec] does not conflict
// with machinery fields the author never declared. The input map is not
// mutated; a nil spec yields a nil result.
func ProjectSpec(spec map[string]any) map[string]any {
	if spec == nil {
		return nil
	}

	reserved := make(map[string]struct{}, len(reservedSpecKeys))
	for _, k := range reservedSpecKeys {
		reserved[k] = struct{}{}
	}

	out := make(map[string]any, len(spec))
	for k, v := range spec {
		if _, ok := reserved[k]; ok {
			continue
		}
		out[k] = v
	}
	return out
}
