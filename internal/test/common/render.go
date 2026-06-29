package common

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/meigma/crossplane-cuefn/internal/render"
)

// Object returns the rendered Kubernetes object for the named resource.
func Object(t *testing.T, res render.Result, name string) map[string]any {
	t.Helper()
	r, ok := res.Resources[name]
	require.True(t, ok, "resource %q not found", name)
	return r.Object
}

// ToInt coerces a decoded JSON/CUE number into an int regardless of its concrete
// Go type.
func ToInt(t *testing.T, v any) int {
	t.Helper()
	switch n := v.(type) {
	case int:
		return n
	case int64:
		return int(n)
	case float64:
		return int(n)
	case json.Number:
		i, err := n.Int64()
		require.NoError(t, err)
		return int(i)
	default:
		t.Fatalf("value %v (%T) is not numeric", v, v)
		return 0
	}
}
