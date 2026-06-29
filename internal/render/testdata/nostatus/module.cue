// Package app is a render test fixture with no #Status and no status field. It
// renders one concrete keyed resource to prove a clean render yields an empty
// Result.Status.
package app

out: {
	input: {...}

	resources: {
		only: {
			object: {
				apiVersion: "v1"
				kind:       "ConfigMap"
				metadata: name: "only"
			}
		}
	}
}
