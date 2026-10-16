#!/usr/bin/env bash
# Run the benchmark suite with real measurements into results/.
set -euo pipefail
cd "$(dirname "$0")/.."

API="${CLEANPOINT_API:-http://127.0.0.1:8099}"
mkdir -p results

if ! curl -s "$API/api/v1/healthz" >/dev/null 2>&1; then
  echo "start the server first: scripts/demo.sh or cleanpoint serve" >&2
  exit 1
fi

echo "==> snapshot capture benchmark (1000 x 32 KiB)"
./bin/cleanpoint.exe benchmark snapshots --files 1000 --file-size-kb 32 \
  --iterations 3 --api "$API" --output results/snapshot-1000.json

echo "==> restore benchmark (1000 x 32 KiB)"
./bin/cleanpoint.exe benchmark restore --files 1000 --file-size-kb 32 \
  --iterations 3 --api "$API" --output results/restore-1000.json

echo "==> API latency (needs a workload from a prior demo/eval run)"
WL=$(curl -s -H "Authorization: Bearer $(grep '^operator=' sandbox/.tokens | cut -d= -f2)" \
  "$API/api/v1/workloads" | python -c "import json,sys; ws=json.load(sys.stdin)['workloads']; print(ws[-1]['id'] if ws else '')")
if [ -n "$WL" ]; then
  python benchmarks/latency.py --api "$API" --workload "$WL" --samples 20 \
    --output results/latency.json
else
  echo "(no workloads yet; skipping latency probe)"
fi

echo "benchmarks complete; see results/"
