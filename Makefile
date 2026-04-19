.PHONY: all build build-master build-agent proto clean test run-master run-agent \
       release release-master release-agent dist

# Build output directory
BIN_DIR := bin
DIST_DIR := dist

# Version info (override with: make build VERSION=v1.0.0)
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT  ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "none")
DATE    ?= $(shell date -u +"%Y-%m-%dT%H:%M:%SZ" 2>/dev/null || echo "unknown")

# Linker flags to embed version info
LDFLAGS := -s -w \
  -X main.version=$(VERSION) \
  -X main.commit=$(COMMIT) \
  -X main.buildDate=$(DATE)

# CGO disabled for pure-Go cross compilation
export CGO_ENABLED=0

all: build

# ============================================================================
# Local Build (current OS/arch)
# ============================================================================

build: build-master build-agent

build-master:
	go build -ldflags "$(LDFLAGS)" -o $(BIN_DIR)/master.exe ./cmd/master

build-agent:
	go build -ldflags "$(LDFLAGS)" -o $(BIN_DIR)/agent.exe ./cmd/agent

# ============================================================================
# Cross-Compile Release Builds
# ============================================================================

# Build all platforms
release: release-linux-amd64 release-linux-arm64 release-windows-amd64 release-darwin-amd64 release-darwin-arm64

release-linux-amd64:
	GOOS=linux GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o $(DIST_DIR)/linux-amd64/craftstack-master ./cmd/master
	GOOS=linux GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o $(DIST_DIR)/linux-amd64/craftstack-agent ./cmd/agent

release-linux-arm64:
	GOOS=linux GOARCH=arm64 go build -ldflags "$(LDFLAGS)" -o $(DIST_DIR)/linux-arm64/craftstack-master ./cmd/master
	GOOS=linux GOARCH=arm64 go build -ldflags "$(LDFLAGS)" -o $(DIST_DIR)/linux-arm64/craftstack-agent ./cmd/agent

release-windows-amd64:
	GOOS=windows GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o $(DIST_DIR)/windows-amd64/craftstack-master.exe ./cmd/master
	GOOS=windows GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o $(DIST_DIR)/windows-amd64/craftstack-agent.exe ./cmd/agent

release-darwin-amd64:
	GOOS=darwin GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o $(DIST_DIR)/darwin-amd64/craftstack-master ./cmd/master
	GOOS=darwin GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o $(DIST_DIR)/darwin-amd64/craftstack-agent ./cmd/agent

release-darwin-arm64:
	GOOS=darwin GOARCH=arm64 go build -ldflags "$(LDFLAGS)" -o $(DIST_DIR)/darwin-arm64/craftstack-master ./cmd/master
	GOOS=darwin GOARCH=arm64 go build -ldflags "$(LDFLAGS)" -o $(DIST_DIR)/darwin-arm64/craftstack-agent ./cmd/agent

# Package release builds into zip archives
dist: release
	@mkdir -p $(DIST_DIR)
	cd $(DIST_DIR)/linux-amd64   && tar czf ../craftstack-linux-amd64.tar.gz .
	cd $(DIST_DIR)/linux-arm64   && tar czf ../craftstack-linux-arm64.tar.gz .
	cd $(DIST_DIR)/windows-amd64 && zip -q ../craftstack-windows-amd64.zip *
	cd $(DIST_DIR)/darwin-amd64  && tar czf ../craftstack-darwin-amd64.tar.gz .
	cd $(DIST_DIR)/darwin-arm64  && tar czf ../craftstack-darwin-arm64.tar.gz .
	@echo "Release archives created in $(DIST_DIR)/"

# ============================================================================
# Protobuf
# ============================================================================

proto:
	buf generate

proto-lint:
	buf lint

# ============================================================================
# Development
# ============================================================================

run-master:
	go run -ldflags "$(LDFLAGS)" ./cmd/master

run-agent:
	go run -ldflags "$(LDFLAGS)" ./cmd/agent

test:
	go test ./... -v -count=1

# ============================================================================
# Clean
# ============================================================================

clean:
	rm -rf $(BIN_DIR) $(DIST_DIR) gen/ data/
