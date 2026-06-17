# mt5-pnl-cli — read-views and reporting expansion (design)

Date: 2026-06-15. Status: approved.

Builds on the v1 design
(`2026-06-13-mt5-pnl-cli-v1-design.md`). This round rounds out the CLI's
*read-views* and reporting depth without changing its character: a
single-purpose, flags-only, pipe-friendly reader of one encrypted
snapshot. No config file, no daemon, no TUI, no charts.

## Goal

The CLI is the **sole reader** of the encrypted snapshot — nothing else
can decrypt it — so every useful view of that data has to live here or
nowhere. Today it exposes two views (`pnl`, `accounts`). This adds three
more (`positions`, `trades`, `cash-flows`), decomposes P&L into its
income and cost components, deepens reporting, generalises the grouping
dimension, fixes output ergonomics, and guards a latent mixed-currency
correctness bug.

Five read-views of one snapshot (`pnl` / `accounts` / `positions` /
`trades` / `cash-flows`) mirror `restic`'s `stats` / `snapshots` / `ls` /
`find`. The unix discipline is preserved by what stays out (interactive
TUI, `--watch`, built-in charts, a template engine, a plugin system),
not by starving the read side.

### What it's for

The CLI is a general-purpose reader for MT5 export data: tracking
progress, reporting, and reviewing trading over any period or cut.
Everything is already in the export, so the CLI's job is to *filter and
surface* what you need — it adds no data of its own.

Tax reporting is one such use, and it motivates a couple of the features
below, so it's worth setting out as a worked example rather than a goal.
A single net-P&L figure hides whether a result came from trades or from
costs; decomposing P&L into its components (trade profit vs commission /
swap / fee) and exposing cash flows (deposits/withdrawals, useful as
reconciliation control figures and to establish opening balance) makes
that visible — handy for understanding performance generally, and
directly useful at tax time. Many tax regimes, for instance, treat
realised trading profit as income and broker commissions/swaps/fees as
deductible expenses — opposite sides of a return — so the filer needs the
components split out, per account and per period. The detail differs by
jurisdiction but the need doesn't. The CLI stays jurisdiction-neutral,
always reporting in the account currency; any home-currency conversion is
the user's job, with the relevant authority's approved rates (see
Non-goals). So `pnl --by month --format csv` and `cash-flows --format csv`
produce per-account, per-month exports that drop straight into a
spreadsheet or tax register.

## Non-goals

- No config file. Flags remain the only vocabulary. If saved views ever
  earn their keep, the shape is a ripgrep-style *default-flags file* or a
  shell alias — never a configuration schema. (See Recorded decisions.)
- No multi-currency reporting and no home-currency conversion. Real
  cross-currency aggregation needs FX rates and a rate source; that is
  out of scope. Converting to a reporting currency for a tax return is
  deliberately left to the user with their tax authority's approved
  rates — the CLI always reports in the account currency. This
  round only adds a guard so the tool never silently sums across
  currencies.
- No human-readable mapping of MT5 integer enums (`type`, `entry`,
  `reason`). The vendored schema documents these only as integers, so
  they are rendered faithfully as integers. A mapping can be added later
  once the convention is pinned in the schema.
- The passphrase remains keychain-only. No env var, no flag. A
  `passphrase_command` escape hatch stays unbuilt until a real headless
  need appears.

## Build order

One spec, three phases, built in order because later phases render
through earlier decisions:

1. **Foundations** — help and exit codes, the `--format` contract, the
   mixed-currency guard, and a verify-and-document pass on deal-time
   timezone.
2. **Richer reporting** — the P&L component breakdown and the deeper
   summary, riding on the new summary render.
3. **New cuts and commands** — `--by symbol|magic`, then `positions`,
   `trades` and `cash-flows`.

## Phase 1 — Foundations

### 1.1 Help and exit codes

- `-h` / `--help` on any subcommand prints that command's hand-written
  help to **stdout** and exits **0**. Genuine parse errors (unknown
  flag, bad value) print to **stderr** and exit **1**.
  - Today both go to stderr and exit 1, because
    `fs.Parse` returns `flag.ErrHelp` and the code treats every parse
    error identically. Fix: distinguish `errors.Is(err, flag.ErrHelp)`
    (print help to stdout, return 0) from other errors (return 1), and
    suppress `flag`'s auto-generated usage in favour of hand-written
    per-command help.
