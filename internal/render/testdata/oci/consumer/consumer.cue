// Package consumer is a module that imports another OCI module (cuefn.test/dep)
// and renders a resource from the imported values. It drives the transitive
// dependency test: it only renders correctly when the loader resolves the dep
// from the registry.
package consumer

import dep "cuefn.test/dep@v0"

out: {
	// input is filled by the engine. The consumer ignores the spec but keeps the
	// field present so the contract is uniform with the example module.
	input: {
		metadata: {
			name: string | *"consumer"
			...
		}
		...
	}

	_name: input.metadata.name

	resources: {
		deployment: {
			ready: "Ready"
			object: {
				apiVersion: "apps/v1"
				kind:       "Deployment"
				metadata: name: _name
				spec: {
					// Both values come from the imported dependency module.
					replicas: 1
					template: spec: containers: [{
						name:  "app"
						image: dep.Image
						ports: [{containerPort: dep.Port}]
					}]
				}
			}
		}
	}
}
