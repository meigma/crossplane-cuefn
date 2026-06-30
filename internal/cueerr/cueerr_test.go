package cueerr_test

import (
	"errors"
	"strings"
	"testing"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/meigma/crossplane-cuefn/internal/cueerr"
)

func cueErr(t *testing.T, schema, value string) error {
	t.Helper()
	ctx := cuecontext.New()
	v := ctx.CompileString(schema).Unify(ctx.CompileString(value))
	err := v.Validate(cue.Concrete(true))
	require.Error(t, err)
	return err
}

func TestWrap_BoundedDefaultCollapsesToSpecificError(t *testing.T) {
	err := cueErr(t, `{replicas: *1 | int & >=1 & <=10}`, `{replicas: 99}`)

	got := cueerr.Wrap(err, "bad spec").Error()

	// One clean line: the specific bound error, with no disjunction wrapper, no
	// misleading default-branch "conflicting values 1", and printed once.
	assert.Equal(t, "bad spec: replicas: invalid value 99 (out of bound <=10)", got)
	assert.NotContains(t, got, "empty disjunction")
	assert.NotContains(t, got, "conflicting values 1")
	assert.Equal(t, 1, strings.Count(got, "out of bound"))
}

func TestWrap_PreservesUnwrap(t *testing.T) {
	err := cueErr(t, `{replicas: *1 | int & >=1 & <=10}`, `{replicas: 99}`)

	// CUE errors are non-comparable lists, so errors.Is-by-identity can't match
	// them; assert the original is reachable through Unwrap instead.
	unwrapped := errors.Unwrap(cueerr.Wrap(err, "bad spec"))
	require.Error(t, unwrapped)
	assert.Equal(t, err.Error(), unwrapped.Error())
}

func TestWrap_RequiredFieldStaysClear(t *testing.T) {
	err := cueErr(t, `{name!: string}`, `{}`)

	got := cueerr.Wrap(err, "bad spec").Error()

	assert.Contains(t, got, "name")
	assert.Contains(t, got, "required")
	assert.NotContains(t, got, "empty disjunction")
}

func TestWrap_TypeMismatchDropsDisjunctionWrapper(t *testing.T) {
	err := cueErr(t, `{image: string | *"x"}`, `{image: 123}`)

	got := cueerr.Wrap(err, "bad spec").Error()

	assert.Contains(t, got, "image")
	assert.Contains(t, got, "mismatched types")
	assert.NotContains(t, got, "empty disjunction")
}

func TestWrap_NonCUEErrorFallsBack(t *testing.T) {
	got := cueerr.Wrap(errors.New("boom"), "context").Error()
	assert.Equal(t, "context: boom", got)
}
