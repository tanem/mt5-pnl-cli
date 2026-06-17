# CLAUDE.md

Go CLI that reads the encrypted snapshot written by mt5-pnl-exporter
(`snapshot.json.gz.age`: JSON → gzip → age scrypt) and prints P&L/account
tables or JSON. Spec: `docs/superpowers/specs/2026-06-13-mt5-pnl-cli-v1-design.md`.

## Commands

```bash
go test ./...                 # all tests (CI runs -race; ubuntu leg uploads coverage to codecov)
go test ./internal/render -update   # regenerate golden files after render changes
go build -o mt5-pnl-cli .
go run github.com/goreleaser/goreleaser/v2@latest check   # validate .goreleaser.yaml
pre-commit install            # gitleaks secret-scan hook (one-time)
pre-commit run --all-files    # run the gitleaks hook manually
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
- `internal/aggregate` — deals → group rows + summary. `--by` chooses the
  grouping: time cuts (day/week/month) emit per-account rows plus a combined
  `ALL` row per period; symbol/magic cuts emit one row per symbol/magic
  aggregated across accounts (no per-account or combined row, Account nil).
  Each row and the summary carry net P&L plus its four components
  (trade_profit / commission / swap / fee, summing to net). The summary also
  carries expectancy, average and largest win/loss, and max drawdown (a
  deal-ordered realised-P&L pass, not equity drawdown). `AccountsInScope`
  exposes the contributing logins for the currency guard. Full-precision
  sums; rounding happens in render only. Breakeven (net == 0) is neither win
  nor loss.
- `internal/secrets` — keychain via zalando/go-keyring, service
  `mt5-pnl-cli`, account `encryption-passphrase`.
- `internal/render` — fixed-width tables (manual writer; ANSI colour
  applied after width padding so it never skews alignment), JSON and CSV;
  all display rounding here. `pnl`/`accounts` take `--format table|json|csv`
  (default `table`).
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
- **`--format`.** `pnl`/`accounts` take `--format table|json|csv`
  (default `table`). CSV is rows-only (no summary).
- **Mixed-currency guard.** `pnl` never sums across currencies: when
  accounts in scope span more than one, combined `ALL` rows and the summary
  are suppressed (`n/a`/`null`/omitted) with a stderr warning. A
  `--by symbol|magic` cut has no per-account row to fall back to, so it
  **refuses** under mixed currency (stderr, exit 1) — narrow `--accounts`.
  Scope is the accounts that contributed deals (`aggregate.AccountsInScope`),
  not the grouped rows (symbol/magic rows carry no account).
- **`--quiet`/`-q`** silences stderr warnings (staleness, mixed-currency); errors still print.
- **`--color`** (pnl only): auto/always/never; auto needs a `*os.File` TTY and honours `NO_COLOR`/`TERM=dumb`.
- **Summary block is table/JSON only.** The two-group performance/breakdown
  summary appears in `--format table` (an aligned key/value block) and
  `--format json`; CSV is rows-only by design. Max drawdown is realised-P&L
  drawdown over the ordered in-scope deals, deliberately distinct from
  broker equity drawdown.
- **`group`/`group_by` are uniform across cuts.** Every `pnl` JSON/CSV row
  carries `group` (period date, symbol, or magic) and `group_by` (the
  `--by` value); the Go field is `aggregate.Row.Group`. `account` is the
  login for per-account time rows and `null` for the combined time row and
  for every symbol/magic row. The table labels the first column
  `PERIOD`/`SYMBOL`/`MAGIC` and drops the `ACCOUNT` column for dimension
  cuts.
- Dependencies are Renovate-managed; don't hand-bump pinned actions or
  module versions.

## Conventions

- British/Commonwealth English in comments and docs.
- TDD; golden files for table output (`-update` to regenerate, then eyeball
  the diff).
- After changing commands, architecture or a gotcha above, update this
  file and README.md in the same change.
