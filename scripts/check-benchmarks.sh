#!/usr/bin/env bash
set -euo pipefail
output=${1:-benchmark-results.txt}
packages=(./internal/mcp ./internal/planner ./internal/redact ./internal/ui)
if [[ "${BENCHMARK_INPUT_ONLY:-0}" == "1" ]]; then
  [[ -s "$output" ]] || { echo "benchmark results file is missing or empty: $output" >&2; exit 2; }
else
  go test -run '^$' -bench 'Benchmark(ChildListTools|BuildLargeCatalog|RedactText|OperationStoreGet)$' -benchmem -benchtime=500ms -count=5 "${packages[@]}" | tee "$output"
fi
for benchmark in BenchmarkChildListTools BenchmarkBuildLargeCatalog BenchmarkRedactText BenchmarkOperationStoreGet; do
  count=$(grep -c "^${benchmark}\([\/-]\)" "$output" || true)
  if (( count < 5 )); then
    echo "benchmark evidence missing for $benchmark: expected 5 samples, found $count" >&2
    exit 1
  fi
done

require_max_ns() {
  local prefix=$1
  local maximum=$2
  local observed
  observed=$(awk -v prefix="$prefix" '
    index($1, prefix) == 1 && $4 == "ns/op" {
      if ($3 + 0 > max + 0) max = $3
      found = 1
    }
    END {
      if (!found) exit 2
      printf "%.3f", max
    }
  ' "$output") || {
    echo "benchmark latency evidence missing for $prefix" >&2
    exit 1
  }
  awk -v observed="$observed" -v maximum="$maximum" -v label="$prefix" 'BEGIN {
    if (observed + 0 > maximum + 0) {
      printf "benchmark latency %s is %.3f ns/op, above %.3f ns/op\n", label, observed, maximum > "/dev/stderr"
      exit 1
    }
  }'
  printf 'benchmark latency %-38s max %.3f ns/op (ceiling %.3f)\n' "$prefix" "$observed" "$maximum"
}

# Ceilings are intentionally several times slower than the accepted baseline.
# They catch order-of-magnitude regressions without pretending microbenchmarks
# are stable enough for narrow cross-run comparisons on shared CI hosts.
require_max_ns 'BenchmarkChildListTools/one-shot-' 10000000
require_max_ns 'BenchmarkChildListTools/persistent-' 1000000
require_max_ns 'BenchmarkBuildLargeCatalog-' 5000000
require_max_ns 'BenchmarkRedactText-' 250000
require_max_ns 'BenchmarkOperationStoreGet-' 500
echo "benchmark evidence gate passed: $output"
