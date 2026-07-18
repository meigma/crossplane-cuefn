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

## 2026-07-17 15:25 — Design doc written
`.journal/010/DESIGN.md` is the authoritative spec: section vocabulary
(closed, hard errors on unknowns), normalized result document (explicit
status:null / requirements:{}), three evaluators (want.cue via
Unify+Validate(Concrete) with txtar-coordinate line padding; want.yaml exact
golden; error.txt substrings), steps contiguous-from-1 sharing base fixtures,
seed/--update/never-in-CI workflow, CLI spec, architecture
(internal/snapshot extraction + internal/testharness core + thin cli
adapter, no noxpkg work, dep: go-internal/txtar), dogfood acceptance via
example/module tests/ + example-test moon task, 4-PR arc, 4 open questions
(publish packaging of tests/, CI detection, diff polish, --run step
addressing). Awaiting developer go-ahead to start PR 1.

## 2026-07-17 16:05 — PR 1 open: snapshot extraction (#69)
Worktree `.wt/refactor-snapshot-extraction` (branch
`refactor/snapshot-extraction` off master 1fc594f). New `internal/snapshot`:
`ParseObject`/`LoadObject` (single-doc XR/env), `ParseObjects` (multi-doc,
embedded `---` safe), `LoadObservedObjects` + bytes-core `KeyObservedObjects`,
`LoadRequiredObjects`, `MatchRequirements`, and
`RenderWithRequiredObjects(ctx, engine, ref, inputs, objs)` (the moved
two-pass fixpoint incl. the `cannot render module %q` wrapping — runRender now
delegates). cli: render.go/validate.go rewired; observed_resources.go +
required_resources.go (+tests) deleted; loader tests moved to snapshot with
exported names + new direct tests (ParseObject, embedded-separator
ParseObjects, KeyObservedObjects). `moduleLoader` deliberately stays in cli
(adapter wiring). Local `moon run root:check` green; PR #69 checks ALL green
(ci 48s, integration 3m04s, e2e 3m21s). Awaiting developer review/approval →
then squash-merge + prune per the usual workflow.

## 2026-07-17 17:20 — PR 1 merged; PR 2 (testharness core) open (#70)
PR #69 squash-merged (master 2e3fbfc), worktree pruned. PR 2 built in
`.wt/feat-testharness-core`: `internal/testharness` = case.go (txtar parse,
closed vocabulary, step grouping/validation), normalize.go (ready→
Ready/NotReady/Unspecified, status:null, requirements always present; JSON→
CompileBytes for the CUE value), evaluate.go (want.cue Unify+Validate(Concrete),
newline-padded so positions cite `<case>.txtar/want.cue:<line>`, synthetic
result-side `N:M` positions filtered; want.yaml exact + LCS diff; error.txt
substrings), run.go (Runner{Loader,Ref}, per-unit runUnit, observed opt-in
check via NEW exported render.UsesObservedResources), update.go
(SeedGoldens inserts after observed section / UpdateGoldens only rewrites
drifted machine-owned want.yaml). go-internal/txtar promoted to direct dep →
flake vendorHash updated (sha256-yYEwrn…, from CI's mismatch report). Failure
output verified by eye — path-qualified conflicts + txtar coordinates, e.g.
`resources.deployment.object.spec.replicas: conflicting values 3 and 5:
case.txtar/want.cue:13:13`. GOTCHA CONFIRMED: local lint green while CI lint
red (goconst/modernize) — the golangci cache must be cleaned IMMEDIATELY
before the final local lint, not just once early. Fix pushed (f2b0cbc);
waiting on PR #70 checks.

## 2026-07-17 18:40 — PR 2 merged; PR 3 (cuefn test command) open (#72)
PR #70 merged (master 099a4c3; user asked whether txtar handling was
hand-rolled — no: rogpeppe/go-internal/txtar Parse/Format, only the ~10-line
sectionContentLine offset scan is ours since txtar.File has no positions).
PR 3 in `.wt/feat-cuefn-test-command`: internal/cli/test.go (newTestCommand +
runTest/runTestCase/seedCase/updateCase; flags --dir/.-default, --run regex,
--update, --fail-fast, --ci, --cache-dir; CI = --ci OR CI env truthy; PASS/
FAIL/SEED/UPDATE lines + summary; non-zero exit on fail OR seed; --update
re-runs after rewrite so want.cue failures still fail; gosec G703 suppressed
in writeCaseFile with justification — first #nosec in repo). RESOLVED design
open question #1: modzip empirically DOES package tests/*.txtar into
published modules → documented-as-intended (embedded tests travel with the
module; caveat in PR 4 docs). Dogfood: example/module/tests/{defaults,
environment-tier,rejects-replicas-over-max,full-render}.txtar — full-render
golden SEEDED BY THE REAL BINARY (whole loop proven live: 3 pass + 1 seed →
exit 1 → --ci re-run 4 pass). New blocking moon task root:example-test
(`cuefn test --dir example/module --ci`) in check. moon check green; PR #72
open, checks running.

## 2026-07-17 19:20 — PR 3 merged; PR 4 (docs) open (#73)
PR #72 merged (master 1632e30; its ci run included the example-test dogfood
gate). PR 4 in `.wt/docs-cuefn-test`: new how-to
docs/docs/how-to/test-a-module.md (first case, section↔render-flag mirror
table, partial-CUE semantics incl. close()/constraints/positional lists,
golden lifecycle, error.txt, readiness steps, CI mode, published-modules-
include-tests note); reference/cli.md gains a full `test` section + table
six→seven commands + noxpkg note (`test` in both builds); README command row;
mkdocs nav + quickstart pointer. docs:build strict + root:check green
locally; PR #73 open, checks running. Release-please has an updated
release PR queued on master (v0.1.6 candidate) — release decision belongs to
the developer, out of scope for this arc.

