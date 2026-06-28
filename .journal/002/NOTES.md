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

## 2026-06-28 12:29 — Phase 2 signed off + merged; Phase 3 launched
Phase 2 (PR #5) outcome — all 7 criteria verdicted **met** by adversarial
verifiers that ran the Docker-backed tests; 1 fix round (critic caught the spike
findings weren't recorded → fixed). I independently re-ran build/vet/full-test
(Docker present, OCI tests genuinely ran, 4.x s) — green.

Before merge I applied two cleanups the critic surfaced (user agreed): removed the
dead+buggy `stripModulePrefix` (GetZip entries are root-relative) and documented
that `OCIConfig.Expect` verifies the **root module ref only**. Re-ran
`moon run root:check` (green) → squash-merged. `master` is now `7fa2199`.
phase-2-oci worktree + branches cleaned up.

Spike findings promoted to TECH_NOTES ("Phase 2 — OCI loading"): explicit
registry injection (not nil-auto, avoids races); `CUE_CACHE_DIR` nonroot caching;
digest verify-after-fetch (semver not digest); **CUE modcache is keyed by
module@version not content digest** → loader owns a digest-keyed root cache +
ref→digest pointer; error classification; the read-only-tempdir cleanup gotcha.

Follow-ups carried (non-blocking, in TECH_NOTES): CI doesn't *assert* the
Docker-backed OCI tests ran (silent `t.Skip` without Docker — fine on
ubuntu-latest today; harden when we touch CI); one untested offline branch.

Phase 3 in flight: worktree `phase-3-function` off `7fa2199`; scope = wire the
engine into a Crossplane v2 composition function (`cuefn function` gRPC server +
image entrypoint), add `cuefn render` (cluster-free local eval), the `Input` type
under `input/v1beta1` (semver ref + optional expected-digest), example XRD/
Composition/XR/EnvironmentConfig/functions.yaml, and prove a real
`crossplane render` loop. New tools this phase: protobuf, controller-tools, the
`crossplane` CLI — must be pinned in mise + wired into moon/CI. Needs Docker for
the `function-environment-configs` step of `crossplane render`.

## 2026-06-28 13:24 — Phase 3 signed off + merged; Chainsaw decision; Phase 4 launched
Phase 3 (PR #6) outcome — all 4 criteria **met**, including the two heavy
integration checks; 2 fix rounds (a gate failure, then a critic blocker). I
independently re-ran the gate **and** the heavy tests under `mise exec`: `moon run
root:check` green, and `go test ./internal/function/...` ran the real
`crossplane render` loop (registry container + crossplane v2.3.3 + Docker +
function-environment-configs) and the apko-image gRPC smoke — both passed on my
machine. Squash-merged → `master` is now `6c36041`. phase-3 cleaned up.

**Chainsaw decision** (user raised it; I agreed with a sharp boundary): Chainsaw
is the harness for **API-server-facing** tests only — primary at **P8** (kind
e2e) and brought forward to **P4** (server-side XRD accept/default/status via
envtest, schema wrapped as a CRD). NOT for the engine/OCI/proto/render-loop/gRPC
tests (those stay Go + testcontainers). Recorded in TECH_NOTES with the rationale,
the envtest-CRD approach, and the instruction to confirm exact Chainsaw+envtest
patterns at implementation time. This also gives the cluster suites run-or-fail CI
(no silent skip) — but the Go-integration silent-skip gap (P2/P3) is separate and
still tracked for a CI-hardening pass (fold into P8).

Phase 4 in flight: worktree `phase-4-schema` off `6c36041`. Scope = `internal/schema`
codegen (CUE #API/#Spec/#Status → structural XRD via the de-risked recipe:
definitions-only reduction, `openapi.Generate` ExpandReferences:false, cycle-
detecting $ref inliner, XRD envelope), `cuefn generate` + `cuefn validate`, the
Go structural self-check (apiextensions-apiserver), AND the Chainsaw+envtest
server-side accept/default/status checks. New tools: apiextensions-apiserver
(dep), Chainsaw + setup-envtest (pinned in mise, dedicated moon/CI task).

## 2026-06-28 14:41 — Phase 4 (recovered) signed off + merged; Phase 5 launched
Phase 4 was a saga. The first full workflow **crashed fatally** at ~48 min: the
Implement agent (a direct schema'd `agent()`) hit the StructuredOutput retry cap
and threw, aborting the run (parallel() coalesces such failures to null, but a
direct await does not). The implementation had been written to the worktree
(uncommitted) and survived. I: (1) hardened the harness — big work agents now
return free text, schemas only on small decision agents, plus a `startAt:"verify"`
finish-mode; (2) independently confirmed the salvaged work (build, unit tests, and
the envtest+chainsaw proof — CRD accept + defaulting + pruning + status
round-trip, all pass); (3) ran the harness in finish-mode → PR #7, all 5 criteria
met, 0 blockers, 0 fix rounds. I re-ran `moon run root:check` (green) + the
validate behaviors + the envtest proof myself. Squash-merged → `master` `c76e1a8`.

Recorded in TECH_NOTES: Phase 4 summary, the harness-crash LESSON, and a
consolidated **CI-execution-assurance debt** (4 self-skip instances; fix as a P8
CI-hardening pass — fail-rather-than-skip + no-args image test + example/xrd.yaml
drift check). User chose to keep momentum: track it for P8, not a separate pass now.

Phase 5 in flight: worktree `phase-5-publish` off `c76e1a8`. Scope = `internal/pkg`
+ `cuefn publish`: build & push an installable Crossplane **Configuration** xpkg
(XRD from P4 + a pipeline Composition `function-environment-configs`→`cuefn` +
`crossplane.yaml` dependsOn the function) from Go via go-containerregistry, with
the **digest lock-step recorded** (Composition input carries module semver ref +
the expected manifest digest resolved at publish time). Opens with the xpkg
packaging de-risk spike (confirm crossplane's xpkg builder is non-importable
internal/; prototype Configuration + embed-runtime Function xpkg; validate with
`crossplane xpkg`) — findings → TECH_NOTES at the gate. Uses the now-hardened
harness (full run, free-text work agents).

## 2026-06-28 15:19 — Phase 5 signed off + merged; Phase 6 launched
Phase 5 (PR #8) outcome — all 4 criteria **met**, 0 blockers, 0 fix rounds; the
hardened harness ran a full phase cleanly. I independently re-ran `moon run
root:check` (green) + the Docker/crossplane-gated pkg+cli tests (push→pull
round-trip, `crossplane xpkg extract`, full `cuefn publish` end-to-end with the
real digest lock-step) — all pass. Squash-merged → `master` `fc3d388`.

The **digest lock-step is now closed end-to-end**: publish records the module's
real resolved manifest digest into the Composition input; the P3 runtime accepts
it and rejects a drifted digest (tested).

Spike findings promoted to TECH_NOTES (importable escape hatch
`crossplane-runtime/v2/pkg/xpkg`; `crossplane xpkg extract` is the acceptance
check since inspect/validate don't exist in 2.3.3; Function-xpkg prototype only
over empty.Image). Carry-forwards recorded: **dep-bloat decision for P6** (publish's
cosign/sigstore deps bloat the shared serve binary → decide build-tag split vs
accept), the `--dir` footgun, and the deprecated `dependsOn.function` field (P8).

Phase 6 in flight: worktree `phase-6-funcpkg` off `fc3d388`. Scope = ship the
function as a SIGNED Crossplane **Function** xpkg: `package/crossplane.yaml`
(kind Function) + embedded Input CRD; build via the P5 assembler over a REAL apko
runtime base (first real embed-runtime use); cosign-sign + SBOM + provenance;
wire release.yml/release-dry-run.yml/security-scan.yml; AND resolve the dep-bloat
decision (measure + build-tag-split publish out of the image binary if needed).
The "release dry-run green in CI" criterion confirms on the PR (CI runs it).
