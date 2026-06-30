package app

// out is the cuefn transform. It emits one requirement (cfg) selecting a
// ConfigMap by input.spec.configName, and guards a Deployment on the fetched
// objects under input.requiredResources.cfg. The selector is a pure function of
// stable inputs (spec/metadata) so it is byte-identical every pass. The
// namespace is guarded so a cluster-scoped (namespaceless) XR yields a concrete,
// namespace-omitting selector. This fixture is import-free and does not import
// the contract; the engine seeds an empty cfg bucket so the guard stays concrete
// on the first pass.
out: {
	input: {
		spec: #Spec
		metadata: {
			name:      string | *"app"
			namespace: string | *""
			...
		}
		environment: {...}
	}

	// Pure function of stable inputs -> byte-identical every pass -> converges.
	requirements: cfg: {
		apiVersion: "v1"
		kind:       "ConfigMap"
		matchName:  input.spec.configName
		if input.metadata.namespace != "" {
			namespace: input.metadata.namespace
		}
	}

	// No-ops on the seeded [] (first pass -> out.resources is a concrete {});
	// emits one Deployment per fetched ConfigMap on later passes, reading
	// cm.data.image off the delivered object.
	resources: {
		for i, cm in input.requiredResources.cfg {
			"deployment-\(i)": {
				ready: "Ready"
				object: {
					apiVersion: "apps/v1"
					kind:       "Deployment"
					metadata: name: "\(input.metadata.name)-\(i)"
					spec: image: cm.data.image
				}
			}
		}
	}

	status: #Status & {
		ready: true
	}
}