## 2026-07-17 19:50 — Arc complete; v0.1.6 release in flight
PR #73 merged (13432b4) after adding reference/test-cases.md (developer asked
for a thorough test-contract reference — sibling of module-contract.md in
nav; how-to keeps workflow, cli.md keeps command). ALL FOUR ARC PRs MERGED
(#69/#70/#72/#73). Developer approved cutting the release: PR #71
(release 0.1.6) verified (changelog #70+#72, all x-release-please-version
stamps bumped) and merged → master cf310ab → release-please tagged v0.1.6 +
draft release targeting cf310ab ✓. release.yml run 29624978103 triggered by
tag PUSH (correct provenance event per session-008 lesson), in progress.
Next: verify all release jobs, then draft assets (5 archives + 5 SBOMs +
checksums.txt), attestation subjects/signer, image + Function xpkg digests;
publication → release-distribute (brew/scoop) verification.

## 2026-07-17 20:40 — v0.1.6 RELEASED AND VERIFIED
release.yml run 29624978103: ALL 10 jobs success. Draft assets exactly 5
archives + 5 SBOMs + checksums.txt. Verified before publication:
darwin_arm64 sha256 8b8af956… matches checksums.txt; `gh attestation verify`
binds subjects to source SHA cf310ab via the isolated attest.yml at
refs/tags/v0.1.6; binary stamps `0.1.6 (cf310ab)` and ships `cuefn test`;
cosign verify OK for ghcr.io/meigma/{crossplane-cuefn,function-cuefn}:v0.1.6;
Function xpkg config Entrypoint=[/usr/bin/cuefn,function] Cmd=null (v0.1.5
normalization holds). Published (gh release edit --draft=false, 01:36Z) →
release-distribute run 29625508567 success; brew formula bumped to 0.1.6 with
hashes == checksums.txt. Real `curl|bash install.sh` end-to-end: resolved
v0.1.6 as latest, checksum + SLSA verified, installed to ~/.local/bin
(NOTE: the developer's local cuefn was thereby upgraded to 0.1.6). Installed
release binary ran `cuefn test --dir example/module --ci` → 4/4 PASS. The
session goal is fully shipped: design → 4 PRs → released v0.1.6 with the
harness proving itself via its own dogfood suite.

## 2026-07-17 21:05 — Close
Session closed. Merged PRs: #69 (snapshot extraction), #70 (testharness
core), #72 (cuefn test command + dogfood), #73 (docs incl. test-cases
reference), #71 (release 0.1.6). master fast-forwarded to cf310ab; all
session worktrees removed; v0.1.6 published + distribution verified.
Hand-off: SUMMARY.md written; TECH_NOTES.md gained the session-010 harness
section (architecture, assertion mechanics, golden lifecycle, gotchas);
deferred fast-follows listed there and in SUMMARY Open Threads. Pre-existing
Dependabot PRs untouched.
