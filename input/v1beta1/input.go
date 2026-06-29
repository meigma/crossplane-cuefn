package v1beta1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Input configures the cuefn composition function for one pipeline step. It is
// embedded directly in a Composition's pipeline step input (apiVersion
// cuefn.meigma.io/v1beta1, kind Input); the function decodes it with
// request.GetInput before rendering.
//
// The embedded ObjectMeta is the upstream function-Input convention: it makes
// controller-gen emit a CustomResourceDefinition for the type, which ships
// inside the Function xpkg (Crossplane reads the Input CRD from the package to
// validate pipeline step inputs). The runtime ignores any metadata an author
// happens to set; request.GetInput decodes only the typed fields below.
//
// +kubebuilder:object:root=true
// +kubebuilder:storageversion
// +kubebuilder:resource:categories=crossplane
type Input struct {
	metav1.TypeMeta `json:",inline"`

	// ObjectMeta is present so controller-gen emits a CRD for the Input type;
	// it is never populated by an author and is ignored by the function.
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// Module is the CUE module to fetch and evaluate, in "path@version" semver
	// form (e.g. "cuefn.example/app@v0.1.0"). It is resolved against the
	// CUE_REGISTRY the function is configured with; the version selects the
	// module, not a digest.
	Module string `json:"module"`

	// ExpectedDigest is the OCI manifest digest the resolved Module must match,
	// in "sha256:..." form. When set it drives the runtime half of the
	// schema<->runtime digest lock-step: the loader verifies the fetched
	// manifest digest against it and fails the render on a mismatch. When empty
	// the module is resolved by version with no digest check.
	ExpectedDigest string `json:"expectedDigest,omitempty"`
}
