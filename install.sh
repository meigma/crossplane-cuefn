#!/usr/bin/env bash
# Download, verify, and install the cuefn CLI from GitHub Releases.
#
#   curl -fsSL https://raw.githubusercontent.com/meigma/crossplane-cuefn/master/install.sh | bash
#
# It resolves the latest published release (drafts and pre-releases are skipped),
# downloads the archive for your OS/arch, verifies it against checksums.txt, and
# — when the GitHub CLI is available and authenticated — verifies the release's
# SLSA provenance with `gh attestation verify`. The stronger, always-available
# verification path is documented at:
#   https://meigma.github.io/crossplane-cuefn/how-to/install-the-cli/
#
# Configuration (environment variables):
#   VERSION   Version to install, e.g. v0.1.3 (default: latest published release).
#   BIN_DIR   Install directory (default: $HOME/.local/bin).
#   BASE_URL  Override the release download base URL (for testing).

set -euo pipefail

REPO="meigma/crossplane-cuefn"
BINARY="cuefn"
SIGNER_WORKFLOW="${REPO}/.github/workflows/attest.yml"

err() { printf 'install: %s\n' "$*" >&2; }
die() { err "$*"; exit 1; }

need() { command -v "$1" >/dev/null 2>&1; }

# Clean up the download directory on exit. tmp is global so the trap can see it.
tmp=""
cleanup() { [ -n "${tmp}" ] && rm -rf "${tmp}"; return 0; }
trap cleanup EXIT

detect_os() {
  case "$(uname -s)" in
    Darwin) printf 'darwin' ;;
    Linux) printf 'linux' ;;
    *) die "unsupported OS '$(uname -s)'. On Windows use Scoop: scoop install meigma/cuefn" ;;
  esac
}

detect_arch() {
  case "$(uname -m)" in
    x86_64 | amd64) printf 'amd64' ;;
    aarch64 | arm64) printf 'arm64' ;;
    *) die "unsupported architecture '$(uname -m)'" ;;
  esac
}

# Follow the /releases/latest redirect to the tag. This skips drafts and
# pre-releases and needs no API token.
resolve_latest() {
  local effective
  effective="$(curl -fsSLI -o /dev/null -w '%{url_effective}' \
    "https://github.com/${REPO}/releases/latest")" \
    || die "could not resolve the latest release"
  case "$effective" in
    */tag/*) printf '%s' "${effective##*/tag/}" ;;
    *) die "unexpected releases URL: ${effective}" ;;
  esac
}

sha256_of() {
  if need sha256sum; then
    sha256sum "$1" | cut -d' ' -f1
  elif need shasum; then
    shasum -a 256 "$1" | cut -d' ' -f1
  else
    die "need sha256sum or shasum to verify the download"
  fi
}

main() {
  need curl || die "curl is required"
  need tar || die "tar is required"

  local os arch tag version bin_dir base_url
  os="$(detect_os)"
  arch="$(detect_arch)"

  tag="${VERSION:-$(resolve_latest)}"
  case "$tag" in v*) ;; *) tag="v${tag}" ;; esac  # normalize to vX.Y.Z
  version="${tag#v}"                               # archive names omit the leading v

  bin_dir="${BIN_DIR:-${HOME}/.local/bin}"
  base_url="${BASE_URL:-https://github.com/${REPO}/releases/download/${tag}}"

  local archive="${BINARY}_${version}_${os}_${arch}.tar.gz"

  tmp="$(mktemp -d)"

  err "downloading ${archive} (${tag})"
  curl -fsSL "${base_url}/${archive}" -o "${tmp}/${archive}" \
    || die "failed to download ${archive} — is ${tag} published for ${os}/${arch}?"
  curl -fsSL "${base_url}/checksums.txt" -o "${tmp}/checksums.txt" \
    || die "failed to download checksums.txt"

  # Integrity: the archive must match its checksums.txt entry.
  local want have
  want="$(awk -v f="${archive}" '$2 == f { print $1 }' "${tmp}/checksums.txt")"
  [ -n "${want}" ] || die "no checksum entry for ${archive}"
  have="$(sha256_of "${tmp}/${archive}")"
  [ "${want}" = "${have}" ] || die "checksum mismatch for ${archive} (expected ${want}, got ${have})"
  err "checksum verified"

  # Authenticity (opportunistic): verify SLSA provenance when gh is available.
  if need gh && gh auth status >/dev/null 2>&1; then
    if gh attestation verify "${tmp}/${archive}" --repo "${REPO}" \
        --signer-workflow "${SIGNER_WORKFLOW}" --source-ref "refs/tags/${tag}" \
        >/dev/null 2>&1; then
      err "SLSA provenance verified via gh attestation"
    else
      err "warning: could not verify SLSA provenance (unpublished attestation or offline); proceeding on checksum"
      err "         verify manually: gh attestation verify ${archive} --repo ${REPO}"
    fi
  else
    err "note: gh not available/authenticated; skipping provenance check (checksum verified)"
  fi

  tar -xzf "${tmp}/${archive}" -C "${tmp}" "${BINARY}"
  mkdir -p "${bin_dir}"
  install -m 0755 "${tmp}/${BINARY}" "${bin_dir}/${BINARY}"
  err "installed ${BINARY} to ${bin_dir}/${BINARY}"

  case ":${PATH}:" in
    *":${bin_dir}:"*) ;;
    *) err "note: ${bin_dir} is not on your PATH — add it, e.g. export PATH=\"${bin_dir}:\$PATH\"" ;;
  esac

  "${bin_dir}/${BINARY}" --version || true
}

main "$@"
