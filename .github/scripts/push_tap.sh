#!/usr/bin/env bash
# Commit a rendered manifest file into an org tap repository and push it.
#
# Usage: push_tap.sh <repo> <src-file> <dest-path> <commit-message>
# The push token is read from $TAP_TOKEN. GitHub Actions masks the secret in
# logs, so the token embedded in the remote URL is redacted automatically.

set -euo pipefail

repo="${1:?repo required, e.g. meigma/homebrew-tap}"
src="${2:?source file required}"
dest="${3:?destination path required}"
message="${4:?commit message required}"
: "${TAP_TOKEN:?TAP_TOKEN must be set}"

[ -f "$src" ] || { echo "push_tap: source $src not found" >&2; exit 1; }

work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT

git clone --depth 1 "https://x-access-token:${TAP_TOKEN}@github.com/${repo}.git" "$work/repo"

dest_full="$work/repo/$dest"
mkdir -p "$(dirname "$dest_full")"
cp "$src" "$dest_full"

cd "$work/repo"
git config user.name "meigma-release-bot"
git config user.email "release-bot@meigma.dev"
git add "$dest"

if git diff --cached --quiet; then
  echo "push_tap: no changes for $repo ($dest already up to date)"
  exit 0
fi

git commit -m "$message"
git push origin HEAD
echo "push_tap: updated $repo:$dest"
