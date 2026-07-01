#!/usr/bin/env python3
"""Render the Homebrew formula and Scoop manifest from a release checksums.txt.

The hashes come straight from the published release's checksums.txt, so they are
guaranteed to match the archives users download. This avoids rebuilding the
archives (which is not byte-reproducible across separate CI runs).
"""

from __future__ import annotations

import argparse
import json
import sys
from pathlib import Path

REPO = "meigma/crossplane-cuefn"
DESCRIPTION = "Crossplane v2 composition function that renders Kubernetes resources from CUE modules"
HOMEPAGE = f"https://github.com/{REPO}"
LICENSE = "Apache-2.0 OR MIT"
BINARY = "cuefn"


class RenderError(RuntimeError):
    """Raised when a manifest cannot be rendered."""


def parse_args(argv: list[str] | None = None) -> argparse.Namespace:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--tag", required=True, help="Release tag, e.g. v0.1.3")
    parser.add_argument("--checksums", required=True, type=Path, help="Path to checksums.txt")
    parser.add_argument("--formula-out", type=Path, help="Write the Homebrew formula here")
    parser.add_argument("--scoop-out", type=Path, help="Write the Scoop manifest here")
    return parser.parse_args(argv)


def load_checksums(path: Path) -> dict[str, str]:
    checksums: dict[str, str] = {}
    for line_number, raw in enumerate(path.read_text(encoding="utf-8").splitlines(), start=1):
        line = raw.strip()
        if not line:
            continue
        parts = line.split()
        if len(parts) != 2:
            raise RenderError(f"invalid checksums line {line_number}: {raw!r}")
        digest, name = parts
        if len(digest) != 64 or any(c not in "0123456789abcdef" for c in digest.lower()):
            raise RenderError(f"invalid sha256 on line {line_number}: {digest!r}")
        checksums[name] = digest.lower()
    if not checksums:
        raise RenderError(f"{path} contained no checksums")
    return checksums


def archive(version: str, goos: str, goarch: str, ext: str) -> str:
    return f"{BINARY}_{version}_{goos}_{goarch}.{ext}"


def digest_for(checksums: dict[str, str], name: str) -> str:
    try:
        return checksums[name]
    except KeyError as exc:
        raise RenderError(f"no checksum entry for {name}") from exc


def url_for(tag: str, name: str) -> str:
    return f"https://github.com/{REPO}/releases/download/{tag}/{name}"


def render_formula(tag: str, version: str, checksums: dict[str, str]) -> str:
    def block(goos: str, goarch: str) -> str:
        name = archive(version, goos, goarch, "tar.gz")
        return (
            f'      url "{url_for(tag, name)}"\n'
            f'      sha256 "{digest_for(checksums, name)}"'
        )

    return f"""# typed: false
# frozen_string_literal: true

# This file is generated on release. DO NOT EDIT.
class Cuefn < Formula
  desc "{DESCRIPTION}"
  homepage "{HOMEPAGE}"
  version "{version}"
  license "{LICENSE}"

  on_macos do
    on_intel do
{block("darwin", "amd64")}
    end
    on_arm do
{block("darwin", "arm64")}
    end
  end

  on_linux do
    on_intel do
{block("linux", "amd64")}
    end
    on_arm do
{block("linux", "arm64")}
    end
  end

  def install
    bin.install "{BINARY}"
  end

  test do
    system "#{{bin}}/{BINARY}", "--version"
  end
end
"""


def render_scoop(tag: str, version: str, checksums: dict[str, str]) -> str:
    name = archive(version, "windows", "amd64", "zip")
    manifest = {
        "version": version,
        "description": DESCRIPTION,
        "homepage": HOMEPAGE,
        "license": LICENSE,
        "architecture": {
            "64bit": {
                "url": url_for(tag, name),
                "bin": [f"{BINARY}.exe"],
                "hash": digest_for(checksums, name),
            }
        },
    }
    return json.dumps(manifest, indent=4) + "\n"


def main(argv: list[str] | None = None) -> int:
    args = parse_args(argv)
    tag = args.tag
    version = tag[1:] if tag.startswith("v") else tag

    try:
        checksums = load_checksums(args.checksums)
        formula = render_formula(tag, version, checksums)
        scoop = render_scoop(tag, version, checksums)
    except RenderError as exc:
        print(f"error: {exc}", file=sys.stderr)
        return 1

    if args.formula_out:
        args.formula_out.write_text(formula, encoding="utf-8")
    if args.scoop_out:
        args.scoop_out.write_text(scoop, encoding="utf-8")
    if not args.formula_out and not args.scoop_out:
        print(formula)
        print(scoop)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
