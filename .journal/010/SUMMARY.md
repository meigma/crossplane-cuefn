---
id: 010
title: Design and ship the first-class `cuefn test` harness (product v0.1.6)
date: 2026-07-17
status: complete
repos_touched: [crossplane-cuefn]
related_sessions: [009, 008, 005]
---

## Goal

Give cuefn module authors a first-class, heavily opinionated `cuefn test`
command inside the existing binary — replacing giant bash scripts wrapping
`cue` — starting from research agents proposing candidate designs, through a
developer-chosen design, to a shipped and released implementation.

## Outcome

**Fully met, through release.** Four research agents produced four candidate
designs (`.journal/010/CANDIDATES.md`); the developer chose **Candidate C**
(one txtar file per case, YAML input sections mirroring the render flags, CUE
expectations); the approved design (`.journal/010/DESIGN.md`) shipped as four
squash-merged PRs (#69 snapshot extraction, #70 testharness core, #72 the CLI
command + dogfood, #73 docs incl. a test-contract reference); then the
developer approved cutting **product v0.1.6** (#71): tag-push release run all
green, checksums/attestations/cosign/Function-entrypoint verified before
publication, brew/scoop distribution verified, and the publicly installed
release binary ran the example module's own test suite 4/4 PASS.

## Key Decisions

- **Candidate C over A (CUE-native suites)** → one self-contained `.txtar`
  per case keeps fixtures paste-able YAML while `want.cue` expectations keep
  CUE's proven path-qualified conflict messages; the assertion core is
  format-agnostic so an A-style suite format could be added later.
- **`Unify().Validate(Concrete)` for assertions, never `Subsume`** →
  experiments showed Subsume's messages are cryptic ("field not present" for
  value conflicts) with mode-dependent optional-field semantics.
- **Negative tests are harness-owned (`error.txt`)** → modern CUE cannot
  catch bottom as a boolean; pure-CUE negative assertions are impossible.
- **`tests/` dir + normal filenames, not `*_test.cue`** → `*_test.cue` is
  reserved but silently ignored by all current CUE tooling (cue#209 open,
  unimplemented).
- **Golden lifecycle: seed on first run, `--update` re-blesses only
  machine-owned `want.yaml`, CI refuses both** → the universal snapshot-
  ecosystem lesson (never auto-bless in CI; never rewrite hand-written intent).
- **Absences normalized explicitly** (`status: null`, `requirements: {}`) →
  open-struct unification cannot assert a missing field but can assert null.
- **`tests/*.txtar` ships in published modules** → verified empirically that
  modzip packages everything; documented as intended (consumers can run a
  pulled module's tests) with a size caveat.
- **Observed fixtures against non-opted-in modules are hard errors** → new
  exported `render.UsesObservedResources`; the harness never silently no-ops.
- **Release verified before publication, then published to trigger
  distribution** → per session-008/009 rules (tag-push provenance, checksum/
  attestation/cosign/entrypoint checks, then brew/install.sh consumer proof).

## Changes

- `internal/snapshot/` (#69) — behavior-preserving extraction of the YAML
  fixture loaders, `MatchRequirements`, and the two-pass fixpoint
  (`RenderWithRequiredObjects`) out of `internal/cli`, with bytes-based
  `Parse*` cores.
- `internal/testharness/` (#70) — txtar case model (closed section
  vocabulary, step grouping), result normalization, the three evaluators
  (want.cue with txtar-coordinate positions, want.yaml with LCS line diff,
  error.txt substrings), seed/update rewriters, runner.
- `internal/render/engine.go` (#70) — exported `UsesObservedResources`.
- `internal/cli/test.go` + `root.go` (#72) — the `cuefn test` command
  (`--dir/--run/--update/--fail-fast/--ci/--cache-dir`), PASS/FAIL/SEED/
  UPDATE reporting, non-zero exit on failure or fresh seed.
- `example/module/tests/*.txtar` + `moon.yml` (#72) — four dogfood cases
  (the full-render golden seeded by the real binary) and the blocking
  `root:example-test` gate.
- `docs/` + `README.md` (#73) — how-to `test-a-module.md`, reference
  `test-cases.md` (the authoritative format contract, sibling of
  module-contract.md), CLI reference `test` section (six → seven commands),
  README row, nav/quickstart pointers.
- `flake.nix` (#70) — vendorHash bump after go-internal became a direct dep.
- Release v0.1.6 (#71) — published; brew/scoop taps updated and verified.

## Open Threads

- Deferred by design (fast-follows, not bugs): testing a published module by
  OCI ref (falls out of `moduleLoader` nearly free), `--output json`/JUnit
  reporting, `--run` step-level addressing (`case/N`), an A-style CUE-suite
  authoring format on the same core.
- Pre-existing Dependabot PRs (#1, #2, #56–#63) remain open and untouched;
  note #60/#63 bump cuelang.org/go and apiextensions-apiserver — merging
  those later should re-run the full gate plus integration.
- The contract module was untouched this session (still v0.3.0); the test
  harness required no contract change.
- Developer-owned untracked `.claude/` and `xr.yaml` preserved; install.sh
  verification upgraded the developer's `~/.local/bin/cuefn` to 0.1.6.

## References

- Design + research: `.journal/010/DESIGN.md`, `.journal/010/CANDIDATES.md`
- PRs: [#69](https://github.com/meigma/crossplane-cuefn/pull/69),
  [#70](https://github.com/meigma/crossplane-cuefn/pull/70),
  [#72](https://github.com/meigma/crossplane-cuefn/pull/72),
  [#73](https://github.com/meigma/crossplane-cuefn/pull/73),
  [#71 (release)](https://github.com/meigma/crossplane-cuefn/pull/71)
- Release: https://github.com/meigma/crossplane-cuefn/releases/tag/v0.1.6
  (run 29624978103; distribution run 29625508567)
- Full log: `.journal/010/NOTES.md`; durable facts: `TECH_NOTES.md` §010

## Lessons

- **Clean the golangci-lint cache immediately before the final local lint.**
  A worktree lint that was green went red in CI (goconst counting test-file
  literals, modernize) because earlier cached results masked findings —
  cleaning once early in the session is not enough.
- **Run the failure output past your own eyes before shipping a test tool.**
  Printing a real want.cue mismatch revealed noise (bare `N:M` positions
  from the JSON-encoded actual) that no assertion would have flagged; the
  message is the product.
- **Verify claimed ecosystem behavior empirically when it drives a design
  decision** — `*_test.cue` being inert, Subsume's message quality, and
  modzip packaging `tests/` were all settled by five-minute experiments, and
  two of the three contradicted plausible assumptions.
