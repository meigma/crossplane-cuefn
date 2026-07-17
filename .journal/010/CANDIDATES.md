# `cuefn test` — candidate designs (session 010 research synthesis)

Four research agents ran 2026-07-17: ecosystem prior art, harness design
patterns, tests-in-CUE experiments (real cue v0.16.1 output), and a codebase
map. Full reports are in the session-010 conversation; the load-bearing facts
and the candidate designs they produced are below.

## Research digest (load-bearing facts)

- **The gap is real.** Upstream Crossplane declined to standardize composition
  testing (crossplane/crossplane#5710, closed not-planned — but it left a
  community-vetted directory + assertion shape: `tests/<name>/` with
  `N-xr.yaml`, `N-xr.assertions.yaml`, `observed.yaml`, `environment.yaml`).
  Upbound filled it with `up test` (CompositionTest, `assertResources`
  expected-object match). CUE itself reserves `*_test.cue` but has NO runner
  (cue-lang/cue#209 open since 2021, unimplemented); **`*_test.cue` files are
  silently ignored by all current CUE tooling** — do not use that naming.
- **Assertion models in the wild:** in-language boolean assert (KCL `kcl test`,
  terraform `assert`), declarative subset match on k8s objects (kuttl,
  chainsaw — the k8s-idiomatic choice, survives irrelevant churn), CEL
  predicates (crossplane-contrib/function-unit-test), expected-object match
  (Upbound), policy (conftest — wrong fit for "did my module render what I
  meant"), golden/snapshot (Pkl examples, insta, Go golden).
- **Pkl `pkl test` is the closest first-party analog:** same-language test
  modules with `facts` (boolean, power-assert output) + `examples` (golden,
  auto-written on first run, `--overwrite` to bless, JUnit output).
- **Golden-file law (every ecosystem):** ship `--update` from day one; never
  auto-bless in CI; redact/normalize unstable fields or regret it; forced
  review beats blind `-u` (Jest's stale-snapshot disease).
- **helm-unittest's cautionary flaw:** typo'd assertion paths silently match
  nothing and PASS. Schema-checked (CUE-authored) test cases kill this bug
  class.
- **Tests-in-CUE experiments (proven, cue v0.16.1):**
  - Unification-as-assertion WORKS and failure messages are genuinely good:
    path-qualified, all mismatches reported (not fail-fast), both values +
    both file:line cited. Partial match is the default (open structs);
    `close()` opts into exact; constraints (`=~`, bounds) assert cleanly.
  - Lists are positional; membership needs explicit `list.Contains`.
  - **Negative assertions are impossible in pure CUE** — bottom can't be
    caught as a boolean anymore (`x != _|_` idiom is broken). The harness
    (Go) must own "this render/validate SHOULD fail".
  - **`Subsume` is a trap** — cryptic messages ("field a not present" when it
    IS present with a conflicting value), mode-dependent optional-field
    semantics, `cue.Schema()` silently drops closedness. Use
    `expected.Unify(actual).Validate(cue.Concrete(true))` + `errors.Details`.
- **Codebase substrate is nearly perfect:** `render.New(loader).Render(ctx,
  ref, Inputs) (Result, error)` already returns everything assertable
  (`Resources` map keyed by author stable name with `.Object`/`.Ready`,
  `Status`, `Requirements`). Reusable as-is from `internal/cli`:
  `readYAMLObject`, `loadObservedObjects`, `loadRequiredObjects`,
  `matchRequirements` + the two-pass fixpoint in `runRender`
  (render.go:98-109), `moduleLoader` (generate.go:92), `schema.Validate`.
  Internal placement is no obstacle (same module). Hexagonal home: new
  **`internal/testharness`** core (mirrors `internal/schema`, depends only on
  render + cueerr) + a thin cobra `newTestCommand` in `internal/cli`.
  Our own `internal/render/readiness_test.go` transition matrices +
  `internal/test/common/testdata/readiness/` are the hand-rolled version of
  exactly what the harness should make declarative.

## What any candidate must cover (from the engine's real API)

Inputs per case: `spec`, `metadata{name, namespace?}` (absent namespace =
cluster-scoped), `environment`, required resources (flat bag matched via the
two-pass fixpoint, seed semantics: none found ⇒ concrete `[]`), observed
resources (stable-name-keyed, opt-in modules only). Assertables: resource key
set, deep object fields, readiness tri-state (Ready/NotReady/**Unspecified** —
must be distinguishable), status (incl. absent), emitted requirements,
render/validate ERRORS (cueerr-summarized substrings). Plus: discovery
convention, `--run` filter, fail-fast, machine output; readiness transition
sequences are the highest-value declarative feature.

## Candidate A — "CUE-native suite" (Pkl-shaped, tests are CUE)

Test suites are CUE files in `tests/` beside the module (NOT `_test.cue` —
inert). Each case declares inputs and a partial `want` for the rendered
result; the Go harness renders via the real engine and unifies
`want & actual` (Unify+Validate(Concrete)), so failure messages are CUE's
proven path-qualified conflicts. Negative cases via a declarative
`expectError: "substring"` field the harness interprets. Optional `facts:`
booleans for logic checks.

```cue
package tests

cases: "prod-env-overrides-tier": {
    input: {
        spec: {replicas: 3}
        metadata: {name: "demo", namespace: "default"}
        environment: {tier: "production"}
    }
    want: resources: workload: {
        ready: "Unspecified"
        object: spec: template: metadata: labels: tier: "production"
    }
}
cases: "rejects-replicas-over-max": {
    input: spec: {replicas: 99}
    expectError: "replicas"
}
```

- Pros: authors already write CUE (they're module authors by definition);
  full constraint power in expectations (`=~`, bounds, `close()` for exact);
  typo'd input fields can be schema-checked against a published test-harness
  contract def (kills the helm-unittest silent-typo class); best failure
  messages of any option, for free.
- Cons: input fixtures must be CUE (can't paste a real XR YAML verbatim;
  `cue import` mitigates); golden auto-update is awkward (a constraint block
  isn't a mechanical dump); readiness sequences need a list-of-steps schema.
- Opinionated surface: one blessed `#Suite`/`#Case` shape published in the
  contract module (`contract/test` package?) so `cue vet` checks test files.

## Candidate B — "Crossplane-native YAML" (the #5710 shape, chainsaw-style subset match)

Directory convention per case, all-YAML, mirroring `cuefn render`'s existing
flags 1:1; assertions are partial expected objects subset-matched in Go
(chainsaw semantics), plus typed assert blocks for readiness/status/
requirements/errors.

```
tests/prod-env/
  xr.yaml            # spec+metadata (same file render --xr takes)
  environment.yaml   # optional
  observed.yaml      # optional (Crossplane snapshot format)
  required.yaml      # optional flat bag
  assert.yaml        # expectations
```

```yaml
# assert.yaml
resources:
  workload:
    ready: Unspecified
    object:            # subset match — omitted fields ignored
      spec: {template: {metadata: {labels: {tier: production}}}}
status:
  workloadReady: false
expectError: null
```

- Pros: zero new syntax for k8s people; fixtures are real manifests
  (copy-paste from clusters); identical mental model to `crossplane render` +
  chainsaw the community already knows; the #5710 shape is pre-vetted;
  trivially seedable from `cuefn render` output.
- Cons: subset-match semantics (esp. lists) must be implemented + documented
  in Go — this is where kuttl went wrong and chainsaw spent its complexity
  budget; no schema safety on assertion paths (mitigate: fail loudly on
  assert keys that match no rendered resource, and validate assert.yaml
  against a published JSON schema); expectations can't express constraints
  (regex/bounds) without growing a predicate mini-language.

## Candidate C — "txtar case files" (one self-contained file per case; YAML inputs + CUE expectations)

The CUE project's own house style (testscript/txtar). One `.txtar` per case
under `tests/`; named sections carry the fixtures; the expectation section is
partial CUE unified by the harness (so failure quality = Candidate A) while
inputs stay neutral YAML (so fixture ergonomics = Candidate B).

```
-- xr.yaml --
apiVersion: example.meigma.io/v1
kind: App
metadata: {name: demo, namespace: default}
spec: {replicas: 3}
-- environment.yaml --
tier: production
-- want.cue --
resources: workload: object: spec: template: metadata: labels: tier: "production"
-- want.cue (exact variant would use close()) --
```

- Pros: one file per case (reviewable, self-contained, great in PRs); mixes
  the best of A and B; `--update` can mechanically rewrite a concrete
  `want.cue` (or a `want.yaml` golden section) from actual output; txtar
  parsing is a tiny dependency (`rogpeppe/go-internal/txtar`).
- Cons: txtar is unfamiliar outside Go/CUE circles; no editor syntax
  highlighting for embedded sections; two languages in one file; still needs
  the harness-side error/readiness-sequence schema.

## Candidate D — "Golden-first snapshots" (Pkl examples / insta loop)

Each case is just inputs (any fixture format); `cuefn test` renders and
compares the FULL normalized output against a committed
`testdata/<case>.golden.yaml`. First run auto-writes (✍️ like Pkl);
`cuefn test --update` re-blesses after review; CI never writes. Targeted
assert blocks are optional sugar on top later.

- Pros: near-zero authoring cost — write inputs, run once, commit; catches
  EVERY output change; trivial to implement over the existing deterministic
  YAML printer (`printRenderResult`); insta-style pending-file + git-diff
  review flow is well-understood.
- Cons: rubber-stamp risk (the Jest disease) — full-object goldens get
  blindly re-blessed; brittle for intentionally-evolving modules; can't
  express "I only care about this label"; readiness matrices become N whole
  goldens; needs normalization/redaction story even though our output is
  mostly deterministic.

## Cross-candidate notes

- All candidates share the same foundation: `internal/testharness` core over
  `render.Engine`, the reused cli loaders + two-pass fixpoint, discovery under
  `tests/`, `--run` regex, `--fail-fast`, deterministic summary output, and a
  first-class `expectError` (negative tests CANNOT live in pure CUE).
- A and C can offer golden-exact per case via `close()`+concrete dump; B and D
  can bolt on predicates later. Hybrids are cheap because the assertion
  evaluator is the same core either way.
- Recommendation lean (mine): **A or C**. The audience already writes CUE;
  CUE-authored expectations get the proven-good failure messages and schema
  safety for free, and the strongest counterargument to A (fixtures should be
  paste-able YAML) is exactly what C solves. B is the safest "meet the
  Crossplane community where it is" play; D is the lowest-cost MVP but the
  weakest opinionated story on its own — better as the golden MODE inside
  A/B/C than as the design.
