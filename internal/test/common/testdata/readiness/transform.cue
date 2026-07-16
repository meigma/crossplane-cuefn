package readiness

// #ObservedResource deliberately normalizes every field used by the readiness
// predicates. Kubernetes commonly omits zero counters, empty status, labels,
// and condition lists; defaults keep those ordinary first observations
// concrete and conservatively NotReady.
#ObservedResource: {
	apiVersion: string | *""
	kind:       string | *""
	metadata: {
		name:       string | *""
		uid:        string | *""
		generation: int | *0
		labels: *{} | {[string]: string}
		...
	}
	spec: {
		replicas: int | *0
		...
	}
	status: {
		observedGeneration: int | *0
		succeeded:          int | *0
		updatedReplicas:    int | *0
		availableReplicas:  int | *0
		conditions: *[] | [...{
			type:   string | *""
			status: string | *""
			...
		}]
		...
	}
	...
}

out: {
	input: {
		spec: #Spec
		metadata: {
			name:       string | *"readiness"
			namespace?: string
			...
		}
		environment: {...}

		// This regular field is the module's explicit observed-resources opt-in.
		// The three concrete buckets make stable-key lookups safe on the empty
		// first pass while the pattern preserves any additional resources.
		observedResources: {
			[string]:  #ObservedResource
			migration: #ObservedResource
			workload:  #ObservedResource
			config:    #ObservedResource
		}
	}

	_name:            input.metadata.name
	_release:         input.spec.release
	_migrationName:   "\(_name)-migration-\(_release)"
	_workloadName:    "\(_name)-workload-\(_release)"
	_configName:      "\(_name)-config-\(_release)"
	_releaseLabel:    "cuefn.example/release"
	_identityLabel:   "cuefn.example/readiness"
	_desiredReplicas: 1

	_migration: input.observedResources.migration
	_workload:  input.observedResources.workload
	_config:    input.observedResources.config

	_migrationReleaseLabels: [
		for key, value in _migration.metadata.labels
		if key == _releaseLabel && value == _release {value},
	]
	_workloadReleaseLabels: [
		for key, value in _workload.metadata.labels
		if key == _releaseLabel && value == _release {value},
	]
	_configReleaseLabels: [
		for key, value in _config.metadata.labels
		if key == _releaseLabel && value == _release {value},
	]
	_migrationIdentityLabels: [
		for key, value in _migration.metadata.labels
		if key == _identityLabel && value == _name {value},
	]
	_workloadIdentityLabels: [
		for key, value in _workload.metadata.labels
		if key == _identityLabel && value == _name {value},
	]
	_configIdentityLabels: [
		for key, value in _config.metadata.labels
		if key == _identityLabel && value == _name {value},
	]

	_migrationIdentityReady: _migration.apiVersion == "batch/v1" &&
		_migration.kind == "Job" &&
		_migration.metadata.name == _migrationName &&
		_migration.metadata.uid != "" &&
		len(_migrationReleaseLabels) == 1 &&
				len(_migrationIdentityLabels) == 1
	_workloadIdentityReady: _workload.apiVersion == "apps/v1" &&
		_workload.kind == "Deployment" &&
		_workload.metadata.name == _workloadName &&
		_workload.metadata.uid != "" &&
		len(_workloadReleaseLabels) == 1 &&
				len(_workloadIdentityLabels) == 1
	_configIdentityReady: _config.apiVersion == "v1" &&
		_config.kind == "ConfigMap" &&
		_config.metadata.name == _configName &&
		_config.metadata.uid != "" &&
		len(_configReleaseLabels) == 1 &&
		len(_configIdentityLabels) == 1

	_jobCompleteConditions: [
		for condition in _migration.status.conditions
		if condition.type == "Complete" && condition.status == "True" {true},
	]
	_jobFailedConditions: [
		for condition in _migration.status.conditions
		if condition.type == "Failed" && condition.status == "True" {true},
	]
	_deploymentAvailableConditions: [
		for condition in _workload.status.conditions
		if condition.type == "Available" && condition.status == "True" {true},
	]

	_migrationReady: _migrationIdentityReady &&
		len(_jobCompleteConditions) >= 1 &&
		len(_jobFailedConditions) == 0 &&
			_migration.status.succeeded >= 1
	_workloadReady: _workloadIdentityReady &&
		_workload.metadata.generation > 0 &&
		_workload.status.observedGeneration == _workload.metadata.generation &&
		len(_deploymentAvailableConditions) >= 1 &&
		_workload.status.updatedReplicas >= _desiredReplicas &&
			_workload.status.availableReplicas >= _desiredReplicas
	_configReady: _configIdentityReady

	resources: {
		migration: {
			if _migrationReady {
				ready: "Ready"
			}
			if !_migrationReady {
				ready: "NotReady"
			}
			object: {
				apiVersion: "batch/v1"
				kind:       "Job"
				metadata: {
					name: _migrationName
					labels: {
						(_releaseLabel):  _release
						(_identityLabel): _name
					}
				}
				spec: {
					suspend: input.spec.suspendMigration
					template: {
						metadata: labels: {
							(_releaseLabel):  _release
							(_identityLabel): _name
						}
						spec: {
							restartPolicy: "Never"
							containers: [{
								name:            "migration"
								image:           "crossplane-cuefn:dev"
								imagePullPolicy: "IfNotPresent"
								args: ["--message", "migration-complete"]
							}]
						}
					}
				}
			}
		}

		workload: {
			if _workloadReady {
				ready: "Ready"
			}
			if !_workloadReady {
				ready: "NotReady"
			}
			object: {
				apiVersion: "apps/v1"
				kind:       "Deployment"
				metadata: {
					name: _workloadName
					labels: {
						(_releaseLabel):  _release
						(_identityLabel): _name
					}
				}
				spec: {
					paused:   input.spec.pauseWorkload
					replicas: _desiredReplicas
					selector: matchLabels: {
						(_releaseLabel):  _release
						(_identityLabel): _name
					}
					template: {
						metadata: labels: {
							(_releaseLabel):  _release
							(_identityLabel): _name
						}
						spec: containers: [{
							name:            "workload"
							image:           "crossplane-cuefn:dev"
							imagePullPolicy: "IfNotPresent"
							args: ["function", "--insecure", "--metrics-address", ""]
						}]
					}
				}
			}
		}

		config: {
			if _configReady {
				ready: "Ready"
			}
			if !_configReady {
				ready: "NotReady"
			}
			object: {
				apiVersion: "v1"
				kind:       "ConfigMap"
				metadata: {
					name: _configName
					labels: {
						(_releaseLabel):  _release
						(_identityLabel): _name
					}
				}
				data: release: _release
			}
		}
	}

	status: #Status & {
		migrationReady: _migrationReady
		workloadReady:  _workloadReady
		configReady:    _configReady
	}
}
