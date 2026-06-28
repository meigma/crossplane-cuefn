//go:build !noxpkg

package cli

import "github.com/spf13/cobra"

// addPackagingCommands registers the xpkg packaging subcommands (publish and
// publish-function). These commands import internal/pkg, which pulls
// crossplane-runtime's xpkg builder and its sigstore/cosign + cloud-credential
// dependency graph. That graph is dead weight in the runtime image the Function
// package embeds (the runtime never packages or signs), so the melange/apko
// image binary is built with `-tags noxpkg`, which selects the no-op
// registration in packaging_noxpkg.go instead. The full CLI build (GoReleaser)
// keeps both commands. Measured impact: ~12 MiB / ~23% of the binary.
func addPackagingCommands(root *cobra.Command, o Options) {
	root.AddCommand(
		newPublishCommand(o),
		newPublishFunctionCommand(o),
	)
}
