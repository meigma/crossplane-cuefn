// Package contract holds the Go-side validation for the shipped CUE contract
// module (the repo's top-level contract/ directory). Its tests load the contract
// and prove its definitions are closed, so a non-conforming author module is
// caught at `cue vet` time. The contract module itself is pure CUE; this package
// carries no runtime code.
package contract
