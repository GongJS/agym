GO ?= go

.PHONY: test build build-purego

test:
	$(GO) test ./...

build:
	mkdir -p dist
	$(GO) build -ldflags="-s -w" -o dist/agym .

build-purego:
	mkdir -p dist
	CGO_ENABLED=0 $(GO) build -ldflags="-s -w" -o dist/agym .
