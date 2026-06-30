package pkg_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/meigma/crossplane-cuefn/internal/pkg"
)

// TestDerivedFunctionName pins the Crossplane package-manager naming rule: a
// dependsOn Function installs under xpkg.ToDNSLabel of the host-stripped
// repository path. These exact mappings were observed on a live Crossplane v2
// cluster, so a generated Composition that references the function by this name
// binds to the auto-installed Function from a single Configuration install.
func TestDerivedFunctionName(t *testing.T) {
	cases := map[string]string{
		"ghcr.io/meigma/function-cuefn":                                      "meigma-function-cuefn",
		"ghcr.io/meigma/function-cuefn:v0.1.1":                               "meigma-function-cuefn",
		"xpkg.meigma.io/cuefn":                                               "cuefn",
		"xpkg.crossplane.io/crossplane-contrib/function-environment-configs": "crossplane-contrib-function-environment-configs",
	}
	for ref, want := range cases {
		got, err := pkg.DerivedFunctionName(ref)
		require.NoError(t, err, ref)
		assert.Equal(t, want, got, ref)
	}
}
