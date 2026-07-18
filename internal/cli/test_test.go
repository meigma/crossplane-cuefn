package cli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testCmdXR = `apiVersion: platform.meigma.io/v1alpha1
kind: XApp
metadata:
  name: demo
  namespace: default
spec:
  replicas: 3
`

const passingCase = "-- xr.yaml --\n" + testCmdXR + `-- want.cue --
resources: deployment: {
	ready: "Ready"
	object: spec: replicas: 3
}
`

// testModuleDir clones the hermetic fixture module into a temp dir so a test
// can add tests/ cases (and let seeding rewrite them) without touching the
// shared fixture.
func testModuleDir(t *testing.T, cases map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	src := filepath.Join("..", "test", "common", "testdata", "module")
	require.NoError(t, os.CopyFS(dir, os.DirFS(src)))
	if len(cases) > 0 {
		require.NoError(t, os.Mkdir(filepath.Join(dir, "tests"), 0o700))
		for name, content := range cases {
			require.NoError(t, os.WriteFile(filepath.Join(dir, "tests", name+".txtar"), []byte(content), 0o600))
		}
	}
	return dir
}

// runTestCommand drives the real `cuefn test` command and returns its output.
func runTestCommand(t *testing.T, args ...string) (string, error) {
	t.Helper()
	var out bytes.Buffer
	root := NewRootCommand(Options{Out: &out, Err: &out})
	root.SetArgs(append([]string{"test"}, args...))
	err := root.ExecuteContext(context.Background())
	return out.String(), err
}

func TestTestCommand_PassAndSummary(t *testing.T) {
	t.Parallel()

	dir := testModuleDir(t, map[string]string{"replicas": passingCase})
	out, err := runTestCommand(t, "--dir", dir)
	require.NoError(t, err)
	assert.Contains(t, out, "PASS replicas")
	assert.Contains(t, out, "1 passed, 0 failed, 0 seeded, 0 updated")
}

func TestTestCommand_RunFilter(t *testing.T) {
	t.Parallel()

	dir := testModuleDir(t, map[string]string{
		"alpha": passingCase,
		"beta":  passingCase,
	})

	out, err := runTestCommand(t, "--dir", dir, "--run", "^alpha$")
	require.NoError(t, err)
	assert.Contains(t, out, "PASS alpha")
	assert.NotContains(t, out, "beta")

	_, err = runTestCommand(t, "--dir", dir, "--run", "nothing-matches")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no test cases match")
}

func TestTestCommand_FailureExitsNonZero(t *testing.T) {
	t.Parallel()

	failing := "This case pins the wrong replica count on purpose.\n\n" +
		"-- xr.yaml --\n" + testCmdXR +
		"-- want.cue --\nresources: deployment: object: spec: replicas: 5\n"
	dir := testModuleDir(t, map[string]string{"wrong": failing})

	out, err := runTestCommand(t, "--dir", dir)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "1 of 1 test cases failed")
	assert.Contains(t, out, "FAIL wrong")
	assert.Contains(t, out, "# This case pins the wrong replica count on purpose.")
	assert.Contains(t, out, "conflicting values")
	assert.Contains(t, out, "wrong.txtar/want.cue:")
}

func TestTestCommand_SeedFlow(t *testing.T) {
	t.Setenv("CI", "0")

	dir := testModuleDir(t, map[string]string{"golden": "-- xr.yaml --\n" + testCmdXR})
	file := filepath.Join(dir, "tests", "golden.txtar")

	out, err := runTestCommand(t, "--dir", dir)
	require.Error(t, err, "a seeding run must exit non-zero for review")
	assert.Contains(t, err.Error(), "seeded 1 test case(s)")
	assert.Contains(t, out, "SEED golden")

	seeded, readErr := os.ReadFile(file)
	require.NoError(t, readErr)
	assert.Contains(t, string(seeded), "-- want.yaml --")

	out, err = runTestCommand(t, "--dir", dir)
	require.NoError(t, err)
	assert.Contains(t, out, "PASS golden")
}

func TestTestCommand_CIRefusesSeeding(t *testing.T) {
	t.Parallel()

	source := "-- xr.yaml --\n" + testCmdXR
	dir := testModuleDir(t, map[string]string{"golden": source})
	file := filepath.Join(dir, "tests", "golden.txtar")

	out, err := runTestCommand(t, "--dir", dir, "--ci")
	require.Error(t, err)
	assert.Contains(t, out, "FAIL golden")
	assert.Contains(t, out, "run `cuefn test` locally to seed")

	unchanged, readErr := os.ReadFile(file)
	require.NoError(t, readErr)
	assert.Equal(t, source, string(unchanged), "CI mode must never write goldens")
}

func TestTestCommand_UpdateFlow(t *testing.T) {
	t.Setenv("CI", "0")

	dir := testModuleDir(t, map[string]string{"golden": "-- xr.yaml --\n" + testCmdXR})
	file := filepath.Join(dir, "tests", "golden.txtar")

	_, err := runTestCommand(t, "--dir", dir)
	require.Error(t, err, "seed run exits non-zero")

	// Drift: the XR changes but the committed golden does not.
	seeded, readErr := os.ReadFile(file)
	require.NoError(t, readErr)
	drifted := strings.Replace(string(seeded), "spec:\n  replicas: 3", "spec:\n  replicas: 4", 1)
	require.NotEqual(t, string(seeded), drifted)
	require.NoError(t, os.WriteFile(file, []byte(drifted), 0o600))

	out, err := runTestCommand(t, "--dir", dir)
	require.Error(t, err)
	assert.Contains(t, out, "FAIL golden")
	assert.Contains(t, out, "golden mismatch")

	out, err = runTestCommand(t, "--dir", dir, "--update")
	require.NoError(t, err)
	assert.Contains(t, out, "UPDATE golden")

	out, err = runTestCommand(t, "--dir", dir)
	require.NoError(t, err)
	assert.Contains(t, out, "PASS golden")
}

func TestTestCommand_CIRefusesUpdate(t *testing.T) {
	t.Parallel()

	dir := testModuleDir(t, map[string]string{"replicas": passingCase})
	_, err := runTestCommand(t, "--dir", dir, "--ci", "--update")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--update is not allowed in CI mode")
}

func TestTestCommand_ErrorCase(t *testing.T) {
	t.Parallel()

	overMax := strings.Replace(testCmdXR, "replicas: 3", "replicas: 99", 1)
	dir := testModuleDir(t, map[string]string{
		"rejects-out-of-bounds": "-- xr.yaml --\n" + overMax + "-- error.txt --\nreplicas\n",
	})

	out, err := runTestCommand(t, "--dir", dir)
	require.NoError(t, err)
	assert.Contains(t, out, "PASS rejects-out-of-bounds")
}

func TestTestCommand_NoCases(t *testing.T) {
	t.Parallel()

	dir := testModuleDir(t, nil)
	_, err := runTestCommand(t, "--dir", dir)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no test cases found")
}
