# DESIGN — `cuefn test`: a first-class test harness for cuefn modules

Status: **approved direction (Candidate C), pre-implementation** · Session 010, 2026-07-17
Decided by: developer, after the four-agent research pass (`.journal/010/CANDIDATES.md`)

## 1. Goal

A heavily opinionated `cuefn test` subcommand in the existing binary that lets
module authors validate a cuefn module's behavior against declarative test
cases — replacing ad-hoc bash scripts that wrap `cue` with boilerplate. One
blessed way to write tests; "go test for cuefn modules," not a general harness.

### Non-goals

- A generalized/configurable test framework (no pluggable matchers, no
  assertion DSL beyond what CUE itself provides).
- Cluster/e2e testing (Chainsaw owns that), policy testing (conftest exists),
  or testing arbitrary CUE (only cuefn modules — things with `#Spec` + `out`).
- An A-style CUE-suite authoring format in v1. The assertion core stays
  format-agnostic so it can be added later without rework.
- Contract module changes. C requires none; the section vocabulary is enforced
  by the harness.

## 2. Test case format

One case = one txtar file (`rogpeppe/go-internal/txtar`) under `tests/`,
non-recursive, discovered as `<module-dir>/tests/*.txtar`. **Case name = the
filename without `.txtar`.** The txtar comment block (text before the first
section marker) is the human description, echoed on failure.

### Section vocabulary (closed — unknown section names are a hard error)

| Section | Required | Mirrors | Content |
|---|---|---|---|
| `xr.yaml` | yes | `render --xr` | Full XR manifest: apiVersion/kind/metadata/spec. Spec is reserved-key-projected exactly as render does. Absent `metadata.namespace` = cluster-scoped. |
| `environment.yaml` | no | `render --env` | Merged EnvironmentConfig data (single doc). |
| `required.yaml` | no | `render --required-resources` | Flat multi-doc bag of cluster objects. Harness runs the same two-pass fixpoint + `DeepEqual` stabilization check as `runRender`; non-stabilizing modules fail the case with the same error. |
| `observed.yaml` | no | `render --observed-resources` | Observed composed resources, Crossplane snapshot format (keyed by `crossplane.io/composition-resource-name` annotation; same validation: missing/empty/duplicate names rejected). |
| `want.cue` | one-of† | — | Partial CUE expectation, unified against the normalized result. |
| `want.yaml` | one-of† | — | Exact golden of the FULL normalized result. Machine-maintained (seed/`--update`). |
| `error.txt` | one-of† | — | Negative test: each non-empty line must be a substring of the cueerr-summarized render error. |
| `N/observed.yaml`, `N/want.cue`, `N/want.yaml` | no | — | Step sections for readiness sequences (§5). |

† Expectation rules: at least one of `want.cue` / `want.yaml` / `error.txt`
(or step sections) — EXCEPT a case with none, which triggers seeding (§6).
`error.txt` is mutually exclusive with all `want.*` and with steps.
`want.cue` + `want.yaml` together is allowed (golden pins everything, CUE
documents intent).

### Hard-failure opinions (the harness is loud, never silent)

- Unknown section name → error naming the section and listing valid ones.
- `observed.yaml` (or step observed) against a module that has NOT opted into
  `out.input.observedResources` → error. Never a silent no-op.
- YAML that parses to a non-object, empty `xr.yaml`, or missing `spec` → error.
- A `want.cue` that references undefined fields or fails to compile → error
  with CUE detail (never "0 assertions matched, pass" — the helm-unittest flaw).

## 3. Normalized result document

`render.Result` is normalized into ONE concrete document before any assertion:

```cue
{
	resources: [name]: {
		ready:  "Ready" | "NotReady" | "Unspecified"   // resource.Ready enum → string
		object: {...}                                   // full rendered object
	}
	status:       {...} | null   // explicit null when the module returns none
	requirements: [name]: {apiVersion: string, kind: string, matchName?: string,
	                       matchLabels?: {[string]: string}, namespace?: string}
	// requirements always present; {} when none emitted
}
```

