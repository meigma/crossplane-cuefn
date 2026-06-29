// Package app is the hermetic test-fixture module: a self-contained,
// import-free XApp module that the unit and integration suites load instead of
// the user-facing example/ module, so the tests never depend on the example (and
// the example is free to import external schemas like cue.dev/x/k8s.io). It
// mirrors the example's module contract — #API/#Spec/#Status and a keyed resource
// map covering all readiness states — and evaluates fully offline.
package app

// #API is the concrete platform API description the CLI decodes to build the XRD
// envelope. A single served version is supported in v1.
#API: {
	group:   "platform.meigma.io"
	version: "v1alpha1"
	kind:    "XApp"
	plural:  "xapps"
	scope:   *"Namespaced" | "Cluster"
}

// #Spec is the authoritative, closed XR spec schema. It feeds runtime
// defaults/validation from the same source the CLI uses for codegen. replicas is
// bounded to demonstrate schema-driven validation.
#Spec: {
	image:    string | *"ghcr.io/stefanprodan/podinfo:6.7.0"
	replicas: *1 | int & >=1 & <=10
}

// #Status is the optional XR status schema. The transform returns a value of
// this shape to be patched onto the composite.
#Status: {
	ready: bool
	url:   string
}
