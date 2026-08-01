#!/usr/bin/env bash
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"
version="${VERSION:-dev}"
buildvcs=false
revision=""
git_root="$(git -C "$ROOT" rev-parse --show-toplevel 2>/dev/null || true)"
root_real="$(cd "$ROOT" && pwd -P)"
git_root_real=""
if [[ -n "$git_root" ]]; then
  git_root_real="$(cd "$git_root" && pwd -P)"
fi
if [[ -n "$git_root_real" && "$git_root_real" == "$root_real" ]]; then
  if [[ -n "$(git status --porcelain=v1 --untracked-files=all)" ]]; then
    echo "build.sh requires a clean tree" >&2
    exit 1
  fi
  revision="git:$(git rev-parse HEAD)"
  buildvcs=true
else
  bash ./scripts/verify-source-manifest.sh >/dev/null
  revision="$(tr -d '\r\n' < SOURCE_REVISION)"
  if [[ ! "$revision" =~ ^(git|unreleased-base):[0-9A-Fa-f]{40}$ ]]; then
    echo "SOURCE_REVISION must contain git:<40-hex> or unreleased-base:<40-hex>" >&2
    exit 1
  fi
fi
rm -rf dist-dev
mkdir -p dist-dev
go test ./...
go test -race ./...
go vet ./...
flags="-s -w -buildid= -X main.version=$version -X main.revision=$revision"
for arch in amd64 arm64; do
  CGO_ENABLED=0 GOOS=windows GOARCH="$arch" go build -trimpath -buildvcs="$buildvcs" -ldflags="$flags" -o "dist-dev/agentstack-windows-$arch.exe" ./cmd/agentstack
  if [[ "$buildvcs" == true ]]; then
    metadata="$(go version -m "dist-dev/agentstack-windows-$arch.exe")"
    grep -q "vcs.revision=${revision#git:}" <<<"$metadata"
    grep -q 'vcs.modified=false' <<<"$metadata"
  fi
done
echo "Unsigned development binaries created in dist-dev from $revision. Public releases must use scripts/release.ps1 with protected signing and provenance gates."
