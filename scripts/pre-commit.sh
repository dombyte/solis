#!/bin/bash
# Pre-commit checks. golangci-lint (.golangci.yml) covers formatting (gofumpt, goimports),
# line/function length, complexity, gosec, staticcheck, global state and the panic
# policy; only what it cannot do runs separately (deadcode, govulncheck, build, tests).
# Check-only: it never rewrites or stages files. Fix formatting with
# `golangci-lint fmt` and lint findings by hand. Mock drift is checked in CI.

set -u

echo "=== Running Pre-commit Checks ==="
echo ""

# Color codes
RED='\033[0;31m'
GREEN='\033[0;32m'
NC='\033[0m' # No Color

# Track failures
FAILED=0

# Helper function to print results
print_result() {
    if [ "$1" -eq 0 ]; then
        echo -e "${GREEN}✓${NC} $2"
    else
        echo -e "${RED}✗${NC} $2"
        FAILED=1
    fi
}

if ! command -v golangci-lint &> /dev/null; then
    echo -e "${RED}✗${NC} golangci-lint not installed (https://golangci-lint.run)"
    exit 1
fi

# 1. Format (gofumpt + goimports): report, never rewrite
echo "Checking formatting..."
FMT=$(golangci-lint fmt --diff --config .golangci.yml 2>&1)
if [ -n "$FMT" ]; then
    echo "$FMT"
    print_result 1 "golangci-lint fmt (run 'golangci-lint fmt' to fix)"
else
    print_result 0 "golangci-lint fmt"
fi

# 2. Lint
echo "Linting..."
golangci-lint run --config .golangci.yml
print_result $? "golangci-lint run"

# 3. Unused exported code (golangci's unused only sees unexported identifiers)
echo "Checking for dead code..."
DEAD=$(go run golang.org/x/tools/cmd/deadcode@v0.50.0 -test ./... 2>&1)
if [ -n "$DEAD" ]; then
    echo "$DEAD"
    print_result 1 "deadcode"
else
    print_result 0 "deadcode"
fi

# 4. Known vulnerabilities in dependencies and the standard library
echo "Checking for vulnerabilities..."
go run golang.org/x/vuln/cmd/govulncheck@v1.8.0 ./...
print_result $? "govulncheck"

# 5. Build
echo "Building..."
go build ./...
print_result $? "build"

# 6. Tests (with the race detector, as in CI)
echo "Running tests..."
go test -race ./...
print_result $? "tests"

echo ""
echo "=== Pre-commit Checks Complete ==="

if [ $FAILED -ne 0 ]; then
    echo -e "${RED}Some checks failed. Please fix the issues before committing.${NC}"
    exit 1
fi
echo -e "${GREEN}All checks passed!${NC}"
exit 0
