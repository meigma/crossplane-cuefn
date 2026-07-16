# How to derive readiness from observed resources

Use `out.input.observedResources` when a module needs to decide whether the
objects it composes are actually ready. Crossplane supplies a point-in-time
snapshot of each observed child under the same stable key used in
`out.resources`; cuefn does not read the API server itself.

This guide builds conservative predicates for three common cases:

- a release-specific Job must complete without a failed condition;
- a one-replica Deployment must be observed at its current generation and have
  enough updated and available replicas;
- a conditionless ConfigMap is ready once the expected object has a UID.

The field is available in contract **v0.3.0**. Resolve that version or newer
before adopting it.

## 1. Opt in and normalize missing fields

Importing the contract leaves `observedResources` optional for compatibility.
Materialize it as a regular field to opt in. An opted-in module receives a
concrete `{}` on the first pass.

Kubernetes omits empty status objects, zero counters, labels, and condition
lists. Give every field used by a predicate a conservative default so a first or
partial observation evaluates to `false`, not an incomplete CUE value:

```cue
import "github.com/meigma/crossplane-cuefn/contract@v0"

#Observed: {
	apiVersion: string | *""
	kind:       string | *""
	metadata: {
		name:       string | *""
		uid:        string | *""
		generation: int | *0
		labels:     *{} | {[string]: string}
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

out: contract.#Transform & {
	input: {
		spec: #Spec
		metadata: {/* ... */}
		environment: {/* ... */}
		observedResources: {
			[string]: #Observed
			migration: #Observed
			workload:  #Observed
			config:    #Observed
		}
	}
	// ...resources and status...
}
```

The three named fields create safe, defaulted lookups on the empty first pass.
The pattern keeps the map open to any other composed resources. Do not default a
readiness fact to true.

## 2. Build identity and health predicates

Stable map keys are necessary but not sufficient. Check that each snapshot is
the object expected for the current release before trusting its status:

```cue
_release:      input.spec.release
_releaseLabel: "platform.example/release"

_job:        input.observedResources.migration
_deployment: input.observedResources.workload
_config:     input.observedResources.config

_jobReleaseMatches: [
	for key, value in _job.metadata.labels
	if key == _releaseLabel && value == _release {true},
]
_deploymentReleaseMatches: [
	for key, value in _deployment.metadata.labels
	if key == _releaseLabel && value == _release {true},
]
_configReleaseMatches: [
	for key, value in _config.metadata.labels
	if key == _releaseLabel && value == _release {true},
]

_jobIdentity: _job.apiVersion == "batch/v1" &&
	_job.kind == "Job" &&
	_job.metadata.name == "\(input.metadata.name)-migration-\(_release)" &&
	_job.metadata.uid != "" &&
	len(_jobReleaseMatches) == 1

_deploymentIdentity: _deployment.apiVersion == "apps/v1" &&
	_deployment.kind == "Deployment" &&
	_deployment.metadata.name == "\(input.metadata.name)-workload-\(_release)" &&
	_deployment.metadata.uid != "" &&
	len(_deploymentReleaseMatches) == 1

_configIdentity: _config.apiVersion == "v1" &&
	_config.kind == "ConfigMap" &&
	_config.metadata.name == "\(input.metadata.name)-config-\(_release)" &&
	_config.metadata.uid != "" &&
	len(_configReleaseMatches) == 1
```

Now add kind-specific health checks:

```cue
_jobComplete: [
	for condition in _job.status.conditions
	if condition.type == "Complete" && condition.status == "True" {true},
]
_jobFailed: [
	for condition in _job.status.conditions
	if condition.type == "Failed" && condition.status == "True" {true},
]
_deploymentAvailable: [
	for condition in _deployment.status.conditions
	if condition.type == "Available" && condition.status == "True" {true},
]

_migrationReady: _jobIdentity &&
	len(_jobComplete) >= 1 &&
	len(_jobFailed) == 0 &&
	_job.status.succeeded >= 1

_desiredReplicas: 1
_workloadReady: _deploymentIdentity &&
	_deployment.metadata.generation > 0 &&
	_deployment.status.observedGeneration == _deployment.metadata.generation &&
	len(_deploymentAvailable) >= 1 &&
	_deployment.status.updatedReplicas >= _desiredReplicas &&
	_deployment.status.availableReplicas >= _desiredReplicas

_configReady: _configIdentity
```

The generation equality rejects a stale Deployment status. Checking both updated
and available counts prevents an `Available=True` condition from accepting an
old or undersized rollout. The Job predicate fails closed if `Failed=True` is
also present. A ConfigMap has no readiness condition, so identity plus a
server-assigned UID is the existence proof.

## 3. Return explicit readiness and diagnostics

Map each predicate to an explicit function readiness hint. Do this on the same
stable key whose observation the predicate reads:

```cue
out: {
	resources: {
		migration: {
			if _migrationReady {ready: "Ready"}
			if !_migrationReady {ready: "NotReady"}
			object: {/* desired Job */}
		}
		workload: {
			if _workloadReady {ready: "Ready"}
			if !_workloadReady {ready: "NotReady"}
			object: {/* desired Deployment */}
		}
		config: {
			if _configReady {ready: "Ready"}
			if !_configReady {ready: "NotReady"}
			object: {/* desired ConfigMap */}
		}
	}
	status: {
		migrationReady: _migrationReady
		workloadReady:  _workloadReady
		configReady:    _configReady
	}
}
```

Crossplane 2.3.3 counts only explicit `Ready` responses. An absent hint is
`Unspecified`, which does not become ready by inspecting the object's own status
conditions. Keep the diagnostic booleans in XR status so operators can see which
predicate is holding the composite.

## 4. Exercise snapshots offline

Create raw observed objects with the standard stable-name annotation. The
physical name may differ from the key:

```yaml title="observed/deployment.yaml"
apiVersion: apps/v1
kind: Deployment
metadata:
  name: demo-workload-r1
  uid: 5d7631d2-5a45-49cb-91c0-c0cbe729b412
  generation: 3
  labels:
    platform.example/release: r1
  annotations:
    crossplane.io/composition-resource-name: workload
spec:
  replicas: 1
status:
  observedGeneration: 3
  updatedReplicas: 1
  availableReplicas: 1
  conditions:
    - type: Available
      status: "True"
```

Pass a file, multi-document file, or directory to the standalone renderer:

```sh
cuefn render cuefn.example/readiness@v0 \
  --dir ./module \
  --xr ./xr.yaml \
  --observed-resources ./observed
```

The loader rejects a missing, empty, or duplicate
`crossplane.io/composition-resource-name`. It preserves the full object body and
does not derive the key from `metadata.name`.

Exercise at least these snapshots before publishing:

- no observed objects;
- all three current and healthy;
- Deployment `observedGeneration` behind `metadata.generation`;
- zero updated or available replicas;
- Job with both `Complete=True` and `Failed=True`;
- ConfigMap without a UID.

For protocol parity, pass the same raw objects to Crossplane's renderer:

```sh
crossplane render xr.yaml composition.yaml functions.yaml \
  --observed-resources ./observed
```

Observed resources are a request snapshot and may lag the API server. A later
reconcile supplies fresher state; keep predicates conservative instead of
assuming a missing or stale value is ready.

## See also

- [Module contract: observing composed resources](../reference/module-contract.md#observing-composed-resources)
- [CLI reference: render](../reference/cli.md#render)
- [Required resources and the fixpoint](../explanation/required-resources-fixpoint.md)
