---
id: 009
title: Resolve Function xpkg render compatibility
started: 2026-07-16
---

## 2026-07-16 19:01 — Kickoff
Goal for the session: Resolve the Function xpkg image-config compatibility defect blocking Catalyst Infra X0, land a narrowly scoped review-ready implementation PR, and pause before any merge or publication without explicit human approval.
Current state of the world: `master` is at `8eebd929b75346d5a5672a5c858546c562b5b560`; published product v0.1.4 works in-cluster but its Function xpkg keeps `function` in `Cmd`, so Crossplane CLI 2.3.3 replaces it with `--insecure` and the container exits; developer-owned untracked `.claude/` and `xr.yaml` are present and must remain untouched.
Plan: Read the Catalyst implementation proposal and current GitHub state, create an isolated Worktrunk branch, first reproduce the defect with a focused failing test, implement the smallest package-only normalization, expand package and standard Docker render coverage, run the requested gates, publish a review-ready PR, and stop for approval.

## 2026-07-16 19:26 — Function package compatibility fix verified
Created the isolated Worktrunk branch `feat/function-xpkg-render-compatibility` from `origin/master`. A focused real-apko regression first failed when the assembled Function package was run exactly as Crossplane CLI 2.3.3 runs it (`IMAGE --insecure`): the image never became ready because Crossplane replaced the image `Cmd` and therefore dropped the `function` subcommand.

The fix is deliberately package-only: after assembling the Function xpkg, append the inherited base `Cmd` to its `Entrypoint`, clear `Cmd`, and write the copied image config back. The generic runtime image and `apko.yaml` are unchanged. Unit coverage now proves source-image non-mutation, retained runtime-layer digests, normalized single- and multi-architecture children, and preserved Function/CRD package metadata.

Added two real integration paths: the exact `docker run PACKAGE --insecure` gRPC smoke test and a standard Crossplane 2.3.3 Docker-runtime render using the assembled package with `PullPolicy: Never`. The latter uses Docker's bridge gateway plus the throwaway registry's published port so independently created render networks work on Linux CI as well as Docker Desktop. Both pending and ready observed-resource snapshots pass without Development mode, an entrypoint override, a substituted runtime image, or a local function server.

Focused package, package-validation, funcpkg, and render tests pass. The components of `root:check` pass; a first aggregate run exposed only local tool-cache/auth issues (GHCR login and a stale golangci-lint worktree path), which were repaired without changing repository state. The remaining work is the final aggregate/integration/E2E gates, commit, push, review-ready PR, and exact-head hosted checks. GitHub release state remains intentionally unchanged: release PR #67 is open, no v0.1.5 tag exists, and no merge or publication is authorized in this session.

## 2026-07-16 19:40 — Review-ready PR published
Portability review found that `RequireDevImage` could skip an opted-in integration run when the local dev image was absent. Tightened the gate so integration mode fails closed on missing Docker or `crossplane-cuefn:dev`, then corrected the stale Moon and workflow comments describing those prerequisites. The reviewer confirmed the finding resolved with no remaining actionable issues.

All local gates now pass on commit `743728ded81e966d5265814e13ef6bfecbfa9579`: `root:check`; the full OCI, schema, publish, Function-package, and render integration matrix; and `root:e2e-test`. One parallel integration attempt exhausted the local Docker daemon's predefined subnet pool because Crossplane CLI left empty `crossplane-render-*` networks behind; every non-render task passed in that attempt, the unused test-created networks were removed, and a clean render retry passed all three cases.

Pushed `feat/function-xpkg-render-compatibility` and opened review-ready PR #68, `fix(pkg): normalize Function xpkg entrypoint`: https://github.com/meigma/crossplane-cuefn/pull/68. GitHub CI is green at the exact head; hosted Integration, E2E, and Release Dry Run are still running. Release PR #67 remains untouched and must not merge until #68 is approved and merged and Release Please refreshes it. Session 009 remains in progress while work pauses for the required human approval; no merge, product tag, package publication, or contract release has occurred.

## 2026-07-16 19:47 — Exact-head hosted verification complete
All hosted checks passed on PR #68 head `743728ded81e966d5265814e13ef6bfecbfa9579`: CI, Integration, E2E, GitHub Pages, Kusari Inspector, both Melange architectures, container-image/Function-xpkg assembly and extraction, and the complete binary release dry run. GitHub reports the open, non-draft PR as cleanly mergeable. PR #67 remains open and unmerged at its original head `b1e7955df53d05767c55b9775d0bf950842ceb3c`; no `v0.1.5` Git tag exists. The implementation branch and journal branch are both clean and synchronized with their remotes. Pausing for explicit human approval as required.

## 2026-07-16 19:53 — Human approval received for merge and release
The developer approved PR #68 and explicitly authorized merging it, then merging the refreshed routine Release Please PR #67 to cut product v0.1.5. Re-audited live state before mutation: #68 remains open, cleanly mergeable, and fully green at reviewed head `743728ded81e966d5265814e13ef6bfecbfa9579`; #67 remains open at `b1e7955df53d05767c55b9775d0bf950842ceb3c`, based on pre-fix master `8eebd929b75346d5a5672a5c858546c562b5b560`, so it must refresh after #68 lands before it can be merged. Plan: squash-merge #68 pinned to its head, wait for Release Please to update #67 and rerun checks, merge #67 pinned to that refreshed head, then verify publication and both published-artifact X0 acceptance layers.

## 2026-07-16 20:03 — Implementation and release PRs merged
Squash-merged PR #68 with `--match-head-commit 743728ded81e966d5265814e13ef6bfecbfa9579`; `master` advanced exactly one commit to `e09e7207a3732037be4ecc522349ae8eb86c813f` (`fix(pkg): normalize Function xpkg entrypoint (#68)`). Release Please run `29551060534` then refreshed PR #67 to head `b377b96d66ceff586e66a5092151ca270eaf2616` on that exact base. The mechanical v0.1.5 release diff contains both the Function xpkg compatibility fix and the release-upload retry, updates only the expected seven release files, and leaves the contract at v0.3.0.

All refreshed #67 gates passed at that exact head: CI, Integration, E2E, Nix flake, GitHub Pages, Kusari, both Melange architectures, container/Function package assembly, and the 7m49s binary release dry run. Reconfirmed the PR was open, cleanly mergeable, and unchanged, then squash-merged it with the pinned head. `master` is now `1fc594f5c3f1265d9db9c9186dfd0add58d15def` (`chore(master): release 0.1.5 (#67)`), with parent `e09e7207a3732037be4ecc522349ae8eb86c813f`. Release verification is in progress: wait for Release Please to create the lightweight v0.1.5 tag and draft release, inspect the tag-push publication workflow and all artifacts, then publish the draft and run both published-artifact acceptance layers.
