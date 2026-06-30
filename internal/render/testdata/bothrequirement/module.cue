// Package app is a render test fixture that emits an invalid requirement: it
// sets BOTH matchName and matchLabels, so readRequirements must reject it with
// the "exactly one" error.
package app

out: {
	input: {...}

	requirements: cfg: {
		apiVersion: "v1"
		kind:       "ConfigMap"
		matchName:  "app-cfg"
		matchLabels: app: "demo"
	}

	resources: {}
}
