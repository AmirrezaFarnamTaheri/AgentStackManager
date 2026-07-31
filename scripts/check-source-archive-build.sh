#!/usr/bin/env bash
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
REVISION="${1:-}"
if [[ -z "$REVISION" ]]; then
  if git -C "$ROOT" rev-parse --is-inside-work-tree >/dev/null 2>&1; then
    REVISION="git:$(git -C "$ROOT" rev-parse HEAD)"
  elif [[ -f "$ROOT/SOURCE_REVISION" ]]; then
    REVISION="$(tr -d '\r\n' < "$ROOT/SOURCE_REVISION")"
  fi
fi
if [[ ! "$REVISION" =~ ^(git|unreleased-base):[0-9A-Fa-f]{40}$ ]]; then
  echo "revision must be git:<40-hex> or unreleased-base:<40-hex>" >&2
  exit 2
fi

echo "Preparing Git-free source copy for $REVISION"
work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT
git -C "$work" init --quiet
archive_root="$work/source"
mkdir -p "$archive_root"
(
  cd "$ROOT"
  tar \
    --exclude='./.git' \
    --exclude='./dist' \
    --exclude='./dist-dev' \
    --exclude='./coverage.out' \
    --exclude='./benchmark-results.txt' \
    -cf - .
) | tar -xf - -C "$archive_root"
echo "Writing ephemeral source provenance"
printf '%s\n' "$REVISION" > "$archive_root/SOURCE_REVISION"
cat > "$archive_root/SOURCE_PROVENANCE.json" <<JSON
{
  "schemaVersion": 1,
  "status": "source-archive-build-check",
  "revision": "$REVISION",
  "note": "Ephemeral CI proof that the source archive verifies and builds without Git metadata."
}
JSON
(
  cd "$archive_root"
  echo "Regenerating and verifying source manifest"
  ./scripts/write-source-manifest.sh
  ./scripts/verify-source-manifest.sh >/dev/null
  echo "Proving unlisted source files fail verification"
  printf 'manifest-negative-control\n' > UNLISTED-INJECTION.txt
  if ./scripts/verify-source-manifest.sh >/dev/null 2>&1; then
    echo "source manifest verifier accepted an unlisted file" >&2
    exit 1
  fi
  rm -f UNLISTED-INJECTION.txt
  ./scripts/verify-source-manifest.sh >/dev/null
  echo "Running supported no-Git build path"
  VERSION=source-archive-check ./scripts/build.sh
  test -s dist-dev/agentstack-windows-amd64.exe
  test -s dist-dev/agentstack-windows-arm64.exe
)
echo "Source archive build check passed for $REVISION"
