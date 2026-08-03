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
awk -v value="$total" 'BEGIN { if (value+0 < 63.0) { printf "total coverage %.1f%% is below 63.0%%\n", value > "/dev/stderr"; exit 1 } }'

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

require_function 'internal/app/service.go:.*ApplyPlanned' 75
require_function 'internal/app/service.go:.*MCPDoctor' 80
require_function 'internal/app/service.go:.*RestoreBackup' 70
require_function 'internal/app/lifecycle.go:.*RemoveOwned' 50
require_function 'internal/runner/runner.go:.*[[:space:]]Run[[:space:]]' 85
require_function 'internal/inventory/inventory.go:.*[[:space:]]Run[[:space:]]' 70
require_function 'internal/session/session.go:.*[[:space:]]Run[[:space:]]' 70
require_function 'internal/supervisor/runtime.go:.*[[:space:]]Run[[:space:]]' 85
require_function 'internal/reviewedplan/executor.go:.*[[:space:]]Execute[[:space:]]' 70
require_function 'internal/releasepack/source.go:.*PackVerifiedSource' 60
require_function 'internal/ui/server.go:.*[[:space:]]Run[[:space:]]' 50
require_function 'internal/ui/operations.go:.*[[:space:]]start[[:space:]]' 70
require_function 'internal/mcp/child_client.go:.*operationContext' 70
require_function 'internal/state/data.go:.*SanitizeEvent' 100
require_function 'internal/skills/installer.go:.*ValidateSkillInventory' 80
require_function 'internal/catalog/catalog.go:.*validateRouterAcquisition' 70
require_function 'internal/planner/planner.go:.*[[:space:]]Build[[:space:]]' 80
require_function 'internal/processctl/process.go:.*GracefulClose' 50
if [[ "$(go env GOOS)" != "windows" ]]; then
  require_function 'internal/processctl/process_unix.go:.*[[:space:]]terminate[[:space:]]' 70
fi
require_function 'internal/safefile/replace.go:.*[[:space:]]Replace[[:space:]]' 30
require_function 'internal/pathenv/path.go:.*MergeWindows' 80
require_function 'internal/pathenv/path.go:.*AppendWindows' 75
require_function 'internal/pathenv/transport.go:.*EncodeWindowsString' 90
require_function 'internal/pathenv/transport.go:.*DecodeWindowsString' 75
require_function 'internal/selfinstall/install.go:.*InstallFrom' 65

# Unified fabric boundaries.
require_function 'internal/resourcehub/audit.go:.*[[:space:]]Audit[[:space:]]' 65
require_function 'internal/resourcehub/sync.go:.*PlanSync' 50
require_function 'internal/resourcehub/sync.go:.*ApplySync' 50
require_function 'internal/resourcehub/refresh.go:.*ApplyRefresh' 65
require_function 'internal/resourcehub/manager.go:.*RestoreBackup' 65
require_function 'internal/contextengine/scan.go:.*[[:space:]]Scan[[:space:]]' 75
require_function 'internal/contextengine/score.go:.*[[:space:]]Score[[:space:]]' 85
require_function 'internal/contextengine/knowledge.go:.*ReadFile' 70
require_function 'internal/contextengine/knowledge.go:.*[[:space:]]Search[[:space:]]' 60
require_function 'internal/contextengine/knowledge.go:.*[[:space:]]Git[[:space:]]' 75
require_function 'internal/contextengine/refresh.go:.*PlanRefresh' 80
require_function 'internal/contextengine/refresh.go:.*ApplyRefresh' 65
require_function 'internal/workspace/store.go:.*[[:space:]]Create[[:space:]]' 70
require_function 'internal/workspace/store.go:.*[[:space:]]Delete[[:space:]]' 60
require_function 'internal/workspace/memory.go:.*[[:space:]]Remember[[:space:]]' 65
require_function 'internal/workspace/memory.go:.*[[:space:]]Recall[[:space:]]' 80
require_function 'internal/workspace/memory.go:.*SearchMemory' 60
require_function 'internal/workspace/prompt.go:.*RenderPrompt' 85
require_function 'internal/workspace/artifact.go:.*AddArtifact' 60
require_function 'internal/workspace/artifact.go:.*VerifyArtifact' 65
require_function 'internal/mcplink/manager.go:.*[[:space:]]Plan[[:space:]]' 70
require_function 'internal/mcplink/manager.go:.*[[:space:]]Apply[[:space:]]' 65
require_function 'internal/routines/manager.go:.*[[:space:]]Run[[:space:]]' 80
require_function 'internal/routines/manager.go:.*ListRuns' 70
require_function 'internal/routines/manager.go:.*reconcileRuns' 80
require_function 'internal/routines/schedule.go:.*NextRun' 55

echo "critical coverage gate passed (total ${total}%)"
