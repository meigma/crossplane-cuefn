// Package disjunction declares a type-crossing disjunction in #Spec — the one
// schema construct Kubernetes structural schemas cannot express, which XRD
// generation must reject naming the field.
package disjunction

#API: {
	group:   "platform.example.com"
	version: "v1alpha1"
	kind:    "XPort"
	plural:  "xports"
}

#Spec: {
	port: string | int
}
