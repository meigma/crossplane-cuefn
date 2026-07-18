package cli

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"strings"

	"cuelang.org/go/cue"
	"github.com/spf13/cobra"

	"github.com/meigma/crossplane-cuefn/internal/check"
	"github.com/meigma/crossplane-cuefn/internal/render"
)

// checkFlags holds the flags for the check subcommand.
type checkFlags struct {
	dir      string
	cacheDir string
	xrd      string
	update   bool
	ci       bool
}

// newCheckCommand builds the `cuefn check` subcommand: the instance-free
// static health gate for a module. It runs three checks — fmt, vet, xrd —
// and reports each with the same PASS/FAIL/SEED/UPDATE vocabulary and golden
// lifecycle as `cuefn test`.
func newCheckCommand(options Options) *cobra.Command {
	f := checkFlags{}

	cmd := &cobra.Command{
		Use:   "check",
		Short: "Run a module's static health checks (fmt, vet, xrd)",
		Long: "Run the module's static health checks, none of which need an XR instance: " +
			"fmt (every CUE file is canonically formatted), vet (the module evaluates cleanly " +
			"without requiring concreteness — the equivalent of `cue vet -c=false ./...`), and " +
			"xrd (the module's XRD generates; with --xrd, that it also matches a reviewed golden " +
			"file). A missing golden is seeded on first run and --update re-blesses a drifted " +
			"one; in CI mode (--ci, or the CI environment variable) both are refused and drift " +
			"fails the run. Use `cuefn test` for instance-driven render behavior and " +
			"`cuefn validate` for one concrete XR against the module's schema.",
		Args:          cobra.NoArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runCheck(cmd.Context(), options, f)
		},
	}

	cmd.Flags().StringVar(&f.dir, "dir", ".",
		"module directory to check")
	cmd.Flags().StringVar(&f.cacheDir, "cache-dir", "",
		"directory for the CUE module cache and dependency downloads (overrides CUE_CACHE_DIR)")
	cmd.Flags().StringVar(&f.xrd, "xrd", "",
		"compare the generated XRD against this golden file (seeded when missing)")
	cmd.Flags().BoolVar(&f.update, "update", false,
		"rewrite a drifted XRD golden from the current generation")
	cmd.Flags().BoolVar(&f.ci, "ci", false,
		"CI mode: refuse golden seeding and --update so drift always fails "+
			"(auto-enabled when the CI environment variable is set)")

	return cmd
}

// runCheck runs the three checks in order, reporting every unit even after a
// failure so one run surfaces everything.
func runCheck(ctx context.Context, options Options, f checkFlags) error {
	ci := f.ci || envCI()
	if ci && f.update {
		return errors.New("--update is not allowed in CI mode: goldens must be re-blessed locally and reviewed")
	}

	files, err := collectCUEFiles(f.dir)
	if err != nil {
		return err
	}

	loader, err := moduleLoader(f.dir, f.cacheDir)
	if err != nil {
		return err
	}
	module, cleanup, loadErr := render.LoadModule(ctx, loader, f.dir)
	if loadErr == nil {
		defer cleanup()
	}

	outcomes := []caseOutcome{checkFmt(files)}
	outcomes = append(outcomes, checkVet(module, loadErr))
	outcomes = append(outcomes, checkXRD(module, loadErr, f, ci))

	for _, outcome := range outcomes {
		fmt.Fprintf(options.Out, "%s %s\n", outcome.status, outcome.name)
		if outcome.detail != "" {
			fmt.Fprint(options.Out, outcome.detail)
		}
	}

	return summarizeChecks(options, outcomes, f.xrd)
}

const (
	unitFmt = "fmt"
	unitVet = "vet"
	unitXRD = "xrd"
)

// checkFmt reports the fmt unit: every collected CUE file is in canonical
// form.
func checkFmt(files map[string][]byte) caseOutcome {
	unformatted, err := check.Fmt(files)
	if err != nil {
		return failedOutcome(unitFmt, indent(err.Error()))
	}
	if len(unformatted) > 0 {
		detail := "not canonically formatted (run `cue fmt` to fix):\n  " +
			strings.Join(unformatted, "\n  ")
		return failedOutcome(unitFmt, indent(detail))
	}
	return caseOutcome{name: unitFmt, status: statusPass}
}

// checkVet reports the vet unit. A module that fails to load reports the load
// error here: CUE surfaces evaluation conflicts eagerly at build, so the load
// failure IS the vet failure (see check.Vet).
func checkVet(module cue.Value, loadErr error) caseOutcome {
	if loadErr != nil {
		return failedOutcome(unitVet, indent(loadErr.Error()))
	}
	if err := check.Vet(module); err != nil {
		return failedOutcome(unitVet, indent(err.Error()))
	}
	return caseOutcome{name: unitVet, status: statusPass}
}

