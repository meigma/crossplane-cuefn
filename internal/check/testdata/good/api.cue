// Package good is a minimal healthy module: a concrete #API, a #Spec with a
// required no-default field (the case bare `cue vet` fails on and Vet must
// accept), and an out transform. It uses no imports so it loads fully offline.
package good

#API: {
	group:   "platform.example.com"
	version: "v1alpha1"
	kind:    "XGadget"
	plural:  "xgadgets"
}

#Spec: {
	name!:    string
	replicas: int & >=1 & <=10 | *3
}

out: {
	input: {
		spec: #Spec
		metadata: {...}
		environment: {...}
	}
	resources: config: object: {
		apiVersion: "v1"
		kind:       "ConfigMap"
		metadata: name: input.metadata.name
		data: name:     input.spec.name
	}
}
