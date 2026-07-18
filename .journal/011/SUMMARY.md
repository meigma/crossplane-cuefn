---
id: 011
title: Design and ship `cuefn check` (product v0.1.7)
date: 2026-07-17
status: complete
repos_touched: [crossplane-cuefn]
related_sessions: [010, 006, 002]
---

## Goal

Evaluate, design, and — after per-stage developer approval — ship a
`cuefn check` command: the instance-free static health gate for a module
(canonical formatting, clean evaluation, XRD generation with an optional
reviewed golden), complementing `cuefn test` the way `go vet` complements
`go test`. Then cut and fully verify the release carrying it.

## Outcome

**Fully met, through a verified public release.** The three-PR arc merged
(#74 core, #76 CLI, #77 docs), release PR #75 cut **product v0.1.7**, and the
release was verified before publication and proven from the consumer path
after distribution. `master` is at `4577625` (the release commit).

- #74 — `internal/textdiff` (LCS diff + normalization extracted from
  testharness) and `internal/check` (Fmt/Vet/XRD/GoldenBytes) with offline
  fixtures; `cue fmt --check` parity pinned against the real pinned CLI.
- #76 — the `cuefn check` command (fmt/vet/xrd units, `cuefn test`'s
  PASS/FAIL/SEED/UPDATE lifecycle, `--xrd` golden with seed/`--update`/CI
  refusal); moon `xrd-check` swapped from a bash `generate`+`diff` script to
  the real CLI; `example/xrd.yaml` reseeded machine-owned (+2 header lines).
- #77 — how-to `check-a-module.md`, `cli.md` check section + eight-command
  table + the check/test/validate three-verb delineation, README/quickstart/
  cross-link pointers.
- v0.1.7 — tag-push run all green with tag-bound provenance; checksums,
  archive attestations, cosign on both images, and the Function-entrypoint
  normalization verified before publication; brew/scoop tap hashes match the
  published `checksums.txt`; real `install.sh` resolved latest → v0.1.7 and
  the released binary ran the example module's `check` (3/3) and `test` (4/4)
  suites clean.

## Key Decisions

- **`check` is static and instance-free; the three-verb split is the API.**
  `check` = module health + XRD drift (no XR needed), `test` = behavioral
  txtar cases, `validate` = one concrete XR. Documented as a table in
  `cli.md`.
- **No `cue mod tidy` in check.** Tidy lives in CUE's `internal/mod/modload`
  and is not importable; shelling out to a `cue` binary would break the
  single-binary promise. Docs point to `cue mod tidy --check` as the manual
  complement.
- **Golden compare is byte-exact after stripping leading full-line comments**
  — lets seeded goldens carry a machine-owned `DO NOT EDIT` header (closing
  the Phase-8 lost-header follow-up) while headerless
  `cuefn generate --output` files pass unchanged.
- **Load failure reports as the vet unit's failure.** Probed empirically: CUE
  evalv3 surfaces regular, definition-nested, and hidden conflicts eagerly at
  `render.LoadModule` build time, so `check.Vet` (the exact
  `cue vet -c=false` option set) is a documented recursive backstop, not the
  primary failure path.
- **Reuse `cuefn test`'s lifecycle vocabulary verbatim** (statuses, CI
  detection, refusal wording, summary/exit shape) so authors learn one model.
- **Dogfood immediately**: the blocking moon `xrd-check` task now runs
  `cuefn check --dir example/module --xrd example/xrd.yaml --ci`, adding the
  repo's first CUE formatting + vet gate for the example module.
- **Deviation from the approved design (recorded in #76):** the existing
  golden gained its header via delete+reseed, not `--update` — the
  comment-tolerant compare correctly treats the headerless golden as
  matching, and `--update` is drift-only by design.

## Changes

- `internal/textdiff/` (#74) — `Lines` + `Normalize`, extracted verbatim;
  testharness call sites rewired.
- `internal/check/` (#74) — `Fmt` (via `cue/format`), `Vet`, `XRD`,
  `GoldenBytes`; testdata modules `good`/`disjunction` + fmt fixtures.
- `internal/cli/check.go` + `root.go` (#76) — the command; `collectCUEFiles`
  via `fs.WalkDir` over `os.DirFS` (gosec G122); golden read/seed/update in
  the adapter.
- `moon.yml` (#76) — `xrd-check` body swapped to the real CLI.
- `example/xrd.yaml` (#76) — reseeded with the machine-owned header.
- `docs/` + `README.md` (#77) — how-to, reference section, delineation
  table, nav/quickstart/cross-links.
- Release v0.1.7 (#75) — published and distributed; changelog lists both
  feats.

## Open Threads

- Deferred by design: a `--fix` flag (auto-format), checking a published
  module by OCI ref (shared fast-follow with `cuefn test`), and the
  session-010 test fast-follows (JSON/JUnit output, `--run` step
  addressing, A-style suites).
- Pre-existing Dependabot PRs #1, #2, #56–#63 remain open; #60 now bumps
  cuelang.org/go 0.16.1→0.17.1 — merging it later must re-verify the
  `cue fmt` parity fixtures and the evalv3 eagerness assumptions alongside
  the full gate.
- The contract module was untouched (still v0.3.0; GitHub draft release
  intentionally unpublished).
- Developer-owned untracked `.claude/` and `xr.yaml` preserved; the
  consumer-proof install went to scratch space, not `~/.local/bin`.

## References

- Design: `.journal/011/DESIGN.md` (approved as written; one recorded
  deviation, above)
- PRs: [#74](https://github.com/meigma/crossplane-cuefn/pull/74),
  [#76](https://github.com/meigma/crossplane-cuefn/pull/76),
  [#77](https://github.com/meigma/crossplane-cuefn/pull/77),
  [#75 (release)](https://github.com/meigma/crossplane-cuefn/pull/75)
- Release: https://github.com/meigma/crossplane-cuefn/releases/tag/v0.1.7
  (tag-push run 29630705541; distribution run 29631172558)
- Full log: `.journal/011/NOTES.md`; durable facts: `TECH_NOTES.md` §011

## Lessons

- **Verify feasibility against the actual dependency source before designing
  around it.** Five minutes in the module cache settled that `cue/format` is
  public and tidy is not — the design's scope boundary came straight from
  those two facts.
- **Probe evaluator behavior empirically before writing failure-path tests.**
  The planned "vet-failing module" fixture was impossible: every candidate
  construct failed at build, not validate. A throwaway probe test surveying
  eight constructs found the truth in minutes and reshaped the CLI's
  error-reporting contract.
- **Select CI runs by tag/title, never by recency.** A wait loop that
  matched "any non-queued run" watched the previous release's distribution
  run to a meaningless success; the real run had to be found by
  `displayTitle`.
- **`gh attestation verify` is silent on success in non-tty contexts** —
  use `--format json` when you need evidence, not just an exit code.
