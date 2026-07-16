package observedoptional

// #Input models the additive contract after observedResources is added. The
// transform only inherits the optional field and does not promote it to regular,
// so the engine must not fill observations into this module.
#Input: {
	spec: _
	metadata: {
		name:       string
		namespace?: string
	}
	environment: {...}
	observedResources?: [string]: {
		apiVersion: string
		kind:       string
		...
	}
}

out: {
	input: #Input & {
		spec: {}
		metadata: name: string | *"demo"
		environment: {}
	}
	resources: fixed: object: {
		apiVersion: "v1"
		kind:       "ConfigMap"
		metadata: name: input.metadata.name
	}
	status: marker: "unchanged"
}
