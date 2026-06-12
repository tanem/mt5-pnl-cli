# CLAUDE.md

Go CLI that reads the encrypted snapshot written by mt5-pnl-exporter
(`snapshot.json.gz.age`: JSON → gzip → age scrypt) and prints P&L/account
tables or JSON. Spec: `docs/superpowers/specs/2026-06-13-mt5-pnl-cli-v1-design.md`.

## Commands

```bash
go test ./...                 # all tests (race-enabled in CI)
go test ./internal/render -update   # regenerate golden files after render changes
go build -o mt5-pnl-cli .
go run github.com/goreleaser/goreleaser/v2@latest check   # validate .goreleaser.yaml
```

## Architecture

- `main.go` — `run(args, stdout, stderr, getPassphrase)` is the testable
  entry point; `main()` wires `os` streams + `secrets.Get`. One
  `flag.FlagSet` per subcommand; `main` is the only caller of `os.Exit`.
- `args.go` / `cmd_common.go` — range parsing (`--last`, `--from/--to`),
  snapshot path resolution (flag > `MT5_PNL_SNAPSHOT` > error), staleness
  warning, account-label filter resolution.
- `internal/snapshot` — schema 1.x structs, streaming age→gzip→JSON read,
  version gate (`CheckSchemaVersion`: same major, minor <= supported).
- `internal/aggregate` — deals → period rows + summary. Full-precision
  sums; rounding happens in render only. Breakeven (net == 0) is neither
  win nor loss.
- `internal/secrets` — keychain via zalando/go-keyring, service
  `mt5-pnl-cli`, account `encryption-passphrase`.
- `internal/render` — tabwriter tables + JSON; all display rounding here.
- `internal/snaptest` — test-only fixture builder (encrypts JSON the way
  the exporter does; low scrypt work factor for speed).

## Gotchas

- **No config file, by design.** Snapshot path via flag/env; everything
  else is flags. Don't add a config file without revisiting the spec.
- **The passphrase has no env var or flag, deliberately** (see spec
  Security section). Don't add one. Tests inject `getPassphrase`;
  cross-process tests can't reach the keychain, so the binary smoke test
  only covers pre-keychain failure paths.
- **CI runs on ubuntu/macos/windows** — keep paths `filepath`-safe and
  don't add tests that need a real keychain (`keyring.MockInit()` only).
- **Schema bumps:** when the exporter ships a new minor, update
  `SupportedMinor`, re-vendor `schema/snapshot.schema.json` from that
  release, and add fields to the structs (additive only).
- **Deal times are Unix seconds bucketed in UTC**; weeks start Monday.
- Dependencies are Renovate-managed; don't hand-bump pinned actions or
  module versions.

## Conventions

- NZ English in comments and docs. No hyperbole.
- TDD; golden files for table output (`-update` to regenerate, then eyeball
  the diff).
- After changing commands, architecture or a gotcha above, update this
  file and README.md in the same change.
