// Package app is a render test fixture that emits an invalid requirement: it
// sets neither matchName nor matchLabels, so readRequirements must reject it with
// the "exactly one" error.
package app

out: {
	input: {...}

	requirements: cfg: {
		apiVersion: "v1"
		kind:       "ConfigMap"
	}

	resources: {}
}
