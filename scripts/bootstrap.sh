#!/usr/bin/env bash
# One-command local bootstrap: build everything and verify the toolchain.
set -euo pipefail
cd "$(dirname "$0")/.."

echo "==> toolchain"
go version
python --version

echo "==> building"
go build -o bin/cleanpoint.exe ./cmd/cli
go build -o bin/cleanpoint-server.exe ./cmd/server
go build -o bin/cleanpoint-simulator.exe ./cmd/simulator

echo "==> tidy"
go mod tidy

echo "bootstrap complete; binaries in ./bin"
