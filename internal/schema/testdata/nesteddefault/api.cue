// Package nesteddefault exercises REQUIRED, fully-defaultable container fields: a
// nested struct whose every field defaults, a keyless map, and a zero-minimum
// list. CUE fills all three from an empty spec, but before codegen emitted
// object/list defaults for them the API server rejected `spec: {}` with
// "Required value" — the validate/render vs in-cluster drift (findings H3/M10).
package nesteddefault

#API: {
	group:   "platform.example.com"
	version: "v1alpha1"
	kind:    "XNested"
	plural:  "xnesteds"
}

#Spec: {
	// required nested struct whose every field carries a default.
	resources: {
		cpu:      string | *"250m"
		memoryMi: int | *256
	}

	// required keyless map: an empty map satisfies it (the M10 case).
	labels: {[string]: string}

	// required zero-minimum list: an empty list satisfies it.
	tags: [...string]

	// required scalar carrying its own default.
	replicas: *2 | int & >=1 & <=10
}
