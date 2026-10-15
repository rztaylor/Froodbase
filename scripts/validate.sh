#!/usr/bin/env bash
# Runs the full local validation gate: vet, lint, tests with race detector, build.
# Usage: scripts/validate.sh
# Requires: Go (see go.mod), golangci-lint v2.
set -euo pipefail
cd "$(dirname "$0")/.."

export GOCACHE="${GOCACHE:-$PWD/.cache/go-build}"
export GOFLAGS="${GOFLAGS:--mod=mod}"
# Pure Go: no C compiler needed; keeps macOS from asking for Xcode's tools.
export CGO_ENABLED=0

echo "==> go vet"
go vet ./...

echo "==> golangci-lint"
if command -v golangci-lint >/dev/null 2>&1; then
  golangci-lint run ./...
else
  echo "golangci-lint not installed; skipping lint (CI runs it)" >&2
fi

echo "==> go test"
go test -race -cover ./...

echo "==> go build"
make build

echo "validation passed"
