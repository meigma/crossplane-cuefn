//go:build e2e

// This file is the kind end-to-end harness. It is gated behind the `e2e` build
// tag so the default `go test ./...` and moon's check graph never compile or run
// it. The dedicated moon `e2e-test` task builds with `-tags e2e` and provides
// Docker, kind, kubectl, helm, chainsaw, and the locally built
// `crossplane-cuefn:dev` image. (The package doc lives in doc.go.)

package e2e

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/daemon"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/stretchr/testify/require"

	"github.com/meigma/crossplane-cuefn/internal/pkg"
	"github.com/meigma/crossplane-cuefn/internal/render"
	"github.com/meigma/crossplane-cuefn/internal/schema"
	"github.com/meigma/crossplane-cuefn/internal/test/common"
)

const (
	devImage = "crossplane-cuefn:dev"

	clusterName = "cuefn-e2e"
	pkgRegName  = "cuefn-e2e-registry" // HTTPS, CA-trusted (Crossplane xpkgs)
	// pkgRegHost is the in-cluster DNS alias for the package registry. It is
	// deliberately DOTTED: Crossplane's package-ref CEL validation rejects a
	// registry host without a dot (it wants registry.example.com/...). The .test
	// TLD is RFC 6761-reserved (never a real domain), and unlike .local it carries
	// no mDNS special-casing, so CoreDNS forwards it to Docker's embedded DNS,
	// which resolves the network alias.
	pkgRegHost = "cuefn-e2e-registry.test"
	// Host-published ports are deliberately off :5000 — macOS Control Center
	// (AirPlay Receiver) binds *:5000. The in-cluster registry port stays :5000
	// (the container's listen port, see Registry.clusterID).
	pkgRegPort   = 5050
	modRegName   = "cuefn-modules" // plain HTTP, +insecure (CUE modules)
	modRegPort   = 5051
	moduleRef    = "cuefn.example/app@v0.1.0"
	moduleDir    = "testdata/module"
	functionName = "cuefn"
	fnTag        = "v0.1.0"
	cfgTag       = "v0.1.0"
)

// requireE2E self-skips the harness unless integration mode is on and every tool
// it shells (Docker, kind, kubectl, helm, chainsaw) plus the locally built dev
// image are present, mirroring the other gated suites' fail-soft behavior.
func requireE2E(t *testing.T) {
	t.Helper()
	if os.Getenv("CUEFN_INTEGRATION") == "" {
		t.Skip("integration test: set CUEFN_INTEGRATION=1 to run (via the e2e moon task/workflow)")
	}
	for _, bin := range []string{"docker", "kind", "kubectl", "helm", "chainsaw"} {
		if _, err := exec.LookPath(bin); err != nil {
			t.Skipf("%s not on PATH; skipping kind e2e", bin)
		}
	}
	if out, err := exec.Command("docker", "image", "inspect", devImage).CombinedOutput(); err != nil {
		t.Skipf("image %s not present (run `mise run image-local`): %s", devImage, out)
	}
}

