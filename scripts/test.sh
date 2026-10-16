#!/usr/bin/env bash
# Run the full automated test suite (>= 92 cases) plus the Python harness.
set -euo pipefail
cd "$(dirname "$0")/.."

echo "==> go vet"
go vet ./...

echo "==> go tests (the mandated matrix)"
go test ./... -count=1

echo "==> case count (must be >= 92)"
CASES=$(go test ./tests/... -v -count=1 2>/dev/null | grep -cE '^    --- PASS')
echo "automated cases passing: ${CASES}"
if [ "${CASES}" -lt 92 ]; then
  echo "FAIL: below the source-mandated minimum of 92 cases" >&2
  exit 1
fi

if command -v python >/dev/null 2>&1; then
  echo "==> python generator determinism check"
  python - <<'EOF'
import sys, os, json, hashlib
sys.path.insert(0, "simulator")
from generate import build_timeline
a, _ = build_timeline("rapid_destructive", 77)
b, _ = build_timeline("rapid_destructive", 77)
ha = hashlib.sha256(json.dumps(a, sort_keys=True).encode()).hexdigest()
hb = hashlib.sha256(json.dumps(b, sort_keys=True).encode()).hexdigest()
assert ha == hb, "generator is not deterministic!"
print("generator determinism: OK (%d events)" % len(a))
EOF
else
  echo "!! python not found; skipping generator determinism check"
fi

echo "ALL TESTS PASS"
