GO ?= go

.PHONY: test build build-purego generate-bindings

test:
	$(GO) test ./...

build:
	mkdir -p dist
	$(GO) build -tags production -o dist/agym .

# Linux CI / sandbox compile check. Wails purego is experimental.
build-purego:
	mkdir -p dist
	CGO_ENABLED=0 $(GO) build -tags purego,production -o dist/agym .

generate-bindings:
	wails3 generate bindings -b -f '-tags=purego' -dir . -d frontend/bindings .
