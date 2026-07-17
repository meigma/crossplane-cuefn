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
// apko runtime image plus the package.yaml layer — accepts Crossplane's
// `--insecure` command override and serves the gRPC FunctionRunnerService. It
// loads the local dev image, assembles the Function package over it via
// internal/pkg, writes the result back into the Docker daemon, runs it exactly
// as Crossplane's Docker runtime does, and dials gRPC. Run after `mise run
// image-local`; integration mode treats the image as a required prerequisite.
func TestFunctionPackageServesGRPC(t *testing.T) {
	docker, pkgTag := loadFunctionPackage(t, "crossplane-cuefn:funcpkg-smoke")

	// Crossplane replaces the package Cmd with runtime flags. Do not repeat the
	// function subcommand here: the package entrypoint must already select it.
	port := strconv.Itoa(common.FreePort(t))
	run := exec.Command(docker, "run", "--rm", "-d",
		"-p", port+":9443",
		pkgTag.String(),
		"--insecure",
	)
	idOut, err := run.CombinedOutput()
	require.NoError(t, err, "docker run: %s", idOut)
	id := strings.TrimSpace(string(idOut))
	t.Cleanup(func() { _ = exec.Command(docker, "rm", "-f", id).Run() })

	// The packaged image answers RunFunction over gRPC, proving it serves the
	// FunctionRunnerService rather than printing help.
	common.WaitForFunction(t, "127.0.0.1:"+port)
}

func loadFunctionPackage(t *testing.T, tag string) (string, name.Tag) {
	t.Helper()

	docker, image := common.RequireDevImage(t)
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

	pkgTag, err := name.NewTag(tag)
	require.NoError(t, err)
	_, err = daemon.Write(pkgTag, img)
	require.NoError(t, err, "load assembled Function package image into the daemon")
	t.Cleanup(func() { _ = exec.Command(docker, "rmi", "-f", pkgTag.String()).Run() })
	return docker, pkgTag
}
