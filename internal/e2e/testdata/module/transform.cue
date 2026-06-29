package app

// input is filled by the engine with the observed XR spec (validated against
// #Spec), its metadata, and the merged environment. This is the e2e variant of
// the example module: it shares the example's #API/#Spec/#Status verbatim (so the
// generated XRD is identical) but marks every composed resource Ready, so the
// composite reconciles to Ready=True on a live cluster (the example module
// deliberately forces a NotReady service to exercise all readiness states in the
// offline render test, which would keep the composite NotReady in-cluster).
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

// resources is the author-keyed map of composed resources. Every entry is marked
// Ready so the live composite reaches Ready=True without depending on the real
// workload's runtime health (the function's readiness hint is authoritative).
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

// status is returned to be patched onto the composite (XR).
status: #Status & {
	ready: true
	url:   "http://\(_name).svc"
}
