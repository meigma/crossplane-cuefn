package cli

import (
	"bytes"
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestValidateCmd_ExitCodes is the CLI-level exit-code table for criterion 4:
// invalid XRs make `cuefn validate` return an error (cobra exits non-zero) with a
// field-located message, while valid and omitted-default XRs return nil (exit
// zero). The module is served offline via --dir.
func TestValidateCmd_ExitCodes(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name      string
		xr        string
		wantErr   bool
		wantField string
	}{
		{name: "valid", xr: "testdata/xr-valid.yaml"},
		{name: "omitted default", xr: "testdata/xr-omitted-default.yaml"},
		{name: "out of bounds", xr: "testdata/xr-out-of-bounds.yaml", wantErr: true, wantField: "replicas"},
		{name: "wrong enum", xr: "testdata/xr-wrong-enum.yaml", wantErr: true, wantField: "tier"},
		{name: "missing required", xr: "testdata/xr-missing-required.yaml", wantErr: true, wantField: "image"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			root := NewRootCommand(Options{Out: &bytes.Buffer{}, Err: &bytes.Buffer{}})
			root.SetArgs([]string{"validate", tc.xr, "--dir", deriskedModuleDir})

			err := root.ExecuteContext(context.Background())
			if !tc.wantErr {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.wantField,
				"error must name the offending field")
		})
	}
}
