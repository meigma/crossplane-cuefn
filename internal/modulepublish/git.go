//go:build !noxpkg

package modulepublish

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"cuelang.org/go/mod/modregistry"
	"cuelang.org/go/mod/module"
	"cuelang.org/go/mod/modzip"
	git "github.com/go-git/go-git/v6"
	gitindex "github.com/go-git/go-git/v6/plumbing/format/index"
)

const (
	licenseFileName = "LICENSE"
	maxStatusPaths  = 3
)

type trackedFile struct {
	path string
	abs  string
}

type trackedFileIO struct{}

func (trackedFileIO) Path(file trackedFile) string {
	return filepath.ToSlash(file.path)
}

func (trackedFileIO) Lstat(file trackedFile) (fs.FileInfo, error) {
	return os.Lstat(file.abs)
}

func (trackedFileIO) Open(file trackedFile) (io.ReadCloser, error) {
	return os.Open(file.abs)
}

func createGitArchive(w io.Writer, mv module.Version, moduleRoot string) (*modregistry.Metadata, error) {
	repo, err := git.PlainOpenWithOptions(moduleRoot, &git.PlainOpenOptions{
		DetectDotGit: true,
	})
	if err != nil {
		return nil, fmt.Errorf("cannot open Git repository for %s: %w", moduleRoot, err)
	}
	worktree, err := repo.Worktree()
	if err != nil {
		return nil, fmt.Errorf("cannot open Git worktree for %s: %w", moduleRoot, err)
	}
	status, err := worktree.Status()
	if err != nil {
		return nil, fmt.Errorf("cannot inspect Git status for %s: %w", moduleRoot, err)
	}
	if !status.IsClean() {
		return nil, errorsWithStatus(status)
	}

	repoRoot := filepath.Clean(worktree.Filesystem().Root())
	moduleRoot = filepath.Clean(moduleRoot)
	modulePrefix, err := filepath.Rel(repoRoot, moduleRoot)
	if err != nil {
		return nil, fmt.Errorf("cannot locate module inside Git worktree: %w", err)
	}
	if modulePrefix == ".." || strings.HasPrefix(modulePrefix, ".."+string(filepath.Separator)) {
		return nil, fmt.Errorf("module directory %s is outside Git worktree %s", moduleRoot, repoRoot)
	}

	index, err := repo.Storer.Index()
	if err != nil {
		return nil, fmt.Errorf("cannot read Git index: %w", err)
	}
	files := trackedFiles(index.Entries, repoRoot, modulePrefix)
	if !slices.ContainsFunc(files, func(file trackedFile) bool { return file.path == licenseFileName }) &&
		modulePrefix != "." {
		if license, ok := trackedRootLicense(index.Entries, repoRoot); ok {
			files = append(files, license)
		}
	}
	// CUE exposes no non-deprecated public API for tracked-file module archives.
	//nolint:staticcheck // Keep this isolated adapter until CUE publishes its replacement.
	if createErr := modzip.Create(w, mv, files, trackedFileIO{}); createErr != nil {
		return nil, createErr
	}

	head, err := repo.Head()
	if err != nil {
		return nil, fmt.Errorf("cannot resolve Git HEAD: %w", err)
	}
	commit, err := repo.CommitObject(head.Hash())
	if err != nil {
		return nil, fmt.Errorf("cannot inspect Git HEAD commit: %w", err)
	}
	return &modregistry.Metadata{
		VCSType:       "git",
		VCSCommit:     head.Hash().String(),
		VCSCommitTime: commit.Committer.When,
	}, nil
}

func trackedFiles(entries []*gitindex.Entry, repoRoot, modulePrefix string) []trackedFile {
	prefix := filepath.ToSlash(modulePrefix)
	files := make([]trackedFile, 0, len(entries))
	for _, entry := range entries {
		name := filepath.ToSlash(entry.Name)
		var relative string
		switch {
		case prefix == ".":
			relative = name
		case strings.HasPrefix(name, prefix+"/"):
			relative = strings.TrimPrefix(name, prefix+"/")
		default:
			continue
		}
		files = append(files, trackedFile{
			path: relative,
			abs:  filepath.Join(repoRoot, filepath.FromSlash(name)),
		})
	}
	return files
}

func trackedRootLicense(entries []*gitindex.Entry, repoRoot string) (trackedFile, bool) {
	for _, entry := range entries {
		if filepath.ToSlash(entry.Name) == licenseFileName {
			return trackedFile{path: licenseFileName, abs: filepath.Join(repoRoot, licenseFileName)}, true
		}
	}
	return trackedFile{}, false
}

func errorsWithStatus(status git.Status) error {
	paths := make([]string, 0, len(status))
	for path := range status {
		paths = append(paths, path)
	}
	slices.Sort(paths)
	if len(paths) > maxStatusPaths {
		paths = append(paths[:maxStatusPaths], "...")
	}
	return fmt.Errorf("git state is not clean: %s", strings.Join(paths, ", "))
}
