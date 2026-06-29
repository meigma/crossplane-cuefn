---
id: 002
title: Implement the full crossplane-cuefn build (PLAN P1–P8) via per-phase workflows
date: 2026-06-29
status: complete
repos_touched: [crossplane-cuefn]
related_sessions: [001]
---

## Goal

Implement the entire design + plan from session 001 — a Crossplane v2 composition
function that renders Kubernetes resources from CUE modules pulled from an OCI
registry, plus the operator CLI — by executing PLAN phases **P1→P8**. Hard
constraint from the user: a reusable multi-agent **workflow** drives each phase, but
**manual human sign-off is required at the end of every phase before any PR is
merged**. Session ran under ultracode (workflow-per-phase).

## Outcome

**Goal met — the full PLAN is implemented and merged.** Eight phases → eight PRs
(**#4–#11**), each independently verified by me and **human-signed-off before a
squash merge**. `ci` + `integration` + `e2e` green on master (`e81d018`).

- **P1** (#4) `internal/render` engine + module-contract v2 + reserved-key projection (offline).
- **P2** (#5) `OCILoader`: transitive deps, nonroot `CUE_CACHE_DIR`, digest verify-after-fetch.
- **P3** (#6) Crossplane function (`RunFunction`), `cuefn function` + `cuefn render`, real `crossplane render` loop.
- **P4** (#7) `internal/schema` CUE→structural XRD codegen, `cuefn generate`/`validate`; **Chainsaw+envtest** server-side proof.
- **P5** (#8) `internal/pkg` + `cuefn publish`: Configuration xpkg, **digest lock-step closed end-to-end**.
- **P6** (#9) signed **Function xpkg** (embed-runtime) + release wiring; **dep-bloat `noxpkg` split (−12.1 MiB)**; **CI-hardening** (see Lessons).
- **P7** (#10) Diátaxis docs set; `cue` CLI pinned; documented the `:8080` metrics endpoint.
- **P8** (#11) **kind e2e via Chainsaw**, green in CI; registry crux solved; fixed two real bugs; metrics flag + cleanups.

## Key Decisions

- **Orchestration:** one reusable per-phase workflow (`scratchpad/phase-build.workflow.js`):
  understand → plan → implement (isolated worktree) → adversarially verify every PLAN
  success criterion → bounded fix loop → open PR → **STOP**. The harness never merges;
  the human merge gate lives between runs (I squash-merge after each sign-off, then
  launch the next phase). Hardened mid-session: big work agents return **free text**
  (no StructuredOutput fatal-failures), plus a `startAt:"verify"` finish-mode.
- **Chainsaw is the e2e harness** for API-server-facing tests (P4 envtest, P8 kind).
- **Dep-bloat:** `publish`/`internal/pkg` (sigstore/cosign deps) gated behind a
  `noxpkg` build tag; the runtime image binary builds `-tags=noxpkg`.
- **CI gate = `moon run root:check`** (not `moon ci`); heavy Docker/crossplane/kind/
  envtest suites are env-gated (`CUEFN_INTEGRATION`) + run in non-blocking
  `integration.yml` / `e2e.yml`.

## Changes

- 8 squash-merged PRs (#4–#11) on `master`. Each verified locally (build, `moon run
  root:check`, the relevant gated suites under `mise exec`) and via the real PR CI.
- Journal (this session, `journal/jmgilman`): `NOTES.md`, `SUMMARY.md`, and large
  `TECH_NOTES.md` additions (one section per phase + the CI-hardening + the Chainsaw
  decision + the two e2e-found bug fixes). `DESIGN.md`/`PLAN.md` unchanged (session 001).

## Open Threads

- **Promote + delete the temporary working docs:** `.journal/001/DESIGN.md` and
  `PLAN.md` are marked temporary; their hardened bits now live in code + TECH_NOTES.
  Promote anything still load-bearing and delete the working docs (session-001 note).
- Minor non-blocking follow-ups (TECH_NOTES "Phase 8"): `example/xrd.yaml` lost its
  "generated" header; no explicit fail-if-skipped guard on `e2e`/`integration`; `ci`
  mise-setup installs all tools (a transient crossplane.io 403 failed the fast gate
  once); the e2e loop calls `internal/pkg` directly rather than the `cuefn publish`
  binary.
- Two unrelated Dependabot PRs (#1, #2) remain open + untouched (from session 001).

## For the next agent (start here)

1. The product is **functionally complete and CI-proven** (P1→P8). Read TECH_NOTES
   top-to-bottom — it now documents the whole architecture + every phase + the gotchas.
2. First housekeeping task: **promote hardened DESIGN/PLAN content into TECH_NOTES and
   delete `.journal/001/DESIGN.md` + `PLAN.md`** (they were always temporary).
3. The reusable phase workflow (`scratchpad/phase-build.workflow.js`) is the template
   for future multi-phase, human-gated builds.
4. New tools pinned this session: `crossplane`, `controller-tools`, `chainsaw`,
   `setup-envtest`, `kind`, `kubectl`, `helm`, `cue`, `syft`, `go-containerregistry`.

## References

- PRs (all merged): #4 render, #5 OCI, #6 function, #7 schema, #8 publish, #9 funcpkg,
  #10 docs, #11 e2e — https://github.com/meigma/crossplane-cuefn/pull/4 … /11
- `.journal/TECH_NOTES.md` (the durable record), `.journal/001/{DESIGN,PLAN}.md`
- Reusable harness: `…/scratchpad/phase-build.workflow.js`

## Lessons

- **Verify the real `ci` Action at every gate, not just local `moon run root:check`.**
  `ci` had been **red on master since P2** (the blocking gate ran the flaky heavy
  Docker suites); my local-only gating missed it for four phases. Fixed in P6 and made
  a standing rule.
- **Harness fragility:** a direct schema'd `agent()` that exceeds the StructuredOutput
  retry cap throws and aborts the whole workflow (lost ~48 min on P4 before salvage).
  Fix: big work agents return free text; structured output only on small
  decision-driving agents. Added a finish-mode to resume an interrupted worktree.
- **macOS-vs-Linux CI gaps bit repeatedly:** `set -o pipefail` under dash; bind the
  test gRPC server on `0.0.0.0` (not `127.0.0.1`) for the Docker bridge; kind API-server
  loopback + `image-local` melange differ on Docker Desktop. The truth is the Linux CI run.
- **The kind-registry crux:** Crossplane's pkg manager is HTTPS-only and its CEL
  validation rejects dotless registry hosts → a CA-trusted HTTPS registry wired into
  both Crossplane (helm `registryCaBundleConfig`) and containerd, with a separate
  plain-HTTP registry for CUE modules.
- **The e2e earned its keep:** it found a real bug (generated Configs merged no
  EnvironmentConfig) that all prior phases missed.
