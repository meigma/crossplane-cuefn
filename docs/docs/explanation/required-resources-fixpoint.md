# Required resources and the fixpoint

A composition function is sometimes not a function of the XR alone: it needs to
read an existing cluster object to render correctly. cuefn supports this without
giving up the property that makes the rest of the system predictable — the render
core stays a **pure function** of its inputs. This page explains how that works
and why it converges.

For the authoring steps, see
[how to require cluster resources](../how-to/require-resources.md); for the field
definitions, see
[the module contract](../reference/module-contract.md#requiring-cluster-resources).

## A symmetric, author-keyed pair

The mechanism is two maps, both keyed by a name the author picks:

- **`out.requirements`** — selectors the module *emits* for Crossplane to fetch.
- **`out.input.requiredResources`** — the fetched objects the engine *fills*,
  keyed by the same name.

A module asks for `cfg` and reads back `input.requiredResources.cfg`. Nothing
about the request or the wire protocol leaks into the module; it sees only
ordinary CUE values, exactly as it does for spec, metadata, and environment.

Required resources are for **external selected objects**. A module does not need
to request children it already composes merely to inspect their status: those
arrive on every function request through the separate, non-iterative
`out.input.observedResources` path. Use requirements when the module must ask
Crossplane to fetch something outside its composed set; use observed resources
for self-observation and readiness.

## Crossplane owns the loop

The function never fetches anything itself. It is a pure function of
`(observed XR + delivered required resources + input + context) → (desired +
requirements)`, and **Crossplane owns the iteration**:

- Each pass, the function returns its desired state **and** its requirements.
  Crossplane fetches the requested objects and re-invokes the function, carrying
  the results forward.
- The loop stops at a **fixpoint**: when two consecutive responses emit
  byte-identical requirements (`proto.Equal`). Crossplane bounds the loop (up to
  a handful of iterations) and errors with `requirements didn't stabilize` if it
  never settles.
- **Only the final, stable response's desired state is applied.** Intermediate
  passes exist only to drive fetches; their composed resources are discarded.
- **Missing means empty, not absent.** A requested object that is not found comes
  back with its map key present and an empty list — the "requested, none found"
  signal, distinct from "never requested".

Because Crossplane discards non-final desired state, a module can safely render
"nothing yet" on the first pass and the real resources once its data arrives.

## The first-pass seed

There is one subtlety. On Crossplane's genuinely-first call the request carries
no required resources at all — the map is empty, not `{cfg: []}`. A guard that
references `input.requiredResources.cfg` would then reference a field that does
not exist, a hard CUE error that fails the concreteness check on `out.resources`.

cuefn closes this engine-side. Before it reads `out.resources`, the engine reads
the emitted `out.requirements` and **seeds an empty list for every requirement
name** that was not already delivered. The guard then collapses to a concrete
empty result, `out.resources` evaluates to a concrete `{}`, and the render
succeeds. The seed touches only `out.input.requiredResources`; it never perturbs
`out.requirements`, so the fixpoint comparison is unaffected. This is why a `for`
comprehension or an `if len(...) > 0` guard "just works" on the first pass with
no author boilerplate — the deciding factor is the concrete seeded list, not the
guard expression.

## Why requirements must be pure of stable inputs

The fixpoint converges only if the requirements a module emits are **a pure
function of stable inputs** — `spec`, `metadata`, `environment` — and never of
the fetched data. If a module emits a new requirement once its first one is
delivered, the requirement set differs every pass, `proto.Equal` never matches,
and Crossplane fails with `requirements didn't stabilize`.

This is an author discipline, not something the engine can paper over, so the
tooling surfaces it loudly. `cuefn render --required-resources` does a fixed
render → match → re-render and asserts that the second pass emits the same
requirements as the first; if not, it fails with a stabilization error rather
than printing a bogus render. That offline check mirrors exactly what Crossplane
would do in-cluster.

## Reads and RBAC {#reads-and-rbac}

Required resources are fetched by Crossplane's **core controller** ServiceAccount,
not by the cuefn function pod. The function only declares *what* it wants; the
controller does the reading. That has a direct operational consequence: for every
kind a module can request, the operator must grant the core controller
`get`/`list`/`watch`, typically via a `ClusterRole` labelled
`rbac.crossplane.io/aggregate-to-crossplane: "true"` (see
[the how-to](../how-to/require-resources.md#4-grant-the-in-cluster-rbac)).

cuefn ships no such RBAC, because it cannot know which kinds an author will
request. Without the grant the fetch silently returns nothing, the delivered
bucket stays empty, and any guarded resource never renders — a silent
under-render. Read scope follows the selector: a selector with a `namespace`
reads that namespace; an omitted `namespace` reads a cluster-scoped kind or lists
across all namespaces. Cross-namespace reads are an intentional, RBAC-gated
property of the upstream feature, not a cuefn-specific boundary.

## Capability gating

Crossplane advertises `CAPABILITY_REQUIRED_RESOURCES` when it supports this loop.
A function emits its requirements on **every** successful call — including the
final stable one — so Crossplane's per-call `proto.Equal` comparison can detect
the fixpoint. Emission is therefore unconditional.

The capability check is **diagnostic only**: when a module emits requirements but
the connected Crossplane does not advertise the capability, it will never iterate
and a module that hides resources behind a requirement guard would render empty
forever. cuefn turns that silent failure into a visible, non-fatal warning on the
response, while still emitting the requirements (which a non-capable Crossplane
harmlessly ignores).

## The v2 naming

Crossplane v1 called this "extra resources" (`extra_resources` on the wire,
`ExtraResources` in the SDK). Crossplane v2 renamed it to **required resources**,
and cuefn speaks only the current wire fields — `required_resources` on the
request and `Requirements.Resources` on the response. The contract field names
(`requiredResources`, `requirements`) follow the v2 vocabulary.

## See also

- [How to require cluster resources](../how-to/require-resources.md) — the
  authoring steps and the RBAC YAML.
- [Module contract: requiring cluster resources](../reference/module-contract.md#requiring-cluster-resources)
  — the exact contract fields.
- [One module, two outputs](one-module-two-outputs.md) — the purity principle
  this feature preserves.
- [Derive readiness from observed resources](../how-to/derive-readiness-from-observed-resources.md)
  — the non-iterative self-observation path.
