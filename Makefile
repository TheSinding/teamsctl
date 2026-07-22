PREFIX ?= $(HOME)/.local
BUILD_DIR ?= bin
BINARY := teamsctl
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w -X thesinding/teamsctl/internal/version.Value=$(VERSION)

.PHONY: all build install uninstall test clean

all: build

build:
	mkdir -p "$(BUILD_DIR)"
	go build -trimpath -ldflags "$(LDFLAGS)" -o "$(BUILD_DIR)/$(BINARY)" ./cmd/teamsctl

install:
	PREFIX="$(PREFIX)" VERSION="$(VERSION)" ./scripts/install.sh

uninstall:
	rm -f "$(PREFIX)/bin/$(BINARY)"

test:
	go test ./...
	go vet ./...

clean:
	rm -rf "$(BUILD_DIR)"
