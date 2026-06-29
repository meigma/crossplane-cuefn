package pkg_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/meigma/crossplane-cuefn/internal/pkg"
	"github.com/meigma/crossplane-cuefn/internal/test/common"
)

// TestPush_UnreachableDestination proves a push to a closed port returns a clear
// non-nil error naming the destination, never a panic (criterion 3).
func TestPush_UnreachableDestination(t *testing.T) {
	img, err := pkg.BuildConfigurationImage(common.BuildFixtureConfiguration(t))
	require.NoError(t, err)

	const ref = "127.0.0.1:1/xapps-configuration:v0.1.0"
	var perr error
	require.NotPanics(t, func() {
		_, perr = pkg.Push(context.Background(), ref, img, true)
	})
	require.Error(t, perr)
	assert.Contains(t, perr.Error(), ref)
}

// TestPush_MalformedReference proves a malformed destination ref errors cleanly.
func TestPush_MalformedReference(t *testing.T) {
	img, err := pkg.BuildConfigurationImage(common.BuildFixtureConfiguration(t))
	require.NoError(t, err)

	var perr error
	require.NotPanics(t, func() {
		_, perr = pkg.Push(context.Background(), "NOT A REF", img, false)
	})
	require.Error(t, perr)
}
