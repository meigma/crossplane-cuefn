package integration_test

import (
	"context"
	"os"
	"os/exec"
	"testing"

	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"sigs.k8s.io/yaml"

	"github.com/meigma/crossplane-cuefn/internal/pkg"
	"github.com/meigma/crossplane-cuefn/internal/test/common"
)

// TestXpkgValidate proves the external crossplane CLI accepts our built packages:
// each artifact is written as a local .xpkg tarball, `crossplane xpkg extract`
// parses it into its cache format, and the extracted stream names the expected
// kinds (criteria 1/1c). The "function" case additionally proves the
// embed-runtime path: the package layer rides on top of the runtime base's
// layers and normalizes its entrypoint for Crossplane runtime flags. No
// registry/Docker needed; crossplane-gated once for both cases.
func TestXpkgValidate(t *testing.T) {
	bin := common.RequireCrossplane(t)

	tests := []struct {
		name  string
		base  string
		build func(t *testing.T) v1.Image
		kinds []string
	}{
		{
			name: "configuration",
			base: "configuration",
			build: func(t *testing.T) v1.Image {
				img, err := pkg.BuildConfigurationImage(common.BuildFixtureConfiguration(t))
				require.NoError(t, err)
				return img
			},
			kinds: []string{"Configuration", "CompositeResourceDefinition", "Composition"},
		},
		{
			name: "function",
			base: "function",
			build: func(t *testing.T) v1.Image {
				base := common.RuntimeBaseImage(t)
				baseLayers, err := base.Layers()
				require.NoError(t, err)

				img, err := pkg.BuildFunctionImage(base, common.FixtureFunction(t))
				require.NoError(t, err)

				// Embed-runtime: the runtime layers survive under the package
				// layer and the serving subcommand moves into the entrypoint.
				imgLayers, err := img.Layers()
				require.NoError(t, err)
				assert.Len(t, imgLayers, len(baseLayers)+1, "package layer must ride on top of the runtime layers")

				cfg, err := img.ConfigFile()
				require.NoError(t, err)
				assert.Equal(
					t,
					[]string{"/usr/bin/cuefn", "function"},
					cfg.Config.Entrypoint,
					"Function package must select the serving subcommand in its entrypoint",
				)
				assert.Empty(
					t,
					cfg.Config.Cmd,
					"Crossplane must be free to replace the package command with runtime flags",
				)
				return img
			},
			kinds: []string{"Function", "CustomResourceDefinition"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			img := tt.build(t)
			kinds := common.ExtractKinds(t, bin, img, tt.base)
			for _, k := range tt.kinds {
				assert.Contains(t, kinds, k)
			}
		})
	}
}

// TestFunctionXpkgSupplyChain proves the supply-chain mechanisms end to end with
// named commands. The Function xpkg is pushed to a throwaway registry ONCE, then
// two independently-gated subtests exercise it: "cosign" signs and verifies the
// pushed image with a locally-generated key (criterion 2, local; no transparency
// log), and "syft" generates and parses an SBOM for it (criterion 2, local). The
// per-tool gate lives inside each subtest, so a machine with only one of cosign /
// syft still runs that half. Docker-gated; run under `mise exec` so the pinned
// cosign/syft win.
func TestFunctionXpkgSupplyChain(t *testing.T) {
	reg := common.StartRegistry(t)

	img, err := pkg.BuildFunctionImage(common.RuntimeBaseImage(t), common.FixtureFunction(t))
	require.NoError(t, err)

	ref := reg.Host() + "/function-cuefn:v0.1.0"
	dst, err := pkg.Push(context.Background(), ref, img, true)
	require.NoError(t, err)

	t.Run("cosign", func(t *testing.T) {
		cosign := common.RequireBinary(t, "cosign")

		dir := t.TempDir()
		keygen := exec.Command(cosign, "generate-key-pair")
		keygen.Dir = dir
		keygen.Env = append(os.Environ(), "COSIGN_PASSWORD=")
		out, err := keygen.CombinedOutput()
		require.NoError(t, err, "cosign generate-key-pair:\n%s", out)

		// Key signing over a throwaway registry, no transparency log: --tlog-upload
		// and --use-signing-config are both disabled so cosign neither reaches the
		// public Rekor/TUF nor a default signing config (offline proof of the
		// mechanism, not a real keyless release).
		digestRef := dst.String()
		sign := exec.Command(cosign, "sign", "--key", "cosign.key", "--yes",
			"--allow-insecure-registry", "--use-signing-config=false", "--tlog-upload=false", digestRef)
		sign.Dir = dir
		sign.Env = append(os.Environ(), "COSIGN_PASSWORD=")
		out, err = sign.CombinedOutput()
		require.NoError(t, err, "cosign sign:\n%s", out)

		verify := exec.Command(cosign, "verify", "--key", "cosign.pub",
			"--allow-insecure-registry", "--insecure-ignore-tlog=true", digestRef)
		verify.Dir = dir
		out, err = verify.CombinedOutput()
		require.NoError(t, err, "cosign verify:\n%s", out)
	})

	t.Run("syft", func(t *testing.T) {
		syft := common.RequireBinary(t, "syft")

		cmd := exec.Command(syft, "scan", "registry:"+dst.String(), "-o", "spdx-json")
		cmd.Env = append(os.Environ(),
			"SYFT_REGISTRY_INSECURE_USE_HTTP=true",
			"SYFT_REGISTRY_INSECURE_SKIP_TLS_VERIFY=true",
		)
		out, err := cmd.Output()
		require.NoError(t, err, "syft scan must produce an SBOM")

		var doc struct {
			SPDXID   string `json:"SPDXID"`
			Packages []struct {
				Name string `json:"name"`
			} `json:"packages"`
		}
		require.NoError(t, yaml.Unmarshal(out, &doc))
		assert.NotEmpty(t, doc.SPDXID, "SBOM must be a parseable SPDX document")
		assert.NotEmpty(t, doc.Packages, "SBOM must list packages")
	})
}
