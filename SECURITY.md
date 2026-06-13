# Security policy

## Scope

This tool handles one secret: the **snapshot decryption passphrase**,
stored in the OS keychain (via `go-keyring`: macOS Keychain / Windows
Credential Manager / Linux Secret Service). It is never accepted via
argument, environment variable, or file, and never written to disk or
logs. The decrypted snapshot exists in memory only — the CLI never
writes plaintext to disk.

Vulnerabilities in scope:

- Passphrase disclosure in any output, error message, or crash
- Unsafe keychain read/write behaviour
- The decryption pipeline (`age` + gzip) bypassed, weakened, or made to
  accept tampered input undetected
- Account data (balances, trade history) leaking anywhere other than
  the requested stdout output
- Dependency vulnerabilities with a plausible exploitation path in this
  tool

Out of scope:

- mt5-pnl-exporter and the MT5/broker side (see the
  [exporter's security policy](https://github.com/tanem/mt5-pnl-exporter/blob/main/SECURITY.md))
- A compromised OS user session — the documented trust boundary; anyone
  with the user's session can read the keychain and run the CLI
- Issues only reproducible with a non-current Go toolchain

## Supply-chain controls

- **GitHub Actions are pinned to commit SHAs** (not mutable tags), so a
  compromised or retagged action cannot inject code into CI.
  [Renovate](https://docs.renovatebot.com/) keeps the pins current via
  `helpers:pinGitHubActionDigests`.
- Dependency update PRs (Renovate) must pass the full CI matrix
  (ubuntu/macos/windows) before auto-merging; majors require manual
  review. See [`renovate.json`](renovate.json).
- **Release binaries are static** (`CGO_ENABLED=0`), built in CI by
  GoReleaser from the tagged commit, and published with a
  `checksums.txt` for verification.

## Reporting

**Do not open a public GitHub issue for security vulnerabilities.**
Public issues expose the vulnerability before a fix is available.

Report privately via the Security tab —
[Report a vulnerability](https://github.com/tanem/mt5-pnl-cli/security/advisories/new).
This opens a private workspace visible only to you and the maintainer.

You will receive a response within 7 days. Once a fix is ready, I'll
agree a disclosure date with you before publishing.
