package integration_test

import (
	"os/exec"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/meigma/crossplane-cuefn/internal/test/common"
)

// TestImageServesFunction_NoArgs proves apko's default `cmd: function` dispatches
// to the function subcommand when the image is run with NO arguments — the path
// Crossplane actually exercises in-cluster (it runs the image's default command,
// not `function --insecure`). With no certs dir and without --insecure,
// function-sdk-go's Serve refuses to start ("no credentials provided"); that very
// error proves the default command reached the gRPC serve path rather than
// printing root help/usage. It self-skips without Docker or the dev image
// (criterion 6 / the tracked no-args cleanup).
func TestImageServesFunction_NoArgs(t *testing.T) {
	docker, image := common.RequireDevImage(t)

	// No command args: rely entirely on apko's `cmd: function`. Capture output so
	// we can distinguish the function serve path from cobra's help text.
	run := exec.Command(docker, "run", "--rm", image)
	out, err := run.CombinedOutput()

	// The default command must run `function`, which fails fast on missing mTLS
	// credentials. A non-zero exit with the credentials error is the proof; a
	// zero exit (cobra help/usage) or a different error would mean the default
	// command path regressed.
	require.Error(
		t,
		err,
		"default command must run `function` and exit non-zero on missing creds, not print help: %s",
		out,
	)
	assert.Contains(t, string(out), "no credentials provided",
		"default `cmd: function` must reach sdk.Serve (got output:\n%s)", out)
	assert.NotContains(t, string(out), "Available Commands",
		"the image must not fall back to printing root help")
}
