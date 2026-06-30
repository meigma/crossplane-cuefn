package app

// out emits one requirement (cfg) selecting a ConfigMap by labels only — the
// matchLabels arm of the oneof — and guards a Deployment on the fetched objects.
// The selector is a pure function of stable inputs so it is byte-identical every
// pass; the engine seeds an empty cfg bucket so the guard stays concrete on the
// first pass.
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

	requirements: cfg: {
		apiVersion: "v1"
		kind:       "ConfigMap"
		matchLabels: app: input.spec.appName
	}

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
