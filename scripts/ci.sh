#!/bin/bash
# CI pipeline for CodeButler
# Runs: vet, tests, coverage, benchmarks
set -euo pipefail

echo "=== CodeButler CI Pipeline ==="
echo ""

# Step 1: Go vet
echo "--- Step 1: go vet ---"
go vet ./...
echo "PASS: vet"
echo ""

# Step 2: Tests
echo "--- Step 2: go test ---"
go test ./... -count=1
echo "PASS: tests"
echo ""

# Step 3: Coverage
echo "--- Step 3: Coverage ---"
go test ./... -coverprofile=coverage.out -covermode=atomic 2>/dev/null
total=$(go tool cover -func=coverage.out | grep total | awk '{print $3}')
echo "Total coverage: $total"

# Check minimum coverage threshold
threshold="75.0"
coverage_num=$(echo "$total" | tr -d '%')
if [ "$(echo "$coverage_num < $threshold" | bc -l 2>/dev/null || echo 0)" = "1" ]; then
    echo "WARNING: Coverage $total below threshold $threshold%"
fi
echo ""

# Step 4: Benchmarks (quick run)
echo "--- Step 4: Benchmarks ---"
go test ./internal/integration/ -bench=. -benchtime=100ms -benchmem 2>&1 | grep -E "^(Benchmark|ok)"
echo ""

# Step 5: Build
echo "--- Step 5: Build ---"
go build ./cmd/codebutler
echo "PASS: build"
echo ""

echo "=== CI Pipeline Complete ==="

# Cleanup
rm -f coverage.out codebutler
