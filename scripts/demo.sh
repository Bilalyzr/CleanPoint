#!/usr/bin/env bash
# One-command demo: start the local server (if needed) and run the guided
# three-scene demonstration.
set -euo pipefail
cd "$(dirname "$0")/.."

API="${CLEANPOINT_API:-http://127.0.0.1:8099}"
export CLEANPOINT_API="$API"

start_server() {
  echo "==> starting CleanPoint server on ${API}"
  CLEANPOINT_ADDR="${API#http://}" ./bin/cleanpoint-server.exe > sandbox/server.log 2>&1 &
  echo $! > sandbox/server.pid
  for _ in $(seq 1 20); do
    if curl -s "$API/api/v1/healthz" >/dev/null 2>&1; then return 0; fi
    sleep 0.5
  done
  echo "server failed to start; see sandbox/server.log" >&2
  exit 1
}

mkdir -p sandbox
if ! curl -s "$API/api/v1/healthz" >/dev/null 2>&1; then
  [ -x ./bin/cleanpoint-server.exe ] || scripts/bootstrap.sh
  start_server
fi

echo "==> running guided demo"
python simulator/demo.py --api "$API"

echo
echo "demo complete. artifacts under sandbox/ (restores in sandbox/restored/)."
echo "stop the server with: kill \$(cat sandbox/server.pid)"
