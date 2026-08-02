#!/usr/bin/env bash
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"
if [[ ! -f SOURCE_MANIFEST.sha256 ]]; then
  echo "SOURCE_MANIFEST.sha256 is missing" >&2
  exit 1
fi

expected="$(mktemp)"
actual="$(mktemp)"
duplicates="$(mktemp)"
trap 'rm -f "$expected" "$actual" "$duplicates"' EXIT

# Verify the manifest grammar and every recorded digest before trusting its
# paths for the exact-set comparison below.
sha256sum --check --strict SOURCE_MANIFEST.sha256

cut -c67- SOURCE_MANIFEST.sha256 | LC_ALL=C sort > "$expected"
uniq -d "$expected" > "$duplicates"
if [[ -s "$duplicates" ]]; then
  echo "SOURCE_MANIFEST.sha256 contains duplicate paths:" >&2
  cat "$duplicates" >&2
  exit 1
fi

unexpected_node="$({
  find . \
    \( -type d \( -path './.git' -o -path './dist' -o -path './dist-dev' \
       -o -path './.cocoindex_code' -o -path './.codegraph' -o -path './.serena' \
       -o -path './.smart-coding-cache' -o -path './graphify-out' -o -name node_modules \) \) -prune -o \
    \( -type l -o \( ! -type f ! -type d \) \) -print
} | LC_ALL=C sort | head -n 1)"
if [[ -n "$unexpected_node" ]]; then
  echo "Source tree contains an unsupported filesystem node: $unexpected_node" >&2
  exit 1
fi

find . \
  \( -type d \( -path './.git' -o -path './dist' -o -path './dist-dev' \
     -o -path './.cocoindex_code' -o -path './.codegraph' -o -path './.serena' \
     -o -path './.smart-coding-cache' -o -path './graphify-out' -o -name node_modules \) \) -prune -o \
  -type f ! -path './SOURCE_MANIFEST.sha256' -print | LC_ALL=C sort > "$actual"

if ! cmp -s "$expected" "$actual"; then
  echo "Source manifest file set does not exactly match the source tree:" >&2
  diff -u "$expected" "$actual" >&2 || true
  exit 1
fi
