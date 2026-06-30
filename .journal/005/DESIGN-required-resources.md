---
title: Required resources for cuefn modules
status: proposal — awaiting human review
date: 2026-06-29
temporary: true   # working design doc — delete at session close (.journal/<id>/ norm)
summary: Let a CUE composition module request additional cluster data at render time via a symmetric, author-keyed out.requirements (emit) / out.input.requiredResources (receive) pair, with Crossplane owning the fetch-and-re-invoke fixpoint and the render core staying pure.
---

> **TEMPORARY.** This is a session-scoped design proposal living under `.journal/<id>/`. Delete it at session close (DESIGN.md/PLAN.md precedent). Status: **proposal — awaiting human review**.

## Summary & goals

Let a CUE module request additional cluster data at render time, the way native Crossplane composition functions do. The mechanism is symmetric and author-chosen in both directions: a module **emits** selectors under `out.requirements` (keyed by a name it picks), and the objects Crossplane fetched reappear under `out.input.requiredResources` keyed by the **same** name. Crossplane owns the fetch-and-re-invoke loop; the function stays a pure function of its inputs.

Target: Crossplane v2, crossplane CLI v2.3.3, `function-sdk-go@v0.7.1`. We speak the current wire fields (`required_resources` on the request, `Requirements.Resources` on the response), not the deprecated `extra_resources`/`ExtraResources`.

**Goals**

- A module declares required cluster resources and consumes the fetched objects, with one author-chosen map key identifying the group in both directions.
- The render core stays pure: all proto/Crossplane types remain in `internal/function`; `internal/render` gains only plain Go shapes (no `fnv1`/proto imports).
- The contract change is additive and backward compatible within v0 (minor bump); existing pinned modules are unaffected.
- First-pass concreteness is guaranteed engine-side (empty-bucket seeding) with zero author boilerplate.
- Offline iteration via `cuefn render --required-resources <file|dir>`.

**Non-goals**

- **No change to the step `Input` type.** Required resources arrive in the request, requirements come from the module output, so `input/v1beta1.Input{Module, ExpectedDigest}` is untouched — no CRD regen.
- **No change to XRD codegen.** `internal/schema` reduces the module to definitions-only and drops `out`; `requirements`/`requiredResources` are runtime-only.
- **Not the v2 static "bootstrap" path** (Composition `step.requirements.requiredResources`); see Operations for the deferred `cuefn publish` follow-up.
- **No reading of the deprecated `extra_resources` field.** The target versions deliver on `required_resources`; the adapter reads only `request.GetRequiredResources`.
- **No new namespace enforcement on the read path** (see Operations); existing enforcement stays scoped to composed/created resources.
- **No relaxation of `cue.Concrete(true)`** anywhere in the core.
- **No example/deploy-asset ClusterRole in the core PRs**; that is a scoped follow-up.

## How Crossplane required resources work

The function is a **pure function** of `(observed XR + observed composed + delivered required resources + input + context) → (desired + requirements)`. Crossplane, not the function, owns iteration:

- **Fixpoint loop** (`internal/xfn` `FetchingFunctionRunner`): `const MaxRequirementsIterations = 5`, loop bound `i <= 5` (up to ~6 `RunFunction` calls). Each pass the function returns desired state **and** `Requirements`; Crossplane fetches the requested objects and re-invokes, carrying context forward. It stops when `proto.Equal(newRequirements, prevRequirements)` (two consecutive identical `Requirements` = fixpoint). A `Fatal` result also stops the loop. If requirements never stabilize, Crossplane errors `requirements didn't stabilize`.
- **Non-final desired is discarded.** Only the **final stable** response's desired state is applied; intermediate passes exist only to drive fetches.
- **Missing → empty.** A requested resource that is not found comes back with the request map key **present** and an **empty** items list — `request.GetRequiredResource(req, name)` returns `ok=true` with an empty slice.
- **Determinism:** list results are sorted by namespace/name; map keys are author-chosen and identify the group in both directions. Requirements **must be a pure function of stable inputs** (spec/metadata/environment), never of fetched data — otherwise the requirements differ each pass and the fixpoint never converges.
- **v2 rename:** "extra resources" → **required resources**. We use the current wire fields.
- **Capability:** Crossplane advertises `CAPABILITY_REQUIRED_RESOURCES`. If it does not, it never iterates, and a module that hides resources behind a requirement guard renders empty forever.

## Module contract shape

This feature adds two symmetric, author-keyed maps to the published contract:

- `out.requirements` — selectors the module **emits** for Crossplane to fetch (the `#Transform` side).
- `out.input.requiredResources` — the fetched objects the engine **fills**, keyed by the same author-chosen name (the `#Input` side).

The step `Input` type is unchanged: required resources arrive in the RunFunction *request*, requirements come from the *module output*.

### Exact closed-definition additions (`contract/contract.cue`)

