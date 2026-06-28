package cli

import (
	"bytes"
	"context"
	"testing"

	"github.com/spf13/viper"
)

func TestVersionFlagPrintsBuildMetadata(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	root := NewRootCommand(Options{
		Out: &stdout,
		Err: &stderr,
		Build: BuildInfo{
			Version: "0.1.0",
			Commit:  "abc1234",
			Date:    "2026-05-08T10:00:00Z",
		},
	})
	root.SetArgs([]string{"--version"})

	if err := root.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("ExecuteContext returned an error: %v", err)
	}
	if got, want := stdout.String(), "cuefn 0.1.0 (abc1234) built 2026-05-08T10:00:00Z\n"; got != want {
		t.Fatalf("stdout = %q, want %q", got, want)
	}
	if got := stderr.String(); got != "" {
		t.Fatalf("stderr = %q, want empty", got)
	}
}

func TestRootCommandPrintsConfiguredMessage(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	root := NewRootCommand(Options{
		Out:   &stdout,
		Viper: viper.New(),
	})
	root.SetArgs([]string{"--message", "hello from cobra"})

	if err := root.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("ExecuteContext returned an error: %v", err)
	}
	if got, want := stdout.String(), "hello from cobra\n"; got != want {
		t.Fatalf("stdout = %q, want %q", got, want)
	}
}

// TestPackagingCommandsRegistered proves the default (untagged) build wires the
// packaging commands into the tree. The `noxpkg` image build registers none of
// them (packaging_noxpkg.go) so the runtime binary omits internal/pkg's heavy
// dependency graph; this test runs untagged, guarding that the full CLI keeps
// publish and publish-function.
func TestPackagingCommandsRegistered(t *testing.T) {
	t.Parallel()

	root := NewRootCommand(Options{})

	registered := map[string]bool{}
	for _, c := range root.Commands() {
		registered[c.Name()] = true
	}
	if !registered["publish"] {
		t.Errorf("publish command not registered in the default build")
	}
	if !registered["publish-function"] {
		t.Errorf("publish-function command not registered in the default build")
	}
}

func TestRootCommandReadsMessageFromEnvironment(t *testing.T) {
	t.Setenv("CUEFN_MESSAGE", "hello from viper")

	var stdout bytes.Buffer
	root := NewRootCommand(Options{
		Out:   &stdout,
		Viper: viper.New(),
	})

	if err := root.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("ExecuteContext returned an error: %v", err)
	}
	if got, want := stdout.String(), "hello from viper\n"; got != want {
		t.Fatalf("stdout = %q, want %q", got, want)
	}
}
