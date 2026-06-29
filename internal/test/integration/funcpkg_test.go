package integration_test

import (
	"os/exec"
	"strconv"
	"strings"
	"testing"

	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/daemon"
	"github.com/stretchr/testify/require"

	"github.com/meigma/crossplane-cuefn/internal/pkg"
	"github.com/meigma/crossplane-cuefn/internal/test/common"
)

// TestFunctionPackageServesGRPC proves the assembled Function xpkg image — the
// apko runtime image plus the package.yaml layer — still runs `cuefn function`
// as its entrypoint and serves the gRPC FunctionRunnerService (criterion 3). It
// loads the local dev image, assembles the Function package over it via
// internal/pkg, writes the result back into the Docker daemon, runs it, and dials
// gRPC. Self-skips without Docker or the dev image (run after `mise run
// image-local`).
func TestFunctionPackageServesGRPC(t *testing.T) {
	docker, image := common.RequireDevImage(t)

	// Load the apko runtime image from the local daemon and assemble the Function
	// xpkg over it (the package image IS the runtime image plus the package layer).
	baseRef, err := name.NewTag(image)
	require.NoError(t, err)
	base, err := daemon.Image(baseRef)
	require.NoError(t, err, "load %s from the docker daemon", image)

	meta, err := pkg.GenerateFunctionMeta(pkg.FunctionMeta{Name: "function-cuefn"})
	require.NoError(t, err)
	fn, err := pkg.DefaultFunction(meta)
	require.NoError(t, err)
	img, err := pkg.BuildFunctionImage(base, fn)
	require.NoError(t, err)

	// Write the assembled package image back into the daemon under its own tag.
	pkgTag, err := name.NewTag("crossplane-cuefn:funcpkg-smoke")
	require.NoError(t, err)
	_, err = daemon.Write(pkgTag, img)
	require.NoError(t, err, "load assembled Function package image into the daemon")
	t.Cleanup(func() { _ = exec.Command(docker, "rmi", "-f", pkgTag.String()).Run() })

	// Run the PACKAGE image as a container, overriding cmd to the serve args. The
	// package layer must not change serving: the entrypoint is still /usr/bin/cuefn.
	port := strconv.Itoa(common.FreePort(t))
	run := exec.Command(docker, "run", "--rm", "-d",
		"-p", port+":9443",
		pkgTag.String(),
		"function", "--insecure", "--address", ":9443",
	)
	idOut, err := run.CombinedOutput()
	require.NoError(t, err, "docker run: %s", idOut)
	id := strings.TrimSpace(string(idOut))
	t.Cleanup(func() { _ = exec.Command(docker, "rm", "-f", id).Run() })

	// The packaged image answers RunFunction over gRPC, proving it serves the
	// FunctionRunnerService rather than printing help.
	common.WaitForFunction(t, "127.0.0.1:"+port)
}
