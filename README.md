# mt5-pnl-cli

[![Licence](https://img.shields.io/github/license/tanem/mt5-pnl-cli)](LICENSE)
[![ci](https://github.com/tanem/mt5-pnl-cli/actions/workflows/ci.yml/badge.svg)](https://github.com/tanem/mt5-pnl-cli/actions/workflows/ci.yml)
[![coverage](https://codecov.io/gh/tanem/mt5-pnl-cli/branch/main/graph/badge.svg)](https://codecov.io/gh/tanem/mt5-pnl-cli)

> Query MT5 P&L from an encrypted [mt5-pnl-exporter](https://github.com/tanem/mt5-pnl-exporter) snapshot. One static binary — no daemon, no database, no third-party service.

```
   ┌──────────────┐  writes   ┌────────────────┐  reads   ┌──────────────┐
   │ mt5-pnl-     │ ────────► │ snapshot.json  │ ───────► │ mt5-pnl-cli  │
   │ exporter     │           │ .gz.age        │          │ (this repo)  │
   │ Windows host │           │ synced via     │          │ macOS, Linux │
   └──────────────┘           │ Dropbox etc.   │          │ or Windows   │
                              └────────────────┘          └──────────────┘
```

The exporter runs on the Windows host where MT5 lives and writes one
encrypted file. This CLI runs wherever you are, decrypts that file in
memory, and prints P&L and account tables — or JSON for scripts and AI
agents.

## Contents

- [Why](#why)
- [Install](#install)
- [Quick start](#quick-start)
- [Demo](#demo)
- [Commands](#commands)
- [How it works](#how-it-works)
- [Schema compatibility](#schema-compatibility)
- [Threat model](#threat-model)
- [Contributing](#contributing)
- [Licence](#licence)

## Why

- **Self-hosted.** Your trading data never touches a third-party
  dashboard. The snapshot is yours; this binary reads it locally.
- **One file in, answers out.** No config file. Point it at the snapshot
  once (env var or flag) and `mt5-pnl-cli pnl` just works.
- **Agent- and script-friendly.** `--format json`
  emits stable machine-readable output, and warnings go to stderr so they
  never corrupt a pipeline. An agent like Claude Code can turn *"show me
  monthly P&L for Q1"* into
  `mt5-pnl-cli pnl --from 2026-01-01 --to 2026-03-31 --by month --format json`.
  Colour is auto-disabled when output is piped or redirected, so pipelines
  stay clean without any extra flags.
- **Secrets stay in the keychain.** The decryption passphrase lives in
  the OS keychain only — there is deliberately no env var or flag for it.

## Install

Download a binary from
[Releases](https://github.com/tanem/mt5-pnl-cli/releases)
(darwin/linux/windows, amd64/arm64, with `checksums.txt`), or build with
Go:

```sh
go install github.com/tanem/mt5-pnl-cli@latest
```

## Quick start

```sh
# once: store the snapshot passphrase in the OS keychain. Use the SAME
# passphrase you gave mt5-pnl-exporter's set-encryption-passphrase.
mt5-pnl-cli set-passphrase

# once: tell the CLI where the synced snapshot lives
export MT5_PNL_SNAPSHOT=~/snapshots/mt5.json.gz.age

mt5-pnl-cli pnl          # last 30 days, grouped by week
mt5-pnl-cli accounts     # balances, equity, freshness
```

## Demo

Per-account and combined (`ALL`) rows per period, with a summary block:

```
$ mt5-pnl-cli pnl --from 2026-01-01 --to 2026-01-31
PERIOD      ACCOUNT      P&L  TRADES  WINS  LOSSES
2026-01-05  Trend EA    5.00       2     1       1
2026-01-05  Scalper EA  0.00       1     0       0
2026-01-05  ALL         5.00       3     1       1
2026-01-12  Trend EA    5.00       1     1       0
2026-01-12  ALL         5.00       1     1       0

Summary
  Performance
    Trades         4
    Win rate       50.0%
    Profit factor  3.50
    Expectancy     2.50
    Avg win        7.00
    Avg loss       -4.00
    Largest win    9.00
    Largest loss   -4.00
    Max drawdown   -4.00
    Gross profit   14.00
    Gross loss     -4.00
  P&L breakdown
    Trade profit   13.00
    Commission     -2.00
    Swap           -1.00
    Fee            0.00
    Net P&L        10.00 USD
```

`--by symbol` aggregates across accounts, one row per symbol:

```
$ mt5-pnl-cli pnl --from 2026-01-01 --to 2026-12-31 --by symbol
SYMBOL    P&L  TRADES  WINS  LOSSES
EURUSD   5.00       2     1       1
XAUUSD  10.00       1     1       0

Summary
  Performance
    Trades         3
    ...
```

```
$ mt5-pnl-cli accounts
LOGIN  LABEL       CURRENCY  BALANCE   EQUITY  LAST SUCCESS          LAST ERROR
111    Trend EA    USD       1000.00  1010.50  2026-06-13T00:00:00Z  -
222    Scalper EA  USD        500.00   500.00  -                     login failed

Snapshot generated: 2026-06-13T00:00:00Z
```

`--format json` emits the same data for machines
(`"account": null` is the combined row):

```
$ mt5-pnl-cli pnl --from 2026-01-01 --to 2026-01-31 --by month --accounts "Trend EA" --format json
{
  "rows": [
    {
      "group": "2026-01-01",
      "group_by": "month",
      "account": 111,
      "pnl": 10,
      "trade_profit": 13,
      "commission": -2,
      "swap": -1,
      "fee": 0,
      "trades": 3,
      "wins": 2,
      "losses": 1,
      "gross_profit": 14,
      "gross_loss": -4
    },
    {
      "group": "2026-01-01",
      "group_by": "month",
      "account": null,
      "pnl": 10,
      "trade_profit": 13,
      "commission": -2,
      "swap": -1,
      "fee": 0,
      "trades": 3,
      "wins": 2,
      "losses": 1,
      "gross_profit": 14,
      "gross_loss": -4
    }
  ],
  "summary": {
    "total_pnl": 10,
    "total_trades": 3,
    "win_rate_pct": 66.7,
    "profit_factor": 3.5,
    "expectancy": 3.33,
    "avg_win": 7,
    "avg_loss": -4,
    "largest_win": 9,
    "largest_loss": -4,
    "max_drawdown": -4,
    "gross_profit": 14,
    "gross_loss": -4,
    "trade_profit": 13,
    "commission": -2,
    "swap": -1,
    "fee": 0
  }
}
```

`--format csv` emits header + rows for spreadsheets (no summary):

```
$ mt5-pnl-cli pnl --from 2026-01-01 --to 2026-01-31 --by month --accounts "Trend EA" --format csv
group,group_by,account_login,account_label,pnl,trade_profit,commission,swap,fee,trades,wins,losses,gross_profit,gross_loss
2026-01-01,month,111,Trend EA,10.00,13.00,-2.00,-1.00,0.00,3,2,1,14.00,-4.00
2026-01-01,month,,ALL,10.00,13.00,-2.00,-1.00,0.00,3,2,1,14.00,-4.00
```

## Commands

- `pnl` — P&L over a date range.
  - Range: `--last Nd|Nw|Nm|Ny` (default `30d`; months and years are
    calendar-accurate) or `--from YYYY-MM-DD [--to YYYY-MM-DD]` (`--to`
    defaults to today). `--last` runs from N units ago through today
    inclusive, so `30d` covers 31 calendar days. Dates, `--last` and
    "today" are all interpreted in **UTC**, and each deal is bucketed by
    its UTC day — so from a far-east timezone (e.g., UTC+12), the UTC day can
    differ from your local day near midnight.
  - **Deal times and broker months.** Each deal's `time` is the value MT5
    records — on most brokers the server's local clock stored as a Unix
    timestamp. The CLI buckets by the UTC day/week/month of that value, so
    monthly and weekly figures line up with what your broker statement
    shows; there is no timezone skew to correct for.
  - `--by day|week|month|symbol|magic` (default `week`; weeks start
    Monday). Time cuts (`day`/`week`/`month`) group per period and account,
    with a combined `ALL` row per period. `symbol` and `magic` instead
    aggregate **across all in-scope accounts**, one row per symbol or magic
    number, with no per-account or `ALL` row — the first column becomes
    `SYMBOL`/`MAGIC` and the totals live in the summary. `magic` groups by
    the raw MT5 magic number (commonly one per strategy/EA). Because a
    `symbol`/`magic` cut sums across accounts, it **refuses** when the
    in-scope accounts span more than one currency — narrow `--accounts` to
    a single currency.
  - `--accounts "Trend EA,Scalper EA"` filters by account label
    (case-insensitive; default all).
  - `--format table|json|csv` (default `table`). The table footer is a
    **Summary** block in two groups — *Performance* (trades, win rate,
    profit factor, expectancy, average and largest win/loss, max drawdown,
    gross profit/loss) and *P&L breakdown* (trade profit, commission, swap,
    fee, and the net). JSON carries the same fields per row and in the
    summary; CSV is header + rows only (no summary), with the component
    columns `pnl,trade_profit,commission,swap,fee` so a `--by month` export
    drops straight into a spreadsheet or tax register. The summary footer
    shows the account currency when all in-scope accounts share one
    (e.g. `Net P&L  10.00 USD`). Every cut emits the same JSON/CSV shape:
    rows carry `group` (the period date, symbol, or magic) and `group_by`
    (the `--by` value); `account` is the login for per-account time rows and
    `null` for the combined time row and for every symbol/magic row.
  - **P&L components.** Net P&L is `trade_profit + commission + swap + fee`.
    Keeping the parts separate shows where a result came from — trading
    versus broker costs — which the net alone hides. Many tax regimes treat
    realised trade profit as income and commission/swap/fee as deductible
    expenses, so `pnl --by month --format csv` gives per-account, per-month
    component columns ready for a return; figures are always in the account
    currency (no home-currency conversion — see Mixed currencies).
  - **Max drawdown** is the largest peak-to-trough decline of the
    *realised* P&L curve over the selected deals (ordered by time,
    accumulated from zero), reported signed-negative. It is **not**
    account-equity drawdown — it excludes deposits, open positions and
    starting balance, so it will not match a broker's equity-drawdown
    figure.
  - `--color auto|always|never` (default `auto`): colourise P&L cells and
    the summary total by sign (green for profit, red for loss). `auto` enables
    colour only when writing to an interactive terminal and honours the
    `NO_COLOR` and `TERM=dumb` environment conventions; output is never
    coloured when piped or redirected. `always` forces ANSI codes regardless;
    `never` disables them unconditionally. On Windows the terminal must already
    have virtual-terminal processing enabled (Windows Terminal does; older
    `cmd.exe` may not).
  - **Mixed currencies.** If the accounts in scope span more than one
    currency, combined `ALL` rows and the summary are suppressed (`n/a` in
    tables, `null` in JSON, omitted from CSV) and a warning goes to
    stderr — the tool never silently sums across currencies. Narrow
    `--accounts` to one currency for combined totals.
- `accounts` — balances, equity and freshness per account, plus the
  snapshot's `generated_at`.
  - `--format table|json|csv` (default `table`).
- `set-passphrase` — store the snapshot decryption passphrase in the OS
  keychain (macOS Keychain / Windows Credential Manager / Linux Secret
  Service). Prompted twice, never echoed.
- `version` — binary version and supported snapshot schema (also
  available as `mt5-pnl-cli --version`).

Both query commands accept `--snapshot PATH` (overrides
`MT5_PNL_SNAPSHOT`) and `--stale-after` (default `2h`) — when the
snapshot is older than that, a warning goes to **stderr**, never stdout,
so machine-output pipelines stay clean. Pass `--quiet` (`-q`) to silence
warnings for scripted use; errors still print.

## How it works

```
snapshot.json.gz.age ──► age decrypt ──► gunzip ──► JSON ──► aggregate ──► table / JSON
(ciphertext on disk)     passphrase      (in memory only)
                         from keychain
```

The snapshot is never written to disk decrypted. Per closed deal, net
P&L = `profit + swap + commission + fee`. A deal with net > 0 is a win,
net < 0 a loss; a breakeven deal counts toward trades but neither
bucket. Sums accumulate at full precision and round only for display.
Profit factor = gross profit / |gross loss|.

**What counts as a trade is decided upstream.** This CLI sums every deal
in the snapshot's `closed_deals` and trusts
[mt5-pnl-exporter](https://github.com/tanem/mt5-pnl-exporter) to have
emitted only closing deals — entry legs and cash flows (deposits,
withdrawals) are filtered out there, not here — using MT5's native
signs, where commission, swap and fees are already negative for costs.
The CLI does not re-check this, so if you change that filtering or those
signs in the exporter, the P&L here changes with no error. Keep the two
in step.

## Schema compatibility

[`schema/snapshot.schema.json`](schema/snapshot.schema.json) is vendored
from the exporter release this build supports. The CLI accepts the same
major schema version and any minor at or below its own, and refuses
anything else with a message naming both versions and which side to
upgrade. `mt5-pnl-cli version` prints the supported schema version.

## Threat model

The trust boundary is your OS user session — the same as the
exporter's.

### What's protected

- **The snapshot at rest** stays `age`-encrypted; this CLI never writes
  a decrypted copy. Sync services and backups only ever see ciphertext.
- **The passphrase** lives in the OS keychain. There is deliberately no
  env var or flag for it: env vars leak via dotfiles, shell history, and
  child processes, and scripts don't need one — the binary reads the
  keychain itself. (`MT5_PNL_SNAPSHOT` carries only a file path.)

### What's not protected

- **A compromised user session.** Anyone with your OS account can read
  the keychain entry and run this CLI. The exporter's
  [threat model](https://github.com/tanem/mt5-pnl-exporter#threat-model)
  draws the same line.
- **Whatever you do with the output.** Tables and JSON contain balances
  and trade history; treat redirected output accordingly.

See [SECURITY.md](SECURITY.md) for scope and private vulnerability
reporting.

## Contributing

Issues and PRs welcome — see [CONTRIBUTING.md](CONTRIBUTING.md).

## Licence

[MIT](LICENSE)
