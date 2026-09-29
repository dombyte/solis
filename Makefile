BINARY_NAME := solis

# Get version info
VERSION := $(shell git describe --tags --always 2>/dev/null || echo "dev")
COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
DATE := $(shell date -u +'%Y-%m-%dT%H:%M:%SZ')
GOVERSION := $(shell go version | awk '{print $$3}')

# Version shown in the web UI (frontend/public/data/version.json); goreleaser overrides it.
VITE_GIT_COMMIT_HASH ?= $(VERSION)

# Build flags for small binary
LDFLAGS := -s -w
BUILD_FLAGS := -ldflags "-X main.Version=$(VERSION) -X main.Commit=$(COMMIT) -X main.BuildDate=$(DATE) -X main.GoVersion=$(GOVERSION) $(LDFLAGS)"

.PHONY: build
build:
	CGO_ENABLED=0 go build $(BUILD_FLAGS) -o $(BINARY_NAME) ./cmd

# Web assets the binary serves from disk (./frontend/dist, ./docs/dist).
.PHONY: frontend
frontend:
	npm --prefix frontend ci
	VITE_GIT_COMMIT_HASH=$(VITE_GIT_COMMIT_HASH) npm --prefix frontend run build

.PHONY: docs
docs:
	npm --prefix docs ci
	npm --prefix docs run build

.PHONY: assets
assets: frontend docs

.PHONY: clean
clean:
	rm -f $(BINARY_NAME)
	rm -rf frontend/dist docs/dist

.PHONY: all
all: assets build

.PHONY: run
run: build
	./$(BINARY_NAME)

.PHONY: check
check:
	./scripts/pre-commit.sh
