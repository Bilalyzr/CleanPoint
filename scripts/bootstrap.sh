#!/usr/bin/env bash
# One-command local bootstrap: build everything and verify the toolchain.
set -euo pipefail
cd "$(dirname "$0")/.."

echo "==> toolchain"
go version
python --version

if command -v npm >/dev/null 2>&1; then
  echo "==> building M2 web console (web/console -> web/console-dist)"
  (cd web/console && ([ -d node_modules ] || npm ci) && npm run build)
else
  echo "==> npm not found; embedding prebuilt web/console-dist"
fi

echo "==> building Go binaries"
rm -f bin/*.exe~
go build -o bin/cleanpoint.exe ./cmd/cli
go build -o bin/cleanpoint-server.exe ./cmd/server
go build -o bin/cleanpoint-simulator.exe ./cmd/simulator

echo "==> tidy"
go mod tidy

echo "bootstrap complete; binaries in ./bin"
