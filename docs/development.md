# Development & Release Guide

## Local Development

```sh
# Run all tests with race detector
go test -race ./...

# Static checks
go vet ./...

# Build unified binary (dist/agym)
make build

# Run CLI commands directly
go run . accounts
go run . who
go run . quota --refresh

# Launch menu bar UI from source
go run . ui
```

## Wails Bindings

The tray frontend uses Wails v3 bindings under `frontend/bindings/`. If you modify exported methods or structs in `internal/appservice`, regenerate bindings with `wails3`:

```sh
make generate-bindings
```

> Note: Keeping `internal/appservice` package path and method signatures stable preserves existing `$Call.ByID` hashes when `wails3` is not installed locally.

## Cross-Platform Notes

- Native builds use CGO on macOS for Cocoa WebView and `/usr/bin/security` Keychain integration.
- `make build-purego` provides a `CGO_ENABLED=0` Linux compile check (`purego,production` build tags).
- Windows is not currently supported (`keychain_other.go` and `slotlock_other.go` return unsupported errors).

## Releasing to Homebrew

Tagging `vX.Y.Z` triggers `.github/workflows/release.yml` on `macos-latest` to build `darwin/arm64` and `darwin/amd64` archives plus `checksums.txt`. Update your Homebrew Tap formula (`Formula/agym.rb`) with the new version and SHA-256 checksums.
