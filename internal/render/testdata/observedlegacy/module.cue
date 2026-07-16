package observedlegacy

// These definitions are the relevant closed v0.2.0 contract shape: there is no
// observedResources field. A blind fill would fail closedness as soon as a child
// exists; conditional omission must keep this module byte-for-byte unchanged.
#InputV020: {
	spec: _
	metadata: {
		name:       string
		namespace?: string
	}
	environment: {...}
	requiredResources?: [string]: [...{
		apiVersion: string
		kind:       string
		...
	}]
}

#TransformV020: {
	input: #InputV020
	resources: [string]: {
		object: {
			apiVersion: string
			kind:       string
			...
		}
		ready?: "Ready" | "NotReady"
	}
	status?: _
}

out: #TransformV020 & {
	input: {
		spec: {}
		metadata: name: string | *"demo"
		environment: {}
	}
	resources: fixed: object: {
		apiVersion: "v1"
		kind:       "ConfigMap"
		metadata: name: input.metadata.name
	}
	status: marker: "v0.2.0"
}
