// Package app is the canonical cuefn example module. It declares the platform
// API (#API), the authoritative XR spec schema (#Spec), and an optional status
// schema (#Status), then renders a keyed resource map from an App XR's spec and
// the merged EnvironmentConfig. It deliberately uses no external imports so the
// module evaluates fully offline.
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
	// configName selects the operator-supplied ConfigMap this XR reads at render
	// time via out.requirements.cfg. It defaults to a sentinel that no fixture
	// (and no operator) ever creates as a ConfigMap, so the requirement stays a
	// pure, concrete function of the spec while an XR that omits it (the demo XR)
	// selects a name that matches nothing: the delivered bucket stays empty and
	// the guarded resource is absent, leaving its composed set byte-identical. The
	// required-resources test sets this explicitly to a distinctly-named operator
	// ConfigMap to exercise the fetch.
	configName: string | *"none"
}

// #Status is the optional XR status schema. The transform returns a value of
// this shape to be patched onto the composite.
#Status: {
	ready: bool
	url:   string
}
