# CLI distribution assessment — brew / scoop / nix / aqua(mise)

Investigation (2026-06-30, session 007) into making the `cuefn` CLI installable
through Homebrew, Scoop, Nix (self-hosted), and aqua/mise. Grounded in the repo's
release pipeline + two current-docs research passes. Also folds in the developer's
directive to **remove `ghd.toml`** in the same PR.

## Current state (what we ship today)

- `.goreleaser.yaml`: builds `cuefn` for **darwin/linux × amd64/arm64** (no Windows),
  archives as **raw binaries** (`formats: [binary]`, named
  `cuefn_${version}_${os}_${arch}`), checksums + per-binary SBOMs.
- `release.yml` runs GoReleaser as **`release --clean --skip=publish`** (build only),
  then a Python script (`stage_ghd_release_assets.py`) stages/validates assets against
  `ghd.toml`, uploads them to a **DRAFT** GitHub release, and an isolated `attest.yml`
  adds SLSA L3 provenance + cosign. **The maintainer publishes the draft by hand.**
- No `homebrew_casks` / `scoops` / `nix` blocks → the existing taps
  (`meigma/homebrew-tap`, `meigma/scoop-bucket`) are **not** fed by this repo.
- Binary install today is Meigma's own **`ghd`** (`ghd.toml`) — **being removed**.

Sibling precedent: **blob-cli** uses full `goreleaser release` (publish mode) with
`brews:` (Formula) + `scoops:`, tar.gz/zip archives, Windows builds, and
`HOMEBREW_TAP_TOKEN` / `SCOOP_BUCKET_TOKEN`. **ghd** uses `homebrew_casks` (raw-binary
cask). cuefn deliberately diverges (draft-first, build-only, isolated attestation).

## Cross-cutting findings (read first — these gate multiple channels)

1. **The draft-first release policy is the universal blocker.** Everything that
   fetches a release asset — mise `github:`/aqua, Homebrew (cask/formula URL), Scoop,
   and a Nix *binary*-fetch derivation — resolves `…/releases/download/<tag>/…`, which
   **404s while the release is a draft**. They only work once the maintainer
   **publishes** the release. (The in-repo Nix *source* flake is the sole exception —
   it builds from the git tag, not release assets.) This aligns with the existing open
   thread "publish the draft releases," but it must become routine, not optional.

2. **`ghd.toml` removal unblocks the archive-format choice.** `ghd.toml` + the staging
   script are the only consumers pinned to **raw** binary names. With ghd gone we are
   free to switch to **tar.gz (unix) + zip (windows)** archives — the conventional shape
   for brew/scoop/aqua and the org's publisher-driven convention (blob-cli). Raw still
   works for brew casks, aqua (`format: raw`), and mise `github:`/`ubi`, but archives
   remove version-in-filename / suffix-stripping / `exe`/`bin` naming friction
   everywhere. **Recommendation: switch to archives.**

3. **ghd removal blast radius** (all in this PR): delete `ghd.toml`,
   `.github/scripts/stage_ghd_release_assets.py` + its test; rework the
   `release.yml` "Stage and validate ghd release assets" step (it also collects
   `dist/release-assets/*` for the draft upload — replace with GoReleaser's own
   `dist/` output or a thin collector); drop the `release-dry-run.yml`
   "Validate ghd-compatible dry-run artifacts" block + the `ghd.toml` change-detect
   pattern; drop the `ghd download` line in the release-inspection summary; remove
   `ghd.toml` from `moon.yml` inputs. No product code touched (release-infra only).

4. **Windows build is new work, needed only for Scoop.** Add `windows/amd64`
   (ignore `windows/arm64`, like blob-cli). Pure-Go cross-compile, cheap.

