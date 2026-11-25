# Architecture & Security Model

## Repository Layout

```text
.
├── main.go                 # Single binary entrypoint (tray by default, CLI on subcommands)
├── frontend/               # Zero-build vanilla HTML/CSS/JS + i18n + Wails bindings
├── internal/
│   ├── cli/                # CLI subcommand router and output formatting
│   ├── gui/                # Wails v3 system tray & popover window setup
│   ├── appservice/         # Wails-bound service layer (safe projections only)
│   ├── accounts/           # Account roster, Keychain access, slot lock, OAuth onboarding
│   ├── usage/              # Isolated agy runner, /usage parser, quota cache & refresh loop
│   ├── catalog/            # Per-account model catalog cache
│   ├── cloudmeta/          # Local ephemeral relay for tier/region/model metadata
│   ├── session/            # External terminal supervision, Stop hook, handoff selection
│   └── envutil/            # Shared environment variable helpers
└── docs/
```

## Credential & Data Boundary

- **Roster files**:
  - `~/Library/Application Support/sparkle/agy-accounts.txt`
  - `~/Library/Application Support/sparkle/agy-manual-only.txt`
- **Cache files**:
  - `~/.cache/agy-tools/quota_cache.json`
  - `~/.cache/agy-tools/models.json`
  - `~/.cache/agy-tools/slot.lock` and `slot.gate`
- **macOS Keychain items**:
  - Live slot: service `gemini`, account `antigravity`
  - Saved backups: service `gemini-agy-backup`, account `<email>`

### Security Guarantees

1. **No credential leakage**: Raw Keychain values and refresh tokens never leave `internal/accounts` or appear in `AccountView`, CLI JSON output, or logs.
2. **Safe writes**: `internal/accounts/keychain_darwin.go` sends secrets to `/usr/bin/security -i` via `stdin` (never via command-line arguments) and performs read-back verification.
3. **Cross-process slot locking**: Quota probes, OAuth onboarding, and interactive account switches serialize on `slot.lock`, while `slot.gate` lets background probes yield to interactive operations and always restore the original live login.
4. **Non-macOS stub**: `internal/accounts/keychain_other.go` explicitly returns `ErrUnsupported` rather than falling back to an unencrypted store.
