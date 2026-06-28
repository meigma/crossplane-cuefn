// Package mutable is variant B of a module published under cuefn.test/mutable.
// It carries the same module path and is published at the same version (v0.1.0)
// as variant A; only the rendered marker differs, so a re-render that returns
// "B" proves the loader re-fetched on the digest change.
package mutable

input: {
	metadata: {
		name: string | *"mutable"
		...
	}
	...
}

resources: {
	config: {
		object: {
			apiVersion: "v1"
			kind:       "ConfigMap"
			metadata: name: input.metadata.name
			// The variant marker the test asserts on.
			data: variant: "B"
		}
	}
}
