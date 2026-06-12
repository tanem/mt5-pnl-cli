# mt5-pnl-cli v1 design

Date: 2026-06-13 (revised same day after spec review)
Status: approved

## Purpose

`mt5-pnl-cli` is the query/reporting consumer of the snapshot produced by
[`mt5-pnl-exporter`](https://github.com/tanem/mt5-pnl-exporter). It reads
`snapshot.json.gz.age`, decrypts and aggregates it locally, and prints P&L
and account tables — or JSON for scripts and agents. Single static Go
binary, cross-compiled for Windows/macOS/Linux.

This is Phase 2 of the repo-split design
(`mt5-pnl-exporter/docs/superpowers/specs/2026-05-31-repo-split-design.md`).
That design's Phase-2 bullets were treated as the starting point for this
brainstorm, not as law; the deviations and their reasons are recorded at
the end of this document.

## Scope (v1)

- `pnl` — P&L over a date range, grouped by day/week/month.
- `accounts` — account balances, equity, freshness.
- `set-passphrase` — store the snapshot decryption passphrase in the OS
  keychain.
- `version` — binary version + supported schema version.

Explicitly deferred: open positions and cash-flow commands (data exists in
the snapshot; add post-v1), `passphrase_command` for headless Linux hosts
without a Secret Service daemon, named account groups (see Inputs).

## CLI surface

```
mt5-pnl-cli pnl  [--last 30d | --from 2026-01-01 [--to 2026-03-31]]
                 [--by day|week|month]      # default: week
                 [--accounts "A,B"]         # filter by account label
                 [--json]
                 [--snapshot PATH] [--stale-after 2h]
mt5-pnl-cli accounts [--json] [--snapshot PATH] [--stale-after 2h]
mt5-pnl-cli set-passphrase
mt5-pnl-cli version
```

Range semantics:

- No range flags → defaults to `--last 30d`; the bare command works out of
  the box.
- `--last` accepts `Nd/Nw/Nm/Ny`, calendar-accurate via `time.AddDate`
  (`6m` = the same day six months back, not 180 days).
- `--from` without `--to` means "until today". `--to` without `--from` is
  a usage error, as is combining `--last` with `--from`/`--to`.

Filtering and output:

- `--accounts` takes comma-separated account labels, resolved
  case-insensitively against the labels in the snapshot; unknown labels
  error listing the valid ones. No filter → all accounts.
- `--json` emits `{rows, summary}` for `pnl` and an account list for
  `accounts`, with stable key order.

Behaviour:

- Staleness warning to **stderr** (never stdout) when `generated_at` is
  older than `--stale-after` (Go duration syntax, default `2h`); message
  points at running `mt5-pnl-exporter export` on the host.
- `accounts` with no snapshot file is an error (exit 1) — accounts live in
  the snapshot, so there is no truthful fallback.
- `accounts` shows `last_success_at` and prints the snapshot's
  `generated_at` in the table footer.
- `set-passphrase` prompts twice without echo. A missing passphrase at read
  time errors with "run `mt5-pnl-cli set-passphrase`"; the CLI never reads
  secrets from env vars or flags, and never prompts during `pnl`/`accounts`
  (keychain-only, by decision — see Security).

## Inputs (no config file)

There is no config file. The tool's only durable setting is where the
snapshot lives:

1. `--snapshot PATH` flag (highest precedence),
2. `MT5_PNL_SNAPSHOT` environment variable,
3. neither → error explaining both options.

`~` is expanded in either source. An env var is appropriate here — and not
for the passphrase — because the path is not a secret; this is the
standard CLI convention (`RESTIC_REPOSITORY`, `BORG_REPO`, `KUBECONFIG`,
`DOCKER_HOST`).

Everything else is a flag with a sensible default (`--by week`,
`--stale-after 2h`, `--accounts` unset = all). Named account groups (the
old `account_groups` YAML) are dropped: `--accounts "A,B"` covers ad-hoc
filtering, and shell aliases or the calling agent cover reuse. If groups
ever earn their keep, a config file can be added later without breaking
anything.

## Architecture

Module `github.com/tanem/mt5-pnl-cli`, `main` package at the repo root,
logic in `internal/` packages:

```
mt5-pnl-cli/
├── main.go              # flag parsing + subcommand dispatch
├── internal/
│   ├── snapshot/        # structs, age-decrypt → gunzip → JSON, version gate
│   ├── aggregate/       # closed deals → period rows + summary
│   ├── secrets/         # go-keyring wrapper
│   └── render/          # tabwriter tables + JSON to stdout
├── schema/snapshot.schema.json   # vendored from exporter v1.0.3 release
└── .goreleaser.yaml
```

Hand-rolled CLI: stdlib `flag` with one `flag.FlagSet` per subcommand and a
switch in `main`. No CLI framework. `main` is the only caller of
`os.Exit`; all internal functions return `error`.

Dependencies: `filippo.io/age`, `github.com/zalando/go-keyring`,
`golang.org/x/term`. Everything else stdlib.

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

Derived from the old `aggregate.py` but corrected rather than ported
verbatim:

- No deal filtering: the exporter pre-filters `closed_deals` to closing
  trades before writing the snapshot (balance-family records go to
  `cash_flows`), so every record in `closed_deals` is aggregated. (The
  exporter's entry-constant bug that dropped reversal deals was fixed in
  exporter v1.0.3, https://github.com/tanem/mt5-pnl-exporter/pull/19.)
- Net per deal = `profit + swap + commission + fee`. Sums accumulate at
  full float64 precision and are rounded to 2 dp only at render — unlike
  the old code, which rounded running totals at every step.
- Win/loss: win is net > 0, loss is net < 0; a breakeven deal (net = 0)
  counts toward `trades` but neither bucket. Win rate = wins / trades.
  (The old code counted breakeven as a win, inflating win rate.)
- Bucket by account + period: day (`YYYY-MM-DD`, UTC), week
  (Monday-start), month (first of month). Dates derive from the deal's
  Unix `time` interpreted in UTC.
- Output: per-account rows plus a combined row (account = null) per
  period, sorted by period; summary block with total P&L, total trades,
  win rate %, profit factor, gross profit, gross loss.

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
  restic-style `passphrase_command` option, not an env var.
- `MT5_PNL_SNAPSHOT` carries only a file path, never secret material.
- Same trust boundary as the exporter: the OS user session. Documented in
  the README threat-model section when written.

## Error handling

- Exit 0 on success, 1 on any failure.
- Errors and warnings to stderr; stdout carries only tables/JSON.
- Messages name the path/value involved and say what to do next (no
  snapshot path given, snapshot file missing, missing passphrase,
  unsupported schema, unknown account label each have specific guidance).

## Testing

TDD throughout. Table-driven unit tests per package:

- `aggregate`: hand-computed expectations over fixture deals, including
  the breakeven, rounding, reversal-deal, and period-boundary cases; the
  old Python suite's cases serve as a cross-reference where the semantics
  overlap.
- `snapshot`: round-trip fixtures built in-test (age encrypt + gzip with a
  test passphrase); version-gate matrix (1.0 ok, higher-minor ok/refused
  per support level, 2.0 refused, garbage refused); wrong passphrase;
  truncated/corrupt file.
- `secrets`: `keyring.MockInit()` — no real keychain in tests.
- `render`: golden files for tables, exact-match for JSON.
- One end-to-end test compiling the binary and running `pnl`/`accounts`
  against an encrypted fixture (path via `--snapshot` and via
  `MT5_PNL_SNAPSHOT`), asserting stdout/stderr/exit codes.

## CI and releases

- GitHub Actions CI: `gofmt` check, `go vet`, `golangci-lint` (default
  linters), `go test -race` on ubuntu/macos/windows matrix.
- Renovate for dependency updates, mirroring the exporter.
- GoReleaser on `v*` tags: darwin/linux/windows × amd64/arm64, archives +
  checksums, version injected via ldflags. Works identically while the
  repo is private and after it goes public.
- Versioning: `v0.x` while iterating; `v1.0.0` once the CLI has been
  trusted in real day-to-day use for a while. Independent of the old
  `mt5-pnl` repo, which never shipped a release and can be archived
  whenever convenient.

## Repo

`github.com/tanem/mt5-pnl-cli`, private until v1 stabilises. MIT licence,
README, CLAUDE.md from day one; CONTRIBUTING and SECURITY mirroring the
exporter's before going public.

## Deviations from the repo-split Phase-2 contract

Recorded for traceability; all agreed during spec review on 2026-06-13:

- **No YAML config / `account_groups`.** Replaced by `--snapshot` +
  `MT5_PNL_SNAPSHOT` and an `--accounts` label filter. The config surface
  had shrunk to the point where a file cost more than it saved.
- **Flags refreshed instead of preserved verbatim.** `--group` →
  `--by`; `--group-name` → `--accounts`; bare `pnl` defaults to
  `--last 30d`; `--last` is calendar-accurate; `--from` alone runs to
  today.
- **Aggregation corrected, not ported.** Round-at-render instead of
  per-step rounding; breakeven deals are no longer counted as wins.
- **No parity gate on v1.0.0.** The old `mt5pnl` was never released, so
  parity ceremony serves no users; v1.0.0 is gated on real-world
  confidence instead.
