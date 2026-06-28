package pkg

import (
	"errors"
	"strings"

	metav1cp "github.com/crossplane/crossplane/apis/v2/pkg/meta/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	// configurationMetaAPIVersion / configurationMetaKind identify the
	// crossplane.yaml package metadata object for a Configuration.
	configurationMetaAPIVersion = "meta.pkg.crossplane.io/v1"
	configurationMetaKind       = "Configuration"
)

// ConfigurationMeta names the Configuration package and its dependency on the
// cuefn composition Function.
type ConfigurationMeta struct {
	// Name is the package metadata.name (e.g. "xapps-configuration").
	Name string
	// CrossplaneConstraint is an optional semver constraint on the Crossplane
	// version the package supports (e.g. ">=v1.14.0-0"); omitted when empty.
	CrossplaneConstraint string
	// FunctionPackage is the dependsOn Function package OCI ref
	// (e.g. "xpkg.meigma.io/cuefn").
	FunctionPackage string
	// FunctionVersion is the dependsOn semver version constraint
	// (e.g. ">=v0.1.0").
	FunctionVersion string
}

// GenerateConfigurationMeta builds the meta.pkg.crossplane.io/v1 Configuration
// object (the package's crossplane.yaml) with a dependsOn entry for the cuefn
// Function, so installing the Configuration pulls the function it relies on.
func GenerateConfigurationMeta(m ConfigurationMeta) (*metav1cp.Configuration, error) {
	if strings.TrimSpace(m.Name) == "" {
		return nil, errors.New("configuration meta requires a name")
	}
	if strings.TrimSpace(m.FunctionPackage) == "" {
		return nil, errors.New("configuration meta requires a function package ref")
	}

	spec := metav1cp.MetaSpec{}
	if strings.TrimSpace(m.CrossplaneConstraint) != "" {
		spec.Crossplane = &metav1cp.CrossplaneConstraints{Version: m.CrossplaneConstraint}
	}

	funcPkg := m.FunctionPackage
	spec.DependsOn = []metav1cp.Dependency{{
		// Crossplane's package manager still resolves a Function dependency from
		// the dependsOn.function field; the apiVersion/kind/package form targets
		// arbitrary typed dependencies, not package images.
		Function: &funcPkg,
		Version:  m.FunctionVersion,
	}}

	return &metav1cp.Configuration{
		TypeMeta: metav1.TypeMeta{
			APIVersion: configurationMetaAPIVersion,
			Kind:       configurationMetaKind,
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: m.Name,
		},
		Spec: metav1cp.ConfigurationSpec{MetaSpec: spec},
	}, nil
}
