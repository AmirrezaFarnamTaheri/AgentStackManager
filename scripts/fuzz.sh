#!/usr/bin/env bash
set -euo pipefail
duration="${1:-30s}"
go test ./internal/catalog -run='^$' -fuzz=FuzzCatalogDecode -fuzztime="$duration"
go test ./internal/mcp -run='^$' -fuzz=FuzzRouterConfigJSON -fuzztime="$duration"
go test ./internal/mcp -run='^$' -fuzz=FuzzRegistrationMap -fuzztime="$duration"
go test ./internal/planner -run='^$' -fuzz=FuzzPlannerPreservation -fuzztime="$duration"
go test ./internal/state -run='^$' -fuzz=FuzzSafeName -fuzztime="$duration"
go test ./internal/safefile -run='^$' -fuzz=FuzzReplacePreservesNewContent -fuzztime="$duration"
