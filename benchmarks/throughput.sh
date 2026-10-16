#!/usr/bin/env bash
# CleanPoint API throughput probe at concurrency 1 / 5 / 10 / 20.
# Each worker counts completed GET /recovery-candidates requests for the
# window; the script reports requests/second per level.
#
#   ./throughput.sh [duration_seconds_per_level] [results_file]
set -euo pipefail

API="${CLEANPOINT_API:-http://127.0.0.1:8080}"
DUR="${1:-10}"
OUT="${2:-results/throughput.json}"
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
TOKEN_FILE="${CLEANPOINT_SANDBOX_ROOT:-$ROOT/sandbox}/.tokens"
[[ -f "$TOKEN_FILE" ]] || TOKEN_FILE="$ROOT/sandbox/.tokens"
TOKEN="$(grep '^operator=' "$TOKEN_FILE" | cut -d= -f2)"

WL=$(curl -s -H "Authorization: Bearer $TOKEN" "$API/api/v1/workloads" | \
     python -c "import json,sys; ws=json.load(sys.stdin)['workloads']; print(ws[0]['id'] if ws else '')")
if [[ -z "$WL" ]]; then
  echo "no workloads registered; run the demo first" >&2
  exit 1
fi
URL="$API/api/v1/workloads/$WL/recovery-candidates"
echo "workload: $WL | endpoint: GET /recovery-candidates | ${DUR}s per level | api: $API"

TMPDIR_RUN="$(mktemp -d)"
trap 'rm -rf "$TMPDIR_RUN"' EXIT

worker() { # $1 = worker id
  local end=$(( $(date +%s) + DUR )) count=0
  while (( $(date +%s) < end )); do
    curl -s -o /dev/null -H "Authorization: Bearer $TOKEN" "$URL" && count=$((count+1))
  done
  echo "$count" > "$TMPDIR_RUN/w$1"
}

echo "{" > "$TMPDIR_RUN/result.json"
first=1
for LEVEL in 1 5 10 20; do
  pids=()
  for i in $(seq 1 "$LEVEL"); do
    worker "$i" & pids+=($!)
  done
  wait "${pids[@]}" 2>/dev/null
  TOTAL=0
  for i in $(seq 1 "$LEVEL"); do
    c=$(cat "$TMPDIR_RUN/w$i" 2>/dev/null || echo 0)
    TOTAL=$((TOTAL + c))
  done
  RPS=$(python -c "print(round($TOTAL / $DUR, 2))")
  echo "concurrency=$LEVEL  requests=${TOTAL}  rps=${RPS}"
  [[ $first -eq 1 ]] || echo "," >> "$TMPDIR_RUN/result.json"
  first=0
  printf '  "concurrency_%s": {"requests": %s, "rps": %s}' "$LEVEL" "$TOTAL" "$RPS" >> "$TMPDIR_RUN/result.json"
done
echo "" >> "$TMPDIR_RUN/result.json"
echo "}" >> "$TMPDIR_RUN/result.json"

mkdir -p "$(dirname "$OUT")"
{ echo "{\"api\": \"$API\", \"endpoint\": \"GET /api/v1/workloads/$WL/recovery-candidates\", \"duration_seconds_per_level\": $DUR, \"levels\":"
  cat "$TMPDIR_RUN/result.json"
  echo "}"
} > "$OUT"
echo "results written to $OUT"
