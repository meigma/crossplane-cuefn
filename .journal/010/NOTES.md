---
id: 010
title: Design a first-class `cuefn test` harness for cuefn packages
started: 2026-07-17
---

## 2026-07-17 14:25 — Kickoff
Goal for the session: not yet stated — the developer started a fresh session and
has not made their first request.
Current state of the world: master at 1fc594f (chore(master): release 0.1.5,
PR #67). Session 009 closed after normalizing the Function xpkg launch config
(PR #68) and publishing + verifying product v0.1.5 (runtime digest
sha256:01e3edbd…, Function digest sha256:59b86653…). Contract v0.3.0 remains
the current published contract via CUE Central (GitHub draft intentionally
unpublished). Open threads carried in: deprecated `actions/attest-sbom` +
non-fatal artifact-metadata warnings in the release workflow (modernization
candidate), crossplane-render-* empty Docker network cleanup convention, and
developer-owned untracked `.claude/` + `xr.yaml` in the repo root.
Plan: await the developer's request, then journal at meaningful checkpoints.

## 2026-07-17 14:30 — Goal stated: `cuefn test` harness research
The developer wants a first-class, heavily opinionated `cuefn test` command in
the existing binary for testing cuefn modules against test cases — explicitly
NOT a generalized test harness. Today's naive alternative is a giant bash
script wrapping `cue` with boilerplate; that is the anti-goal. First step:
spawn research agents (ecosystem prior art, opinionated-harness design
patterns, CUE-native test idioms, local codebase grounding) and present a few
candidate designs for the developer to consider. No implementation yet.

## 2026-07-17 14:45 — Research complete; candidates written
All four agents reported. Durable synthesis + research digest saved to
`.journal/010/CANDIDATES.md` (read that, not this note, for detail). Headlines:
Crossplane upstream punted on composition testing (#5710 closed not-planned,
leaves a vetted directory/assert shape); `*_test.cue` is reserved but INERT in
current CUE tooling (cue#209 open since 2021); unification-as-assertion proven
viable with genuinely good failure messages (Unify+Validate(Concrete), NOT
Subsume — Subsume messages are a trap); negative assertions impossible in pure
CUE (harness must own expectError); Pkl `pkl test` (facts+auto-golden) and
chainsaw subset-match are the strongest external models; engine
Inputs/Result API already exposes everything assertable, harness home is a new
`internal/testharness` core + thin cobra adapter. Four candidates presented to
the developer: A CUE-native suites, B Crossplane-native YAML dirs (#5710
shape, subset match), C txtar one-file-per-case (YAML inputs + CUE want), D
golden-first snapshots. My lean: A or C; D better as a mode than a design.
Next: developer picks a direction (or hybrid) → then design doc.

## 2026-07-17 15:10 — DECISION: Candidate C (txtar cases)
The developer reviewed expanded C and an example A suite, and chose **C**:
one self-contained `.txtar` file per case under `tests/`, YAML input sections
mirroring the render flags (xr/environment/required/observed), expectations as
partial CUE (`want.cue`, Unify+Validate(Concrete)), exact golden (`want.yaml`,
seeded on first run, `--update` rewrites, never in CI), `error.txt` for
negative tests, numbered step dirs (`1/observed.yaml`…) for readiness
sequences. Assertion core stays format-agnostic in `internal/testharness` so
an A-style suite format could be added later. Next: implementation-work
breakdown delivered to developer; awaiting go-ahead on plan/PR arc.
