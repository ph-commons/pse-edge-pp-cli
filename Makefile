.PHONY: build test lint install clean build-mcp install-mcp build-all install-all

BIN_EXT := $(if $(filter windows,$(shell go env GOOS)),.exe,)

# Prefer an explicit VERSION=…; else git describe (strip leading v). Falls back
# to 0.0.0-dev when git metadata is unavailable (e.g. source tarball).
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null | sed 's/^v//' || echo 0.0.0-dev)
LDFLAGS := -s -w -X github.com/ngpestelos/pse-edge-pp-cli/internal/cli.version=$(VERSION)

build:
	go build -ldflags "$(LDFLAGS)" -o bin/pse-edge-pp-cli$(BIN_EXT) ./cmd/pse-edge-pp-cli

test:
	go test ./...

lint:
	golangci-lint run

install:
	go install -ldflags "$(LDFLAGS)" ./cmd/pse-edge-pp-cli

clean:
	rm -rf bin/

build-mcp:
	go build -ldflags "$(LDFLAGS)" -o bin/pse-edge-pp-mcp$(BIN_EXT) ./cmd/pse-edge-pp-mcp

install-mcp:
	go install -ldflags "$(LDFLAGS)" ./cmd/pse-edge-pp-mcp

build-all: build build-mcp

install-all: install install-mcp