Absences are explicit (`status: null`, `requirements: {}`) because open-struct
unification cannot assert a missing field, but it CAN assert `status: null`.
This same document, marshaled with the existing deterministic YAML printer, is
the golden (`want.yaml`) content — one normalization, two consumers.

## 4. Assertion semantics

- **`want.cue`** — compiled in an isolated context, then
  `want.Unify(actual).Validate(cue.Concrete(true))`. NOT `Subsume` (proven
  trap: cryptic messages, mode-dependent optional-field semantics). Errors are
  rendered via `errors.Details`/`cueerr` — path-qualified, all mismatches at
  once, both values shown. Semantics that fall out of CUE and are documented
  as-is, not papered over:
  - Partial by default (open structs — omitted fields ignored).
  - `close()` opts a struct into exact matching.
  - Constraints are legal expectations (`>=1 & <=10`, `=~"^ghcr.io/"`).
  - Lists are positional; membership via explicit `list.Contains`.
  - Compile with filename `<case>.txtar/want.cue` and newline-pad so reported
    line numbers match the txtar file's coordinates.
- **`want.yaml`** — byte-exact compare against the normalized document's
  deterministic YAML (newline-normalized for Windows); mismatch prints a
  unified diff.
- **`error.txt`** — render must fail; each non-empty line must appear in the
  summarized error string. If render succeeds, the case fails with the
  rendered resource key set shown.

## 5. Readiness sequences (steps)

Numbered step directories inside one case share the base fixtures and run in
order against the same module + XR:

```
-- xr.yaml --            (shared)
-- environment.yaml --   (shared, optional)
-- required.yaml --      (shared, optional)
-- 1/observed.yaml --
-- 1/want.cue --
-- 2/observed.yaml --
-- 2/want.yaml --
```

Rules: steps must be contiguous from `1`; each step needs `observed.yaml` and
at least one `want.*`; base-level `observed.yaml`/`want.*` are mutually
exclusive with steps; v1 steps vary ONLY observed+want (no per-step spec/env
overrides). This is the declarative twin of the hand-rolled readiness
transition matrices in `internal/render/readiness_test.go`. Ships in v1 — it
is the single highest-value feature.

## 6. Seeding and update workflow

Pkl's loop with insta's discipline:

- **Seed:** a case with no expectation sections renders and APPENDS a
  `want.yaml` section (per step when steps present), reports `✍ seeded`, and
  the run exits non-zero — review before commit.
- **Update:** `cuefn test --update` re-renders and rewrites ONLY drifted
  `want.yaml` sections in place (txtar parse → replace section → format →
  write). It NEVER touches `want.cue` or `error.txt` — hand-written intent is
  never machine-blessed.
- **CI:** when `CI` env var is truthy or `--ci` is set, seeding and `--update`
  are refused; any would-seed case or golden drift is a plain failure.
- No redaction machinery in v1 — engine output is deterministic. Revisit only
  if a real instability appears.

## 7. CLI specification

```
cuefn test [--dir .] [--run <regex>] [--update] [--fail-fast] [--ci]
           [--cache-dir <dir>] [-v]
```

- `--dir` — module directory (default `.`); loader via the existing
  `moduleLoader(dir, cacheDir)` (dependency-aware LocalLoader; `CUE_REGISTRY`
  honored). Testing a published module by OCI ref: deferred fast-follow (falls
  out of `moduleLoader` nearly free).
- `--run` — regex over case names (and `case/step` for steps).
- Output: one line per case (`PASS` / `FAIL` / `✍ SEEDED`), failures followed
  by their diffs/errors, then a summary (`N passed, N failed, N seeded`).
  Human output → `options.Err`, summary/results → `options.Out` per house
  convention; exit non-zero on any failure or seed. Machine-readable
  (`--output json`) and JUnit: deferred.
- House style: `newTestCommand(options Options)` + `testFlags`,
  `SilenceUsage/SilenceErrors`, `RunE` → `runTest`. Registered
  unconditionally in `root.go` — needs only `internal/render`, so NO `noxpkg`
  seam work and no runtime-image bloat.

