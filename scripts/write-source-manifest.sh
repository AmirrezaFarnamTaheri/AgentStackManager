#!/usr/bin/env bash
set -euo pipefail
ROOT="${1:-$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)}"
cd "$ROOT"
tmp="$(mktemp)"
trap 'rm -f "$tmp"' EXIT
find . -type f \
  ! -path './.git/*' \
  ! -path './SOURCE_MANIFEST.sha256' \
  ! -path './dist/*' \
  ! -path './dist-dev/*' \
  -print0 | LC_ALL=C sort -z | xargs -0 sha256sum > "$tmp"
mv "$tmp" SOURCE_MANIFEST.sha256
