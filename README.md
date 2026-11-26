# agym

[简体中文](./README.zh-CN.md) | English

A lightweight macOS menu bar tray app and CLI for managing multiple `agy` Google accounts, monitoring Gemini/Claude quota windows, and safely switching Keychain credentials.

Built as a single Go binary with [Wails v3](https://wails.io/) and a zero-build vanilla HTML/CSS/JS popup.

## Features

- **Menu Bar Tray & CLI in One Binary**: Run `agym` (or `agym tray`) for the macOS menu bar popup, or invoke subcommands directly from the terminal.
- **Safe Keychain Switching**: Stores account backups under `gemini-agy-backup/<email>` in macOS Keychain and coordinates all reads/writes with a cross-process `slot.lock`. Raw tokens never leave the backend.
- **Official OAuth Onboarding**: Sign in to new accounts directly from the tray via official `agy` OAuth, with automatic quota fetching right after an account is added.
- **Quota & Model Visibility**: Displays Gemini and Claude 5-hour / weekly quota windows, subscription tier, exit region, and per-account model & thinking preferences.
- **Bilingual UI**: Supports English and Simplified Chinese (`EN` / `中文` toggle, auto-detected from system locale).
- **Session Handoff Supervisor**: Optional CLI `supervise` mode + Stop hook for quota-exhaustion account rotation in external terminals.

## Prerequisites

- **macOS** (Keychain integration and native WebView tray)
- Official `agy` CLI installed (expected at `~/.local/bin/agy`)

## Installation

### Quick Install (`curl`)

```sh
curl -fsSL https://raw.githubusercontent.com/GongJS/agym/main/install.sh | sh
```

### Homebrew (Tap)

Once published to a Homebrew Tap:

```sh
brew install gongjs/tap/agym
brew services start agym
```

### From Source

```sh
git clone https://github.com/GongJS/agym.git
cd agym
make build
cp dist/agym ~/.local/bin/agym
```

## Usage

### Menu Bar Tray

```sh
# Launch the menu bar tray app (default when no subcommand is given)
agym
# or explicitly
agym tray
```

### CLI Commands

```sh
agym accounts                 # Print safe account metadata as JSON
agym who                      # Print the active agy account
agym switch <email>           # Switch the live Keychain login to a saved account
agym quota [--refresh]        # Print cached quota JSON (or force refresh)
agym add <email>              # Add an existing Keychain backup to the roster
agym remove <email>           # Remove from roster (keeps Keychain backup)
agym rotation <email> on|off  # Toggle automatic-rotation membership
agym install-hook             # Install the quota handoff Stop hook
agym remove-hook              # Remove the agym Stop hook
agym supervise [--pinned] [--] <agy args...>
```

## Documentation

- [Architecture & Security Model](./docs/architecture.md)
- [Development & Release Guide](./docs/development.md)
- [Contributing](./CONTRIBUTING.md)
- [Security Policy](./SECURITY.md)

## License

[MIT](./LICENSE)
