// Package app is a render test fixture whose single keyed resource is left
// non-concrete (object.metadata.name is an abstract string). It proves the
// engine rejects non-concrete resources and names the offending field/key.
package app

out: {
	input: {...}

	resources: {
		broken: {
			object: {
				apiVersion: "v1"
				kind:       "ConfigMap"
				metadata: name: string
			}
		}
	}
}