Dropped into the existing closed `#Input`/`#Transform`; `#API`, `#Resource`, and closedness are unchanged. These are the **prototype-verified** shapes (cue v0.16.1, under both `&` unification and the engine's exact `FillPath` + `Validate(cue.Concrete(true))`).

```cue
// #Required is one cluster object Crossplane fetched for a requirement, surfaced
// under out.input.requiredResources[name]. Intentionally open: an arbitrary
// fetched Kubernetes object whose fields the author dereferences.
#Required: {
	apiVersion: string
	kind:       string
	...
}

// #Requirement is one selector the module emits under out.requirements for
// Crossplane (or `cuefn render --required-resources`) to fetch. Exactly one of
// matchName or matchLabels must be set; this is enforced at render time by the
// engine's readRequirements (the embedded-disjunction form inside the closed
// struct is unverified against closedness and is deferred).
#Requirement: {
	apiVersion:   string
	kind:         string
	matchName?:   string
	matchLabels?: [string]: string
	namespace?:   string
}
```

`#Input` (closed) gains an **optional** pattern field — the form the prototype verified:

```cue
#Input: {
	// ...existing spec / metadata / environment...

	// requiredResources are the cluster objects Crossplane delivered for the
	// requirements this module emitted, keyed by requirement name. A populated
	// entry holds the matched objects; an empty list means "requested, none
	// found". Inside cuefn the engine seeds an empty list per declared
	// requirement so guards stay concrete (see Implementation).
	requiredResources?: [string]: [...#Required]
}
```

`#Transform` (closed) gains an **optional** field, so modules that emit no requirements still satisfy the definition:

```cue
#Transform: {
	// ...existing input / resources / status...
	requirements?: [string]: #Requirement
}
```

Notes verified against the prototype (see Appendix):

- Closed `#Input`/`#Transform` reject misspellings (`out.requirments`, `out.input.requiredResorces` → `field not allowed`) and accept the correct fields both empty and populated.
- Closedness holds via the pattern constraint. The optional field does **not** by itself guarantee per-key concreteness on the first pass — that is the engine seed (and, for raw out-of-engine evaluation only, the author `| *[]` default). A bare `requiredResources: {}` with the key absent still errors `undefined field: <name>`; an absent optional field errors `cannot reference optional field: requiredResources`.
- `#Required` is intentionally open (`...`).

### Authoring pattern (proven idiom)

A module declares what it needs under `out.requirements` and guards data-dependent `out.resources` on the matching `input.requiredResources[name]`. `out.requirements` must be a **pure function of stable inputs** (`spec`/`metadata`/`environment`), never of fetched data, so it is byte-identical every pass and the `proto.Equal` fixpoint converges.

Two equivalent guard idioms (both proven): `for i, x in input.requiredResources.<name>` to emit one resource per match, or `if len(input.requiredResources.<name>) > 0 { … [0] … }` to gate a single resource on the first match. The deciding factor is not the guard expression but that `input.requiredResources.<name>` exists as a *concrete list* — guaranteed by the engine seed (see Implementation).

**Namespace and cluster scope.** `#Input.metadata.namespace?` is optional and `#API` allows `scope: "Cluster"`. A selector that writes `namespace: input.metadata.namespace` becomes non-concrete for a cluster-scoped (namespaceless) XR, which fails `readRequirements`'s concreteness check and returns Fatal. Authors that must support both scopes default `metadata.namespace` and guard emission of the selector's `namespace`:

### Complete worked CUE example

```cue
import "github.com/meigma/crossplane-cuefn/contract@v0"

#Spec: {configName: string | *"app-cfg"}
#Status: {ready: bool}

out: contract.#Transform & {
	input: {
		spec: #Spec
		// Default namespace to "" so a cluster-scoped XR yields a concrete value
		// and the guard below omits the selector's namespace.
		metadata: {name: string, namespace: string | *""}
	}

	// Pure function of stable inputs -> byte-identical every pass -> converges.
	requirements: cfg: contract.#Requirement & {
		apiVersion: "v1"
		kind:       "ConfigMap"
		matchName:  input.spec.configName
		if input.metadata.namespace != "" {
			namespace: input.metadata.namespace
		}
	}

	resources: {
		// No-ops on the seeded [] (first pass -> out.resources is concrete {});
		// emits one Deployment per fetched ConfigMap on later passes.
		for i, cm in input.requiredResources.cfg {
			"deployment-\(i)": {
				ready: "Ready"
				object: {
					apiVersion: "apps/v1"
					kind:       "Deployment"
					metadata: name: "\(input.metadata.name)-\(i)"
					spec: image: cm.data.image
				}
			}
		}
	}
}
```

Optional aid for **raw** `cue export`/`cue vet` of a module *outside* cuefn (no engine to seed): add `input: requiredResources: cfg: [...contract.#Required] | *[]`. This is **not** load-bearing inside cuefn (the engine seed covers it) and is omitted from the canonical example and test fixtures to keep authoring boilerplate-free.

## Implementation

Three files change outside the contract: the pure core (`internal/render/engine.go`), the proto edge (`internal/function/function.go`), and the offline CLI (`internal/cli/render.go`, plus a new `internal/cli/required_resources.go`). `input/v1beta1.Input` is unchanged. (Line numbers below reference the analyzed tree at HEAD `69cc959`; re-anchor on current HEAD.)

### The first-pass seed (the one load-bearing mechanism, stated once)

On Crossplane's genuinely-first `RunFunction` call the request carries **no** required resources at all (the map is empty), not `{cfg: []}`. A data-dependent guard then references `input.requiredResources.cfg`, which does not exist — a hard CUE error (`cannot reference optional field: requiredResources`), not a recoverable "incomplete" — failing `Validate(cue.Concrete(true))` on `out.resources`. The fix: **the engine seeds an empty `[]` bucket for every requirement name read from `out.requirements`** before reading `out.resources`. The guard then collapses to a concrete empty struct, `out.resources` is a concrete `{}`, and validation passes. The same seed+refill ordering keeps a data-dependent `out.status` concrete, since `readStatus` runs after the seed. Seeding writes only `out.input.requiredResources`; it never perturbs `out.requirements`, so the fixpoint is unaffected. Everywhere else in this doc, "the seed" refers back to here.

### `internal/render/engine.go` — pure core (stays proto-free)

**`Inputs` struct (after `Environment`, ~line 51):**

```go
// RequiredResources holds the cluster objects Crossplane fetched for the
// requirements this module emitted on a previous pass, keyed by the author's
// requirement name. An empty list means "requested but none found". omitempty
// keeps it off out.input before any requirement is delivered.
RequiredResources map[string][]map[string]any `json:"requiredResources,omitempty"`
```

It surfaces at `out.input.requiredResources.<name>` automatically through the existing `fillInput` JSON-marshal + `FillPath("out.input")` (~line 145). `json.Marshal` of `map[string][]map[string]any` is fully supported; `omitempty` still emits `{"cfg": []}` (length-0 map value inside a non-empty map), which is the "requested, none found" signal.

**New `Requirement` type (near `rawResource`, ~line 156):**

```go
// Requirement is one entry of out.requirements: a selector the engine returns
// for Crossplane to fetch. Exactly one of MatchName/MatchLabels is set.
type Requirement struct {
	APIVersion  string            `json:"apiVersion"`
	Kind        string            `json:"kind"`
	MatchName   string            `json:"matchName,omitempty"`
	MatchLabels map[string]string `json:"matchLabels,omitempty"`
	Namespace   string            `json:"namespace,omitempty"`
}
```

**`Result` struct (after `Status`, ~line 73):**

```go
// Requirements holds the selectors the module emitted under out.requirements,
// keyed by requirement name. nil when the module declares none.
Requirements map[string]Requirement
```

**`Render` (~lines 109–124) — reorder to read requirements before resources, seed, re-fill only when seeding added keys:**

```go
v, err = fillInput(v.Context(), v, in)            // existing
if err != nil {
	return Result{}, err
}

requirements, err := readRequirements(v)          // NEW: pure fn of stable inputs
if err != nil {
	return Result{}, err
}

if seeded := seedRequiredResources(in.RequiredResources, requirements); seeded != nil {
	in.RequiredResources = seeded
	v, err = fillInput(v.Context(), v, in)        // re-fill only when seeding added keys
	if err != nil {
		return Result{}, err
	}
}

resources, err := readResources(v)                // existing
if err != nil {
	return Result{}, err
}

status, err := readStatus(v)                       // existing
if err != nil {
	return Result{}, err
}

return Result{Resources: resources, Status: status, Requirements: requirements}, nil
```

Re-calling `fillInput` is safe: `ProjectSpec` is idempotent and re-validating `cue.Concrete(true)` on `out.input` is harmless. No relaxation of `cue.Concrete(true)` anywhere.

**New `readRequirements` (next to `readStatus`, ~line 187) — absent-is-OK, and enforces "exactly one of matchName/matchLabels":**

```go
func readRequirements(v cue.Value) (map[string]Requirement, error) {
	req := v.LookupPath(cue.ParsePath("out.requirements"))
	if !req.Exists() {
		return nil, nil //nolint:nilnil // a module that needs nothing is valid.
	}
	if err := req.Validate(cue.Concrete(true)); err != nil {
		return nil, wrapCUE(err, "`requirements` did not fully evaluate")
	}
	var out map[string]Requirement
	if err := req.Decode(&out); err != nil {
		return nil, wrapCUE(err, "cannot decode `requirements`")
	}
	for name, r := range out {
		if (r.MatchName != "") == (len(r.MatchLabels) > 0) { // neither or both
			return nil, fmt.Errorf(
				"requirement %q must set exactly one of matchName or matchLabels", name)
		}
	}
	return out, nil
}
```

This is the single enforcement point for "exactly one"; both `setRequirements` (function adapter) and `matchRequirements` (CLI) may then trust it.

**New `seedRequiredResources` (pure map copy, no CUE):**

```go
func seedRequiredResources(existing map[string][]map[string]any, reqs map[string]Requirement) map[string][]map[string]any {
	var out map[string][]map[string]any
	for name := range reqs {
		if _, ok := existing[name]; ok {
			continue
		}
		if out == nil {
			out = make(map[string][]map[string]any, len(reqs))
			maps.Copy(out, existing)
		}
		out[name] = []map[string]any{} // non-nil empty list -> concrete cfg: []
	}
	return out
}
```

Returns `nil` when nothing was added, so the common no-requirements path does exactly one fill and never mutates the caller's map.

### `internal/function/function.go` — proto edge (all proto stays here)

Imports already include `fnv1`, `request`, `resource`, `maps` — no import changes.

**Read delivered required resources (in `RunFunction`, after `oxr` ~line 84, before the `inputs` literal ~line 92):**

```go
required, err := request.GetRequiredResources(req) // map[string][]resource.Required
if err != nil {
	response.Fatal(rsp, errors.Wrap(err, "cannot get required resources"))
	return rsp, nil
}
```

Then add to the `render.Inputs` literal:

```go
RequiredResources: requiredToInputs(required),
```

`GetRequiredResources` reads the current `required_resources` wire field, which Crossplane v2 / CLI v2.3.3 deliver. No `HasCapability` gate on the read path (older Crossplane simply delivers an empty map; the seed covers it).

**New `requiredToInputs` (proto/unstructured → plain maps, preserving the empty-but-present signal):**

```go
func requiredToInputs(in map[string][]resource.Required) map[string][]map[string]any {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string][]map[string]any, len(in))
	for name, items := range in {
		objs := make([]map[string]any, 0, len(items)) // keep empty (non-nil)
		for _, it := range items {
			if it.Resource != nil {
				objs = append(objs, it.Resource.Object)
			}
		}
		out[name] = objs
	}
	return out
}
```

**New `setRequirements` (near `setDesiredComposed`, ~line 129) — built directly; v0.7.1 has no setter (only `response.RequireSchema` for schemas). Uses the current `Resources` map (proto field 2), not the deprecated `ExtraResources`. `response.To` does not pre-populate `rsp.Requirements`, so the nil-guards are load-bearing:**

```go
func setRequirements(rsp *fnv1.RunFunctionResponse, result render.Result) {
	if len(result.Requirements) == 0 {
		return
	}
	if rsp.Requirements == nil {
		rsp.Requirements = &fnv1.Requirements{}
	}
	if rsp.Requirements.Resources == nil {
		rsp.Requirements.Resources = make(map[string]*fnv1.ResourceSelector, len(result.Requirements))
	}
	for name, r := range result.Requirements {
		sel := &fnv1.ResourceSelector{ApiVersion: r.APIVersion, Kind: r.Kind}
		if r.Namespace != "" {
			ns := r.Namespace
			sel.Namespace = &ns
		}
		switch {
		case r.MatchName != "":
			sel.Match = &fnv1.ResourceSelector_MatchName{MatchName: r.MatchName}
		case len(r.MatchLabels) > 0:
			sel.Match = &fnv1.ResourceSelector_MatchLabels{
				MatchLabels: &fnv1.MatchLabels{Labels: r.MatchLabels},
			}
		}
		rsp.Requirements.Resources[name] = sel
	}
}
```

(`readRequirements` guarantees exactly one match arm is taken, so a `Match`-less selector cannot be emitted.)

**Call site + capability warning (after `setCompositeStatus` ~line 116, before `response.Normalf` ~line 118):**

```go
if len(result.Requirements) > 0 &&
	!request.HasCapability(req, fnv1.Capability_CAPABILITY_REQUIRED_RESOURCES) {
	response.Warning(rsp, errors.New(
		"module emitted required-resource requirements but Crossplane does not advertise "+
			"CAPABILITY_REQUIRED_RESOURCES; they will be ignored"))
}
setRequirements(rsp, result)
```

`setRequirements` runs on **every** successful call (including the final stable one) so Crossplane's per-call `proto.Equal` comparison detects the fixpoint. Since requirements are a pure function of stable inputs, they are identical each pass → converges. Do **not** gate emission on "first pass"; the capability check controls only the diagnostic (emitting to a Crossplane that ignores them is harmless, but the Warning turns a silent empty render into a visible condition).

### `internal/cli/render.go` (+ new `internal/cli/required_resources.go`) — offline iteration

`--required-resources PATH` takes a flat bag of real K8s objects (file or dir) and matches each against the *selectors the function emits* (apiVersion + kind + matchName|matchLabels + namespace), grouping matches under the function's chosen requirement name — not filename-keying.

Because requirements are by design a pure function of stable inputs, the static-input case provably converges in exactly **two** passes; there is no need to re-implement Crossplane's bounded loop. Crossplane owns the real loop; the faithful end-to-end test drives the real `crossplane render` (see Testing plan). We do a fixed render → match → render and assert stabilization:

**`renderFlags` (~lines 15–20):** add `requiredResources string`.

**Flag registration (after ~line 50):**

```go
cmd.Flags().StringVar(&f.requiredResources, "required-resources", "",
	"path to a YAML file or directory of cluster objects matched against the "+
		"module's emitted requirements (mirrors crossplane render --required-resources)")
```

**`runRender` (~lines 57–74):** replace the single `Render` (~line 68):

```go
objs, err := loadRequiredObjects(f.requiredResources) // nil when flag unset
if err != nil {
	return err
}

result, err := render.New(loader).Render(ctx, ref, inputs)
if err != nil {
	return fmt.Errorf("cannot render module %q: %w", ref, err)
}

if len(objs) > 0 && len(result.Requirements) > 0 {
	inputs.RequiredResources = matchRequirements(objs, result.Requirements)
	second, err := render.New(loader).Render(ctx, ref, inputs)
	if err != nil {
		return fmt.Errorf("cannot render module %q: %w", ref, err)
	}
	// Pure requirements stabilize in one re-render; a mismatch means the
	// module's requirements depend on fetched data (the non-convergence
	// hazard). Surface it the way Crossplane does, instead of silently
	// printing a bogus render.
	if !reflect.DeepEqual(second.Requirements, result.Requirements) {
		return fmt.Errorf("requirements did not stabilize for module %q: "+
			"out.requirements must be a pure function of stable inputs", ref)
	}
	result = second
}
return printRenderResult(options, result)
```

**New helpers in `internal/cli/required_resources.go`:**

- `loadRequiredObjects(path) ([]map[string]any, error)` — file or directory walk, splitting multi-doc YAML. The existing `readYAMLObject` (~lines 112–122) reads a single object only, so this is a new multi-doc/dir reader.
- `matchRequirements(objs, reqs) map[string][]map[string]any` — for each requirement, filter `objs` by `apiVersion ==`, `kind ==`, `namespace` (when set) `== metadata.namespace`, and `matchName == metadata.name` **or** `matchLabels ⊆ metadata.labels`; **sort each bucket by namespace/name** for determinism. Always include the key with an empty slice when nothing matches, so the module sees `cfg: []`.

**`renderOutput` (~lines 131–136):** print emitted requirements so authors discover what to supply even with no `--required-resources`; `omitempty` keeps existing golden output unchanged for modules without requirements:

```go
type renderRequirement struct {
	APIVersion  string            `json:"apiVersion"`
	Kind        string            `json:"kind"`
	MatchName   string            `json:"matchName,omitempty"`
	MatchLabels map[string]string `json:"matchLabels,omitempty"`
	Namespace   string            `json:"namespace,omitempty"`
}

type renderOutput struct {
	Resources    map[string]renderResource    `json:"resources"`
	Requirements map[string]renderRequirement `json:"requirements,omitempty"`
	Status       map[string]any               `json:"status,omitempty"`
}
```

**`printRenderResult` (~lines 140–154):** populate `out.Requirements` from `result.Requirements`, omitting when empty.

## Contract module + versioning

The contract module (`github.com/meigma/crossplane-cuefn/contract`) is at `0.1.0`. The additions — a new optional `#Transform.requirements?`, an optional `#Input.requiredResources?` pattern field, and `#Required`/`#Requirement` — are purely additive, so modules pinned at `@v0` keep validating unchanged.

- **Bump: minor → `0.2.0`** (within v0: feature → minor; optional/additive fields are backward compatible).
- **Release path unchanged.** Land with a `feat(contract): …` Conventional Commit so release-please's `contract` component cuts the `contract/v0.2.0` tag and the OIDC workflow (`.github/workflows/release-contract.yml`) publishes it to the CUE Central Registry. The contract major stays welded to the function major (both v0); authors keep pinning `@v0`.
- **Backward-compat invariant (verified).** The field must exist in the contract *before* the engine ever fills it. This holds automatically: the engine only fills `out.input.requiredResources` for a module that emitted `out.requirements` (the field is `omitempty`), and a module that cannot reference the new contract fields cannot emit `requirements`. The prototype confirmed the reverse failure mode — filling `requiredResources` against the *old* closed `#Input` yields `field not allowed` — which is why the contract bump must precede any module opting in.
- **XRD codegen unaffected.** `internal/schema` reduces the module to definitions-only and drops `out`; `requirements`/`requiredResources` are runtime-only.

## Operations

### RBAC: the aggregate-to-crossplane ClusterRole

Required-resource reads go through Crossplane's **core controller** ServiceAccount, **not** the function pod. For every kind a module can request, the operator must grant the core controller `get`/`list`/`watch`. The portable way is a `ClusterRole` carrying the aggregation label, which Crossplane's aggregated `crossplane` ClusterRole absorbs:

```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: crossplane-cuefn-required-configmaps
  labels:
    rbac.crossplane.io/aggregate-to-crossplane: "true"
rules:
  - apiGroups: [""]
    resources: ["configmaps"]
    verbs: ["get", "list", "watch"]
```

This is an **operator responsibility, not a function responsibility** — `crossplane-cuefn` ships no RBAC for arbitrary requested kinds because it cannot know which kinds an author will request. Without the rule, the fetch silently returns nothing, the delivered bucket stays `[]`, and any guarded resource never renders (a silent under-render).

### Namespace read scope (UNVERIFIED — security note)

No enforced namespace boundary was found on the read path for namespaced XRs; the engine's namespace enforcement applies to composed/created resources only. **Treat as a security note to confirm against real cluster behavior** — it is not a blocker. Follow-up probe: a namespaced XR in namespace `a` emits a requirement selecting a ConfigMap in namespace `b`; observe whether it is delivered, then narrow docs/RBAC accordingly.

### Capability gating

`request.HasCapability(req, fnv1.Capability_CAPABILITY_REQUIRED_RESOURCES)` inspects the request meta's advertised capabilities. Gating is **diagnostic only**: emission is unconditional (harmless, and needed for the `proto.Equal` fixpoint); the gate controls a single non-fatal `response.Warning` when a module emits requirements but Crossplane does not advertise the capability (it will never iterate, so guarded resources would render empty forever). There is no capability gate on the read path.

### Optional v2 static bootstrap (deferred)

Crossplane v2 also supports declaring requirements statically on a Composition pipeline step via `apiextensions/v1 FunctionRequirements{ RequiredResources []RequiredResourceSelector{ RequirementName, APIVersion, Kind, Namespace*, Name*, MatchLabels } }`, pre-fetched before the first call. A future `cuefn publish` flag could emit this `step.requirements.requiredResources` block to save the first iteration; we defer it because the request-driven path is strictly more expressive (selectors can depend on `input.spec`) and keeps codegen/the XRD decoupled from runtime fetch semantics. No contract or engine change is needed to add it later.

## Testing plan

Repo layout: genuine unit tests stay in-package; integration/e2e live under `internal/test/{integration,e2e,common}`. e2e is a real kind cluster + Chainsaw behind the `e2e` build tag; integration is gated on `CUEFN_INTEGRATION`.

### Shared fixture (contract-resolution note)

Add a fixture module at `internal/test/common/testdata/required/` that **mirrors `internal/test/common/testdata/module/` exactly: self-contained and import-free** (`source: {kind: "self"}`, inlined `#API`/`#Spec`/`#Status`, a bare `out: {…}` struct — **it does not import the contract**). This matters for the rollout: the contract additions (PR1) are author-time-only (`cue vet`/editor); the render engine reads the runtime fields regardless and never unifies against the published contract. So PR2's render unit tests resolve no contract and are **independent of PR1's Central Registry publish latency** — exactly how `testdata/module/` works today. The fixture emits one requirement and guards a resource on it (using the namespace-default idiom so cluster-scope tests work):

```cue
package app

#API: {group: "platform.meigma.io", version: "v1alpha1", kind: "XApp", plural: "xapps", scope: *"Namespaced" | "Cluster"}
#Spec: {configName: string | *"app-cfg"}
#Status: {ready: bool}

out: {
	input: {
		spec: #Spec
		metadata: {name: string | *"app", namespace: string | *""}
		environment: {...}
	}
	requirements: cfg: {
		apiVersion: "v1"
		kind:       "ConfigMap"
		matchName:  input.spec.configName
		if input.metadata.namespace != "" {
			namespace: input.metadata.namespace
		}
	}
	resources: {
		for i, cm in input.requiredResources.cfg {
			"deployment-\(i)": {ready: "Ready", object: {
				apiVersion: "apps/v1"
				kind:       "Deployment"
				metadata: name: "\(input.metadata.name)-\(i)"
				spec: image: cm.data.image
			}}
		}
	}
}
```

Add a `HermeticRequiredModuleDir(t)` helper to `internal/test/common/paths.go` (sibling of `HermeticModuleDir`/`HermeticRenderloopDir`).

### Unit — render core (`internal/render/engine_test.go`, in-package `render_test`)

- `TestRenderEmitsRequirements` — render with no `Inputs.RequiredResources`; assert `res.Requirements["cfg"]` carries the expected `APIVersion`/`Kind`/`MatchName`/`Namespace`, and that `res.Resources` is concrete and **omits** the guarded Deployment. The load-bearing first-pass seed proof.
- `TestRenderRequiredResourcesSurfaced` — render with `{"cfg": [{…ConfigMap with data.image…}]}`; assert the Deployment appears and its image equals the fetched value.
- `TestRenderRequiredResourcesNotFound` — pass `{"cfg": []}`; assert the Deployment is omitted and the result is concrete (proves the explicit empty-list signal, behaviorally identical to the seed path).
- `TestRenderRequirementClusterScoped` — render the fixture with `Metadata.Namespace` unset (cluster-scoped); assert `res.Requirements["cfg"]` omits `Namespace` and `Render` does **not** return Fatal (proves the namespace-guard idiom).
- `TestRenderRequirementMatchValidation` — a fixture requirement that sets neither / both of `matchName`/`matchLabels`; assert `Render` returns the "exactly one" error.

### Unit — function edge (`internal/function/function_test.go`)

Add a second factory pointing at the required fixture (the existing `localFactory` is fixed to `testdata/module`). Use the existing `baseRequest`/`run` helpers.

- `TestRunFunction_EmitsRequirements` — advertise the capability (`req.Meta.Capabilities = []fnv1.Capability{fnv1.Capability_CAPABILITY_REQUIRED_RESOURCES}`) so no warning fires; assert `rsp.GetRequirements().GetResources()["cfg"]` has the expected `GetApiVersion()`/`GetKind()`/`GetMatchName()`/`GetNamespace()`.
- `TestRunFunction_EmitsMatchLabels` — fixture variant emitting a `matchLabels`-only requirement; assert `sel.GetMatchLabels().GetLabels()` round-trips (covers the other oneof arm flagged as a mis-mapping risk).
- `TestRunFunction_ReceivesRequiredResources` — deliver a fetched ConfigMap on the request and assert the desired Deployment uses its data:
  ```go
  req.RequiredResources = map[string]*fnv1.Resources{
      "cfg": {Items: []*fnv1.Resource{{Resource: resource.MustStructJSON(
          `{"apiVersion":"v1","kind":"ConfigMap","metadata":{"name":"app-cfg","namespace":"default"},"data":{"image":"img:9"}}`)}}},
  }
  ```
- `TestRunFunction_WarnsWithoutCapability` — module emits requirements, request does **not** set `Meta.Capabilities`; assert a non-fatal `Warning` is present **and** `rsp.GetRequirements().GetResources()["cfg"]` is still set (emission stays unconditional).
- `TestRunFunction_InvalidRequirement` — a fixture that sets neither/both match fields; assert the response is `Fatal` (the engine's "exactly one" error propagates).

### Integration — offline render loop (`CUEFN_INTEGRATION`)

New `internal/test/integration/required_resources_test.go`, sibling of `renderloop_test.go`. Drives the **real** `crossplane render` against our `cuefn function` server — this is where the multi-pass fixpoint loop is actually owned by Crossplane:

- Publish the required fixture to the throwaway registry (`reg.Publish`), serve the function (`common.ServeFunction`), write `functions.yaml` (`common.WriteFunctions`).
- New assets under `internal/test/common/testdata/requiredloop/` (composition.yaml, xr.yaml, configmap.yaml) modeled on `testdata/renderloop/`, plus a `HermeticRequiredloopDir(t)` helper.
- Invoke `crossplane render … --required-resources <dir>` where `<dir>` holds the matching ConfigMap; assert the rendered Deployment carries the ConfigMap's `data.image`. Use the current `--required-resources` flag (keep the existing renderloop test's deprecated `--extra-resources` for its EnvironmentConfig as-is). Give `--timeout 10m` for cold function-runtime container pulls.

### e2e — kind, fetches a real ConfigMap (heaviest)

The single `TestE2E_Kind` builds one cluster (~25 min); reuse it.

1. **RBAC (mandatory):** append the aggregate-to-crossplane `ClusterRole` for `configmaps` (above) to `functionInstallManifest` in `internal/test/e2e/e2e_test.go` (the `fmt.Appendf` block ~lines 236–277). The one piece without which the read silently returns nothing.
2. **Requirement-emitting module:** have the e2e fixture emit one requirement for an operator-supplied ConfigMap. Pick a `matchName` distinct from any composed resource name (the existing module composes a `ConfigMap` named `demo`) so the function does not read its own output. The selector stays a pure function of `input.spec`.
3. **New chainsaw test** `test/chainsaw/e2e/required-resources.yaml`, run via an added `runChainsaw(…)` step in `TestE2E_Kind`: `apply` a real ConfigMap (`app-cfg` in `default`, `data.image: img:e2e`); `apply` an XR whose `spec.configName` selects it; `assert` the composed Deployment reflects `spec.image: img:e2e` and the XR reaches `Ready=True` — proving the core controller fetched it via the aggregated RBAC and the second pass rendered the data-dependent resource. Existing `reconcile.yaml` assertions stay valid because the `demo` XR's requirement finds no matching ConfigMap (empty bucket, only the guarded field omitted).
4. **Namespace probe (follow-up, not a blocker):** once (1)–(3) pass, add the cross-namespace read probe from the security note.

## Risks & open questions

- **First-pass concreteness (highest).** Without the seed, a data-dependent guard references an absent field → hard CUE error → `Validate(cue.Concrete(true))` fails on `out.resources`. Closed by the engine seed (proven). The author `| *[]` default is only for raw out-of-engine evaluation; the contract pattern field does **not** by itself protect per-key concreteness.
- **Non-convergence (author discipline).** If an author lets `out.requirements` depend on fetched data, the requirements differ each pass, `proto.Equal` never matches, and Crossplane fails with `requirements didn't stabilize`. `cuefn render` mirrors this with its two-pass stabilization check; document the hazard loudly.
- **"Exactly one" of matchName/matchLabels** is enforced only at render time in `readRequirements` (the embedded-disjunction contract refinement is unverified against closedness and deferred). Open question: confirm the disjunction-inside-closed-struct form before tightening the contract.
- **RBAC.** Missing aggregate-to-crossplane rule → silent empty fetch → guarded resource never renders.
- **Capability.** Crossplane without `CAPABILITY_REQUIRED_RESOURCES` never iterates; mitigated by the non-fatal Warning.
- **SDK helper gap.** v0.7.1 has no requirement-resource setter; we build the `ResourceSelector` oneof and `Namespace *string` by hand, covered by both-oneof-arm unit tests.
- **Security (UNVERIFIED).** No enforced namespace boundary on the read path; confirm before claiming isolation (follow-up probe).
- **Error-path requirements.** If `out.resources` errors for a non-guard reason, `Render` returns the error and `Requirements` is dropped — acceptable, since a non-concrete `out.resources` is a Fatal the loop cannot recover from. Noted for review.

## Phased rollout (ordered PRs — each independently reviewable, human sign-off)

1. **`feat(contract)`: additive defs + minor bump.** Add `#Required`, `#Requirement`, optional `#Input.requiredResources?`, optional `#Transform.requirements?` to `contract/contract.cue`; cut `contract/v0.2.0` via release-please and publish via OIDC. Lands first so authors have the field before any module opts in.
2. **`feat(render)`: pure core.** Add `Inputs.RequiredResources`, the `Requirement` type, `Result.Requirements`, `readRequirements` (incl. exactly-one validation), `seedRequiredResources`; reorder `Render` to read requirements and seed before resources. Ship the import-free `testdata/required/` fixture. *Tests: see Testing plan (render unit).* Independent of PR1's publish (fixture imports no contract).
3. **`feat(function)`: edge adapter.** Read `request.GetRequiredResources` into `Inputs.RequiredResources` (`requiredToInputs`), map `Result.Requirements` onto `rsp.Requirements.Resources` (`setRequirements`, current `Resources` field, built directly), add the `HasCapability` Warning gate. *Tests: see Testing plan (function unit).*
4. **`feat(cli)`: offline iteration.** Add `--required-resources <file|dir>`, the new `internal/cli/required_resources.go` (multi-doc/dir loader + selector matcher with namespace/name sort), the fixed two-pass + stabilization check, and `renderOutput.Requirements`. *Tests: see Testing plan (integration).*
5. **`test(e2e)`: kind + RBAC.** Chainsaw `required-resources.yaml` fetching a real ConfigMap, wired into `TestE2E_Kind`, plus the mandatory aggregate-to-crossplane ClusterRole in the install manifest. Park the namespace-read-scope probe as a follow-up. *Tests: see Testing plan (e2e).*
6. **`docs`:** how-to (`require-resources.md`), explanation (`required-resources-fixpoint.md`), reference updates (module-contract, cli, input "unchanged" note), mkdocs nav wiring.

## Appendix: verified CUE prototype

All artifacts under `/private/tmp/claude-501/-Users-josh-code-meigma-crossplane-cuefn/26c9dcb6-a313-4122-84dd-1c0f1dad5c45/scratchpad/` (cue v0.16.1). The Go harness (`harness/`, `cuelang.org/go` v0.16.1) replicates the engine's exact `FillPath("out.input")` + `Validate(cue.Concrete(true))`; `rr-proof/` holds the contract + author modules + YAML fills; `rr-proof/old/` the old (no-field) contract for backward-compat.

### Verdict summary

| # | Claim | Verdict |
|---|-------|---------|
| 1 | MISSING pass (empty list) → `out.resources == {}` | **PASS** for `cfg: []`; "absent → default empty" is **FALSE** without an author default or engine seed |
| 2 | PRESENT pass → full concrete resources | **PASS** |
| 3 | Closedness rejects misspellings | **PASS** |
| 4 | Backward-compat (old closed `#Input`) | **PASS** (identical under real `FillPath`) |
| 5 | `len(x)>0` idiom | **PARTIAL** — the deciding factor is a *concrete list per key*, supplied by the engine seed (or per-key `| *[]`), not the guard expression |

### The exact additions that worked (optional form)

```cue
// #Input gains (optional):
requiredResources?: [string]: [...#Required]

// #Transform gains (optional):
requirements?: [string]: {
	apiVersion:   string
	kind:         string
	matchName?:   string
	matchLabels?: [string]: string
	namespace?:   string
}
```

### Robust author idiom (the proven snippet)

```cue
out: contract.#Transform & {
	input: {
		spec: #Spec
		metadata: {name: string | *"app", namespace?: string}
		environment: {tier: string | *"unset", ...}
		// Raw-eval aid only; inside cuefn the engine seed makes this non-load-bearing.
		requiredResources: cfg: [...{...}] | *[]
	}
	requirements: cfg: {apiVersion: "v1", kind: "ConfigMap", matchName: "my-config"}
	_name: input.metadata.name
	resources: {
		for i, cm in input.requiredResources.cfg {
			"deployment-\(i)": {
				ready: "Ready"
				object: {apiVersion: "apps/v1", kind: "Deployment",
				         metadata: name: "\(_name)-\(i)", spec: configTier: cm.data.tier}
			}
		}
	}
	status: #Status & {ready: true, url: "http://\(_name).svc"}
}
```

### Commands + outputs proving the two-pass concreteness (re-runnable)

**Pass 1 — MISSING (empty list) → `out.resources == {}`, requirements emitted:**

```
$ mise exec -- cue export ./contract.cue ./module.cue ./in_empty.yaml -e out.resources --out json
{}
$ mise exec -- cue export ./contract.cue ./module.cue ./in_empty.yaml -e out.requirements --out json
{ "cfg": { "apiVersion": "v1", "kind": "ConfigMap", "matchName": "my-config" } }
$ mise exec -- cue vet -c ./contract.cue ./module.cue ./in_empty.yaml   # exit 0
```
Engine-faithful (Go harness, `requiredResources.cfg=[]`):
```
FILLPATH out.input Validate(Concrete) OK
FILLPATH out.resources Validate(Concrete) OK -> {}
```

**Pass 2 — PRESENT → full concrete resource reading `cfg[0].data.tier`:**

```
$ mise exec -- cue export ./contract.cue ./module.cue ./in_present.yaml -e out.resources --out json
{ "deployment": { "object": { "apiVersion": "apps/v1", "kind": "Deployment",
  "metadata": {"name":"demo"}, "spec": {"configTier":"gold"} }, "ready": "Ready" } }
```
The harness (FillPath) produced the identical full concrete set.

**Closedness (claim 3):**

```
$ mise exec -- cue vet ./contract.cue ./module.cue ./bad_toplevel.cue ./in_empty.yaml
out.requirments: field not allowed:               # misspelled top-level requirements
$ mise exec -- cue vet ./contract.cue ./module.cue ./bad_input.cue ./in_empty.yaml
out.input.requiredResorces: field not allowed:    # misspelled requiredResources
```

**Backward-compat (claim 4) — old closed `#Input` with no field:**

```
# omitempty fill (no requiredResources) -> safe:
$ mise exec -- cue export ./old/contract_old.cue ./old/module_old.cue ./old/fill_safe.yaml -e out.resources --out json
{ "cm": { "object": { "apiVersion":"v1","kind":"ConfigMap","metadata":{"name":"demo"} } } }
# non-empty requiredResources fill -> closed-struct conflict:
$ mise exec -- cue vet ./old/contract_old.cue ./old/module_old.cue ./old/fill_rr.yaml
out.input.requiredResources: field not allowed:
```

**The deciding factor (claim 5) — isolation tests under the FillPath harness, ABSENT/empty-map fills:**

```
len()>0,   NO default, ABSENT             -> ERROR: cannot reference optional field: requiredResources
for-loop,  NO default, ABSENT             -> ERROR: cannot reference optional field: requiredResources
len()>0,   WITH default, ABSENT           -> OK -> {}
for-loop,  WITH default, ABSENT           -> OK -> {}
for-loop,  WITH default, PRESENT          -> OK -> {"deployment-0":{...,"configTier":"gold"}}
requiredResources:{} (key absent), NO default   -> ERROR: undefined field: cfg
requiredResources:{} (key absent), WITH default -> OK -> {}
```

Conclusion: both `len()>0` and `for` work **iff** the key is a concrete list. The engine seed (Implementation) supplies that for every declared requirement name; the per-key `| *[]` author default is the raw-evaluation complement.

### Surprises flagged

1. **FillPath vs `&` unification — no divergence in v0.16.1.** The Go `v.FillPath("out.input", json)` path produced byte-identical outcomes to CLI `&` unification for every scenario (closedness rejection, optional-field/non-concrete errors). So `cue vet`/`cue export` is a faithful stand-in for the engine's FillPath for these closedness/concreteness questions. Closedness is enforced because `out.input` already exists and is closed via `#Transform`; FillPath does not bypass it.
2. **Absent vs empty-list — design-critical.** With the optional `#Input.requiredResources?`: `cfg = []` → clean (`out.resources → {}`); field absent (omitempty) → `cannot reference optional field: requiredResources`; `requiredResources = {}` (key absent) → `undefined field: cfg`. These are definite errors, not "incomplete," so the guard cannot recover on its own — closed by the engine-fill seed and/or the author default.
