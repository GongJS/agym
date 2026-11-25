# Security Policy

## Security Boundaries

`agym` handles Google/agy authentication slots in the macOS Keychain:

- Secrets are read from and written to macOS Keychain via `/usr/bin/security -i` (`stdin` only, never CLI flags).
- Account projections returned to Wails (`AccountView`, `OAuthStatusView`) and CLI JSON commands contain only metadata (`email`, `current`, `inRoster`, `rotation`, `hasSavedLogin`, quota/model summaries).
- Temporary OAuth and quota probes isolate `--gemini_dir` and restore the original live Keychain slot even on timeout or cancellation.

## Reporting a Vulnerability

Please do **not** open a public GitHub issue for security vulnerabilities involving credential leakage or Keychain handling. Instead, report them privately via GitHub Security Advisories or contact the repository maintainer directly.
