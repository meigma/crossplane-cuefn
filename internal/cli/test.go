package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/meigma/crossplane-cuefn/internal/testharness"
)

// testFlags holds the flags for the test subcommand.
type testFlags struct {
	dir      string
	cacheDir string
	run      string
	update   bool
	failFast bool
	ci       bool
}

// newTestCommand builds the `cuefn test` subcommand: it discovers the module's
// txtar test cases under tests/, renders each through the same engine the
// served function uses, and evaluates the declared expectations. One blessed
// layout, loud failures, and a machine-owned golden lifecycle (seed on first
// run, --update to re-bless) — the opinionated alternative to hand-rolled
// cue-wrapping test scripts.
func newTestCommand(options Options) *cobra.Command {
	f := testFlags{}

	cmd := &cobra.Command{
		Use:   "test",
		Short: "Run a module's test cases from tests/*.txtar",
		Long: "Run the module's txtar test cases under tests/. Each case supplies render " +
			"inputs (xr.yaml, environment.yaml, required.yaml, observed.yaml) and expectations: " +
			"want.cue (partial CUE unified with the rendered output), want.yaml (an exact " +
			"machine-maintained golden), or error.txt (substrings a failing render must report). " +
			"Numbered step sections (1/observed.yaml, 1/want.cue, ...) replay readiness sequences. " +
			"A case with no expectations is seeded: its rendered output is written to a want.yaml " +
			"section for review. In CI mode (--ci, or the CI environment variable) seeding and " +
			"--update are refused and drift fails the run.",
		Args:          cobra.NoArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runTest(cmd.Context(), options, f)
		},
	}

	cmd.Flags().StringVar(&f.dir, "dir", ".",
		"module directory; test cases are discovered in its tests/ subdirectory")
	cmd.Flags().StringVar(&f.cacheDir, "cache-dir", "",
		"directory for the CUE module cache and dependency downloads (overrides CUE_CACHE_DIR)")
	cmd.Flags().StringVar(&f.run, "run", "",
		"run only cases whose name matches this regular expression")
	cmd.Flags().BoolVar(&f.update, "update", false,
		"rewrite drifted want.yaml goldens from the rendered output (never touches want.cue or error.txt)")
	cmd.Flags().BoolVar(&f.failFast, "fail-fast", false,
		"stop after the first failing case")
	cmd.Flags().BoolVar(&f.ci, "ci", false,
		"CI mode: refuse golden seeding and --update so drift always fails "+
			"(auto-enabled when the CI environment variable is set)")

	return cmd
}

// caseOutcome is one case's reported result.
type caseOutcome struct {
	name   string
	status string // PASS, FAIL, SEED, UPDATE
	detail string // failure messages / seed note, already indented
}

// runTest discovers, runs, and reports the module's test cases.
func runTest(ctx context.Context, options Options, f testFlags) error {
	ci := f.ci || envCI()
	if ci && f.update {
		return errors.New("--update is not allowed in CI mode: goldens must be re-blessed locally and reviewed")
	}

	var pattern *regexp.Regexp
	if f.run != "" {
		var err error
		if pattern, err = regexp.Compile(f.run); err != nil {
			return fmt.Errorf("invalid --run pattern: %w", err)
		}
	}

	files, err := discoverCases(f.dir)
	if err != nil {
		return err
	}

	loader, err := moduleLoader(f.dir, f.cacheDir)
	if err != nil {
		return err
	}
	runner := &testharness.Runner{Loader: loader, Ref: f.dir}

	var outcomes []caseOutcome
	selected := 0
	for _, file := range files {
		if pattern != nil && !pattern.MatchString(caseName(file)) {
			continue
		}
		selected++

		outcome := runTestCase(ctx, runner, file, ci, f.update)
		outcomes = append(outcomes, outcome)
		fmt.Fprintf(options.Out, "%s %s\n", outcome.status, outcome.name)
		if outcome.detail != "" {
			fmt.Fprint(options.Out, outcome.detail)
		}

		if outcome.status == statusFail && f.failFast {
			break
		}
	}

	if selected == 0 {
		if pattern != nil {
			return fmt.Errorf("no test cases match --run %q", f.run)
		}
		return fmt.Errorf("no test cases found in %s", filepath.Join(f.dir, testsDir))
	}

	return summarize(options, outcomes)
}

const testsDir = "tests"

const (
	statusPass   = "PASS"
	statusFail   = "FAIL"
	statusSeed   = "SEED"
	statusUpdate = "UPDATE"
)

// discoverCases lists the module's txtar case files in name order.
func discoverCases(dir string) ([]string, error) {
	files, err := filepath.Glob(filepath.Join(dir, testsDir, "*.txtar"))
	if err != nil {
		return nil, fmt.Errorf("cannot discover test cases: %w", err)
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("no test cases found in %s", filepath.Join(dir, testsDir))
	}
	sort.Strings(files)
	return files, nil
}

// runTestCase executes one case file, applying the golden lifecycle (seed when
// expectations are absent, --update when requested) outside CI mode.
func runTestCase(ctx context.Context, runner *testharness.Runner, file string, ci, update bool) caseOutcome {
	name := caseName(file)

	raw, err := os.ReadFile(file)
	if err != nil {
		return failedOutcome(name, indent(err.Error()))
	}
	c, err := testharness.ParseCase(file, raw)
	if err != nil {
		return failedOutcome(name, indent(err.Error()))
	}

	res, err := runner.Run(ctx, c)
	if err != nil {
		return failedOutcome(name, indent(err.Error()))
	}

	if res.NeedsSeed() {
		return seedCase(file, name, raw, c, res, ci)
	}

	if update && !ci && hasGoldenDrift(res) {
		if outcome, handled := updateCase(ctx, runner, file, name, raw, res); handled {
			return outcome
		}
	}

	if failures := res.Failures(); len(failures) > 0 {
		return failedOutcome(name, formatFailures(c, failures))
	}
	return caseOutcome{name: name, status: statusPass}
}

