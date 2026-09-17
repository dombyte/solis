BINARY_NAME := solis

# Get version info
VERSION := $(shell git describe --tags --always 2>/dev/null || echo "dev")
COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
DATE := $(shell date -u +'%Y-%m-%dT%H:%M:%SZ')
GOVERSION := $(shell go version | awk '{print $$3}')

# Build flags for small binary
LDFLAGS := -s -w
BUILD_FLAGS := -ldflags "-X main.Version=$(VERSION) -X main.Commit=$(COMMIT) -X main.BuildDate=$(DATE) -X main.GoVersion=$(GOVERSION) $(LDFLAGS)"

.PHONY: build
build:
	CGO_ENABLED=0 go build $(BUILD_FLAGS) -o $(BINARY_NAME) ./cmd

.PHONY: clean
clean:
	rm -f $(BINARY_NAME)

.PHONY: all
all: build

.PHONY: run
run: build
	./$(BINARY_NAME) serve -c configs/config.yaml

.PHONY: check
check:
	./scripts/pre-commit.sh
