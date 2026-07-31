#!/usr/bin/env bash
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"
if [[ -n "$(git status --porcelain=v1 --untracked-files=all)" ]]; then
  echo "build.sh requires a clean tree" >&2
  exit 1
fi
version="${VERSION:-dev}"
revision="$(git rev-parse HEAD)"
rm -rf dist-dev
mkdir -p dist-dev
go test ./...
go test -race ./...
go vet ./...
flags="-s -w -buildid= -X main.version=$version"
for arch in amd64 arm64; do
  CGO_ENABLED=0 GOOS=windows GOARCH="$arch" go build -trimpath -buildvcs=true -ldflags="$flags" -o "dist-dev/agentstack-windows-$arch.exe" ./cmd/agentstack
  metadata="$(go version -m "dist-dev/agentstack-windows-$arch.exe")"
  grep -q "vcs.revision=$revision" <<<"$metadata"
  grep -q 'vcs.modified=false' <<<"$metadata"
done
echo "Unsigned development binaries created in dist-dev. Public releases must use scripts/release.ps1 with Authenticode credentials."
