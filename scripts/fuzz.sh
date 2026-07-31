#!/usr/bin/env bash
set -euo pipefail
duration="${1:-30s}"
parallel="${FUZZ_PARALLEL:-1}"
test_timeout="${FUZZ_TIMEOUT:-2m}"

run_fuzz() {
  local package=$1
  local target=$2
  go test "$package" -run='^$' -fuzz="$target" -fuzztime="$duration" -parallel="$parallel" -timeout="$test_timeout"
}

# A single fuzz worker keeps campaigns deterministic and prevents one
# pathological input from being multiplied across all available CPUs. Each
# target also has a hard test timeout so a stuck case fails visibly in CI.
run_fuzz ./internal/catalog FuzzCatalogDecode
run_fuzz ./internal/mcp FuzzRouterConfigJSON
run_fuzz ./internal/mcp FuzzRegistrationMap
run_fuzz ./internal/planner FuzzPlannerPreservation
run_fuzz ./internal/state FuzzSafeName
run_fuzz ./internal/safefile FuzzReplacePreservesNewContent
