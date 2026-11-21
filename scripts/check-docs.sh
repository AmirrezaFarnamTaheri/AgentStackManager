#!/usr/bin/env bash
set -euo pipefail
root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$root"
required=(
  README.md
  docs/CLI_REFERENCE.md
  docs/USER_GUIDE.md
  docs/SECURITY.md
  docs/PRIVACY.md
  docs/OPERATIONS.md
  docs/SUPPLY_CHAIN.md
  docs/THREAT_MODEL.md
  docs/GOVERNANCE.md
  docs/RELEASE.md
  docs/architecture.md
  docs/UX_DESIGN.md
  docs/CONVERGENCE.md
  docs/convergence/DONOR_ANALYSIS.md
  docs/convergence/TRUST_AND_STATE.md
  docs/convergence/OMISSION_AUDIT.md
  docs/convergence/PREMORTEM.md
  docs/convergence/VALIDATION.md
  docs/convergence/RUNBOOK.md
  docs/convergence/ADOPTION.csv
  docs/convergence/SURFACES.csv
  docs/convergence/TEST_TRACEABILITY.csv
  docs/audit/ASM-001-040-closure.md
  docs/audit/EXTERNAL-REPORT-ACCEPTED-ITEMS.md
  docs/audit/EXTERNAL-REPORT-ACCEPTED-ITEMS.json
)
for file in "${required[@]}"; do
  [[ -s "$file" ]] || { echo "missing or empty documentation: $file" >&2; exit 1; }
done
ledger='docs/audit/ASM-001-040-closure.md'
for number in $(seq -w 1 40); do
  count="$(grep -c "| ASM-0${number} |" "$ledger" || true)"
  [[ "$count" -eq 1 ]] || { echo "closure ledger must contain ASM-0${number} exactly once; found $count" >&2; exit 1; }
done
if grep -Eq '\| ASM-[0-9]{3} \| (Open|Unaddressed|Deferred|Rejected) \|' "$ledger"; then
  echo 'closure ledger contains an unresolved finding status' >&2
  exit 1
fi
corpus=(README.md docs/*.md docs/audit/*.md)
if grep -RInE 'agentstack apply --profile|terminates (them|children) after (each|the) (request|call)|Linux amd64 console CLI|profile essential\|recommended' "${corpus[@]}"; then
  echo "stale documentation contract detected" >&2
  exit 1
fi
for required_text in \
  'agentstack apply --plan-id' \
  'agentstack backup restore --id' \
  'agentstack data policy' \
  'agentstack owned remove' \
  'Go 1.26.5' \
  'Authenti' \
  'Windows Job Object resource ceilings' \
  'operation-status surface'; do
  grep -Rqs "$required_text" "${corpus[@]}" || { echo "required documentation text missing: $required_text" >&2; exit 1; }
done
profile_count="$(jq '.profiles | length' internal/catalog/default.json)"
[[ "$profile_count" -eq 10 ]] || { echo "unexpected profile count: $profile_count" >&2; exit 1; }
for profile in $(jq -r '.profiles[].id' internal/catalog/default.json); do
  grep -Rqs -- "\`$profile\`" README.md docs/CLI_REFERENCE.md docs/USER_GUIDE.md || {
    echo "profile missing from user documentation: $profile" >&2
    exit 1
  }
done
echo "Documentation contracts passed."
