// Package app is the hermetic required-resources test-fixture module: a
// self-contained, import-free XApp module that the render unit suite loads to
// exercise out.requirements (emit) and out.input.requiredResources (receive). It
// mirrors testdata/module's shape — #API/#Spec/#Status and a bare out:{...}
// struct — and deliberately does not import the contract, so the tests resolve
// no registry and evaluate fully offline.
package app

// #API is the concrete platform API description. scope allows Cluster so the
// namespace-guard idiom (cluster-scoped XR -> selector omits namespace) can be
// exercised.
#API: {
	group:   "platform.meigma.io"
	version: "v1alpha1"
	kind:    "XApp"
	plural:  "xapps"
	scope:   *"Namespaced" | "Cluster"
}

// #Spec is the closed XR spec schema. configName selects the ConfigMap the
// module requires.
#Spec: {
	configName: string | *"app-cfg"
}

// #Status is the optional XR status schema.
#Status: {
	ready: bool
}
