#!/bin/bash

set -e

echo "=== Running Pre-commit Checks ==="
echo ""

# Color codes
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Track failures
FAILED=0

# Helper function to print results
print_result() {
    if [ $1 -eq 0 ]; then
        echo -e "${GREEN}✓${NC} $2"
    else
        echo -e "${RED}✗${NC} $2"
        FAILED=1
    fi
}

# 1. Run gofmt
echo "Running gofmt..."
gofmt -w -s . 2>/dev/null
print_result $? "gofmt"

# 2. Run goimports
echo "Running goimports..."
if command -v goimports &> /dev/null; then
    goimports -w -d . 2>/dev/null
    print_result $? "goimports"
else
    echo -e "${YELLOW}⚠${NC} goimports not installed, skipping"
fi

# 3. Run golangci-lint
echo "Running golangci-lint..."
if command -v golangci-lint &> /dev/null; then
    golangci-lint run --config .golangci.yml --fix 2>/dev/null || true
    golangci-lint run --config .golangci.yml 2>/dev/null
    print_result $? "golangci-lint"
else
    echo -e "${YELLOW}⚠${NC} golangci-lint not installed, skipping"
fi

# 4. Run gosec
echo "Running gosec..."
if command -v gosec &> /dev/null; then
    gosec ./... 2>/dev/null
    print_result $? "gosec"
else
    echo -e "${YELLOW}⚠${NC} gosec not installed, skipping"
fi

# 5. Run staticcheck
echo "Running staticcheck..."
if command -v staticcheck &> /dev/null; then
    staticcheck ./... 2>/dev/null
    print_result $? "staticcheck"
else
    echo -e "${YELLOW}⚠${NC} staticcheck not installed, skipping"
fi

# 6. Check function length
echo "Checking function lengths..."
MAX_FUNC_LINES=40
LONG_FUNCS=$(awk -v MAX="$MAX_FUNC_LINES" '
FNR == 1 {
    in_func = 0
    lines = 0
}
/^func / {
    if (in_func && lines > MAX) {
        print FILENAME ": line " start ": function is " lines " lines (> " MAX ")"
    }
    in_func = 1
    start = FNR
    lines = 1
    next
}
in_func {
    lines++
}
END {
    if (in_func && lines > MAX) {
        print FILENAME ": line " start ": function is " lines " lines (> " MAX ")"
    }
}' $(find . -name "*.go" -not -name "*_test.go" -not -path "./.git/*" -not -path "./vendor/*") 2>/dev/null || true)
if [ -n "$LONG_FUNCS" ]; then
    echo "Found functions exceeding $MAX_FUNC_LINES lines:"
    echo "$LONG_FUNCS"
    print_result 1 "function length check"
else
    print_result 0 "function length check"
fi

# 7. Check line length
echo "Checking line lengths..."
LONG_LINES=$(awk 'length > 100 {print FILENAME":"FNR": Line too long ("length" chars)}' $(find . -name "*.go" -not -name "*_test.go" -not -path "./.git/*" -not -path "./vendor/*") 2>/dev/null || true)
if [ -n "$LONG_LINES" ]; then
    echo "Found lines exceeding 100 characters:"
    echo "$LONG_LINES"
    print_result 1 "line length check"
else
    print_result 0 "line length check"
fi


# 8. Run tests
echo "Running tests..."
if go test ./... 2>/dev/null; then
    print_result 0 "tests"
else
    print_result 1 "tests"
fi



# 9. Build check
echo "Building..."
if go build ./... 2>/dev/null; then
    print_result 0 "build"
else
    print_result 1 "build"
fi

# 10. Check for global state
echo "Checking for global state..."
GLOBAL_VARS=$(grep -rn "^var [a-z]" --include="*.go" . | grep -v "_test.go" | grep -v "// " | grep -v "mock" | grep -v "^var [a-z].*=.*$" || true)
if [ -n "$GLOBAL_VARS" ]; then
    echo "Found package-level global state:"
    echo "$GLOBAL_VARS"
    print_result 1 "global state check"
else
    print_result 0 "global state check"
fi

# 11. Check for panic usage
echo "Checking for panic usage..."
PANICS=$(grep -rn "panic(" --include="*.go" . | grep -v "_test.go" | grep -v "recover" | grep -v "// " || true)
if [ -n "$PANICS" ]; then
    echo "Found panic calls:"
    echo "$PANICS"
    print_result 1 "panic check"
else
    print_result 0 "panic check"
fi

echo ""
echo "=== Pre-commit Checks Complete ==="

if [ $FAILED -ne 0 ]; then
    echo -e "${RED}Some checks failed. Please fix the issues before committing.${NC}"
    exit 1
else
    echo -e "${GREEN}All checks passed!${NC}"

    # Add files if they were modified by formatting
    git add -u
    exit 0
fi
