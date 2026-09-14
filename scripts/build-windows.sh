#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/../server"
GOOS=windows GOARCH=amd64 go build -ldflags="-s -w" -o ../dist/photoserver.exe ./cmd/photoserver
echo "wrote dist/photoserver.exe"
