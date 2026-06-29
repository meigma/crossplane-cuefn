package app

// out is the cuefn transform: the engine fills out.input and reads out.resources
// and out.status. #API/#Spec/#Status stay top-level in api.cue. This hermetic
// fixture is self-contained (no imports) so the offline tests never touch a
// registry; it deliberately does not import the contract module.
out: {
	// input is filled by the engine with the observed XR spec (validated against
	// #Spec), its metadata, and the merged environment.
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

	// resources covers all readiness states: an explicit Ready, an explicit
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

	status: #Status & {
		ready: true
		url:   "http://\(_name).svc"
	}
}
