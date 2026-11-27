GO ?= go
VERSION ?= $(shell git describe --tags --abbrev=0 2>/dev/null || echo dev)
LDFLAGS := -s -w -X github.com/gongjs/agym/internal/updater.Version=$(VERSION)

.PHONY: test build build-purego

test:
	$(GO) test ./...

build:
	mkdir -p dist
	$(GO) build -ldflags="$(LDFLAGS)" -o dist/agym .

build-purego:
	mkdir -p dist
	CGO_ENABLED=0 $(GO) build -ldflags="$(LDFLAGS)" -o dist/agym .
