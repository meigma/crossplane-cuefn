//go:build envtest

// The chainsaw/envtest server-side harness is gated behind the `envtest` build
// tag so the default `go test ./...` (and moon's check graph) stays portable and
// fast on a machine without chainsaw or the envtest binaries. The dedicated moon
// `schema-test` task builds with `-tags envtest` and provides those tools.
package schema_test

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	extv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	k8sschema "k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	"sigs.k8s.io/yaml"

	"github.com/meigma/crossplane-cuefn/internal/render"
	"github.com/meigma/crossplane-cuefn/internal/schema"
)

const (
	deriskedDir       = "testdata/derisked"
	chainsawAssetsDir = "../../test/chainsaw/schema"
)

// TestSchema_Chainsaw is the functional, server-side proof for criteria 1 and 3.
// It boots a real apiserver via controller-runtime's envtest, generates the
// de-risked module's XRD, wraps its openAPIV3Schema as a real CRD, and drives
// chainsaw (over a kubeconfig serialized from envtest's rest.Config) to exercise
// structural acceptance, apiserver defaulting, unknown-field pruning, and a
// status subresource round-trip.
//
// It self-skips when the chainsaw binary or the envtest assets are absent, so
// `go test ./...` stays green on a machine without them — mirroring the
// Docker-gated render-loop test. The dedicated moon `schema-test` task runs it
// where those tools are present.
func TestSchema_Chainsaw(t *testing.T) {
	chainsawBin, err := exec.LookPath("chainsaw")
	if err != nil {
		t.Skip("chainsaw binary not found on PATH; skipping server-side schema test")
	}

	assets := envtestAssets(t)
	if assets == "" {
		t.Skip("envtest assets not found; run `setup-envtest use` to install them")
	}

	env := &envtest.Environment{BinaryAssetsDirectory: assets}
	cfg, err := env.Start()
	if err != nil {
		t.Skipf("cannot start envtest apiserver (assets at %s): %v", assets, err)
	}
	t.Cleanup(func() { _ = env.Stop() })
	require.NotNil(t, cfg)

	// Stage the chainsaw assets plus the freshly generated CRD in one run dir.
	runDir := t.TempDir()
	copyFile(t, filepath.Join(chainsawAssetsDir, "chainsaw-test.yaml"), filepath.Join(runDir, "chainsaw-test.yaml"))
	copyFile(t, filepath.Join(chainsawAssetsDir, "widget.yaml"), filepath.Join(runDir, "widget.yaml"))
	writeFile(t, filepath.Join(runDir, "crd.yaml"), generateCRDYAML(t, deriskedDir))

	// Serialize envtest's admin credentials into a kubeconfig for chainsaw.
	kubeconfig := filepath.Join(runDir, "kubeconfig")
	require.NotEmpty(t, env.KubeConfig, "envtest must expose an admin kubeconfig")
	writeFile(t, kubeconfig, env.KubeConfig)

	// --skip-delete leaves the CRD installed so the status round-trip phase below
	// can reuse it on the same apiserver.
	cmd := exec.CommandContext(context.Background(), chainsawBin, "test", runDir,
		"--test-file", "chainsaw-test.yaml", "--skip-delete")
	cmd.Env = append(os.Environ(), "KUBECONFIG="+kubeconfig)
	out, err := cmd.CombinedOutput()
	t.Logf("chainsaw output:\n%s", out)
	require.NoError(t, err, "chainsaw must accept the generated CRD and pass acceptance/defaulting/pruning")

	// chainsaw 0.2.x cannot write a subresource, so the status round-trip is
	// driven by a typed client against the very same envtest apiserver.
	assertStatusRoundTrip(t, cfg, deriskedDir)
}

// assertStatusRoundTrip proves a #Status write survives the status subresource:
// it creates a widget, updates its status via the /status subresource, reads it
// back, and asserts the status fields are preserved (not pruned). The CRD is
// already installed by the chainsaw phase.
func assertStatusRoundTrip(t *testing.T, cfg *rest.Config, moduleDir string) {
	t.Helper()
	ctx := context.Background()

	gvk := groupVersionKind(t, moduleDir)
	c, err := client.New(cfg, client.Options{})
	require.NoError(t, err)

	widget := &unstructured.Unstructured{}
	widget.SetGroupVersionKind(gvk)
	widget.SetNamespace("default")
	widget.SetName("status-widget")
	require.NoError(t, unstructured.SetNestedField(widget.Object, "ghcr.io/podinfo:1", "spec", "image"))
	// An unknown field the structural schema must prune (the client does not
	// request strict field validation, so the apiserver prunes rather than
	// rejects).
	require.NoError(t, unstructured.SetNestedField(widget.Object, "should-be-pruned", "spec", "bogus"))

	// The CRD is established asynchronously after chainsaw applies it; retry the
	// create until the apiserver serves the new type.
	require.Eventually(t, func() bool {
		return client.IgnoreAlreadyExists(c.Create(ctx, widget)) == nil &&
			widget.GetResourceVersion() != ""
	}, 60*time.Second, time.Second, "widget type must become servable")

	// Pruning: the unknown spec.bogus field must not survive the create.
	pruned := &unstructured.Unstructured{}
	pruned.SetGroupVersionKind(gvk)
	require.NoError(t, c.Get(ctx, client.ObjectKey{Namespace: "default", Name: "status-widget"}, pruned))
	_, found, err := unstructured.NestedString(pruned.Object, "spec", "bogus")
	require.NoError(t, err)
	require.False(t, found, "the apiserver must prune the unknown spec.bogus field")
	// Defaulting also applies through the typed client path.
	replicas, found, err := unstructured.NestedInt64(pruned.Object, "spec", "replicas")
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, int64(3), replicas)

	require.NoError(t, unstructured.SetNestedField(widget.Object, true, "status", "ready"))
	require.NoError(t, unstructured.SetNestedField(widget.Object, "https://widget.example", "status", "endpoint"))
	require.NoError(t, c.Status().Update(ctx, widget))

	got := &unstructured.Unstructured{}
	got.SetGroupVersionKind(gvk)
	require.NoError(t, c.Get(ctx, client.ObjectKey{Namespace: "default", Name: "status-widget"}, got))

	ready, found, err := unstructured.NestedBool(got.Object, "status", "ready")
	require.NoError(t, err)
	require.True(t, found, "status.ready must survive the round-trip, not be pruned")
	require.True(t, ready)

	endpoint, found, err := unstructured.NestedString(got.Object, "status", "endpoint")
	require.NoError(t, err)
	require.True(t, found, "status.endpoint must survive the round-trip")
	require.Equal(t, "https://widget.example", endpoint)
}