5. **Cross-repo auth is mostly solved.** The repo already mints scoped tokens via
   `actions/create-github-app-token` from `MEIGMA_RELEASE_APP`
   (`vars.MEIGMA_RELEASE_APP_ID` + `secrets.MEIGMA_RELEASE_APP_PRIVATE_KEY`). The same
   action can scope a token to `homebrew-tap` + `scoop-bucket` **iff the App is
   installed there** — confirm/extend that. Alternative: the org
   `HOMEBREW_TAP_TOKEN`/`SCOOP_BUCKET_TOKEN` secrets blob-cli already uses (couldn't
   confirm they're visible to this repo — 403 on org secrets).

6. **Publish vs draft design decision (the main architectural call).** GoReleaser's
   brew/scoop/nix publishers run in **publish phase** — skipped today by
   `--skip=publish`. To keep draft-first AND feed the taps, run the publishers in a
   **post-publish job** (`on: release: { types: [released] }`) so asset URLs resolve.
   Two ways: (a) re-run GoReleaser with only brew/scoop publishers (reuses its mature
   manifest templating; rebuilds artifacts deterministically to recompute sha256 — some
   waste); (b) hand-template the cask/scoop/nix files and push (full control, matches
   the ghd-era "we publish ourselves" ethos, more code). **Recommendation: (a)** —
   `release: disable: true` already, so GoReleaser won't fight over the GitHub release;
   it just pushes manifests to the taps.

## Per-channel

### 1. Homebrew  — tap `meigma/homebrew-tap` exists (has `Casks/` + `Formula/`)
- **Work:** add a `homebrew_casks` (or `brews` formula) block; wire a tap-scoped token;
  run it in the post-publish job. Raw binaries already work (see `ghd.rb`); with the
  archive switch, casks/formula consume the tarball.
- **Sub-decision — cask vs formula:** **cask** = modern, GoReleaser-recommended, ghd
  precedent, but **macOS-only**. **formula** = macOS **and** Linux(brew), blob-cli
  precedent, but GoReleaser marks `brews` legacy. cuefn is Linux-relevant → formula
  buys Linux brew users; cask is cleaner if macOS-only is acceptable.
- **Effort:** Low (once the post-publish job + token exist).

### 2. Scoop — bucket `meigma/scoop-bucket` exists (blob/blobber/whzbox manifests)
- **Work:** **add the Windows build** (the only real new thing) + a **zip** archive for
  windows; add a `scoops` block + `SCOOP_BUCKET_TOKEN` (or App token); run in the
  post-publish job. Windows-only by nature.
- **Effort:** Low–Medium (Windows build is the delta).

### 3. Nix — self-hosted, **no nixpkgs PR** (developer's constraint)
- **Recommendation: in-repo `flake.nix` with `buildGoModule`** (build from source).
  - No artifact change (ignores release assets), **no second repo, no token, immune to
    draft-first**. Users: `nix run github:meigma/crossplane-cuefn`,
    `nix profile install github:meigma/crossplane-cuefn`, or as a flake input.
  - `pkgs.buildGoModule { pname="cuefn"; version; src=./.; vendorHash="sha256-…";
    subPackages=["cmd/cuefn"]; env.CGO_ENABLED=0; ldflags=["-s" "-w"
    "-X main.version=${version}"]; meta.mainProgram="cuefn"; }`. No `vendor/` dir →
    `vendorHash` required; it tracks **`go.sum`**, so a release that doesn't change deps
    needs **no flake edit** (near-zero per-release toil). Users need flakes enabled.
  - Optional: CI `nix build` smoke check; a Cachix cache for instant (no-compile)
    installs.
- **Rejected: GoReleaser `nix` publisher** — explicitly **cannot consume raw
  (`format: binary`) artifacts** and needs published (non-draft) releases + a second
  repo + PAT. Even after the archive switch it's strictly more infra than the flake for
  a single CLI.
- **Effort:** Low–Medium (write flake, compute `vendorHash`). **Independent** of the
  release-pipeline changes — can land on its own.

### 4. aqua / mise  — "the reason is mise integration"
- **Best mise path (≈ free): the `github:` backend.** `mise use github:meigma/crossplane-cuefn`
  with `bin = "cuefn"`. **Zero registry**, handles raw or archived binaries, and
  **verifies GitHub Artifact Attestations + SLSA by default** — exactly what `attest.yml`
  already produces. (mise's `ubi:` backend is now deprecated in favor of `github:`;
  `ubi` does **not** verify provenance.) Needs **published** releases. Cost: a docs line.
- **For `aqua:` / aqua-CLI discoverability — three tiers:**
  - **(a) Custom in-repo registry** (`registry.yaml`, `type: github_content`): **no
    upstream PR**. Works in mise via `[settings] aqua.registries = ["https://github.com/meigma/crossplane-cuefn"]`
    then `aqua:meigma/crossplane-cuefn`. For the aqua **CLI** it also needs a Policy
    allow (`aqua policy allow`); mise has no such gate.
  - **(b) Upstream PR to `aquaproj/aqua-registry`** → `aqua g -i github.com/meigma/crossplane-cuefn`
    works out of the box and mise's baked registry picks it up. Use `aqua gr -cmd cuefn
    meigma/crossplane-cuefn` to scaffold; declare `checksums.txt` + cosign **bundle**
    (aqua ≥ v2.47) + `github_artifact_attestations`. Requires the registry's `cmdx s`
    scaffold, **signed commits**, green multi-OS CI; merge time variable (often days).
  - **(c) Both:** custom registry now, upstream later for discoverability.
- **Recommendation:** ship the **mise `github:` backend doc immediately** (covers "mise
  integration" with provenance, zero infra); add a **custom in-repo aqua registry** for
  aqua-CLI users; pursue the **upstream aqua-registry PR** only if broad `aqua g -i`
  discoverability is wanted.
- **asdf (fallback):** needs a separate maintained `asdf-cuefn` plugin repo (bin/
  scripts), no built-in verification — strictly worse than `github:`/aqua for mise
  users. Only for plain-asdf holdouts. **Recommend skipping.**
- **Effort:** Very low (mise `github:` = docs) → Low (custom registry) → Medium (upstream PR).

## Suggested sequencing

- **P0 — release pipeline refactor (shared lever, one PR with ghd removal):** remove
  ghd; switch to tar.gz/zip archives; add the Windows build; add the post-publish
  (`release: released`) job + App-scoped tap token. Establish "publish the release" as
  routine (the universal unblocker).
- **P1 — Homebrew + Scoop:** add the publisher blocks; they ride the P0 post-publish job.
- **Nix + mise `github:` doc — parallel/independent:** the flake and the mise docs don't
  depend on P0 and can land anytime (the flake doesn't even need archives).
- **aqua registry (custom, then optional upstream):** after archives exist (cleaner
  registry entry), or with `format: raw` now.

## Key decisions for the developer
1. **Archive format:** switch to tar.gz/zip (recommended) vs keep raw.
2. **Homebrew:** cask (macOS-only, modern) vs formula (macOS+Linux, legacy).
3. **aqua depth:** mise `github:` doc only / + custom registry / + upstream PR.
4. **Release publishing:** OK to make "publish the draft" a routine release step
   (required for brew/scoop/aqua/mise to actually resolve)?
5. **Scope now:** stop at this plan, or proceed to implement (and in what order)?
