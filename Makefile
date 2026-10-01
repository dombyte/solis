BINARY_NAME := solis

# Get version info
VERSION := $(shell git describe --tags --always 2>/dev/null || echo "dev")
COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
DATE := $(shell date -u +'%Y-%m-%dT%H:%M:%SZ')
GOVERSION := $(shell go version | awk '{print $$3}')

# Build flags for small binary
LDFLAGS := -s -w
BUILD_FLAGS := -ldflags "-X main.Version=$(VERSION) -X main.Commit=$(COMMIT) -X main.BuildDate=$(DATE) -X main.GoVersion=$(GOVERSION) $(LDFLAGS)"

# The web assets are built with the same version (VITE_APP_VERSION) as the binary; the
# frontend compares it with GET /api/version. goreleaser passes VERSION=<release version>.

# Embeds whatever frontend/dist and docs/dist hold: run `make assets` first (or `make all`).
.PHONY: build
build:
	CGO_ENABLED=0 go build $(BUILD_FLAGS) -o $(BINARY_NAME) ./cmd

# Web assets embedded into the binary (frontend/dist, docs/dist); build them before `build`.
.PHONY: frontend
frontend:
	npm --prefix frontend ci
	VITE_APP_VERSION=$(VERSION) npm --prefix frontend run build

.PHONY: docs
docs:
	npm --prefix docs ci
	VITE_APP_VERSION=$(VERSION) npm --prefix docs run build

.PHONY: assets
assets: frontend docs

# Dev image (docker-compose.dev.yaml) with the same build information as `make build`.
.PHONY: docker
docker:
	VERSION=$(VERSION) COMMIT=$(COMMIT) BUILD_DATE=$(DATE) \
		docker compose -f docker-compose.dev.yaml build

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
