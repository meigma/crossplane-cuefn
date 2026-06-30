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
			name: string | *"app"
			// Default to "" so a cluster-scoped (namespaceless) XR still yields a
			// concrete value and the requirement below omits the selector's
			// namespace; a namespaced XR fills this with its real namespace at
			// render time, scoping the read to the XR's own namespace.
			namespace: string | *""
			...
		}
		environment: {
			tier: string | *"unset"
			...
		}

		// requiredResources is the bucket the cuefn engine seeds (one empty list
		// per declared requirement) before reading resources, so a data-dependent
		// guard stays concrete on the first pass. The `| *[]` default is a raw-eval
		// aid only — it lets this module `cue vet`/`cue export` standalone and is
		// NOT load-bearing inside cuefn (the engine seed and the delivered objects
		// both override it).
		requiredResources: cfg: [...] | *[]
	}

	_name: input.metadata.name
	_tier: input.environment.tier

	// requirements asks Crossplane's core controller to fetch the operator-supplied
	// ConfigMap named by spec.configName, scoped to the XR's namespace. It is a
	// PURE function of stable spec/metadata inputs (never of fetched data), so it
	// is byte-identical on every pass and Crossplane's proto.Equal fixpoint
	// converges. For the existing demo XR no ConfigMap of this name exists, so the
	// delivered bucket is empty and the guarded resource below never renders.
	requirements: cfg: {
		apiVersion: "v1"
		kind:       "ConfigMap"
		matchName:  input.spec.configName
		if input.metadata.namespace != "" {
			namespace: input.metadata.namespace
		}
	}

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

		// The data-dependent, guarded resource. It renders ONLY when Crossplane
		// delivered a ConfigMap for the `cfg` requirement (a non-empty bucket): for
		// the demo XR the bucket is the engine-seeded empty list, so this loop emits
		// nothing and the composed set stays byte-identical to before. When an
		// operator ConfigMap is supplied it mirrors that ConfigMap's data.image into
		// a new ConfigMap, proving the second render pass consumed fetched cluster
		// data. The name is derived from metadata.name so it never collides with the
		// composed resources above.
		for i, cm in input.requiredResources.cfg {
			"required-\(i)": {
				ready: "Ready"
				object: {
					apiVersion: "v1"
					kind:       "ConfigMap"
					metadata: {
						name: "\(_name)-cfg-\(i)"
						labels: {app: _name, tier: _tier}
					}
					data: image: cm.data.image
				}
			}
		}
	}

	status: #Status & {
		ready: true
		url:   "http://\(_name).svc"
	}
}
