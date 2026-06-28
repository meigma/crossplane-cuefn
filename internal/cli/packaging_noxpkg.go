//go:build noxpkg

package cli

import "github.com/spf13/cobra"

// addPackagingCommands is the noxpkg build's no-op: the image binary omits the
// publish / publish-function commands so it does not link internal/pkg and its
// sigstore/cosign + cloud-credential dependency graph. See packaging.go for the
// measured rationale. The function/render/generate/validate commands the runtime
// actually serves stay registered unconditionally in root.go.
func addPackagingCommands(_ *cobra.Command, _ Options) {}
