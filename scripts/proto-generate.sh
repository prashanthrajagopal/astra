#!/usr/bin/env bash
# Regenerate Go code from .proto files. Run from repo root.
set -e
cd "$(dirname "$0")/.."
export PATH="$(go env GOPATH)/bin:$PATH"
if ! command -v buf &>/dev/null; then
  echo "buf not found. Install with: go install github.com/bufbuild/buf/cmd/buf@latest"
  exit 1
fi
buf generate
