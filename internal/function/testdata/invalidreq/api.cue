// Package app is a hermetic, import-free fixture whose single requirement sets
// BOTH matchName and matchLabels, violating the engine's exactly-one rule. The
// render engine's readRequirements rejects it, and the function adapter surfaces
// that error as a Fatal result.
package app

#API: {
	group:   "platform.meigma.io"
	version: "v1alpha1"
	kind:    "XApp"
	plural:  "xapps"
	scope:   *"Namespaced" | "Cluster"
}

// #Spec is the closed XR spec schema.
#Spec: {
	configName: string | *"app-cfg"
}

// #Status is the optional XR status schema.
#Status: {
	ready: bool
}