## 8. Architecture

```
internal/snapshot     NEW (extraction) — bytes-based cores + path wrappers:
                      observed loader, required loader/readYAMLObjects,
                      matchRequirements, fixpoint helper. cli + testharness
                      both consume it. No behavior change.
internal/testharness  NEW core (mirrors internal/schema's position):
                      txtar Case model + validation, input assembly →
                      render.Inputs, result normalization, the three
                      evaluators, seed/update writer, runner.
                      Depends ONLY on render, snapshot, cueerr, txtar.
internal/cli/test.go  NEW thin adapter: flags, discovery, moduleLoader
                      wiring, output formatting.
```

The core never imports cobra; the CLI never implements matching. An A-style
suite format later = a second Case source feeding the same core.

New dependency: `github.com/rogpeppe/go-internal/txtar` (tiny, stable).

## 9. Proof and gates

- Unit tests (blocking `root:test`): parse/validate matrix, normalization,
  each evaluator incl. failure-message content, seed/update rewrite
  idempotence — against `internal/test/common/testdata/module` and small
  purpose-built fixtures. CLI-level tests drive the real command.
- **Dogfood acceptance:** a real `tests/` for `example/module`, including a
  readiness-transition steps case mirroring `readiness_test.go` scenarios
  (Go tests stay — they pin the ENGINE; txtar twins prove the HARNESS).
  New blocking moon task `example-test` beside `example-check` in `check`.
- Functional verification before "done" (house rule): run the real binary
  against the example module, exercise seed → fail → `--update` → pass, and
  a deliberate mismatch to inspect the failure output quality.
- No Docker/cluster/integration suites needed — fully offline.

## 10. Documentation (strict-gated)

- How-to: "Test a module" (authoring walkthrough: first case, seed, golden
  vs want.cue, negative test, readiness steps, CI wiring).
- Reference: `cuefn test` command + the full section vocabulary table +
  assertion semantics (partial/close()/lists/constraints).
- Update: CLI reference (currently states exactly six commands), README
  command table, quickstart pointer.

## 11. Open questions (resolve during PR 2/3)

1. **`cue mod publish` packaging** — does `tests/*.txtar` inside the module
   root ship in published modules? Verify empirically. If yes: document as
   harmless OR bless `tests/` outside the module root. (Research flagged,
   unverified.)
2. **CI detection precedence** — `CI` env truthiness rules vs `--ci`/`--no-ci`
   override shape.
3. **Failure-diff polish for `want.yaml`** — unified diff library choice or
   hand-rolled line diff.
4. **`--run` matching for steps** — `case` matches all its steps vs
   `case/N` addressing; pick during implementation.

## 12. PR arc

1. `refactor(cli): extract snapshot loading into internal/snapshot` —
   behavior-preserving move + bytes cores. Existing tests carry.
2. `feat(testharness): txtar case model and assertion core` — §2–§6 core +
   unit tests (no CLI yet).
3. `feat(cli): add cuefn test command` — §7 command, discovery, seed/update,
   example-module `tests/`, `example-test` moon task.
4. `docs: document cuefn test` — §10.

Each PR: Conventional-Commit title, squash-merge via GitHub, blocking
`moon run root:check` + real `ci` Action green at every gate.

## 13. Key research facts this design leans on

Full digest: `.journal/010/CANDIDATES.md`. Load-bearing: `*_test.cue` is
inert in current CUE tooling (cue#209 open, unimplemented — hence `tests/`
dir, normal filenames); `Subsume` rejected for assertions (proven bad
messages); negative tests impossible in pure CUE (bottom uncatchable — hence
`error.txt` owned by the harness); crossplane#5710 closed not-planned (the
gap is real; its directory shape inspired the section vocabulary); Pkl
`pkl test` seed/overwrite loop and chainsaw subset-match are the models;
never-auto-bless-in-CI is the universal golden-file lesson.
