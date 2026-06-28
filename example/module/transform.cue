package app

// input is filled by the engine with the observed XR spec (validated against
// #Spec), its metadata, and the merged environment. Defaults keep metadata and
// environment concrete even when the caller omits them.
input: {
	spec: #Spec
	metadata: {
		name:       string | *"app"
		namespace?: string
		...
	}
	environment: {
		tier: string | *"unset"
		...
	}
}

_name: input.metadata.name
_tier: input.environment.tier

// resources is the author-keyed map of composed resources. Keys are stable,
// author-chosen names used verbatim as the Crossplane composed-resource name.
// The three entries cover all readiness states: an explicit Ready, an explicit
// NotReady, and an absent hint (unspecified).
resources: {
	deployment: {
		ready: "Ready"
		object: {
			apiVersion: "apps/v1"
			kind:       "Deployment"
			metadata: {
				name: _name
				labels: {app: _name, tier: _tier}
			}
			spec: {
				replicas: input.spec.replicas
				selector: matchLabels: app: _name
				template: {
					metadata: labels: {app: _name, tier: _tier}
					spec: containers: [{
						name:  "app"
						image: input.spec.image
						ports: [{containerPort: 9898}]
					}]
				}
			}
		}
	}
	service: {
		ready: "NotReady"
		object: {
			apiVersion: "v1"
			kind:       "Service"
			metadata: {
				name: _name
				labels: {app: _name, tier: _tier}
			}
			spec: {
				selector: app: _name
				ports: [{port: 80, targetPort: 9898}]
			}
		}
	}
	config: {
		object: {
			apiVersion: "v1"
			kind:       "ConfigMap"
			metadata: {
				name: _name
				labels: {app: _name, tier: _tier}
			}
			data: tier: _tier
		}
	}
}

// status is returned to be patched onto the composite (XR).
status: #Status & {
	ready: true
	url:   "http://\(_name).svc"
}