- Each subcommand gets a short hand-written help string (usage line,
  one-line flag descriptions) rather than the raw `flag` dump. The
  top-level `help` / `-h` / `--help` already behave correctly and only
  need their text refreshed to list the new commands and flags.
- Add a top-level `--version` flag as an alias of the `version`
  subcommand (some callers reach for the flag form). `-v` is **not**
  added, to avoid the verbose/version ambiguity.

### 1.2 Output format: `--format table|json|csv`

- Replace the `--json` bool with `--format string` (default `table`),
  accepting `table`, `json`, `csv`. Invalid values error to stderr,
  exit 1, listing the valid set.
- `--json` is kept as a **documented alias** for `--format json` for
  back-compat. If both are given and disagree (`--json --format csv`),
  error; if both agree, accept.
- `--format` applies to `pnl`, `accounts`, `positions`, `trades` and
  `cash-flows`. It does not apply to `set-passphrase` or `version`.
- **CSV** is written with `encoding/csv`: a header row then data rows,
  **rows only**. The summary block (Phase 2) and table footers
  (`generated_at`, totals) are omitted from CSV — a summary does not fit
  a flat table, and CSV is the spreadsheet/`mlr` import path. The
  summary remains available in `table` and `json`.
- CSV column schemas (stable, snake_case headers):
  - `pnl`: `group, group_by, account_login, account_label, pnl,
    trade_profit, commission, swap, fee, trades, wins, losses,
    gross_profit, gross_loss`, where `pnl` is the net and equals
    `trade_profit + commission + swap + fee` (see 2.4). For the combined
    time row `account_login` is empty and `account_label` is `ALL`; for
    `symbol`/`magic` cuts both account columns are empty.
  - `accounts`: `login, label, currency, balance, equity,
    last_success_at, last_error`.
  - `positions`: `account_login, account_label, symbol, type, volume,
    price_open, price_current, profit, swap`.
  - `trades`: `time, account_login, account_label, symbol, type, entry,
    volume, price, profit, swap, commission, fee, net, magic, ticket,
    order, position_id, reason, comment, external_id` (full fidelity for
    export).
  - `cash-flows`: `time, account_login, account_label, type, amount,
    comment, ticket`, where `amount` is the balance-family deal's cash
    amount (MT5 carries it in the `profit` field) and `type` is the raw
    MT5 integer (deposit / withdrawal / credit / charge / correction /
    bonus / commission — see 3.5).

### 1.3 Mixed-currency guard

Invariant: **never sum across currencies.** `Currency` lives on
`AccountSnapshot`. Determine the distinct currencies among the accounts
in scope (those selected by `--accounts`, else all accounts that
contribute rows).

- If all in-scope accounts share one currency (the common case):
  unchanged behaviour.
- If more than one currency is in scope:
  - Emit a warning to **stderr** naming the currencies.
  - **Time cuts** (`day`/`week`/`month`): per-account rows still print;
    the combined `ALL` per-period rows and the whole summary block (every
    aggregate metric, drawdown included) are suppressed — `n/a` in the
    table, `null` in JSON — because each would sum across currencies.
  - **`symbol`/`magic` cuts** inherently aggregate across accounts and
    have no per-account row to fall back to, so the command **refuses**:
    exit 1 with guidance to narrow `--accounts` to one currency.
  - **`positions`** total floating P&L is suppressed (`n/a` / `null`)
    with a warning; per-position rows still print.

This is a footgun-guard, framed honestly as such in the docs — not
multi-currency support.

### 1.4 Deal-time timezone (verify and document)

Monthly and weekly buckets need to line up with what a broker statement
shows; otherwise reconciliation — and any figures derived from it, tax
included — can't be trusted. Whether they line up depends on what the
exporter stores in a deal's Unix `time`:

- If `time` is MT5's server-time value (the common MT5 quirk: a
  server-local clock stored as an epoch), the CLI's UTC monthly buckets
  already line up with the broker statements — no skew.
- If `time` is true UTC, deals in the first/last hours of a month can
  fall in a different month than the broker PDF.

Action: confirm which it is against `mt5-pnl-exporter`, then state the
answer in the README so monthly figures can be trusted (or their limits
known). A `--tz` option is **not** built unless this turns out to matter.

## Phase 2 — Richer reporting

