// Package mutable is variant A of a module published under cuefn.test/mutable.
// The digest-drift test publishes this variant at v0.1.0, renders it, then
// republishes variant B at the SAME version to prove the loader keys its cache
// on the manifest digest, not the (immutable-by-convention) version tag.
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
			data: variant: "A"
		}
	}
}
