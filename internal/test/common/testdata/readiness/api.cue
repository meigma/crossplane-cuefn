// Package readiness is the hermetic observed-readiness test fixture. It is
// self-contained and import-free so renderer tests never need a module registry.
package readiness

// #API describes the composite used by the live readiness scenario.
#API: {
	group:   "platform.meigma.io"
	version: "v1alpha1"
	kind:    "XReadiness"
	plural:  "xreadinesses"
	scope:   *"Namespaced" | "Cluster"
}

// #Spec exposes the two gates independently so a caller can release the
// migration Job before unpausing the workload Deployment.
#Spec: {
	release:          =~"^[a-z0-9]([-a-z0-9]*[a-z0-9])?$" | *"r1"
	suspendMigration: bool | *true
	pauseWorkload:    bool | *true
}

// #Status makes each observed-resource predicate independently diagnosable.
#Status: {
	migrationReady: bool
	workloadReady:  bool
	configReady:    bool
}
