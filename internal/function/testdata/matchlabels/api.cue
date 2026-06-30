// Package app is a hermetic, import-free fixture exercising the matchLabels arm
// of the requirements oneof: it emits a single ConfigMap requirement selected by
// labels (no matchName), so the function adapter's setRequirements maps it onto
// fnv1.ResourceSelector_MatchLabels. It mirrors testdata/required's shape but
// swaps matchName for matchLabels.
package app

#API: {
	group:   "platform.meigma.io"
	version: "v1alpha1"
	kind:    "XApp"
	plural:  "xapps"
	scope:   *"Namespaced" | "Cluster"
}

// #Spec is the closed XR spec schema. appName is the label value the emitted
// requirement matches on.
#Spec: {
	appName: string | *"app"
}

// #Status is the optional XR status schema.
#Status: {
	ready: bool
}
