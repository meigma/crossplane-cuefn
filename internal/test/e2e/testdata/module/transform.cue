package app

// out is the cuefn transform. This is the e2e variant of the example module: it
// shares the example's #API/#Spec/#Status verbatim (so the generated XRD is
// identical) but marks every composed resource Ready, so the composite reconciles
// to Ready=True on a live cluster (the example deliberately forces a NotReady
// service to exercise all readiness states in the offline render test).
out: {
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

	// Every entry is marked Ready so the live composite reaches Ready=True without
	// depending on the real workload's runtime health.
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
			ready: "Ready"
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
			ready: "Ready"
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
