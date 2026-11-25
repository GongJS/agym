# Contributing to agy-tools

Thank you for considering contributing to `agym`!

## Workflow

1. Fork the repository and create a branch from `main`.
2. Keep changes focused and minimal.
3. Ensure all checks pass before opening a pull request:
   ```sh
   go vet ./...
   go test -race ./...
   make build
   ```
4. If you add or update UI strings in `frontend/`, update both `zh-CN` and `en` entries in `frontend/i18n.js`.
5. Never expose raw Keychain secrets, tokens, or authorization codes in logs, CLI output, or Wails bindings.