// checkXRD reports the xrd unit: generation always, the golden comparison and
// its seed/update lifecycle when --xrd is set.
func checkXRD(module cue.Value, loadErr error, f checkFlags, ci bool) caseOutcome {
	if loadErr != nil {
		return failedOutcome(unitXRD, indent("module did not load; see vet"))
	}

	golden, haveGolden, outcome := readGolden(f.xrd, ci)
	if outcome != nil {
		return *outcome
	}

	generated, diff, err := check.XRD(module, golden, haveGolden)
	if err != nil {
		return failedOutcome(unitXRD, indent(err.Error()))
	}

	if f.xrd != "" && !haveGolden {
		if err := writeGolden(f.xrd, generated); err != nil {
			return failedOutcome(unitXRD, indent(err.Error()))
		}
		return caseOutcome{name: unitXRD, status: statusSeed,
			detail: indent(fmt.Sprintf("wrote %s; review and commit", f.xrd))}
	}

	if diff != "" {
		if f.update && !ci {
			if err := writeGolden(f.xrd, generated); err != nil {
				return failedOutcome(unitXRD, indent(err.Error()))
			}
			return caseOutcome{name: unitXRD, status: statusUpdate,
				detail: indent(fmt.Sprintf("rewrote drifted golden %s", f.xrd))}
		}
		return failedOutcome(unitXRD, indent("golden mismatch (-golden +generated):\n"+diff))
	}
	return caseOutcome{name: unitXRD, status: statusPass}
}

// readGolden reads the --xrd golden. A missing file is not an error — it
// triggers seeding (refused in CI, reported as the returned outcome).
func readGolden(path string, ci bool) ([]byte, bool, *caseOutcome) {
	if path == "" {
		return nil, false, nil
	}
	golden, err := os.ReadFile(path)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		if ci {
			o := failedOutcome(unitXRD, indent(fmt.Sprintf(
				"golden %s does not exist; run `cuefn check` locally to seed it, review, and commit", path)))
			return nil, false, &o
		}
		return nil, false, nil
	case err != nil:
		o := failedOutcome(unitXRD, indent(fmt.Sprintf("cannot read golden: %v", err)))
		return nil, false, &o
	}
	return golden, true, nil
}

// writeGolden writes the machine-owned golden (header plus generation).
func writeGolden(path string, generated []byte) error {
	//nolint:gosec // an XRD golden is non-secret.
	err := os.WriteFile(path, check.GoldenBytes(generated, path), 0o644)
	if err != nil {
		return fmt.Errorf("cannot write golden %s: %w", path, err)
	}
	return nil
}

// collectCUEFiles walks dir and returns every .cue file keyed by dir-relative
// slash path. Hidden directories and CUE's legacy vendoring directories under
// cue.mod (pkg, gen, usr) are skipped; cue.mod/module.cue is included.
func collectCUEFiles(dir string) (map[string][]byte, error) {
	files := map[string][]byte{}
	fsys := os.DirFS(dir)
	err := fs.WalkDir(fsys, ".", func(name string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if name == "." {
				return nil
			}
			if strings.HasPrefix(d.Name(), ".") ||
				name == "cue.mod/pkg" || name == "cue.mod/gen" || name == "cue.mod/usr" {
				return fs.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(name, ".cue") {
			return nil
		}
		data, err := fs.ReadFile(fsys, name)
		if err != nil {
			return err
		}
		files[name] = data
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("cannot collect CUE files in %s: %w", dir, err)
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("no CUE files found in %s — is it a module directory?", dir)
	}
	return files, nil
}

// summarizeChecks prints the run totals and returns a non-nil error when the
// run must exit non-zero (failures or a freshly seeded golden).
func summarizeChecks(options Options, outcomes []caseOutcome, xrdPath string) error {
	counts := map[string]int{}
	for _, outcome := range outcomes {
		counts[outcome.status]++
	}
	fmt.Fprintf(options.Out, "\n%d passed, %d failed, %d seeded, %d updated\n",
		counts[statusPass], counts[statusFail], counts[statusSeed], counts[statusUpdate])

	if counts[statusFail] > 0 {
		return fmt.Errorf("%d of %d checks failed", counts[statusFail], len(outcomes))
	}
	if counts[statusSeed] > 0 {
		return fmt.Errorf("seeded the XRD golden %s; review and commit", xrdPath)
	}
	return nil
}
