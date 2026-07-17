package common

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/google/go-containerregistry/pkg/v1/random"
	"github.com/google/go-containerregistry/pkg/v1/tarball"
	"github.com/stretchr/testify/require"
)

// FakeRuntimeBase builds a synthetic runtime image base standing in for the apko
// image: a valid linux image for the given arch with the entrypoint /usr/bin/cuefn
// and cmd ["function"] the real image carries. The tests assert the Function
// package specializes that generic base without mutating it. The layer count is
// unified (256-byte, 2-layer) across the previously divergent copies.
func FakeRuntimeBase(t *testing.T, arch string) v1.Image {
	t.Helper()

	img, err := random.Image(256, 2)
	require.NoError(t, err)

	cfg, err := img.ConfigFile()
	require.NoError(t, err)
	cfg = cfg.DeepCopy()
	cfg.OS = "linux"
	cfg.Architecture = arch
	cfg.Config.Entrypoint = []string{"/usr/bin/cuefn"}
	cfg.Config.Cmd = []string{"function"}

	img, err = mutate.ConfigFile(img, cfg)
	require.NoError(t, err)
	return img
}

// WriteRuntimeBaseTarball writes a synthetic runtime base image (see
// FakeRuntimeBase) to a docker/OCI tarball and returns its path, standing in for
// the apko image.tar the publish-function command loads.
func WriteRuntimeBaseTarball(t *testing.T, arch string) string {
	t.Helper()

	img := FakeRuntimeBase(t, arch)

	path := filepath.Join(t.TempDir(), "image-"+arch+".tar")
	tag, err := name.NewTag("crossplane-cuefn:dev-" + arch)
	require.NoError(t, err)
	require.NoError(t, tarball.WriteToFile(path, tag, img))
	return path
}

// RuntimeBaseImage loads the real apko-built runtime image when it is present as a
// local tarball (the image.tar `mise run image-local` writes, or the path in
// CUEFN_RUNTIME_IMAGE) and otherwise falls back to a synthetic runtime stand-in,
// so the gated tests prove the embed-runtime path over the real image in the
// funcpkg-test task while staying runnable elsewhere.
func RuntimeBaseImage(t *testing.T) v1.Image {
	t.Helper()

	candidates := []string{
		os.Getenv("CUEFN_RUNTIME_IMAGE"),
		filepath.Join(RepoRoot(t), "image.tar"),
	}
	for _, p := range candidates {
		if p == "" {
			continue
		}
		if _, err := os.Stat(p); err != nil {
			continue
		}
		img, err := tarball.ImageFromPath(p, nil)
		require.NoError(t, err, "load runtime base tarball %q", p)
		t.Logf("using real runtime base image from %s", p)
		return img
	}

	t.Log(
		"no runtime image tarball found; using a synthetic runtime base (set CUEFN_RUNTIME_IMAGE or run `mise run image-local`)",
	)
	return FakeRuntimeBase(t, "amd64")
}
