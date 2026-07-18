package testharness

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const parseFixture = `Deployment must track replicas.

-- xr.yaml --
apiVersion: platform.meigma.io/v1alpha1
kind: XApp
metadata:
  name: demo
  namespace: default
spec:
  replicas: 3
-- environment.yaml --
tier: production
-- want.cue --
resources: deployment: ready: "Ready"
`

func TestParseCase(t *testing.T) {
	t.Parallel()

	c, err := ParseCase("tests/replicas.txtar", []byte(parseFixture))
	require.NoError(t, err)

	assert.Equal(t, "replicas", c.Name)
	assert.Equal(t, "tests/replicas.txtar", c.Path)
	assert.Equal(t, "Deployment must track replicas.", c.Description)
	assert.Contains(t, string(c.XR), "kind: XApp")
	assert.Equal(t, "tier: production\n", string(c.Environment))
	require.Len(t, c.Units, 1)

	base := c.Units[0]
	assert.Empty(t, base.Label)
	assert.Empty(t, base.SectionPrefix())
	assert.NotNil(t, base.WantCUE)
	assert.False(t, base.NeedsSeed())
	// The want.cue content starts on line 14 of the txtar file: description,
	// blank line, three section markers, and the fixture bodies precede it.
	assert.Equal(t, 14, base.WantCUELine)
}

func TestParseCaseSteps(t *testing.T) {
	t.Parallel()

	c, err := ParseCase("steps.txtar", []byte(`-- xr.yaml --
spec: {}
-- 1/observed.yaml --
kind: Deployment
-- 1/want.cue --
resources: {}
-- 2/observed.yaml --
kind: Deployment
-- 2/want.yaml --
resources: {}
`))
	require.NoError(t, err)
	require.Len(t, c.Units, 2)
	assert.Equal(t, "1", c.Units[0].Label)
	assert.Equal(t, "1/", c.Units[0].SectionPrefix())
	assert.NotNil(t, c.Units[0].WantCUE)
	assert.Equal(t, "2", c.Units[1].Label)
	assert.NotNil(t, c.Units[1].WantYAML)
}

func TestParseCaseNeedsSeed(t *testing.T) {
	t.Parallel()

	c, err := ParseCase("seed.txtar", []byte("-- xr.yaml --\nspec: {}\n"))
	require.NoError(t, err)
	require.Len(t, c.Units, 1)
	assert.True(t, c.Units[0].NeedsSeed())
}

func TestParseCaseErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		content string
		want    string
	}{
		{
			name:    "unknown section",
			content: "-- xr.yaml --\nspec: {}\n-- enviroment.yaml --\ntier: dev\n",
			want:    `unknown section "enviroment.yaml"`,
		},
		{
			name:    "duplicate section",
			content: "-- xr.yaml --\nspec: {}\n-- xr.yaml --\nspec: {}\n",
			want:    `duplicate section "xr.yaml"`,
		},
		{
			name:    "missing xr",
			content: "-- want.cue --\nresources: {}\n",
			want:    `missing required section "xr.yaml"`,
		},
		{
			name:    "error.txt with want.cue",
			content: "-- xr.yaml --\nspec: {}\n-- want.cue --\nresources: {}\n-- error.txt --\nboom\n",
			want:    "error.txt cannot be combined with want.cue or want.yaml",
		},
		{
			name:    "empty error.txt",
			content: "-- xr.yaml --\nspec: {}\n-- error.txt --\n\n",
			want:    "at least one non-empty line",
		},
		{
			name: "base observed with steps",
			content: "-- xr.yaml --\nspec: {}\n-- observed.yaml --\nkind: X\n" +
				"-- 1/observed.yaml --\nkind: X\n-- 1/want.cue --\nresources: {}\n",
			want: "cannot be combined with step sections",
		},
		{
			name: "error.txt with steps",
			content: "-- xr.yaml --\nspec: {}\n-- error.txt --\nboom\n" +
				"-- 1/observed.yaml --\nkind: X\n-- 1/want.cue --\nresources: {}\n",
			want: "error.txt cannot be combined with step sections",
		},
		{
			name: "non-contiguous steps",
			content: "-- xr.yaml --\nspec: {}\n" +
				"-- 1/observed.yaml --\nkind: X\n-- 1/want.cue --\nresources: {}\n" +
				"-- 3/observed.yaml --\nkind: X\n-- 3/want.cue --\nresources: {}\n",
			want: "numbered contiguously from 1 (found step 3 without step 2)",
		},
		{
			name:    "step without observed",
			content: "-- xr.yaml --\nspec: {}\n-- 1/want.cue --\nresources: {}\n",
			want:    "step 1 needs an observed.yaml section",
		},
		{
			name:    "step error.txt",
			content: "-- xr.yaml --\nspec: {}\n-- 1/observed.yaml --\nkind: X\n-- 1/error.txt --\nboom\n",
			want:    "steps allow only observed.yaml, want.cue, and want.yaml",
		},
		{
			name:    "zero step index",
			content: "-- xr.yaml --\nspec: {}\n-- 0/observed.yaml --\nkind: X\n",
			want:    `unknown section "0/observed.yaml"`,
		},
		{
			name:    "non-numeric step label",
			content: "-- xr.yaml --\nspec: {}\n-- first/observed.yaml --\nkind: X\n",
			want:    `unknown section "first/observed.yaml"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, err := ParseCase("case.txtar", []byte(tt.content))
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.want)
			assert.Contains(t, err.Error(), `test case "case"`)
		})
	}
}
