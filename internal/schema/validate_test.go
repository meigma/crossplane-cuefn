package schema

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestValidate_Library is the library-level table for criterion 4: each invalid
// spec is rejected with an error naming the offending field, while a valid spec
// and a spec that omits a defaulted field both pass (defaults applied).
func TestValidate_Library(t *testing.T) {
	module := loadModule(t, "testdata/derisked")

	cases := []struct {
		name      string
		spec      map[string]any
		wantField string // empty means the spec must validate
	}{
		{
			name: "valid",
			spec: map[string]any{"image": "ghcr.io/x:1", "replicas": 5, "tier": "premium"},
		},
		{
			name: "omitted defaulted field passes",
			spec: map[string]any{"image": "ghcr.io/x:1"},
		},
		{
			name:      "out of bounds replicas",
			spec:      map[string]any{"image": "ghcr.io/x:1", "replicas": 99},
			wantField: "replicas",
		},
		{
			name:      "wrong enum tier",
			spec:      map[string]any{"image": "ghcr.io/x:1", "tier": "gold"},
			wantField: "tier",
		},
		{
			name:      "missing required image",
			spec:      map[string]any{"replicas": 2},
			wantField: "image",
		},
		{
			name:      "pattern violation",
			spec:      map[string]any{"image": "NOT VALID"},
			wantField: "image",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := Validate(module, tc.spec)
			if tc.wantField == "" {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.wantField,
				"error should name the offending field")
		})
	}
}

// TestValidate_StripsReservedKeys proves Crossplane machinery keys on the
// observed spec do not conflict with the closed #Spec.
func TestValidate_StripsReservedKeys(t *testing.T) {
	module := loadModule(t, "testdata/derisked")
	err := Validate(module, map[string]any{
		"image":      "ghcr.io/x:1",
		"crossplane": map[string]any{"compositionRef": map[string]any{"name": "c"}},
	})
	require.NoError(t, err)
}
