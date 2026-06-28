// Package disjunction is a NEGATIVE codegen fixture: #Spec contains a
// type-crossing disjunction (string|int) which OpenAPI renders as a oneOf and a
// Kubernetes structural schema cannot express. Codegen must reject it with a
// DisjunctionError that names the offending field, never a panic.
package disjunction

#API: {
	group:   "platform.example.com"
	version: "v1alpha1"
	kind:    "XThing"
	plural:  "xthings"
	scope:   "Namespaced"
}

#Spec: {
	// type-crossing disjunction: not expressible as a structural schema.
	value: string | int
}
