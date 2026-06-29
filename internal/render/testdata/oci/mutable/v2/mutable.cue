// Package mutable is variant B of a module published under cuefn.test/mutable.
// The digest-drift test publishes both variants at the SAME version to prove the
// loader keys its cache on the manifest digest, not the version tag.
package mutable

out: {
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
}
