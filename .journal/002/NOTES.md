---
id: 002
title: Implement PLAN Phase 1 — render engine core + module contract
started: 2026-06-28
---

## 2026-06-28 09:23 — Kickoff
Goal for the session: Begin implementing the design produced in session 001.
Per the PLAN, implementation starts at **Phase 1 — the render engine core +
module contract (offline)**: `internal/render` hexagonal core with an `Engine`
and a `ModuleLoader` port, a `LocalLoader` adapter for offline/tests, the
module-contract-v2 shape (`#API`, `#Spec`, `input`/`resources`), reserved-key
stripping before unifying with the closed `#Spec`, and the keyed-map output
contract (`resources: {<stableName>: {object, ready?}}` + optional `status`).

Current state of the world:
- Session 001 closed. Repo `crossplane-cuefn` rebranded from `template-go`
  (PR #3 merged). No product code yet — only the scaffold + supply-chain tooling.
- Authoritative spec lives in `.journal/001/DESIGN.md` (resolved decisions §13)
  and `.journal/001/PLAN.md` (8 phases + falsifiable success criteria).
- Proven reference spike (runtime half) at
  `/Users/josh/work/catalyst-infra/.wt/experiment-platform-mvp/platform/mvp/cuefn`
  — port `internal/render`, `fn.go`, loaders, adapting to the richer contract.
- Proven stack: `cuelang.org/go v0.16.1`, `function-sdk-go v0.7.1`,
  `crossplane-runtime/v2 v2.3.1`, `crossplane/apis/v2 v2.3.1`,
  `apimachinery v0.35.3`. CLI is cobra/viper.
- Two non-obvious runtime traps to honor: strip `spec.crossplane` (+ legacy
  machinery keys) before unifying with the closed `#Spec`; no digest-by-ref
  (CUE loads by semver — verify manifest digest after fetch).

Plan: re-read DESIGN §13 and PLAN Phase 1 in full, load the `cue` skill, start a
fresh implementation worktree off `origin/master`, port + adapt the engine, write
functional + unit tests offline, open one PR for the phase.

## 2026-06-28 09:59 — Orchestration design + Phase 1 launched (ultracode)
User switched to **ultracode** and asked for a workflow to complete **all 8
phases**, with a hard rule: **manual human sign-off at the end of each phase
before any PR is merged**; otherwise the workflow does whatever the plan/design
requires.

Key constraint resolved: a background Workflow runs to completion with no
mid-run human-pause primitive, so a single all-phases workflow that merges would
violate the gate. **Decision: one reusable per-phase workflow, run once per
phase, with the main loop (me) holding the merge gate between runs.** Each run
does understand → plan → implement (in an isolated worktree) → adversarially
verify every PLAN success criterion (one verifier per criterion + completeness
critic + independent build/test/check runner) → bounded fix loop → open a PR and
**STOP**. The workflow has **no merge step** — nothing reaches master without
explicit human sign-off.

Reusable harness: `…/scratchpad/phase-build.workflow.js` (parameterized via
`args` = phase spec + success criteria + worktree path + context pointers).
Hit + fixed an args-stringification footgun (script now JSON.parses string args).

Phase 1 in flight:
- Worktree `phase-1-engine` at `.wt/phase-1-engine`, branched off `origin/master`.
- Workflow run `wf_06e60105-ad4` (task `w1cdyeycc`) launched. Output = a PR
  awaiting sign-off; I do NOT merge until the user approves.

Open question for the user (non-blocking, answer before P1 lands): after sign-off
should I (a) squash-merge + auto-start the next phase, or (b) wait for an explicit
"go" each time; and (c) does the user merge or do I, post-sign-off.

## 2026-06-28 11:00 — Phase 1 signed off + merged; Phase 2 launched
User chose **auto-continue** (I squash-merge on sign-off, then immediately start
the next phase) and **I merge after approval**.

Phase 1 (PR #4) outcome — all 7 success criteria verdicted **met** by independent
adversarial verifiers + an independent gate runner; I also re-ran `go build`/`go
test` in the worktree myself (green). Squash-merged → `master` is now `b3a15d1`.
`internal/render` (engine + LocalLoader + reserved-key projection), `example/module/`,
3 testdata fixtures, 15 tests. phase-1-engine worktree + local/remote branch
cleaned up. Non-blocking carry-forwards: OCILoader load-failure branches dead
until P2; `#API` only structurally present until P4 codegen; core couples to
dir-based `load.Instances` by design (port yields a dir, not fs.FS).

Harness hardening: the `understand:design` sub-agent had returned placeholder
text ("test fact one") in P1 — outcome was unaffected (full phase spec is embedded
in the Plan/Implement prompts), but I added a `NO_PLACEHOLDER` instruction to all
three understand prompts.

Phase 2 in flight: worktree `phase-2-oci` off `b3a15d1`; run `wf_2eb7d664-3d3`
(task `w911fuob4`). Scope: OCILoader (modconfig/modregistry → zip → unzip →
engine), transitive OCI deps, nonroot `CUE_CACHE_DIR`, digest verify-after-fetch;
opens with a de-risk spike whose findings I will promote to TECH_NOTES at the gate.
Needs Docker (testcontainers) — a new risk vs P1's pure-offline run.
