#!/usr/bin/env bash
# The checks CI runs, in the order that fails fastest. Run before every commit.
# GOLANGCI_LINT=/path/to/golangci-lint picks a specific binary (see docs/agents/code-layout.md).
set -euo pipefail
cd "$(dirname "$0")/.."

unformatted=$(gofmt -l .)
if [ -n "$unformatted" ]; then
  echo "gofmt needed:" >&2
  echo "$unformatted" >&2
  exit 1
fi
go vet ./...
go test ./...
"${GOLANGCI_LINT:-golangci-lint}" run
