package v1beta1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Input configures the cuefn composition function for one pipeline step. It is
// embedded directly in a Composition's pipeline step input (apiVersion
// cuefn.meigma.io/v1beta1, kind Input); the function decodes it with
// request.GetInput before rendering.
//
// +kubebuilder:object:root=true
// +kubebuilder:storageversion
// +kubebuilder:resource:categories=crossplane
type Input struct {
	metav1.TypeMeta `json:",inline"`

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