// TestE2E_Kind drives the full author->publish->install->instantiate->reconcile
// loop on a real kind cluster and asserts the distinctive in-cluster behaviors via
// chainsaw: XR Ready=True with composed Deployment/Service/ConfigMap, status from
// #Status, API-server defaulting of an omitted field, the EnvironmentConfig value
// surfacing in a composed resource, and the digest-drift guard.
func TestE2E_Kind(t *testing.T) {
	requireE2E(t)

	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Minute)
	defer cancel()

	// --- Registries -----------------------------------------------------------
	// Registry B: plain-HTTP, serves the CUE modules the function fetches.
	modReg, err := StartHTTPRegistry(ctx, modRegName, modRegPort)
	require.NoError(t, err)
	t.Cleanup(func() { _ = modReg.Close(context.Background()) })

	// Registry A: HTTPS with a self-signed CA, serves the Crossplane xpkgs.
	pkgReg, err := StartTLSRegistry(ctx, pkgRegName, pkgRegHost, pkgRegPort)
	require.NoError(t, err)
	t.Cleanup(func() { _ = pkgReg.Close(context.Background()) })

	// --- Author + publish -----------------------------------------------------
	// Publish the CUE module to the module registry (host port), then resolve the
	// digest the Configuration must lock against.
	common.PublishModule(t, modReg.HostRef(), moduleRef, moduleDir)
	expectedDigest := resolveDigest(ctx, t, modReg.HostRef(), moduleRef)
	t.Logf("module %s resolved to %s", moduleRef, expectedDigest)

	fnHostRef := pkgReg.HostRef() + "/function-cuefn:" + fnTag
	fnClusterRef := pkgReg.ClusterRef() + "/function-cuefn:" + fnTag
	fnRepo := pkgReg.ClusterRef() + "/function-cuefn"
	pushFunctionPackage(ctx, t, pkgReg, fnHostRef)

	cfgHostRef := pkgReg.HostRef() + "/configuration-xapp:" + cfgTag
	cfgClusterRef := pkgReg.ClusterRef() + "/configuration-xapp:" + cfgTag
	pushConfigurationPackage(ctx, t, pkgReg, cfgHostRef, expectedDigest, fnRepo)

	// --- Cluster + Crossplane -------------------------------------------------
	cluster, err := NewCluster(ctx, clusterName)
	require.NoError(t, err)
	t.Cleanup(func() {
		// Dump diagnostics on failure before tearing the cluster down.
		if t.Failed() {
			dumpDiagnostics(t, cluster)
		}
		_ = cluster.Delete(context.Background())
	})

	require.NoError(t, modReg.Connect(ctx), "connect module registry to kind network")
	require.NoError(t, pkgReg.Connect(ctx), "connect package registry to kind network")
	require.NoError(t, cluster.TrustRegistry(ctx, pkgReg), "trust package registry in containerd")
	require.NoError(t, InstallCrossplane(ctx, cluster, pkgReg.CABundle()))

	// --- Install Function + Configuration -------------------------------------
	manifest := functionInstallManifest(fnClusterRef, modReg.ClusterRef())
	out, err := cluster.Apply(ctx, manifest)
	require.NoError(t, err, "apply function install manifest: %s", out)

	waitFor(t, cluster, "5m", "function.pkg.crossplane.io/"+functionName, "Healthy")
	waitFor(t, cluster, "5m", "function.pkg.crossplane.io/function-environment-configs", "Healthy")

	out, err = cluster.Apply(ctx, configurationManifest(cfgClusterRef))
	require.NoError(t, err, "apply configuration: %s", out)
	waitFor(t, cluster, "5m", "configuration.pkg.crossplane.io/xapp-configuration", "Healthy")

	// The XRD must be Established before XRs of the new kind can be created.
	waitFor(t, cluster, "3m",
		"compositeresourcedefinition.apiextensions.crossplane.io/xapps.platform.meigma.io",
		"Established")

	// --- Reconcile assertions (criteria 1-4) ----------------------------------
	runChainsaw(ctx, t, cluster, "reconcile.yaml", true /* skipDelete */)

	// --- Digest-drift guard (criterion 5) -------------------------------------
	// Republish DIFFERENT content under the SAME version, then force a reconcile.
	driftDir := filepath.Join(common.RepoRoot(t), "example/module")
	common.PublishModule(t, modReg.HostRef(), moduleRef, driftDir)
	driftDigest := resolveDigest(ctx, t, modReg.HostRef(), moduleRef)
	require.NotEqual(t, expectedDigest, driftDigest, "drift content must change the digest")
	out, err = cluster.Kubectl(ctx, "-n", "default", "annotate", "xapp", "demo",
		"cuefn.meigma.io/drift="+time.Now().Format("150405"), "--overwrite")
	require.NoError(t, err, "annotate to force reconcile: %s", out)

	runChainsaw(ctx, t, cluster, "drift.yaml", false /* skipDelete */)
}

// resolveDigest resolves ref's current manifest digest from the module registry,
// the same value the Configuration locks against.
func resolveDigest(ctx context.Context, t *testing.T, host, ref string) string {
	t.Helper()
	loader, err := render.NewOCILoader(render.OCIConfig{
		CacheDir: t.TempDir(),
		Env:      append(os.Environ(), "CUE_REGISTRY="+host+"+insecure"),
	})
	require.NoError(t, err)
	dg, err := loader.ResolveDigest(ctx, ref)
	require.NoError(t, err)
	return dg
}

// pushFunctionPackage assembles the Function xpkg over the local dev image and
// pushes it to the TLS package registry over a CA-trusted connection.
func pushFunctionPackage(ctx context.Context, t *testing.T, reg *Registry, ref string) {
	t.Helper()
	baseRef, err := name.NewTag(devImage)
	require.NoError(t, err)
	base, err := daemon.Image(baseRef)
	require.NoError(t, err, "load %s from the docker daemon", devImage)

	meta, err := pkg.GenerateFunctionMeta(pkg.FunctionMeta{Name: "function-cuefn"})
	require.NoError(t, err)
	fn, err := pkg.DefaultFunction(meta)
	require.NoError(t, err)
	img, err := pkg.BuildFunctionImage(base, fn)
	require.NoError(t, err)

	_, err = pkg.Push(ctx, ref, img, false, remote.WithTransport(reg.Transport()))
	require.NoError(t, err, "push Function xpkg to %s", ref)
}

