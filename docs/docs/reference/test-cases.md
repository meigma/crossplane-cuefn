# Test case format

The authoritative specification of the `cuefn test` case format: the txtar
section vocabulary, the input semantics, the normalized result document that
expectations are evaluated against, the matching rules, and the golden
lifecycle. For a task-oriented introduction, see
[How to test a module](../how-to/test-a-module.md); for the command's flags
and exit behavior, see the [CLI reference](cli.md#test).

## Files and discovery

- A test case is one [txtar](https://pkg.go.dev/golang.org/x/tools/cmd/txtar#hdr-Txtar_format)
  file: free comment text, then named sections introduced by `-- name --`
  marker lines.
- Cases live in the module directory's `tests/` subdirectory and are
  discovered as `tests/*.txtar`, non-recursively, and run in filename order.
- The **case name** is the filename without its `.txtar` extension.
- The comment block before the first section is the case's **description**,
  echoed when the case fails.
- An empty `tests/` directory (or a `--run` pattern matching no case) is an
  error, not a silent pass.

## Section vocabulary

The vocabulary is **closed**: a section name outside this table (or a
misordered variant such as `observed.yml`) fails the case's parse. Sections
may appear in any order; duplicate names are errors.

| Section | Required | Role |
|---------|----------|------|
| `xr.yaml` | yes | The observed XR manifest. |
| `environment.yaml` | no | Merged `EnvironmentConfig` data. |
| `required.yaml` | no | Flat bag of cluster objects for required-resource matching. |
| `observed.yaml` | no† | Observed composed-resource snapshot (base case). |
| `want.cue` | ‡ | Partial CUE expectation, unified with the result. |
| `want.yaml` | ‡ | Exact golden of the full normalized result (machine-owned). |
| `error.txt` | ‡ | Declares the render must fail; lines are required substrings. |
| `N/observed.yaml`, `N/want.cue`, `N/want.yaml` | no† | Step sections; see [Steps](#steps). |

† Base `observed.yaml` and step sections are mutually exclusive.
‡ Expectation rules below.

### Expectation rules

- A case (or step) may combine `want.cue` and `want.yaml` — the golden pins
  everything while the CUE documents intent.
- `error.txt` is mutually exclusive with `want.cue`, `want.yaml`, and steps,
  and must contain at least one non-empty line.
- A case or step with **no** expectation sections triggers
  [seeding](#golden-lifecycle).

## Input semantics

Each input section carries exactly the content of the corresponding
`cuefn render` flag's file, decoded with the same code.

**`xr.yaml`** — a single YAML document. `spec` must be present as an object
(possibly empty); Crossplane-reserved keys are projected out of it exactly as
the runtime does. `metadata.name` and `metadata.namespace` are read
defensively; an absent namespace means cluster-scoped semantics (a module's
namespace-guarded requirement selectors omit `namespace`).

**`environment.yaml`** — a single YAML document; its top-level keys become
`out.input.environment`.

**`required.yaml`** — a multi-document stream (documents split only at a
column-0 `---`, so embedded YAML inside values is safe). The objects form a
flat bag matched against the module's emitted `out.requirements` by
apiVersion, kind, optional namespace, and `matchName` or `matchLabels`
(subset), with each matched bucket sorted by `namespace/name`. The harness
runs the same two-pass loop as `cuefn render`: render to discover selectors,
match, re-render with matches delivered, and fail if the emitted requirements
do not stabilize. With no `required.yaml`, declared requirements receive the
engine's seeded empty bucket — resources guarded on delivered objects are
omitted, and the emitted selectors are still assertable under `requirements`.

**`observed.yaml`** — a multi-document stream of raw composed objects, each
required to carry a non-empty `crossplane.io/composition-resource-name`
annotation; that value is the stable map key (duplicates are errors). Valid
only for modules that declare `out.input.observedResources` as a regular
field — supplying observed snapshots to a module that never opted in is an
error, not a no-op.

## The normalized result document

Every expectation is evaluated against one concrete document derived from the
render:

```cue
{
	resources: [name]: {
		ready:  "Ready" | "NotReady" | "Unspecified"
		object: {...}
	}
	status:       {...} | null
	requirements: [name]: {
		apiVersion:   string
		kind:         string
		matchName?:   string
		matchLabels?: {[string]: string}
		namespace?:   string
	}
}
```

- `resources` is keyed by the module author's stable resource names.
- `ready` uses the module contract's author vocabulary: a module `ready:
  "Ready"` hint maps to `"Ready"`, `"NotReady"` to `"NotReady"`, and an
  absent hint to `"Unspecified"` — the three states are distinguishable.
- Absences are explicit so they are assertable: a module returning no status
  yields `status: null`; a module emitting no requirements yields
  `requirements: {}` (always present). Empty selector fields (`matchName`,
  `matchLabels`, `namespace`) are omitted from requirement entries.
- Serialized as YAML (for `want.yaml` goldens and seeding), the document is
  deterministic: keys sort lexically.

## `want.cue` matching semantics

The section is compiled as standalone CUE (no package clause) and **unified**
with the normalized document; the unification must be conflict-free and fully
concrete. Consequences:

- **Partial by default.** Open structs ignore every field the expectation
  omits.
- **`close()` opts into exactness** for any struct: unexpected fields inside a
  closed struct are conflicts (`close({})` asserts emptiness).
- **Constraints are valid expectations**: bounds (`>=1 & <=10`), patterns
  (`=~"^ghcr\\.io/"`), enums, and disjunctions all work; a satisfied
  constraint unifies to the concrete actual value.
- **Asserting a field the render did not produce fails** (the unification is
  not concrete there), so misspelled assertion paths cannot silently pass.
- **Lists are positional.** An expectation list must match element-for-element
  and length-for-length; use `list.Contains` (or a comprehension) explicitly
  for membership-style assertions.
- **Failures report every mismatch at once**, path-qualified with both values,
  and cite positions in the txtar file's own coordinates
  (`<case>.txtar/want.cue:<line>:<col>`).

## `want.yaml` matching semantics

Compared byte-for-byte against the normalized document's YAML, after
normalizing line endings (`\r\n` → `\n`) and trailing newlines. A mismatch
fails with a full line diff (`- ` expected, `+ ` rendered). `want.yaml`
content is machine-owned — written by seeding and `--update`, reviewed by
humans in the git diff.

## `error.txt` semantics

The render (including `#Spec` unification) must fail, and every non-empty,
whitespace-trimmed line of the section must appear as a substring of the
error message. A succeeding render fails the case and reports the rendered
resource names. Error messages are the CLI's summarized CUE errors — match on
stable fragments such as field names, not whole messages.

## Steps

Step sections replay a readiness sequence: the same module, XR, environment,
and required objects rendered against successive observed snapshots.

- Steps are numbered directories: `1/observed.yaml`, `1/want.cue`,
  `1/want.yaml`, `2/...`. Numbering must be contiguous from `1`.
- Every step requires an `observed.yaml`; expectations follow the
  [expectation rules](#expectation-rules) per step (`error.txt` is not
  available in steps).
- When any step section is present, the base case may carry only shared
  fixtures — base `observed.yaml`, `want.*`, and `error.txt` are rejected.
- Steps run in order; each is rendered and evaluated independently (state does
  not carry between steps — the observed snapshot is the state).

## Golden lifecycle

- **Seed:** a case or step with no expectations is rendered and its
  normalized output is written into a `want.yaml` section (placed after the
  step's `observed.yaml` when present). The run reports `SEED` and exits
  non-zero so the golden is reviewed before it is committed.
- **Update:** `cuefn test --update` rewrites only *drifted* `want.yaml`
  sections and re-runs the case, so remaining `want.cue`/`error.txt` failures
  still fail. Hand-written sections are never rewritten.
- **CI:** with `--ci`, or whenever the `CI` environment variable is set to
  anything but empty/`0`/`false`, seeding and `--update` are refused — a
  missing or drifted golden always fails the build. Goldens change only
  through a locally reviewed commit.

## Packaging

`cue mod publish` packages every file in the module directory, so
`tests/*.txtar` ships inside the published module. This is intended — a
consumer can run a pulled module's own tests — at the cost of module size;
keep fixture snapshots trimmed to the fields the assertions need.
