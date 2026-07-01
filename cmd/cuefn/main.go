package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"runtime/debug"
	"strings"
	"syscall"

	"github.com/meigma/crossplane-cuefn/internal/cli"
)

//nolint:gochecknoglobals // GoReleaser injects these values with ldflags during releases.
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

// resolveBuildInfo fills version/commit/date from the module build info when they
// were not stamped by ldflags. Release builds (GoReleaser, Nix, melange) set the
// version via ldflags, so this only affects `go install ...@vX.Y.Z` (which reports
// the module version) and plain `go build` from a checkout (which reports the VCS
// revision and time).
func resolveBuildInfo() (string, string, string) {
	v, c, d := version, commit, date
	if v != "dev" {
		return v, c, d
	}

	info, ok := debug.ReadBuildInfo()
	if !ok {
		return v, c, d
	}
	if mv := info.Main.Version; mv != "" && mv != "(devel)" {
		v = strings.TrimPrefix(mv, "v")
	}
	for _, setting := range info.Settings {
		switch setting.Key {
		case "vcs.revision":
			if c == "none" {
				c = setting.Value
			}
		case "vcs.time":
			if d == "unknown" {
				d = setting.Value
			}
		}
	}

	return v, c, d
}

func main() {
	os.Exit(run())
}

func run() int {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	buildVersion, buildCommit, buildDate := resolveBuildInfo()
	root := cli.NewRootCommand(cli.Options{
		In: os.Stdin,
		Build: cli.BuildInfo{
			Version: buildVersion,
			Commit:  buildCommit,
			Date:    buildDate,
		},
		Out: os.Stdout,
		Err: os.Stderr,
	})
	if err := root.ExecuteContext(ctx); err != nil {
		if _, writeErr := fmt.Fprintln(os.Stderr, err); writeErr != nil {
			return 1
		}

		return 1
	}

	return 0
}
