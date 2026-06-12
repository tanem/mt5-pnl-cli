# mt5-pnl-cli v1 design

Date: 2026-06-13
Status: approved

## Purpose

`mt5-pnl-cli` is the query/reporting consumer of the snapshot produced by
[`mt5-pnl-exporter`](https://github.com/tanem/mt5-pnl-exporter). It reads
`snapshot.json.gz.age`, decrypts and aggregates it locally, and prints P&L
and account tables — or JSON for scripts and agents. Single static Go
binary, cross-compiled for Windows/macOS/Linux.

This is Phase 2 of the repo-split design
(`mt5-pnl-exporter/docs/superpowers/specs/2026-05-31-repo-split-design.md`),
which fixed the contract: Go module, vendored schema, hand-written structs,
version check on read, and feature parity with the old `mt5pnl` `pnl` and
`accounts` commands including their flags.

## Scope (v1)

Parity only:

- `pnl` — P&L over a date range, grouped by day/week/month.
- `accounts` — account balances, equity, freshness.
- `set-passphrase` — store the snapshot decryption passphrase in the OS
  keychain.
- `version` — binary version + supported schema version.

Explicitly deferred: open positions and cash-flow commands (data exists in
the snapshot; add post-v1), `passphrase_command` config for headless Linux
hosts without a Secret Service daemon.

## CLI surface

```
mt5-pnl-cli pnl  (--last 30d | --from 2026-01-01 --to 2026-03-31)
                 [--group day|week|month]   # default: week
                 [--group-name NAME]        # config group or account label
                 [--json]
                 [--config PATH]
mt5-pnl-cli accounts [--json] [--config PATH]
mt5-pnl-cli set-passphrase
mt5-pnl-cli version
```

Flag semantics preserved verbatim from the old `mt5pnl`:

- `--last` accepts `Nd/Nw/Nm/Ny`; month ≈ 30 days, year ≈ 365 days.
- `pnl` requires `--last` or both `--from` and `--to`; anything else is a
  usage error.
- `--group-name` resolves case-insensitively against config
  `account_groups` first, then against single account labels (from the
  snapshot); unknown names error listing valid groups and labels.
- `--json` emits the same shapes as the old tool: `{rows, summary}` for
  `pnl`, an account list for `accounts`.

Behaviour:

- Staleness warning to **stderr** (never stdout) when `generated_at` is
  older than `staleness_warn_hours` (default 2); message points at running
  `mt5-pnl-exporter export` on the host.
- `accounts` with no snapshot file is an error (exit 1) — accounts live in
  the snapshot now, so there is no truthful fallback.
- `accounts` shows `last_success_at` (schema 1.0 name) and prints the
  snapshot's `generated_at` in the table footer.
- `set-passphrase` prompts twice without echo. A missing passphrase at read
  time errors with "run `mt5-pnl-cli set-passphrase`"; the CLI never reads
  secrets from env vars or flags, and never prompts during `pnl`/`accounts`
  (keychain-only, by decision — see Security).

## Architecture

Module `github.com/tanem/mt5-pnl-cli`, `main` package at the repo root,
logic in `internal/` packages:

```
mt5-pnl-cli/
├── main.go              # flag parsing + subcommand dispatch
├── internal/
│   ├── snapshot/        # structs, age-decrypt → gunzip → JSON, version gate
│   ├── aggregate/       # closed deals → period rows + summary
│   ├── config/          # YAML config load + validation
│   ├── secrets/         # go-keyring wrapper
│   └── render/          # tabwriter tables + JSON to stdout
├── schema/snapshot.schema.json   # vendored from exporter 1.0 release
└── .goreleaser.yaml
```

Hand-rolled CLI: stdlib `flag` with one `flag.FlagSet` per subcommand and a
switch in `main`. No CLI framework. `main` is the only caller of
`os.Exit`; all internal functions return `error`.

Dependencies: `filippo.io/age`, `github.com/zalando/go-keyring`,
`gopkg.in/yaml.v3`, `golang.org/x/term`. Everything else stdlib.

## Snapshot reading (`internal/snapshot`)

Pipeline, fully streaming: open file → `age.Decrypt` with an
`age.ScryptIdentity` built from the keychain passphrase → `gzip.Reader` →
`json.Decoder`. Hand-written structs for `Snapshot`, `AccountSnapshot`,
`ClosedDeal`, `OpenPosition`, `CashFlow`, matching schema 1.0
field-for-field.

Version gate: parse `schema_version` as `major.minor`. Accept major == 1
and minor ≤ the CLI's supported minor; refuse otherwise with a message
naming both versions and which side to upgrade. Wrong passphrase (age "no
identity matched") is translated to "decryption failed: wrong passphrase,
or the file is corrupt".

## Aggregation (`internal/aggregate`)

A port of the old `aggregate.py`, collapsed to one stage since aggregation
now happens entirely at query time from raw `closed_deals`:

- No deal filtering: the exporter pre-filters `closed_deals` to closing
  trades before writing the snapshot (balance-family records go to
  `cash_flows`), so every record in `closed_deals` is aggregated. The old
  CLI's entry/type filter existed only because its fixture source didn't
  pre-filter. (Side note recorded during design: the exporter's
  `DEAL_ENTRY_INOUT = 3` was confirmed to be mislabelled `DEAL_ENTRY_OUT_BY`,
  dropping reversal deals — fixed in the exporter via
  https://github.com/tanem/mt5-pnl-exporter/pull/19, not compensated for
  here.)
- Net per deal = `profit + swap + commission + fee`; running sums rounded
  to 2 dp as in the Python implementation.
- Bucket by account + period: day (`YYYY-MM-DD`, UTC), week
  (Monday-start), month (first of month). Dates derive from the deal's
  Unix `time` interpreted in UTC, matching the old behaviour.
- Output: per-account rows plus a combined row (account = null) per
  period, sorted by period; summary block with total P&L, total trades,
  win rate %, profit factor, gross profit, gross loss. A deal with net ≥ 0
  counts as a win (matches the old code).

## Config (`internal/config`)

Default path `~/.config/mt5-pnl-cli/config.yaml` on macOS/Linux,
`%AppData%\mt5-pnl-cli\config.yaml` on Windows; `--config` overrides.
Strict YAML decoding — unknown keys are errors.

```yaml
snapshot_path: ~/snapshots/mt5.json.gz.age   # required, ~ expanded
staleness_warn_hours: 2                       # optional, default 2
account_groups:                               # optional
  prop:
    - Trend EA
    - Scalper EA
```

`account_groups` reference account labels, which live in the snapshot —
so label/group validation happens at query time against the loaded
snapshot, not at config load.

## Security

- The decryption passphrase lives only in the OS keychain, service
  `mt5-pnl-cli`, account `encryption-passphrase`, via `zalando/go-keyring`
  (macOS Keychain / Windows Credential Manager / Linux Secret Service).
- No env-var or flag path for the passphrase, deliberately: env vars leak
  via dotfiles, shell history, and child-process inheritance, and the
  automation story (agents, cron, scripts) doesn't need them — the binary
  reads the keychain itself. This follows age's own stance (passphrases
  are interactive-only upstream) rather than the restic/borg env-var
  pattern; if a headless need ever appears, the planned escape hatch is a
  restic-style `passphrase_command` config option, not an env var.
- Same trust boundary as the exporter: the OS user session. Documented in
  the README threat-model section when written.

## Error handling

- Exit 0 on success, 1 on any failure.
- Errors and warnings to stderr; stdout carries only tables/JSON.
- Messages name the path/value involved and say what to do next (missing
  config, missing snapshot, missing passphrase, unsupported schema,
  unknown group name each have specific guidance).

## Testing

TDD throughout. Table-driven unit tests per package:

- `aggregate`: expectations ported from the old Python test suite — same
  deals in, same rows/summary out — so parity is proven.
- `snapshot`: round-trip fixtures built in-test (age encrypt + gzip with a
  test passphrase); version-gate matrix (1.0 ok, higher-minor ok/refused
  per support level, 2.0 refused, garbage refused); wrong passphrase;
  truncated/corrupt file.
- `secrets`: `keyring.MockInit()` — no real keychain in tests.
- `render`: golden files for tables, exact-match for JSON.
- One end-to-end test compiling the binary and running `pnl`/`accounts`
  against an encrypted fixture, asserting stdout/stderr/exit code.

## CI and releases

- GitHub Actions CI: `gofmt` check, `go vet`, `golangci-lint` (default
  linters), `go test -race` on ubuntu/macos/windows matrix.
- Renovate for dependency updates, mirroring the exporter.
- GoReleaser on `v*` tags: darwin/linux/windows × amd64/arm64, archives +
  checksums, version injected via ldflags. Works identically while the
  repo is private and after it goes public.
- Versioning: start at `v0.1.0`; tag `v1.0.0` once parity with the old
  `mt5pnl` is confirmed in real use, which also triggers retiring the old
  `mt5-pnl` repo per the repo-split design.

## Repo

`github.com/tanem/mt5-pnl-cli`, private until v1 stabilises. MIT licence,
README, CLAUDE.md from day one; CONTRIBUTING and SECURITY mirroring the
exporter's before going public.
