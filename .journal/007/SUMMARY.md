---
id: 007
title: README revamp + CLI distribution (brew/scoop/nix/mise/curl) with a post-release sanity check and fix
date: 2026-07-01
status: complete
repos_touched: [crossplane-cuefn]
related_sessions: [006, 004, 002]
---

## Goal

Two developer-driven pieces, in order: (1) revamp the README into a concise
landing page, and (2) make the `cuefn` CLI readily installable via Homebrew,
Scoop, Nix (self-hosted, no nixpkgs PR), and aqua/asdf — the last one aimed at
mise integration. A `curl | bash` method and a post-`v0.1.3` sanity check of every
method were added mid-session.

## Outcome

**Fully met.** `master` at `9a4f3ca`. Eight PRs merged (#47–#54, no #50), plus
product **`v0.1.3` cut + published** by the developer, and the taps populated and
**verified working end-to-end**.

- **README revamp (#47):** 278 → 130 lines; value prop, a verified minimal CUE
  example, a command table, docs hand-off. Dev/supply-chain content moved to
  CONTRIBUTING.md.
- **Distribution (phased):** #48 archives (tar.gz/zip) + Windows build + **ghd
  removed**; #49 Homebrew formula + Scoop; #51 in-repo Nix flake; #52 install docs;
  #53 verified `curl | bash` `install.sh`. **aqua was dropped** — mise uses its
  native `github:` backend (zero registry, verifies our SLSA attestations), which
  is the actual "mise integration" the developer wanted.
- **Post-release fix (#54):** a sanity check of the real v0.1.3 release found
  **brew and scoop broken** (wrong hashes) and **install.sh resolving the wrong
  "latest"**. Fixed both; also fixed `go install` version stamping. The v0.1.3 tap
  entries were corrected in place by re-running `release-distribute`.

Final verified matrix (against v0.1.3): brew ✅, scoop ✅ (hash), install.sh ✅
(checksum + SLSA), mise ✅, nix ✅, go install ✅ (functional; version stamp lands
on the next release).

## Key Decisions

- **Drop aqua, keep mise pure.** mise's `github:` backend installs from GitHub
  releases with zero registry and verifies GitHub attestations + SLSA by default —
  it *is* the mise integration goal. aqua (standard-registry PR or custom registry)
  added infra for no extra benefit here.
- **Nix = in-repo source-build flake, not the GoReleaser nix publisher.** The
  publisher can't consume our archives cleanly and needs published (non-draft)
  releases + a second repo; the source flake builds from the pinned git ref, so it
  is immune to the draft-first flow and needs no token/second repo. `nixpkgs`
  ships `go-1.26.4`, matching `go.mod`.
- **Keep raw-binary-friendly choices, but switch to archives once ghd was removed.**
  `ghd.toml` was the only raw-binary consumer; removing it (developer's call) freed
  the archive-format switch that brew/scoop/aqua all prefer. Homebrew **formula**
  (not cask) for macOS+Linux coverage.
- **Tap push runs post-publish** (`release: released`), not at tag time, because the
  repo is draft-first and asset URLs only resolve once a maintainer publishes.
- **Tap manifests are templated from `checksums.txt`, NOT a GoReleaser rebuild.**
  The original #49 rebuilt archives to compute hashes on the assumption they were
  byte-reproducible; #54 proved they are **not** (same GoReleaser v2.16.0, same
  commit, different bytes — LICENSE/README mtimes). Templating from the published
  `checksums.txt` is correct by construction.
- **install.sh authenticity = opportunistic `gh attestation verify`, not cosign.**
  Binaries are attested via the isolated `attest.yml` (SLSA L3), not a cosign blob;
  adding a cosign `signs` block would regress that signing-token isolation.

## Changes

Eight squash-merged PRs on `master`:

- **#47** `README.md` (rewrite), `CONTRIBUTING.md` (absorb dev/release), `docs/docs/index.md`.
- **#48** `.goreleaser.yaml` (archives + windows), removed `ghd.toml` +
  `.github/scripts/stage_ghd_release_assets.py` (+test), `release.yml`,
  `release-dry-run.yml`, `moon.yml`.
- **#49** `.goreleaser.yaml` (brews/scoops — later removed), new
  `.github/workflows/release-distribute.yml`, `release-dry-run.yml`.
- **#51** `flake.nix` + `flake.lock`, `.github/workflows/nix.yml`,
  `release-please-config.json` (flake.nix in extra-files), `.gitignore`.
- **#52** new `docs/docs/how-to/install-the-cli.md`, `README.md`, `quickstart.md`,
  `mkdocs.yml`.
- **#53** new `install.sh`, `docs/docs/how-to/install-the-cli.md`, `README.md`.
- **#54** new `.github/scripts/render_tap_manifests.py` + `push_tap.sh`, rewrote
  `release-distribute.yml`, `release-dry-run.yml`, `.goreleaser.yaml` (drop
  brews/scoops), `cmd/cuefn/main.go` (debug.ReadBuildInfo), `install.sh` (API resolver).
- **Ops:** set `HOMEBREW_TAP_TOKEN` + `SCOOP_BUCKET_TOKEN` repo secrets from the
  1Password `Homelab` item "Meigma scoop/tap token". Re-ran `release-distribute`
  (`workflow_dispatch tag=v0.1.3`) to correct the v0.1.3 tap hashes.

## Open Threads

- **`go install …@latest` reports `dev` until v0.1.4.** The stamping fix (#54) is on
  master but postdates v0.1.3; it ships correct on the next release.
- **Scoop was not run live** (no Windows host) — the manifest hash was verified to
  match `checksums.txt`, same code path as brew (which was run live). Confirm on a
  real Windows box when convenient.
- **Older GitHub release drafts** (`v0.1.0`, `v0.1.1`, contract `v0.1.0`/`v0.2.0`)
  remain unpublished — pre-existing.
- **GoReleaser `brews` deprecation** is now moot (we dropped it), but the general
  no-Linux-cask limitation stands if anyone revisits Homebrew via GoReleaser.
- Stray untracked `xr.yaml` + `.claude/` in the repo root (not session work).

## References

- PRs (all merged): #47, #48, #49, #51, #52, #53, #54 —
  https://github.com/meigma/crossplane-cuefn/pull/54 (the tap-checksums fix).
- Distribution investigation: `.journal/007/DISTRIBUTION.md`. Full running log:
  `.journal/007/NOTES.md`.
- Research (agents): aqua/mise, self-hosted nix, curl|bash installer landscape.
- `.journal/TECH_NOTES.md` — "Session 007" section.

## Lessons

- **A post-release sanity check earns its keep.** The three passing methods (mise,
  nix, install.sh-with-version) proved the *artifacts* were sound, but running the
  real `brew install` / default `install.sh` surfaced two ship-blocking bugs
  (wrong tap hashes, wrong "latest") that were invisible without exercising the
  actual published release.
- **"Reproducible build" is not a safe assumption across separate CI runs.** A
  same-machine back-to-back GoReleaser build was byte-identical, so I trusted the
  rebuild approach — but two *CI* runs at the same version+commit differed. Depend
  on the published `checksums.txt`, not a re-derivation.
- **GitHub's "latest" is unreliable for `curl|bash`.** The `/releases/latest` web
  redirect tracks most-recently-*published* (and can surface a `contract/*` draft
  during churn); resolve the newest product `v*` from the API list instead.
- **Match the mechanism to the actual supply chain.** "optional cosign" was the
  ask, but the binaries are SLSA/gh-attested (isolated `attest.yml`), so the honest
  implementation opportunistically runs `gh attestation verify` — adding cosign
  would have regressed the L3 isolation.
