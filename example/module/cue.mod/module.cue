module: "cuefn.example/app@v0"
language: {
	version: "v0.16.0"
}
source: {
	kind: "self"
}
deps: {
	"cue.dev/x/k8s.io@v0": {
		v:       "v0.7.0"
		default: true
	}
	"github.com/meigma/crossplane-cuefn/contract@v0": {
		v: "v0.1.0"
	}
}
