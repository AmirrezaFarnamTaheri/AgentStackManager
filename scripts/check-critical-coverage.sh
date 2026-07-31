#!/usr/bin/env bash
set -euo pipefail
profile=${1:-coverage.out}
if [[ ! -f "$profile" ]]; then
  echo "coverage profile not found: $profile" >&2
  exit 2
fi
report=$(mktemp)
trap 'rm -f "$report"' EXIT
go tool cover -func="$profile" > "$report"

total=$(awk '/^total:/ {gsub(/%/,"",$NF); print $NF}' "$report")
awk -v value="$total" 'BEGIN { if (value+0 < 60.0) { printf "total coverage %.1f%% is below 60.0%%\n", value > "/dev/stderr"; exit 1 } }'

require_function() {
  local pattern=$1
  local minimum=$2
  local line
  line=$(grep -E "$pattern" "$report" | head -n 1 || true)
  if [[ -z "$line" ]]; then
    echo "critical coverage target missing: $pattern" >&2
    exit 1
  fi
  local value
  value=$(awk '{gsub(/%/,"",$NF); print $NF}' <<<"$line")
  awk -v value="$value" -v minimum="$minimum" -v label="$pattern" 'BEGIN { if (value+0 < minimum+0) { printf "critical coverage %s is %.1f%%, below %.1f%%\n", label, value, minimum > "/dev/stderr"; exit 1 } }'
}

require_function 'internal/app/service.go:.*ApplyPlanned' 65
require_function 'internal/app/service.go:.*MCPDoctor' 80
require_function 'internal/app/service.go:.*RestoreBackup' 70
require_function 'internal/app/lifecycle.go:.*RemoveOwned' 50
require_function 'internal/runner/runner.go:.*[[:space:]]Run[[:space:]]' 85
require_function 'internal/inventory/inventory.go:.*[[:space:]]Run[[:space:]]' 70
require_function 'internal/session/session.go:.*[[:space:]]Run[[:space:]]' 70
require_function 'internal/ui/server.go:.*[[:space:]]Run[[:space:]]' 50
require_function 'internal/processctl/process.go:.*GracefulClose' 50
require_function 'internal/selfinstall/install.go:.*InstallFrom' 65

echo "critical coverage gate passed (total ${total}%)"
