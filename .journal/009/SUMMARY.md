---
id: 009
title: Resolve Function xpkg render compatibility and release v0.1.5
date: 2026-07-16
status: complete
repos_touched: [crossplane-cuefn]
related_sessions: [008]
---

## Goal
Resolve the Function xpkg image-config defect blocking Catalyst Infra X0, land a
narrow package-only fix without changing the generic runtime image, and—after
human approval—release and prove the corrected published artifacts.

## Outcome
The goal was fully met. PR #68 was squash-merged at reviewed head
`743728ded81e966d5265814e13ef6bfecbfa9579`, Release Please refreshed and PR #67
was squash-merged, and product v0.1.5 was published from exact commit
`1fc594f5c3f1265d9db9c9186dfd0add58d15def`. Hosted CI, the complete release
supply chain, distribution taps, ordinary Crossplane Docker renders, and the
focused live Kind/Chainsaw X0 proof all passed against the published digests.

## Key Decisions
- Normalize only the assembled Function xpkg config by appending the inherited
  `Cmd` to a copied `Entrypoint` and clearing `Cmd`. Crossplane CLI replaces
  `Cmd` with runtime flags, while the generic runtime image must retain its
  replaceable `cuefn` + `function` command split.
- Derive the normalized config from the assembled base and apply it to every
  multi-architecture child. This avoids hard-coded launch details, preserves
  runtime layers and package metadata, and keeps the source image immutable.
- Treat rebuilt or Development-mode images as insufficient release evidence.
  Final acceptance used the immutable v0.1.5 Function and runtime digests in
  both ordinary offline render and live Crossplane reconciliation paths.
- Leave contract v0.3.0 and its existing GitHub draft unchanged. This was a
  product packaging correction and did not change the CUE contract.

## Changes
- `internal/pkg/function.go` and `internal/pkg/function_test.go` - normalize the
  Function package image config and pin source immutability, layer, metadata,
  single-architecture, and multi-architecture behavior.
- `internal/test/integration/funcpkg_test.go` and
  `internal/test/integration/observed_resources_test.go` - prove the exact
  `docker run PACKAGE --insecure` path and standard Crossplane 2.3.3 Docker
  rendering without entrypoint, runtime-image, or local-server overrides.
- `internal/test/common`, `moon.yml`, `.github/workflows/integration.yml`, and
  related documentation - make opted-in integration prerequisites fail closed,
  support the disposable registry path, and document the corrected behavior.
- Release metadata (`CHANGELOG.md`, `.release-please-manifest.json`,
  `apko.yaml`, `melange.yaml`, `flake.nix`, and example Function refs) - bump
  product version and pinned examples to v0.1.5.
- Published `ghcr.io/meigma/crossplane-cuefn` at
  `sha256:01e3edbd344ffe839a887ef3d959a93dae94c2ba3692eecc8bf4143aba72fa19`
  and `ghcr.io/meigma/function-cuefn` at
  `sha256:59b86653c05d73f17ad5f3f1b1cb772d4420b4e81fb8c34ae116737739f5d7ad`.

## Open Threads
- The contract v0.3.0 GitHub release remains an intentional draft because
  contract distribution is through CUE Central and the shared release
  distributor is product-only.
- Release jobs emitted non-fatal artifact-metadata storage warnings, and the
  workflow still uses deprecated `actions/attest-sbom`; direct verification
  confirmed the v0.1.5 signatures, provenance, and SBOM attestations, but the
  workflow can be modernized separately.
- Crossplane CLI can leave empty `crossplane-render-*` Docker networks behind;
  integration and acceptance cleanup should continue removing only the exact,
  empty networks created by the test run.
- Developer-owned untracked `.claude/` and `xr.yaml`, plus the unrelated
  `oidc-smoke` Kind cluster, were deliberately preserved.

## References
- [PR #68 — Function xpkg compatibility fix](https://github.com/meigma/crossplane-cuefn/pull/68)
- [PR #67 — product v0.1.5 release](https://github.com/meigma/crossplane-cuefn/pull/67)
- [Product v0.1.5](https://github.com/meigma/crossplane-cuefn/releases/tag/v0.1.5)
- `.journal/008/SUMMARY.md` - observed-resource readiness and the v0.1.4 baseline.

## Lessons
- A container image whose `Cmd` supplies a required subcommand can work in
  cluster yet fail under Crossplane CLI's Docker runtime, because render-time
  flags replace `Cmd`. The Function package therefore needs a self-contained
  `Entrypoint`; the generic runtime image does not.
- Release confidence required both configuration inspection on every platform
  and execution through the consumer's ordinary runtime path. Either check
  alone would have left a meaningful compatibility gap.