// groupVersionKind reads the served GVK from the module's #API.
func groupVersionKind(t *testing.T, moduleDir string) k8sschema.GroupVersionKind {
	t.Helper()
	module, cleanup, err := render.LoadModule(context.Background(), render.LocalLoader{Dir: moduleDir}, "ignored")
	require.NoError(t, err)
	defer cleanup()

	xrd, err := schema.GenerateXRD(module)
	require.NoError(t, err)
	return k8sschema.GroupVersionKind{
		Group:   xrd.Spec.Group,
		Version: xrd.Spec.Versions[0].Name,
		Kind:    xrd.Spec.Names.Kind,
	}
}

// envtestAssets resolves an already-installed envtest binary directory without
// triggering a network download, honoring KUBEBUILDER_ASSETS when set.
func envtestAssets(t *testing.T) string {
	t.Helper()

	if dir := os.Getenv("KUBEBUILDER_ASSETS"); dir != "" {
		return dir
	}

	bin, err := exec.LookPath("setup-envtest")
	if err != nil {
		return ""
	}
	out, err := exec.Command(bin, "use", "-p", "path", "-i").Output() //nolint:gosec // resolved binary, fixed args.
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// generateCRDYAML builds the module's XRD and wraps its openAPIV3Schema as a real
// CustomResourceDefinition, enabling the status subresource when the schema
// carries a status. The CRD is structural with pruning on (no
// x-kubernetes-preserve-unknown-fields), so the apiserver defaults and prunes.
func generateCRDYAML(t *testing.T, moduleDir string) []byte {
	t.Helper()

	module, cleanup, err := render.LoadModule(context.Background(), render.LocalLoader{Dir: moduleDir}, "ignored")
	require.NoError(t, err)
	defer cleanup()

	xrd, err := schema.GenerateXRD(module)
	require.NoError(t, err)
	require.Len(t, xrd.Spec.Versions, 1)

	var props extv1.JSONSchemaProps
	require.NoError(t, json.Unmarshal(xrd.Spec.Versions[0].Schema.OpenAPIV3Schema.Raw, &props))

	_, hasStatus := props.Properties["status"]

	crdVersion := extv1.CustomResourceDefinitionVersion{
		Name:    xrd.Spec.Versions[0].Name,
		Served:  true,
		Storage: true,
		Schema:  &extv1.CustomResourceValidation{OpenAPIV3Schema: &props},
	}
	if hasStatus {
		crdVersion.Subresources = &extv1.CustomResourceSubresources{
			Status: &extv1.CustomResourceSubresourceStatus{},
		}
	}

	singular := strings.ToLower(xrd.Spec.Names.Kind)
	crd := &extv1.CustomResourceDefinition{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "apiextensions.k8s.io/v1",
			Kind:       "CustomResourceDefinition",
		},
		ObjectMeta: metav1.ObjectMeta{Name: xrd.Name},
		Spec: extv1.CustomResourceDefinitionSpec{
			Group: xrd.Spec.Group,
			Names: extv1.CustomResourceDefinitionNames{
				Kind:     xrd.Spec.Names.Kind,
				ListKind: xrd.Spec.Names.Kind + "List",
				Plural:   xrd.Spec.Names.Plural,
				Singular: singular,
			},
			Scope:    extv1.ResourceScope(xrd.Spec.Scope),
			Versions: []extv1.CustomResourceDefinitionVersion{crdVersion},
		},
	}

	out, err := yaml.Marshal(crd)
	require.NoError(t, err)
	return out
}

func copyFile(t *testing.T, src, dst string) {
	t.Helper()
	data, err := os.ReadFile(src) //nolint:gosec // test-controlled path.
	require.NoError(t, err)
	writeFile(t, dst, data)
}

func writeFile(t *testing.T, path string, data []byte) {
	t.Helper()
	require.NoError(t, os.WriteFile(path, data, 0o600))
}
