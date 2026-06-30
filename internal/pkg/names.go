package pkg

import (
	"fmt"

	xpkg "github.com/crossplane/crossplane-runtime/v2/pkg/xpkg"
	"github.com/google/go-containerregistry/pkg/name"
)

// DerivedFunctionName returns the metadata.name Crossplane's package manager
// gives a Function it auto-installs from a Configuration's dependsOn entry for
// ref. Crossplane strips the registry host and DNS-labelizes the repository path
// (xpkg.ToDNSLabel over name.RepositoryStr), so e.g.
// "ghcr.io/meigma/function-cuefn" installs as "meigma-function-cuefn".
//
// A generated Composition must reference the function by this exact name for the
// pipeline step to bind to the auto-installed Function. Referencing the bare last
// path segment ("function-cuefn") leaves the step unresolved with "cannot find an
// active FunctionRevision", and hand-installing a separate Function to match would
// poison the package Lock (two Functions on one package source).
func DerivedFunctionName(ref string) (string, error) {
	parsed, err := name.ParseReference(ref)
	if err != nil {
		return "", fmt.Errorf("cannot parse function package ref %q: %w", ref, err)
	}
	return xpkg.ToDNSLabel(parsed.Context().RepositoryStr()), nil
}
