package textdiff_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/meigma/crossplane-cuefn/internal/textdiff"
)

func TestLines(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		want string
		got  string
		diff string
	}{
		{
			name: "equal texts diff to unchanged lines",
			want: "a\nb\n",
			got:  "a\nb\n",
			diff: "  a\n  b\n",
		},
		{
			name: "changed line is a removal and an addition",
			want: "a\nb\nc\n",
			got:  "a\nx\nc\n",
			diff: "  a\n- b\n+ x\n  c\n",
		},
		{
			name: "trailing addition",
			want: "a\n",
			got:  "a\nb\n",
			diff: "  a\n+ b\n",
		},
		{
			name: "trailing removal",
			want: "a\nb\n",
			got:  "a\n",
			diff: "  a\n- b\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.diff, textdiff.Lines(tt.want, tt.got))
		})
	}
}

func TestNormalize(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "a\nb\n", textdiff.Normalize([]byte("a\r\nb\r\n")), "CRLF becomes LF")
	assert.Equal(t, "a\n", textdiff.Normalize([]byte("a\n\n\n")), "trailing newlines collapse to one")
	assert.Equal(t, "a\n", textdiff.Normalize([]byte("a")), "a missing trailing newline is added")
}