Two strands: a **P&L component breakdown** carried on every row and in
the summary (2.4 — the income/cost split), and deeper **performance metrics**
that are summary-only (2.1–2.3) because per-period they would bloat the
table. The metrics need a deal-ordered pass and per-deal extremes, so
`aggregate` gains that pass and both the `Row` and `Summary` structs
grow.

### 2.1 New summary fields

Existing: total P&L, total trades, win rate %, profit factor, gross
profit, gross loss. Added:

- **Expectancy** = total P&L / total trades. Average P&L per trade.
  `null` when trades == 0. Signed.
- **Average win** = gross profit / wins. `null` when wins == 0.
- **Average loss** = gross loss / losses (negative). `null` when
  losses == 0.
- **Largest win** = max single-deal net among winning deals. `null`
  when no wins.
- **Largest loss** = min single-deal net among losing deals (negative).
  `null` when no losses.
- **Max drawdown** — see definition below. `null` when no deals; `0.00`
  when deals exist but the curve never retraces.

### 2.2 Max drawdown — pinned definition

Realised-P&L drawdown over the **selected deals**: take every in-scope
deal (after account and range filters, within a single currency),
**order by `time` then `time_msc`**, accumulate net P&L into a
cumulative curve **starting at 0** at range start, track the running
peak, and report the largest peak-to-trough decline. Reported
**signed-negative** to match the `gross_loss: -4.00` convention.

This is **not** account-equity drawdown (what a broker reports, which
includes deposits, open positions and starting balance). The README
states this explicitly to pre-empt "this doesn't match my broker"
confusion.

### 2.3 Summary render

The current single summary line cannot hold this much. Replace it with a
compact aligned key/value block in two labelled groups — performance and
the P&L breakdown — e.g.:

```
Summary
  Performance
    Trades         4
    Win rate       50.0%
    Profit factor  3.50
    Expectancy     2.50
    Avg win        7.00
    Avg loss       -4.00
    Largest win    5.00
    Largest loss   -4.00
    Max drawdown   -4.00
    Gross profit   14.00
    Gross loss     -4.00
  P&L breakdown
    Trade profit   13.00
    Commission     -2.00
    Swap           -1.00
    Fee            0.00
    Net P&L        10.00
```

`null` metrics render as `n/a`. This changes the table golden files and
the README Demo — both regenerated/updated in the same change. JSON
summary gains `expectancy`, `avg_win`, `avg_loss`, `largest_win`,
`largest_loss`, `max_drawdown` (nullable `*float64`) and the
`trade_profit`, `commission`, `swap`, `fee` subtotals (plain `float64`,
with `pnl` the net).

### 2.4 P&L component breakdown

`aggregate` currently derives net P&L as `profit + swap + commission +
fee` and keeps only the net. Keeping the four parts separate shows where
P&L actually came from — trading versus costs — which the net alone
hides, and it is also what tax reporting needs (e.g. regimes that treat
trade profit as income and commission/swap/fee as deductible expenses). So
each `Row` and the `Summary` gain four signed subtotals alongside the
existing net (`pnl`):

- `trade_profit` — sum of the deal `profit` field
- `commission` — sum of `commission`
- `swap` — sum of `swap`
- `fee` — sum of `fee`

`pnl` (net) continues to equal their sum. These are **distinct from**
`gross_profit`/`gross_loss`, which remain the win/loss partition of net
used for profit factor — the word "profit" therefore carries two
deliberately separate meanings, kept apart by the field names.

Surfacing: present in JSON (per row and in the summary) and in CSV (per
1.2). The **table** stays readable — the per-row `pnl` column remains net
only, and the breakdown appears in the summary block (2.3) and in full
via json/csv. With this, `pnl --by month --format csv` yields
per-account, per-month `trade_profit` and `commission` columns ready for
a spreadsheet or tax register. Whole-range totals live in the summary
block (`--format table`/`json` only — CSV is rows-only, per 1.2): filter
to one account over the full range for that account's annual figures, or
sum the monthly CSV rows in the spreadsheet.

## Phase 3 — New cuts and commands

### 3.1 `--by symbol|magic`

`--by` accepts `day|week|month|symbol|magic`. It remains a **single
dimension** — no cross-tabs (no "symbol × month").

