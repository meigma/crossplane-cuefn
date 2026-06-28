// Package nostatus is a codegen fixture WITHOUT a #Status definition. It proves
// the generated XRD omits .properties.status (and the wrapped CRD omits the
// status subresource) when the module declares no status schema.
package nostatus

#API: {
	group:   "platform.example.com"
	version: "v1alpha1"
	kind:    "XGadget"
	plural:  "xgadgets"
	scope:   "Cluster"
}

#Spec: {
	name!:     string
	replicas:  *1 | int & >=1 & <=5
}
