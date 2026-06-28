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
