// Package v1beta1 contains the Input type for the cuefn composition function.
//
// Input is the KRM-like object a Composition pipeline step passes to cuefn. It
// is not a cluster CRD the user installs; its schema is generated only to
// describe the step's shape. The function decodes it from the pipeline step via
// request.GetInput, so the type carries controller-gen-generated deepcopy
// methods (it must satisfy runtime.Object).
//
// +kubebuilder:object:generate=true
// +groupName=cuefn.meigma.io
// +versionName=v1beta1
package v1beta1
