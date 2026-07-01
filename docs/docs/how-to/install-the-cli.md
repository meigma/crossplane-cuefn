# How to install the cuefn CLI

Every method below installs the same `cuefn` binary. Releases ship as signed,
multi-arch archives with a `checksums.txt` and SLSA provenance, so prefer a method
that verifies them where you can.

## Homebrew (macOS and Linux)

```sh
brew install meigma/tap/cuefn
```

`meigma/tap` is the [meigma/homebrew-tap](https://github.com/meigma/homebrew-tap)
repository; Homebrew adds it automatically on first use.

## Scoop (Windows)

```sh
scoop bucket add meigma https://github.com/meigma/scoop-bucket
scoop install meigma/cuefn
```

## mise

mise installs `cuefn` straight from GitHub releases with its `github` backend — no
registry to configure:

```sh
mise use -g "github:meigma/crossplane-cuefn[bin=cuefn]"
```

Or pin it in a project's `mise.toml`:

```toml
[tools]
"github:meigma/crossplane-cuefn" = { version = "latest", bin = "cuefn" }
```

The `github` backend verifies the release's GitHub artifact attestations and SLSA
provenance by default. `bin=cuefn` names the binary because the repository is
`crossplane-cuefn`.

## Nix (flakes)

Run it once, or install it into your profile:

```sh
nix run github:meigma/crossplane-cuefn -- --version
nix profile install github:meigma/crossplane-cuefn
```

The flake builds from source and tracks the ref you pin, e.g.
`github:meigma/crossplane-cuefn/v0.1.2`. It requires flakes to be enabled
(`experimental-features = nix-command flakes`). To consume it from another flake,
add it as an input and use `packages.${system}.default`.

## Shell script (Linux and macOS)

```sh
curl -fsSL https://raw.githubusercontent.com/meigma/crossplane-cuefn/master/install.sh | bash
```

The script installs the latest published release into `~/.local/bin`. It always
verifies the archive against the release `checksums.txt`, and — when the GitHub CLI
is installed and authenticated — verifies the release's SLSA provenance with `gh
attestation verify`. Configure it with environment variables:

```sh
curl -fsSL https://raw.githubusercontent.com/meigma/crossplane-cuefn/master/install.sh \
  | VERSION=v0.1.2 BIN_DIR=/usr/local/bin bash
```

Piping to a shell runs code you have not read; if you would rather not, [read the
script](https://github.com/meigma/crossplane-cuefn/blob/master/install.sh) first,
or use one of the package managers above.

## Go

```sh
go install github.com/meigma/crossplane-cuefn/cmd/cuefn@latest
```

## Prebuilt archives

Each [release](https://github.com/meigma/crossplane-cuefn/releases) attaches
per-platform archives (`cuefn_<version>_<os>_<arch>.tar.gz`, `.zip` on Windows), a
`checksums.txt`, and per-archive SBOMs. Verify an archive's SLSA provenance against
the release workflow with the GitHub CLI — the strongest check, and the one the
shell installer runs when `gh` is available:

```sh
gh attestation verify cuefn_<version>_<os>_<arch>.tar.gz \
  --repo meigma/crossplane-cuefn \
  --signer-workflow meigma/crossplane-cuefn/.github/workflows/attest.yml
```

## Verify the install

```sh
cuefn --version
```

The runtime composition **function** is not installed this way — it ships as a
signed Crossplane Function package at `ghcr.io/meigma/function-cuefn` and is pulled
automatically when you install a generated Configuration (see
[Publish a Configuration](publish-configuration.md)).