- For `symbol`/`magic`, rows have **no account or `ALL` dimension**: the
  output aggregates across all in-scope accounts, one row per
  symbol/magic. The first column becomes `SYMBOL` / `MAGIC`. There is no
  per-period combined row — the summary carries the totals.
- Default sort: by group key ascending (symbol alphabetical, magic
  numeric). (`--sort` is out of scope this round.)
- `magic` is rendered as its raw integer. `--by magic` is built because
  traders commonly assign a distinct magic number per strategy or EA, so
  it groups P&L by strategy.
- Mixed currency + `symbol`/`magic` refuses (see 1.3).

### 3.2 Uniform JSON shape for `pnl`

All cuts emit one shape (pre-1.0, so the one-time break from `period` is
acceptable and preferred over maintaining two shapes):

```json
{
  "rows": [
    { "group": "2026-01-01", "group_by": "month", "account": 111,
      "pnl": 10, "trade_profit": 13, "commission": -2, "swap": -1,
      "fee": 0, "trades": 3, "wins": 2, "losses": 1,
      "gross_profit": 14, "gross_loss": -4 },
    { "group": "2026-01-01", "group_by": "month", "account": null,
      "pnl": 10, "trade_profit": 13, "commission": -2, "swap": -1,
      "fee": 0, "trades": 3, "wins": 2, "losses": 1,
      "gross_profit": 14, "gross_loss": -4 }
  ],
  "summary": { ...the Phase 2 summary fields... }
}
```

- `group` is always a string (date for time cuts, symbol, or stringified
  magic). `group_by` is the `--by` value (`day|week|month|symbol|magic`).
- `account` is the login for per-account time rows, `null` for the
  combined time row and for every `symbol`/`magic` row.
- The CSV `group`/`group_by` columns mirror this.

### 3.3 `positions` command

Open positions and their floating P&L, from the snapshot's
`open_positions` (decoded today but not yet surfaced).

- Columns (table): `ACCOUNT`, `SYMBOL`, `TYPE`, `VOLUME`, `PRICE OPEN`,
  `PRICE NOW`, `FLOATING P&L`, `SWAP`. Footer: total floating P&L
  (currency guard applies) and `Snapshot generated: <generated_at>`.
- `type` rendered as raw integer (faithful — see Non-goals).
- Flags: `--accounts`, `--format`, `--snapshot`, `--stale-after`. **No
  date range** — positions are current as of the snapshot.
- JSON envelope: `{ "positions": [ ...open_position objects... ],
  "total_floating_pnl": <number|null> }` — wrapped (like `pnl`) because
  it carries a total; the total is `null` under mixed currency. CSV: per
  1.2.

### 3.4 `trades` command

The raw closed deals in a range — the export and drill-down view.

- Filters: `--from/--to/--last` (same parsing as `pnl`), `--accounts`.
  Default sort `time` ascending.
- Table shows a readable subset: `TIME` (RFC3339 UTC), `ACCOUNT`,
  `SYMBOL`, `TYPE`, `VOLUME`, `PRICE`, `NET` (where net = profit + swap +
  commission + fee). JSON and CSV carry full per-deal fidelity (per the
  1.2 CSV schema). JSON is a bare array of deal objects (like
  `accounts` today) — there is no total to wrap.
- `type`/`entry` rendered as raw integers (faithful).
- Flags: `--accounts`, `--format`, `--snapshot`, `--stale-after`, plus
  the range flags.

### 3.5 `cash-flows` command

The balance-family records the snapshot already carries in `cash_flows`
(deposits, withdrawals, credits, charges, corrections, bonuses,
standalone commissions) — decoded today but, like `open_positions`, not
yet surfaced. They are deliberately excluded from trading P&L; this
command exposes them in their own view for reconciliation and to
establish opening balance from the first transfer off zero.

- Columns (table): `TIME` (RFC3339 UTC), `ACCOUNT`, `TYPE`, `AMOUNT`,
  `COMMENT`. Footer: total deposits and total withdrawals (the
  mixed-currency guard applies — a cross-currency total is suppressed to
  `n/a` with a warning). `AMOUNT` is the cash amount MT5 carries in the
  deal `profit` field; `TYPE` is the raw MT5 integer.
- Filters: `--from/--to/--last` (same parsing as `pnl`), `--accounts`.
  Default sort `time` ascending.
- Flags: `--accounts`, `--format`, `--snapshot`, `--stale-after`, plus
  the range flags.
