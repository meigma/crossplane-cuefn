// Package app is an intentionally NON-CONVERGING fixture: out.requirements
// depends on the fetched data (it emits a second requirement only once the first
// is delivered), so the requirement set differs between render passes and never
// reaches a fixpoint. This is the author anti-pattern cuefn render must surface
// as "requirements did not stabilize" rather than print a bogus second pass.
package app

#API: {
	group:   "platform.meigma.io"
	version: "v1alpha1"
	kind:    "XApp"
	plural:  "xapps"
	scope:   *"Namespaced" | "Cluster"
}

#Spec: {
	configName: string | *"app-cfg"
}

out: {
	input: {
		spec: #Spec
		metadata: {
			name:      string | *"app"
			namespace: string | *"default"
		}
		// Default cfg to [] so out.requirements (which impurely references it) stays
		// concrete on the first pass, before the engine seeds delivered resources.
		// The engine overrides this with the delivered objects on later passes.
		requiredResources: cfg: [...{...}] | *[]
		environment: {...}
	}

	requirements: {
		cfg: {
			apiVersion: "v1"
			kind:       "ConfigMap"
			matchName:  input.spec.configName
			namespace:  input.metadata.namespace
		}
		// Impure: appears only after cfg is delivered, so pass 2's requirement set
		// differs from pass 1's and the stabilization check fails.
		if len(input.requiredResources.cfg) > 0 {
			extra: {
				apiVersion: "v1"
				kind:       "Secret"
				matchName:  "app-secret"
				namespace:  input.metadata.namespace
			}
		}
	}

	resources: {}
}
