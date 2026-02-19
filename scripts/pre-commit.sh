#!/usr/bin/env bash
set -euo pipefail

echo "🔍 Running pre-commit checks..."

# Check gofmt
UNFORMATTED=$(gofmt -l . 2>&1 || true)
if [ -n "$UNFORMATTED" ]; then
    echo "❌ The following files need gofmt:"
    echo "$UNFORMATTED"
    echo "Run: gofmt -w ."
    exit 1
fi
echo "✅ gofmt passed"

# Run go vet
if ! go vet ./... 2>&1; then
    echo "❌ go vet failed"
    exit 1
fi
echo "✅ go vet passed"

# Run short tests
if ! go test ./... -short -count=1 2>&1; then
    echo "❌ tests failed"
    exit 1
fi
echo "✅ tests passed"

echo "🎉 All pre-commit checks passed!"
