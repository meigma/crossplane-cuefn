# How to test a module

`cuefn test` runs a module's declarative test cases — no bash scripts wrapping
`cue`, no cluster. Each case is one [txtar](https://pkg.go.dev/golang.org/x/tools/cmd/txtar#hdr-Txtar_format)
file under the module's `tests/` directory: a few named sections supply the
render inputs and declare what the output must look like. The harness renders
through the exact engine the in-cluster function uses, so a passing case is a
statement about production behavior. This page teaches the workflow; the
[test case format](../reference/test-cases.md) reference specifies the full
contract. For the module's *static* health — formatting, vet, and XRD schema
drift — see [How to check a module](check-a-module.md).

## Write your first case

Create `tests/defaults.txtar` beside your module's CUE files:

```
The #Spec defaults must materialize when the XR omits them.

-- xr.yaml --
apiVersion: platform.meigma.io/v1alpha1
kind: XApp
metadata:
  name: demo
  namespace: default
spec: {}
-- want.cue --
resources: deployment: {
	ready: "Ready"
	object: spec: replicas: 1
}
```

The free text before the first section is the case's description, echoed when
it fails. `xr.yaml` is exactly the file you would pass to `cuefn render --xr`.
Run from the module directory:

```sh
cuefn test
PASS defaults

1 passed, 0 failed, 0 seeded, 0 updated
```

Use `--dir` to run from elsewhere (`cuefn test --dir example/module`). Cases
run in filename order; the case's name is its filename without `.txtar`.

Each section mirrors a `cuefn render` flag, so anything you can render you can
test:

| Section | Mirrors | Content |
|---------|---------|---------|
| `xr.yaml` | `--xr` | The observed XR (required). |
| `environment.yaml` | `--env` | Merged `EnvironmentConfig` data. |
| `required.yaml` | `--required-resources` | Flat multi-document bag of cluster objects, matched against the module's emitted `out.requirements` with the same two-pass loop as render. |
| `observed.yaml` | `--observed-resources` | Observed composed objects, keyed by their `crossplane.io/composition-resource-name` annotation. Only valid for modules that [opt into observed resources](derive-readiness-from-observed-resources.md). |

The vocabulary is closed: a misspelled section name (`enviroment.yaml`) is an
error, never a silently ignored file.

## Assert with partial CUE

`want.cue` is unified with the rendered result. It is **partial by default** —
assert only the fields you care about; everything you omit is ignored:

```
-- want.cue --
resources: {
	deployment: object: spec: template: metadata: labels: tier: "production"
	config: ready: "Unspecified"
}
status: url: "http://demo.svc"
```

The document you are matching against has this shape:

```cue
{
	resources: [name]: {
		ready:  "Ready" | "NotReady" | "Unspecified"
		object: {...}          // the full rendered Kubernetes object
	}
	status:       {...} | null // null when the module returns no status
	requirements: [name]: {...} // {} when the module emits none
}
```

Because CUE constraints are values, expectations can be rules, not just
literals:

```
-- want.cue --
resources: deployment: object: spec: {
	replicas: >=1 & <=10                                    // bounds
	template: spec: containers: [{image: =~"^ghcr\\.io/"}]  // pattern
}
requirements: close({})     // close(): assert *nothing* else — here, no requirements
status: null                // explicit absence is assertable
```

Wrap any struct in `close()` to switch from "at least these fields" to
"exactly these fields". Lists match positionally; for order-independent
membership use `list.Contains` explicitly.

A failure prints every mismatch at once, path-qualified, with positions in
your txtar file's own coordinates:

```
FAIL defaults
    [want.cue] resources.deployment.object.spec.replicas: conflicting values 1 and 3:
        defaults.txtar/want.cue:14:13
```

## Pin the full output with a golden

A case with **no** expectation sections is *seeded*: the harness renders it,
appends the complete normalized output as a `want.yaml` section, reports
`SEED`, and exits non-zero so you review before committing:

```sh
cuefn test
SEED full-render
    wrote want.yaml golden(s); review and commit, or refine into want.cue
```

From then on `want.yaml` is compared exactly; any drift fails with a line
diff. After an intentional module change, re-bless with:

```sh
cuefn test --update
UPDATE full-render
    rewrote drifted want.yaml golden(s)
```

`--update` rewrites **only** machine-owned `want.yaml` sections — it never
touches `want.cue` or `error.txt`, which are hand-written intent. A case may
carry both: `want.yaml` pins everything while `want.cue` documents the fields
that matter.

## Expect a failure

Schema rejections deserve first-class tests. An `error.txt` section asserts
the render **must fail** and that the error contains every non-empty line:

```
-- xr.yaml --
apiVersion: platform.meigma.io/v1alpha1
kind: XApp
metadata:
  name: demo
spec:
  replicas: 99
-- error.txt --
replicas
```

If the render succeeds, the case fails and lists what was rendered.
`error.txt` cannot be combined with `want.*` sections or steps.

## Replay a readiness sequence

Modules that [derive readiness from observed resources](derive-readiness-from-observed-resources.md)
are really state machines: same XR, different observations, different
readiness. Numbered step sections replay that sequence in one case — steps
share the base `xr.yaml`/`environment.yaml`/`required.yaml` and vary only the
observation:

```
-- xr.yaml --
apiVersion: platform.meigma.io/v1alpha1
kind: XApp
metadata:
  name: demo
spec: {}
-- 1/observed.yaml --
apiVersion: apps/v1
kind: Deployment
metadata:
  name: workload-physical
  annotations:
    crossplane.io/composition-resource-name: workload
status:
  observedGeneration: 1
-- 1/want.cue --
resources: workload: ready: "NotReady"
-- 2/observed.yaml --
apiVersion: apps/v1
kind: Deployment
metadata:
  name: workload-physical
  annotations:
    crossplane.io/composition-resource-name: workload
status:
  observedGeneration: 2
  availableReplicas: 1
-- 2/want.cue --
resources: workload: ready: "Ready"
```

Steps must be numbered contiguously from `1` and each needs an
`observed.yaml`. Supplying observed snapshots to a module that never opted
into `observedResources` is an error — the harness refuses the silent no-op.

## Select and iterate

```sh
cuefn test --run '^readiness'   # only cases whose name matches the regex
cuefn test --fail-fast          # stop at the first failing case
```

## Run in CI

In CI mode, seeding and `--update` are refused, so a missing or drifted
golden always fails the build:

```sh
cuefn test --ci
```

CI mode also engages automatically whenever the `CI` environment variable is
set (as on GitHub Actions runners). Goldens are re-blessed locally, reviewed
in the diff, and committed — never silently regenerated in CI.

!!! note "Published modules include their tests"
    `cue mod publish` packages every file in the module directory, so
    `tests/*.txtar` ships inside the published module. That is by design —
    anyone who pulls your module can run its tests — but it means test
    fixtures add to the module's size. Keep observed snapshots trimmed to the
    fields your assertions need.
