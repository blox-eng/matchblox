#!/usr/bin/env bash
# The checks CI runs, in the same order, for a run before you push.
set -euo pipefail
cd "$(dirname "$0")/.."

echo "gofmt";  test -z "$(gofmt -l .)" || { gofmt -l .; echo "run gofmt -w ."; exit 1; }
echo "vet";    go vet ./...
if command -v golangci-lint >/dev/null; then echo "lint"; golangci-lint run --timeout=5m; fi
echo "test";   go test ./...
echo "smoke";  go run ./cmd/matchblox status --fixtures testdata/machine --text >/dev/null
echo "cross";  for t in linux/amd64 linux/arm64 darwin/amd64 darwin/arm64 windows/amd64; do
  GOOS=${t%/*} GOARCH=${t#*/} CGO_ENABLED=0 go build -o /dev/null ./cmd/matchblox
done
echo "OK"
