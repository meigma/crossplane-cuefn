//go:build !noxpkg

package modulepublish

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"cuelang.org/go/mod/modregistry"
	git "github.com/go-git/go-git/v6"
	"github.com/go-git/go-git/v6/plumbing"
	"github.com/go-git/go-git/v6/plumbing/object"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const gitModuleRef = "cuefn.example/gitapp@v0.1.0"

func TestPrepareGitSourceUsesTrackedFilesAndVCSMetadata(t *testing.T) {
	t.Parallel()

	fixture := newGitFixture(t)
	artifact, err := Prepare(context.Background(), gitModuleRef, fixture.moduleDir, map[string]string{
		"dev.meigma.owner": "platform",
	})
	require.NoError(t, err)

	var manifest ocispec.Manifest
	require.NoError(t, json.Unmarshal(artifact.manifest, &manifest))
	assert.Equal(t, "git", manifest.Annotations["org.cuelang.vcs-type"])
	assert.Equal(t, fixture.commit.String(), manifest.Annotations["org.cuelang.vcs-commit"])
	assert.Equal(t, fixture.commitTime.Format(time.RFC3339), manifest.Annotations["org.cuelang.vcs-commit-time"])
	assert.Equal(t, "platform", manifest.Annotations["dev.meigma.owner"])

	archive := artifactBlob(t, artifact, "application/zip")
	reader, err := zip.NewReader(bytes.NewReader(archive), int64(len(archive)))
	require.NoError(t, err)
	var names []string
	for _, file := range reader.File {
		names = append(names, file.Name)
	}
	assert.Contains(t, names, "cue.mod/module.cue")
	assert.Contains(t, names, "app.cue")
	assert.Contains(t, names, "LICENSE", "root LICENSE should be inherited by a nested module")
	assert.NotContains(t, names, "secret.ignored")
}

func TestPrepareGitSourceRejectsDirtyStateAndMetadataCollision(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		arrange func(t *testing.T, fixture gitFixture) map[string]string
		want    string
	}{
		{
			name: "dirty worktree",
			arrange: func(t *testing.T, fixture gitFixture) map[string]string {
				writeTestFile(t, filepath.Join(fixture.moduleDir, "untracked.cue"), "package app\n")
				return nil
			},
			want: "not clean",
		},
		{
			name: "generated metadata collision",
			arrange: func(_ *testing.T, _ gitFixture) map[string]string {
				return map[string]string{"org.cuelang.vcs-type": "override"}
			},
			want: "conflicts",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			fixture := newGitFixture(t)
			metadata := tt.arrange(t, fixture)
			_, err := Prepare(context.Background(), gitModuleRef, fixture.moduleDir, metadata)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.want)
		})
	}
}

func TestPrepareGitSourceSupportsLinkedWorktree(t *testing.T) {
	gitPath, err := exec.LookPath("git")
	if err != nil {
		t.Skip("git executable is unavailable for linked-worktree compatibility test")
	}
	fixture := newGitFixture(t)
	cmd := exec.Command(gitPath, "-C", fixture.root, "config", "extensions.worktreeConfig", "true")
	output, err := cmd.CombinedOutput()
	require.NoError(t, err, "enable worktree-specific config: %s", output)

	linkedRoot := filepath.Join(t.TempDir(), "linked")
	cmd = exec.Command(gitPath, "-C", fixture.root, "worktree", "add", "-b", "linked-test", linkedRoot)
	output, err = cmd.CombinedOutput()
	require.NoError(t, err, "create linked worktree: %s", output)

	artifact, err := Prepare(context.Background(), gitModuleRef, filepath.Join(linkedRoot, "module"), nil)
	require.NoError(t, err)
	assert.NotEmpty(t, artifact.Digest())
}

type gitFixture struct {
	root       string
	moduleDir  string
	commit     plumbing.Hash
	commitTime time.Time
}

func newGitFixture(t *testing.T) gitFixture {
	t.Helper()

	root := t.TempDir()
	moduleDir := filepath.Join(root, "module")
	require.NoError(t, os.MkdirAll(filepath.Join(moduleDir, "cue.mod"), 0o755))
	writeTestFile(t, filepath.Join(root, ".gitignore"), "*.ignored\n")
	writeTestFile(t, filepath.Join(root, "LICENSE"), "fixture license\n")
	writeTestFile(t, filepath.Join(moduleDir, "cue.mod", "module.cue"), `module: "cuefn.example/gitapp@v0"
language: version: "v0.16.0"
source: kind: "git"
`)
	writeTestFile(t, filepath.Join(moduleDir, "app.cue"), "package app\n")
	writeTestFile(t, filepath.Join(moduleDir, "secret.ignored"), "not published\n")

	repo, err := git.PlainInit(root, false)
	require.NoError(t, err)
	worktree, err := repo.Worktree()
	require.NoError(t, err)
	for _, path := range []string{".gitignore", "LICENSE", "module/cue.mod/module.cue", "module/app.cue"} {
		_, addErr := worktree.Add(path)
		require.NoError(t, addErr)
	}
	commitTime := time.Date(2026, time.July, 19, 18, 30, 0, 0, time.UTC)
	commit, err := worktree.Commit("test fixture", &git.CommitOptions{
		Author:    &object.Signature{Name: "Test", Email: "test@example.com", When: commitTime},
		Committer: &object.Signature{Name: "Test", Email: "test@example.com", When: commitTime},
	})
	require.NoError(t, err)
	return gitFixture{
		root:       root,
		moduleDir:  moduleDir,
		commit:     commit,
		commitTime: commitTime,
	}
}

func writeTestFile(t *testing.T, path, contents string) {
	t.Helper()
	require.NoError(t, os.WriteFile(path, []byte(contents), 0o644))
}

func artifactBlob(t *testing.T, artifact *Artifact, mediaType string) []byte {
	t.Helper()
	for _, blob := range artifact.blobs {
		if blob.desc.MediaType == mediaType {
			return blob.data
		}
	}
	t.Fatalf("artifact has no blob with media type %q", mediaType)
	return nil
}

var _ modregistry.Resolver = staticResolver{}
