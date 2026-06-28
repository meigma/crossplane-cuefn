// Package app is a render test fixture with concrete resources but a
// non-concrete status (url left as an abstract string). It proves the engine
// rejects a non-concrete status and names the offending field.
package app

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

status: {
	url: string
}
