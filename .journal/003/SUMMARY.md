---
id: 003
title: Survey and reorganize the integration/E2E test suite into internal/test
date: 2026-06-29
status: complete
repos_touched: [crossplane-cuefn]
related_sessions: [001, 002]
---

## Goal

The session opened with no stated goal. The developer then asked to (1) survey
every integration/E2E test in the repo, (2) assess and propose how to bring the
suite in line with the org standard — all integration tests under
`internal/test/integration`, all e2e under `internal/test/e2e`, shared infra in
`internal/test/common` — consolidating duplicate helpers and mostly-duplicate
tests, and (3) execute the plan, with a hard rule that **every PR needs human
approval before merge**. Ran under `ultracode` (workflow-per-phase).

## Outcome

**Goal fully met.** The suite is reorganized and consolidated, merged in two
human-approved PRs, CI green throughout. `master` at `5c9a363`.

- **Survey:** 24 Go integration/E2E test functions across 6 packages + 3 Chainsaw
  declarative `Test`s in 2 suites; all infra-gated via `CUEFN_INTEGRATION` + tool
  presence, with build tags `envtest`/`e2e`.
- **Assessment:** a 5-agent workflow (4 auditors + synthesis) found both org
  principles broken — co-location everywhere, plus the `internal/cli` publish
  tests were white-box `package cli`. Produced a temporary proposal
  (`.journal/003/PROPOSAL-test-reorg.md`, since deleted) with 8 decisions; the
  developer agreed with all recommendations.
- **PR #12 (relocate):** new `internal/test/common` + `integration` + `e2e`;
  helpers deduped; boundary fixed; latent `!noxpkg` tag fixed. All 6 gated suites
  run locally + green on Linux CI.
- **PR #13 (consolidate):** C2–C6 merges + C3 deletions, coverage preserved;
  `moon -run` regexes re-derived to a clean partition. Integration tests 23 → 17.
- **Regression caught + fixed:** PR #12 accidentally committed a **61 MiB `cuefn`
  binary** to the repo root (a workflow gate ran a bare `go build ./cmd/cuefn` +
  `git add -A`). #13 untracks it and gitignores `/cuefn`.

## Key Decisions

- **One flat `internal/test/integration` + a separate `internal/test/e2e` + an
  importable `internal/test/common`** (rejected per-area sub-packages): test names
  are already unique, helper collisions are removed by hoisting to `common`, and
  the flat layout lets each moon task keep a single package path so the `-run`
  regexes survive the move.
- **Two phases, two PRs** (relocate, then consolidate): isolates the `-run` regex
  churn from the pure move, keeping each diff reviewable.
- **C3 — delete the library round-trips** (`TestConfigurationRoundTrip`,
  `TestFunctionXpkgRoundTrip`) rather than relocate them: their byte-identity
  assertion largely tests go-containerregistry, and the build→push→pull→parse path
  is covered by the CLI E2Es; moved the residual unique coverage (digest-stability,
  real apko runtime base) into `TestPublish_EndToEnd`/`TestPublishFunction_EndToEnd`.
- **Bundled the binary-removal fix into #13** rather than a separate PR — avoids a
  fragile rebase of the consolidation branch onto a cleaned master.
- **Workflow-driven with self-run ground truth:** every phase was implement →
  adversarial verify → fix via a workflow, but I personally ran `go vet ./...`,
  `moon run root:check`, and the gated suites — the workflow's self-reported
  "green" was scoped and once missed staying-file compile errors.

## Changes

- **PR #12** (`refactor(test): relocate integration and e2e tests …`):
  `internal/test/common` (9 files), `internal/test/integration` (23 tests),
  `internal/test/e2e` (kind harness moved wholesale + testdata); deleted the 4
  per-package helper files; deduped `requireDocker ×4`, `registryImage ×5`,
  `startRegistry ×4`, etc.; `internal/cli` publish tests made external;
  `//go:build !noxpkg` added to the migrated + staying publish files; `moon.yml`
  six gated tasks repointed + `cueModules` e2e-testdata path; `.golangci.yml`
  scoped exclusion for the new buildable `common` package.
- **PR #13** (`refactor(test): consolidate duplicate integration tests; untrack
  stray cuefn binary`): C2 (fold `TestImageServesFunction`), C3 (delete the two
  round-trips + CLI-E2E enhancements), C4 (fold index round-trip → both-platforms
  assert), C5 (merge xpkg-validate → table test), C6 (merge cosign+syft →
  `TestFunctionXpkgSupplyChain`); `moon` `publish-test`/`funcpkg-test` regexes
  re-derived; removed orphaned `common.PackageYAMLBytes`; refreshed moon comments;
  `git rm cuefn` + `/cuefn` in `.gitignore`.
- **Journal** (`journal/jmgilman`): NOTES checkpoints throughout; `TECH_NOTES.md`
  gained a durable "Test layout" section; the temporary `PROPOSAL-test-reorg.md`
  was deleted at close.

## Open Threads

- None from this task. Final integration test count is 17; layout + conventions
  are documented in `TECH_NOTES.md` ("Test layout").
- Pre-existing, unrelated follow-ups remain (from prior sessions / TECH_NOTES):
  two untouched Dependabot PRs (#1, #2); assorted non-blocking CI niceties.

## References

- PRs (merged): #12 (relocate) https://github.com/meigma/crossplane-cuefn/pull/12,
  #13 (consolidate) https://github.com/meigma/crossplane-cuefn/pull/13
- `.journal/TECH_NOTES.md` — "Test layout — integration/E2E under internal/test"
- Workflows: assess `wf_a5a87d1f-5e8`, Phase 1 `wf_592377f7-f99`, Phase 2 `wf_cb71f8e5-ff9`
- `.journal/003/NOTES.md` (full running log)

## Lessons

- **Never `git add -A` after a bare `go build ./cmd/cuefn`** — it writes a 60+ MiB
  `cuefn` binary to the repo root that gets staged (the #12 regression). Build
  gates should use `-o /tmp/...` or `bin/`. `/cuefn` is now gitignored.
- **Trust `go vet ./...` over IDE/LSP diagnostics mid-refactor.** Stale LSP
  snapshots reported phantom compile errors in staying files during the Phase-1
  workflow; the committed tree was actually clean.
- **A workflow's self-reported "green" can be scoped** (e.g. `go test` on one
  package) and miss whole-module breakage. Always run the real blocking gate
  (`moon run root:check`) and the gated suites yourself before opening the PR.