// pushConfigurationPackage generates the XRD + Composition from the module, locks
// the expected digest and function dependency, and pushes the Configuration xpkg
// to the TLS package registry.
func pushConfigurationPackage(ctx context.Context, t *testing.T, reg *Registry, ref, expectedDigest, fnRepo string) {
	t.Helper()

	mod, cleanup, err := render.LoadModule(ctx, render.LocalLoader{Dir: moduleDir}, moduleRef)
	require.NoError(t, err)
	defer cleanup()

	xrd, err := schema.GenerateXRD(mod)
	require.NoError(t, err)

	meta, err := pkg.GenerateConfigurationMeta(pkg.ConfigurationMeta{
		Name:            "xapp-configuration",
		FunctionPackage: fnRepo,
		FunctionVersion: ">=v0.1.0",
	})
	require.NoError(t, err)

	comp, err := pkg.GenerateComposition(xrd, pkg.CompositionInput{
		Module:                moduleRef,
		ExpectedDigest:        expectedDigest,
		FunctionName:          functionName,
		EnvironmentConfigRefs: []string{"app-environment"},
	})
	require.NoError(t, err)

	img, err := pkg.BuildConfigurationImage(pkg.Configuration{Meta: meta, XRD: xrd, Composition: comp})
	require.NoError(t, err)

	_, err = pkg.Push(ctx, ref, img, false, remote.WithTransport(reg.Transport()))
	require.NoError(t, err, "push Configuration xpkg to %s", ref)
}

// functionInstallManifest renders the in-cluster Function install: the cuefn
// Function pinned to its package ref with a DeploymentRuntimeConfig injecting the
// module registry and a writable cache, plus function-environment-configs.
func functionInstallManifest(fnRef, modClusterRef string) []byte {
	return fmt.Appendf(nil, `apiVersion: pkg.crossplane.io/v1beta1
kind: DeploymentRuntimeConfig
metadata:
  name: cuefn-runtime
spec:
  deploymentTemplate:
    spec:
      selector: {}
      template:
        spec:
          containers:
            - name: package-runtime
              env:
                - name: CUE_REGISTRY
                  value: "%s+insecure"
                - name: CUE_CACHE_DIR
                  value: "/cuefn-cache"
              volumeMounts:
                - name: cue-cache
                  mountPath: /cuefn-cache
          volumes:
            - name: cue-cache
              emptyDir: {}
---
apiVersion: pkg.crossplane.io/v1
kind: Function
metadata:
  name: %s
spec:
  package: %s
  runtimeConfigRef:
    name: cuefn-runtime
---
apiVersion: pkg.crossplane.io/v1
kind: Function
metadata:
  name: function-environment-configs
spec:
  package: xpkg.crossplane.io/crossplane-contrib/function-environment-configs:v0.7.2
`, modClusterRef, functionName, fnRef)
}

// configurationManifest renders the Configuration package install.
func configurationManifest(cfgRef string) []byte {
	return fmt.Appendf(nil, `apiVersion: pkg.crossplane.io/v1
kind: Configuration
metadata:
  name: xapp-configuration
spec:
  package: %s
`, cfgRef)
}

// waitFor blocks until the named resource reports the given condition true.
func waitFor(t *testing.T, cluster *Cluster, timeout, resource, condition string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	out, err := cluster.Kubectl(ctx, "wait", "--for=condition="+condition, resource, "--timeout="+timeout)
	require.NoError(t, err, "wait for %s condition %s: %s", resource, condition, out)
}

// runChainsaw runs a single chainsaw Test file against the cluster, pointing
// chainsaw at the cluster's kubeconfig. skipDelete leaves applied resources in
// place so a later Test (the drift guard) can observe the same XR.
func runChainsaw(ctx context.Context, t *testing.T, cluster *Cluster, testFile string, skipDelete bool) {
	t.Helper()
	chainsawDir := filepath.Join(common.RepoRoot(t), "test/chainsaw/e2e")
	args := []string{"test", chainsawDir, "--test-file", testFile}
	if skipDelete {
		args = append(args, "--skip-delete")
	}
	cmd := exec.CommandContext(ctx, "chainsaw", args...)
	cmd.Env = append(os.Environ(), "KUBECONFIG="+cluster.Kubeconfig())
	out, err := cmd.CombinedOutput()
	t.Logf("chainsaw %s output:\n%s", testFile, out)
	require.NoError(t, err, "chainsaw %s must pass", testFile)
}

// dumpDiagnostics logs Crossplane and XR state to aid debugging a failed run.
func dumpDiagnostics(t *testing.T, cluster *Cluster) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	for _, args := range [][]string{
		{"get", "pkg", "-A"},
		{"get", "xapp", "-A", "-o", "yaml"},
		{"get", "deploy", "demo", "-n", "default", "-o", "yaml"},
		{"get", "cm", "demo", "-n", "default", "-o", "yaml"},
		{"get", "deploy,svc,cm", "-n", "default", "--show-labels"},
		{"get", "pods", "-n", "crossplane-system"},
	} {
		out, _ := cluster.Kubectl(ctx, args...)
		t.Logf("kubectl %v:\n%s", args, out)
	}
}
