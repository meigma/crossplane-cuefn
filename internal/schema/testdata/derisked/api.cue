// Package derisked is the canonical author-time codegen fixture. It deliberately
// exercises every construct the codegen de-risk spike proved out, so the unit
// tests and the chainsaw CRD share one source of truth:
//
//   - a bounded number with a default (the ExpandReferences:true bug case),
//   - a nested definition referenced as a list-of-objects (forces the inliner),
//   - a same-type enum disjunction,
//   - a regex (pattern) constraint,
//   - a bool default,
//   - a string map (additionalProperties),
//   - required (!) and optional (?) fields,
//   - printerColumns / categories / shortNames on #API.
//
// It uses no external imports so it loads fully offline.
package derisked

// #API is the concrete platform API description the codegen decodes into the XRD
// envelope. shortNames/categories/printerColumns are optional and carry through
// when present.
#API: {
	group:   "platform.example.com"
	version: "v1alpha1"
	kind:    "XWidget"
	plural:  "xwidgets"
	scope:   *"Namespaced" | "Cluster"
	shortNames: ["wdg"]
	categories: ["platform"]
	printerColumns: [{
		name:     "Replicas"
		type:     "integer"
		jsonPath: ".spec.replicas"
	}, {
		name:     "Tier"
		type:     "string"
		jsonPath: ".spec.tier"
	}]
}

// #Port is a nested definition. Referencing it from a list forces the codegen
// $ref inliner, since structural schemas forbid $ref.
#Port: {
	name: string
	// bounded number nested inside a referenced definition.
	port: int & >=1 & <=65535
}

// #Spec is the authoritative, closed XR spec schema.
#Spec: {
	// bounded number with a default: the ExpandReferences:true bug case.
	replicas: *3 | int & >=1 & <=10

	// required field with a regex (pattern) constraint.
	image!: string & =~"^[a-z0-9./:@-]+$"

	// same-type enum disjunction (stays an enum, not a oneOf).
	tier: *"standard" | "premium" | "dedicated"

	// bool default.
	exposed: *false | bool

	// list-of-objects referencing a nested definition (forces the inliner).
	ports?: [...#Port]

	// string map (additionalProperties; structural-safe, unlike patternProperties).
	labels?: {[string]: string}
}

// #Status is the optional XR status schema. Its presence drives the status
// subresource and .properties.status in the generated XRD.
#Status: {
	ready:     bool
	endpoint?: string
}