- JSON: a bare array of cash-flow objects (like `trades`). CSV: per 1.2.

## Recorded decisions (no build)

- **No config file; flags-only stays.** Confirmed best practice for this
  tool class: stateless single-purpose tools (`jq`, `fd`, `ffmpeg`,
  `restic`) run on flags + env with zero required config; the tools that
  carry rich config (`git`, `kubectl`, `gh`) are stateful/multi-domain, a
  different class. When the puritan tools do add config it is a *file of
  default flags* (`RIPGREP_CONFIG_PATH`, `bat.conf`, `.curlrc`,
  `FZF_DEFAULT_OPTS`), not a new schema. If saved account-group views are
  ever wanted, that — or a shell alias — is the shape, keeping flags the
  single source of truth.
- **`passphrase_command`** remains the sanctioned future escape hatch for
  a headless need, unbuilt until one exists.

## Open questions (revisit once we've seen real output)

Both are provisional defaults, deliberately deferred until a real
snapshot shows what reads well — neither blocks implementation:

- **`trades` table columns.** Shipping the readable subset (`TIME,
  ACCOUNT, SYMBOL, TYPE, VOLUME, PRICE, NET`); full fidelity stays in
  json/csv. May split out profit/swap/commission/fee in the table later
  if the single `NET` column hides too much.
- **Raw integer enums.** `type`/`entry` render as raw MT5 integers for
  now (faithful to the schema). May add a `buy`/`sell` mapping later,
  once the convention is confirmed and pinned in the schema/docs.

## Error handling and exit codes

Unchanged model: exit 0 on success, 1 on any failure; errors and
warnings to stderr, stdout carries only tables/JSON/CSV. New cases:

- `-h`/`--help` → help to stdout, exit 0.
- Invalid `--format` value → stderr, exit 1, listing valid values.
- `--json` conflicting with `--format` → stderr, exit 1.
- Mixed currency on `symbol`/`magic` cut → stderr, exit 1, guidance to
  narrow `--accounts`.
- `positions`/`trades`/`cash-flows` reuse the existing snapshot-path,
  passphrase, schema, and unknown-account-label errors.

## Testing

TDD throughout, table-driven per package.

- `aggregate`: the P&L component subtotals (trade_profit/commission/
  swap/fee summing to net, signs preserved); new summary metrics with
  hand-computed expectations (expectancy, avg/largest win and loss,
  breakeven and empty-bucket edges); max drawdown over ordered fixtures
  (monotone-up → 0, single trough, multiple troughs, ties on `time`);
  `--by symbol`/`magic` grouping and ordering; mixed-currency refusal for
  symbol/magic.
- `render`: golden files for the two-group summary block and the
  per-command tables (`positions`, `trades`, `cash-flows`); exact-match
  JSON for the uniform `group`/`group_by` shape, the component subtotals
  and the new summary fields; CSV exact-match per schema, including the
  component columns and the empty-account columns for combined and
  symbol/magic rows; `n/a`/`null` rendering under mixed currency.
- `main`/args: `--format` parsing and the `--json` alias/conflict;
  `-h`/`--help` to stdout exit 0 vs parse error to stderr exit 1;
  `--version` flag; mixed-currency suppression vs refusal per cut.
- e2e: `positions`, `trades` and `cash-flows` against an encrypted
  fixture (path via `--snapshot` and `MT5_PNL_SNAPSHOT`), asserting
  stdout/stderr/exit; `pnl --by month --format csv` carrying the
  component columns. The `snaptest` fixture builder gains
  `open_positions`, `cash_flows`, and
  multi-currency/multi-symbol/magic fixtures.

## Docs to update (same change)

- **README**: new `positions`/`trades`/`cash-flows` commands; `--format`
  and `--version`; the P&L component breakdown and a short worked
  **reporting/tax example** (`pnl --by month --format csv` for per-month
  component columns; `cash-flows` for control totals); the richer
  summary and the drawdown definition note; the deal-time timezone note
  (1.4); the updated Demo output; the mixed-currency behaviour.
- **CLAUDE.md**: the new commands (`positions`/`trades`/`cash-flows`),
  the P&L component breakdown, and the
  `--format`/grouping/currency/deal-time gotchas, per its own "update in
  the same change" rule.
- **Golden files**: regenerated via `go test ./internal/render -update`,
  diff eyeballed.
