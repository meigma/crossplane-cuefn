package schema

import "fmt"

// DisjunctionError reports a type-crossing disjunction in a module's schema
// definitions — for example string|int or a struct union {a}|{b}. OpenAPI
// renders these as a oneOf, which a Kubernetes structural schema cannot express,
// so codegen rejects them up front (and names the field) rather than emitting a
// non-structural XRD or panicking. Same-type disjunctions (enums such as
// "a"|"b" or 80|443) render as an enum and are accepted.
type DisjunctionError struct {
	// Field is the dotted path to the offending schema field, rooted at the
	// definition that contained it (for example "Spec.value").
	Field string

	// Detail explains why the disjunction is not expressible.
	Detail string
}

// Error implements error.
func (e *DisjunctionError) Error() string {
	return fmt.Sprintf("type-crossing disjunction at %s: %s", e.Field, e.Detail)
}
