package observed

import "encoding/json"

// This fixture explicitly opts in by declaring observedResources as a regular
// field. It copies an open, kind-specific status value into its output so tests
// prove the object survives the engine boundary without projection.
out: {
	input: {
		spec: {...}
		metadata: {
			name:       string | *"demo"
			namespace?: string
			...
		}
		environment: {...}
		observedResources: [string]: {
			apiVersion: string
			kind:       string
			status: {
				custom: {
					nested: string | *""
					...
				}
				...
			}
			...
		}
	}

	_matches: [
		for name, object in input.observedResources
		if name == "workload"
		if object.kind == "Deployment"
		if object.status.custom.nested == "seen" {
			object.status.custom.nested
		},
	]
	_evidence:    string | *""
	_observation: string | *"{}"
	if len(_matches) == 1 {
		_evidence:    _matches[0]
		_observation: json.Marshal(input.observedResources.workload)
	}

	resources: probe: {
		if len(_matches) == 1 {
			ready: "Ready"
		}
		if len(_matches) != 1 {
			ready: "NotReady"
		}
		object: {
			apiVersion: "v1"
			kind:       "ConfigMap"
			metadata: name: input.metadata.name
			data: {
				observedCount: "\(len(input.observedResources))"
				evidence:      _evidence
				observation:   _observation
			}
		}
	}
	status: {
		observedCount: len(input.observedResources)
		workloadReady: len(_matches) == 1
	}
}
