// Package contract is the cuefn module contract: closed CUE definitions an
// author's module unifies against to validate the cuefn module shape at author
// time (cue vet / editor), before the module is ever published or rendered.
//
// Import it and constrain the well-known fields:
//
//	import "github.com/meigma/crossplane-cuefn/contract@v0"
//
//	#API: contract.#API & {group: "...", version: "...", kind: "...", plural: "..."}
//	#Spec: {...}      // your own schema
//	#Status: {...}    // your own schema
//
//	out: contract.#Transform & {
//		input: {spec: #Spec, metadata: {...}, environment: {...}}
//		resources: {deployment: {object: ..., ready: "Ready"}, ...}
//		status: #Status & {...}
//	}
//
// Because #Transform/#API/#Input/#Resource are closed, a misspelled or unknown
// field (e.g. `resorces`) is rejected by `cue vet`. The schema definitions
// (#Spec/#Status) are deliberately NOT wrapped: they feed the XRD codegen and
// stay your own import-free schemas.
package contract

// #API is the platform API envelope the cuefn CLI decodes to build the XRD.
#API: {
	// group is the API group, e.g. "platform.example.com".
	group: string
	// version is the single served version, e.g. "v1alpha1".
	version: string
	// kind is the composite resource kind, e.g. "XApp".
	kind: string
	// plural is the lowercase plural of kind, e.g. "xapps".
	plural: string
	// scope is the resource scope; defaults to Namespaced.
	scope: *"Namespaced" | "Cluster"
}

// #Resource is one composed resource: a finished Kubernetes object plus an
// optional readiness hint the engine maps to Crossplane readiness. The object is
// open (any Kubernetes kind); instantiate it from a schema such as
// cue.dev/x/k8s.io for stronger guarantees.
#Resource: {
	object: {
		apiVersion: string
		kind:       string
		...
	}
	ready?: "Ready" | "NotReady"
}

// #Input is the value the engine fills under out.input. Tighten spec to your own
// #Spec (out.input.spec: #Spec) so render-time defaults/validation come from the
// same schema the XRD is generated from.
#Input: {
	// spec is the observed XR spec, projected and unified against your #Spec.
	spec: _
	// metadata is the composite's identifying metadata.
	metadata: {
		name:       string
		namespace?: string
	}
	// environment is the merged EnvironmentConfig data; open so a module can read
	// arbitrary keys.
	environment: {
		...
	}
}

// #Transform is the closed transform contract. Unify your top-level `out` field
// against it: the engine fills out.input and reads out.resources and the optional
// out.status. Closedness rejects an unknown top-level field at author time.
#Transform: {
	input: #Input
	resources: [string]: #Resource
	status?: _
}
