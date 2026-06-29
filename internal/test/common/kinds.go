package common

import (
	"bytes"
	"compress/gzip"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	xpkg "github.com/crossplane/crossplane-runtime/v2/pkg/xpkg"
	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/tarball"
	"github.com/stretchr/testify/require"
	"sigs.k8s.io/yaml"
)

// YAMLDoc is the decode target for one document's identity fields.
type YAMLDoc struct {
	APIVersion string         `json:"apiVersion"`
	Kind       string         `json:"kind"`
	Metadata   map[string]any `json:"metadata"`
	Spec       map[string]any `json:"spec"`
}

// StreamKinds returns the set of Kinds present in a multi-document YAML stream
// (documents separated by "\n---\n").
func StreamKinds(stream []byte) map[string]bool {
	kinds := map[string]bool{}
	for chunk := range bytes.SplitSeq(stream, []byte("\n---\n")) {
		var doc struct {
			Kind string `json:"kind"`
		}
		if err := yaml.Unmarshal(chunk, &doc); err == nil && doc.Kind != "" {
			kinds[doc.Kind] = true
		}
	}
	return kinds
}

// ExtractKinds writes img as a local .xpkg, runs `crossplane xpkg extract` over
// it, and returns the set of Kinds present in the extracted (gzipped) stream. It
// fails the test if the CLI exits non-zero, which is the validation assertion.
func ExtractKinds(t *testing.T, bin string, img v1.Image, base string) map[string]bool {
	t.Helper()

	dir := t.TempDir()
	xpkgPath := filepath.Join(dir, base+".xpkg")
	tag, err := name.NewTag("local/" + base + ":test")
	require.NoError(t, err)
	require.NoError(t, tarball.WriteToFile(xpkgPath, tag, img))

	outPath := filepath.Join(dir, "out.gz")
	cmd := exec.Command(bin, "xpkg", "extract", "--from-xpkg", xpkgPath, "-o", outPath)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "crossplane xpkg extract must accept the package:\n%s", out)

	raw, err := os.ReadFile(outPath)
	require.NoError(t, err)
	gz, err := gzip.NewReader(bytes.NewReader(raw))
	require.NoError(t, err)
	defer gz.Close()
	stream, err := io.ReadAll(gz)
	require.NoError(t, err)

	return StreamKinds(stream)
}

// PackageYAMLBytes reads the image's package.yaml layer back into bytes.
func PackageYAMLBytes(img v1.Image) ([]byte, error) {
	rc, err := xpkg.ExtractPackageYAML(img)
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	return io.ReadAll(rc)
}

// SplitStream splits a multi-document YAML stream into its non-empty documents,
// decoding each into a YAMLDoc.
func SplitStream(t *testing.T, stream []byte) []YAMLDoc {
	t.Helper()

	var docs []YAMLDoc
	for chunk := range strings.SplitSeq(string(stream), "\n---\n") {
		if strings.TrimSpace(chunk) == "" {
			continue
		}
		var d YAMLDoc
		require.NoError(t, yaml.Unmarshal([]byte(chunk), &d), "unmarshal stream document:\n%s", chunk)
		docs = append(docs, d)
	}
	return docs
}
