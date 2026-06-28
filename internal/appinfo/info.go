package appinfo

const name = "cuefn"

// Summary returns the default message printed by the cuefn CLI.
func Summary() string {
	return name + " — CUE-over-OCI Crossplane composition function"
}
