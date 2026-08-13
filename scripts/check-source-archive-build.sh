#!/usr/bin/env bash
set -euo pipefail

revision="${1:?usage: check-source-archive-build.sh git:<40-hex>}"
if [[ ! "$revision" =~ ^git:[0-9a-fA-F]{40}$ ]]; then
  echo "expected git:<40-hex> revision, got: $revision" >&2
  exit 2
fi

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
# Keep the staged tree on the checkout volume. Git Bash may mount /tmp from
# WSL, while a native go.exe cannot safely lock module files through that path.
work="$(mktemp -d "$root/.source-archive-check.XXXXXX")"
trap 'rm -rf "$work"' EXIT
stage="$work/source"
archive="$work/source.zip"
go_bin="$(command -v go || command -v go.exe || true)"
if [[ -z "$go_bin" ]]; then
  echo "Go is required to verify the source archive" >&2
  exit 127
fi

# git archive deliberately creates a source tree without .git metadata and
# without untracked workstation material. The staged metadata is sufficient
# for the source-manifest contract and avoids assuming a checkout is clean.
mkdir -p "$stage"
git -C "$root" archive --format=tar HEAD | tar -xf - -C "$stage"

cat > "$stage/SOURCE_REVISION" <<EOF
$revision
EOF
cat > "$stage/SOURCE_PROVENANCE.json" <<EOF
{"schemaVersion":1,"status":"source-archive-check","revision":"${revision#git:}","baseRevision":"","candidateRevision":null}
EOF

(
  cd "$stage"
  "$go_bin" run ./cmd/releasepack --root . --manifest-mode write
  "$go_bin" run ./cmd/releasepack --root . --out "$archive" --prefix source --manifest-mode require
)
