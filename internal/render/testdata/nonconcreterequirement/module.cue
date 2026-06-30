// Package app is a render test fixture whose requirement is structurally valid
// but non-concrete: matchName is left as `string`, so readRequirements' concrete
// check must reject it with "requirements did not fully evaluate" before the
// exactly-one match check is ever reached.
package app

out: {
	input: {...}

	requirements: cfg: {
		apiVersion: "v1"
		kind:       "ConfigMap"
		matchName:  string
	}

	resources: {}
}
