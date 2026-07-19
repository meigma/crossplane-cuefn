//go:build !noxpkg

package modulepublish

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"cuelang.org/go/mod/modfile"
	"cuelang.org/go/mod/modregistry"
	"cuelang.org/go/mod/module"
	"cuelang.org/go/mod/modzip"
	"golang.org/x/mod/semver"
)

func createArchive(mv module.Version, dir string) ([]byte, *modregistry.Metadata, error) {
	root, err := filepath.Abs(dir)
	if err != nil {
		return nil, nil, fmt.Errorf("cannot resolve module directory %q: %w", dir, err)
	}
	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		return nil, nil, fmt.Errorf("cannot resolve module directory %q: %w", dir, err)
	}
	moduleFilePath := filepath.Join(root, "cue.mod", "module.cue")
	moduleFileData, err := os.ReadFile(moduleFilePath)
	if err != nil {
		return nil, nil, fmt.Errorf("cannot read %s: %w", moduleFilePath, err)
	}
	mf, err := modfile.Parse(moduleFileData, moduleFilePath)
	if err != nil {
		return nil, nil, err
	}

	sourceKind, err := sourceKind(mf)
	if err != nil {
		return nil, nil, err
	}
	var archive bytes.Buffer
	switch sourceKind {
	case "self":
		if err := modzip.CreateFromDir(&archive, mv, root); err != nil {
			return nil, nil, err
		}
		return archive.Bytes(), nil, nil
	case "git":
		meta, err := createGitArchive(&archive, mv, root)
		if err != nil {
			return nil, nil, err
		}
		return archive.Bytes(), meta, nil
	default:
		return nil, nil, fmt.Errorf("unsupported module source kind %q", sourceKind)
	}
}

func sourceKind(mf *modfile.File) (string, error) {
	if mf.Source != nil {
		return mf.Source.Kind, nil
	}
	if mf.Language == nil || semver.Compare(mf.Language.Version, "v0.9.0-alpha.2") < 0 {
		return "self", nil
	}
	return "", errors.New(
		"publishing a module requires source.kind in cue.mod/module.cue; choose \"self\" or \"git\"",
	)
}
