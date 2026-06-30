package app

// out emits a requirement that sets BOTH matchName and matchLabels. The values
// are concrete (so readRequirements' Validate(Concrete) passes), but the
// exactly-one check then fails, and Render returns the "exactly one" error that
// the function adapter maps to a Fatal result.
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
		apiVersion:  "v1"
		kind:        "ConfigMap"
		matchName:   input.spec.configName
		matchLabels: app: "x"
	}

	resources: {}
}
