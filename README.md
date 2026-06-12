# mt5-pnl-cli

Query MT5 P&L from an encrypted [mt5-pnl-exporter](https://github.com/tanem/mt5-pnl-exporter)
snapshot. Single static binary; reads `snapshot.json.gz.age` (age decrypt →
gunzip → JSON), aggregates locally, prints tables or JSON. No daemon, no
database, no third-party service.

## Install

Download a binary from [Releases](https://github.com/tanem/mt5-pnl-cli/releases),
or with Go:

```sh
go install github.com/tanem/mt5-pnl-cli@latest
```

## Quick start

```sh
# once: store the snapshot passphrase in the OS keychain
mt5-pnl-cli set-passphrase

# once: tell the CLI where the snapshot lives (or pass --snapshot per call)
export MT5_PNL_SNAPSHOT=~/snapshots/mt5.json.gz.age

mt5-pnl-cli pnl                      # last 30 days, by week
mt5-pnl-cli pnl --last 6m --by month
mt5-pnl-cli pnl --from 2026-01-01 --to 2026-03-31 --accounts "Trend EA" --json
mt5-pnl-cli accounts
```

## Commands

- `pnl` — P&L over a date range. `--last Nd|Nw|Nm|Ny` (default `30d`,
  calendar-accurate) or `--from`/`--to`; `--by day|week|month` (default
  `week`); `--accounts "A,B"` filters by label; `--json` for machine
  output.
- `accounts` — balances, equity and freshness per account.
- `set-passphrase` — store the snapshot decryption passphrase in the OS
  keychain (macOS Keychain / Windows Credential Manager / Linux Secret
  Service).
- `version` — binary version and supported snapshot schema.

A staleness warning goes to stderr when the snapshot is older than
`--stale-after` (default `2h`).

## Security

The decryption passphrase lives only in the OS keychain — there is no env
var or flag for it, deliberately (env vars leak via dotfiles, shell
history and child processes; scripts and agents don't need one because the
binary reads the keychain itself). `MT5_PNL_SNAPSHOT` carries only a file
path. The trust boundary is your OS user session, the same as the
exporter's; see its
[threat model](https://github.com/tanem/mt5-pnl-exporter#threat-model).

## Schema compatibility

The snapshot schema is vendored from the exporter release this build
supports (`schema/snapshot.schema.json`). The CLI accepts the same major
version and any minor at or below its own, and refuses anything else with
a message naming both versions.

## Licence

[MIT](LICENSE)
