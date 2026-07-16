package render

import (
	"testing"

	"cuelang.org/go/cue/cuecontext"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUsesObservedResourcesRequiresRegularField(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		cue  string
		want bool
	}{
		{
			name: "regular field opts in",
			cue:  `out: input: observedResources: [string]: _`,
			want: true,
		},
		{
			name: "inherited optional field does not opt in",
			cue: `
#Input: {observedResources?: [string]: _}
out: input: #Input
`,
		},
		{
			name: "required field does not opt in",
			cue:  `out: input: observedResources!: [string]: _`,
		},
		{
			name: "regular field materializes optional contract field",
			cue: `
#Input: {observedResources?: [string]: _}
out: input: #Input & {observedResources: [string]: _}
`,
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			value := cuecontext.New().CompileString(tt.cue)
			require.NoError(t, value.Err())
			got, err := usesObservedResources(value)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}