// seedCase writes freshly rendered goldens for a case with no expectations.
// In CI mode nothing is written and the case fails.
func seedCase(
	file, name string,
	raw []byte,
	c *testharness.Case,
	res *testharness.CaseResult,
	ci bool,
) caseOutcome {
	if ci {
		return failedOutcome(name,
			indent("case has no expectations; run `cuefn test` locally to seed its golden, review, and commit"))
	}
	seeded, changed := testharness.SeedGoldens(raw, res)
	if changed {
		if err := writeCaseFile(file, seeded); err != nil {
			return failedOutcome(name, indent(fmt.Sprintf("cannot write seeded golden: %v", err)))
		}
	}
	outcome := caseOutcome{name: name, status: statusSeed,
		detail: indent("wrote want.yaml golden(s); review and commit, or refine into want.cue")}
	if failures := res.Failures(); len(failures) > 0 {
		outcome.status = statusFail
		outcome.detail += formatFailures(c, failures)
	}
	return outcome
}

// updateCase rewrites drifted want.yaml goldens and re-runs the case so any
// remaining non-golden failures still fail it. handled is false when nothing
// drifted (the caller reports the case normally).
func updateCase(
	ctx context.Context,
	runner *testharness.Runner,
	file, name string,
	raw []byte,
	res *testharness.CaseResult,
) (caseOutcome, bool) {
	updated, changed := testharness.UpdateGoldens(raw, res)
	if !changed {
		return caseOutcome{}, false
	}
	if err := writeCaseFile(file, updated); err != nil {
		return failedOutcome(name, indent(fmt.Sprintf("cannot write updated golden: %v", err))), true
	}

	c, err := testharness.ParseCase(file, updated)
	if err != nil {
		return failedOutcome(name, indent(err.Error())), true
	}
	rerun, err := runner.Run(ctx, c)
	if err != nil {
		return failedOutcome(name, indent(err.Error())), true
	}
	if failures := rerun.Failures(); len(failures) > 0 {
		return failedOutcome(name, formatFailures(c, failures)), true
	}
	return caseOutcome{name: name, status: statusUpdate,
		detail: indent("rewrote drifted want.yaml golden(s)")}, true
}

// caseName derives a case's report name from its file path.
func caseName(file string) string {
	return strings.TrimSuffix(filepath.Base(file), filepath.Ext(file))
}

// failedOutcome builds a FAIL outcome with pre-indented detail.
func failedOutcome(name, detail string) caseOutcome {
	return caseOutcome{name: name, status: statusFail, detail: detail}
}

// writeCaseFile rewrites a discovered case file in place. The path always
// comes from discoverCases' glob under the module's tests/ directory — the
// golden lifecycle exists precisely to rewrite the user's own case files.
func writeCaseFile(path string, data []byte) error {
	return os.WriteFile(
		filepath.Clean(path),
		data,
		0o600,
	) // #nosec G703 -- rewriting the user's own discovered test case
}

// hasGoldenDrift reports whether any failure is a want.yaml mismatch —
// the only kind --update may repair.
func hasGoldenDrift(res *testharness.CaseResult) bool {
	for _, failure := range res.Failures() {
		if failure.Kind == "want.yaml" {
			return true
		}
	}
	return false
}

// formatFailures renders a case's failures, prefixed with its description so
// the author sees the case's intent next to what broke.
func formatFailures(c *testharness.Case, failures []testharness.Failure) string {
	var b strings.Builder
	if c.Description != "" {
		b.WriteString(indent("# " + c.Description))
	}
	for _, failure := range failures {
		label := failure.Kind
		if failure.Unit != "" {
			label = "step " + failure.Unit + " " + failure.Kind
		}
		b.WriteString(indent("[" + label + "] " + failure.Message))
	}
	return b.String()
}

// summarize prints the run totals and returns a non-nil error when the run
// must exit non-zero (failures or freshly seeded goldens).
func summarize(options Options, outcomes []caseOutcome) error {
	counts := map[string]int{}
	for _, outcome := range outcomes {
		counts[outcome.status]++
	}
	fmt.Fprintf(options.Out, "\n%d passed, %d failed, %d seeded, %d updated\n",
		counts[statusPass], counts[statusFail], counts[statusSeed], counts[statusUpdate])

	if counts[statusFail] > 0 {
		return fmt.Errorf("%d of %d test cases failed", counts[statusFail], len(outcomes))
	}
	if counts[statusSeed] > 0 {
		return fmt.Errorf("seeded %d test case(s); review the want.yaml golden(s) and commit", counts[statusSeed])
	}
	return nil
}

// indent prefixes every non-empty line with four spaces and terminates it,
// nesting multi-line failure messages under their case line.
func indent(message string) string {
	var b strings.Builder
	for line := range strings.SplitSeq(strings.TrimRight(message, "\n"), "\n") {
		b.WriteString("    ")
		b.WriteString(line)
		b.WriteByte('\n')
	}
	return b.String()
}

// envCI reports whether the CI environment variable claims a CI environment
// ("", "0", and "false" do not).
func envCI() bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv("CI")))
	return v != "" && v != "0" && v != "false"
}
