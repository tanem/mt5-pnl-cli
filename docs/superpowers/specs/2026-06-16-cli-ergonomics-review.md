# mt5-pnl-cli — CLI ergonomics & feature review

Date: 2026-06-16. Status: approved (recommendations accepted; nothing
scheduled for build).

A sweep of the CLI's behaviour, flags and output against the
[Command Line Interface Guidelines](https://clig.dev) and a set of
well-regarded open-source CLIs, to surface cleanups and additions worth
making. Everything here is filtered through the tool's established
character: a single-purpose, flags-only, pipe-first reader of one
encrypted snapshot (see the v1 design,
`2026-06-13-mt5-pnl-cli-v1-design.md`).

This complements — and is cross-referenced against — the approved
reporting roadmap (`2026-06-15-cli-read-views-and-reporting-design.md`),
whose Phase 2 and Phase 3 are specced but not yet built. Items already
designed there are tagged so they are not mistaken for net-new ideas.

## The lens

The discipline is "do one thing well": the value comes as much from what
stays out (no TUI, no `--watch`, no charts, no plugins, no config schema)
as from what goes in. Recommendations are weighed on value, surface area
and fit with that character — convention from exemplars is followed where
it pulls in the same direction, not for its own sake.

Exemplars surveyed: `git`, `gh`, `ripgrep` / `fd` / `bat`, `jq`,
`restic`, `kubectl`, `ffmpeg`, `curl`.

### What the CLI already gets right

Measured against clig.dev, the basics are already in place and need no
change:

- Primary output to **stdout**, warnings and errors to **stderr** — so
  pipelines stay clean.
- Machine-readable output via `--format json` (and `csv`).
- `--version` as a flag and subcommand, with **no `-v`** (avoids the
  verbose/version clash).
- `-h` / `--help` print hand-written help to stdout and exit 0; genuine
  parse errors go to stderr and exit 1.
- Conventional exit codes (0 success, non-zero failure).
- Named flags throughout rather than positional arguments.

The gaps cluster in four areas: colour, output polish, verbosity control
and discoverability.

## Recommendations

| # | Item | Verdict | Note |
|---|------|---------|------|
| 1 | Colour output (ANSI, sign-based) | **Adopt** | See 1 below — auto-off is load-bearing |
| 2 | Right-align numeric columns | **Adopt** | tabwriter has no per-column alignment; see 2 |
| 3 | Examples in `--help` | **Adopt** | clig.dev: lead with examples |
| 4 | `--quiet` / `-q` | **Adopt** | Silence stderr warnings for pipelines |
| 5 | Richer `--version` (commit + date) | **Adopt** | ldflags; goreleaser already in use |
| 6 | Show account currency in `pnl` | **Adopt (light)** | In the summary footer, not per-row |
| 13 | Drop the `--json` alias | **Drop** | Single spelling `--format`; pre-1.0 break |
| 7 | `--sort` (by pnl / trades) | **Adopt — with Phase 3** | Earns its keep alongside `--by symbol\|magic` |
| 8 | `-` (stdin) for `--snapshot` | **Adopt (low)** | clig composability; most optional adopt |
| 9 | `--cumulative` running P&L | **Defer → Phase 2** | Reuses the ordered-deal pass drawdown needs |
| 10 | NDJSON (`--format jsonl`) | **Defer → Phase 3** | Only meaningful once `trades`/`cash-flows` exist |
| 11 | Shell completion | **Defer** | stdlib `flag` has none; reconsider after Phase 3 |
| 12 | Repeatable `--accounts` | **Drop** | Comma-separated form already covers it |

### Adopt — the ergonomics & polish bundle

These six are small, cohesive, and independent of the reporting roadmap;
they could ship as their own pass (see Sequencing).

**1. Colour output.** Emit ANSI colour driven by the sign of a P&L value
(profit / loss), the textbook use-case for colour in a CLI (`git`, `ls`,
`bat`, `ripgrep`). The ANSI emission itself needs no dependency.

The behaviour around it is **load-bearing, not optional** — the whole
value proposition is clean piped output, so colour must never reach a
pipe or file:

- Default `--color=auto`: colour only when stdout is an interactive
  terminal.
- Honour the `NO_COLOR` environment variable (any non-empty value
  disables colour) and `TERM=dumb`.
- `--color=always|never` for explicit control.

Implementation note (honest): `--color=auto` is the one part that needs
TTY detection. The Go standard library has no `isatty`; the clean,
cross-platform choice is `golang.org/x/term` (a quasi-standard,
Renovate-manageable module). The colour codes and the `always`/`never`/
`NO_COLOR` paths need nothing extra — only `auto` pulls in that module.

**2. Right-align numeric columns.** P&L, balance and equity columns are
currently left-aligned, so decimal points do not line up and figures are
harder to scan. Right-aligning the numeric columns is a clear readability
win for a financial table.

Implementation note (honest): `text/tabwriter` only right-aligns *all*
cells, which would also right-align the text columns (`PERIOD`,
`ACCOUNT`, labels) and read oddly. Per-column alignment therefore means
pre-formatting numeric cells to a common width (a width pass per numeric
column) rather than a single tabwriter flag. Modest, but not a one-liner.

**3. Examples in `--help`.** The per-command help lists flags but shows
no examples; clig.dev (and `gh`, `fd`, `ripgrep`) lead with them. Append
a short `Examples:` block to each command's help string — e.g. a relative
range, an explicit range with monthly grouping, and a CSV export. Text
only.

**4. `--quiet` / `-q`.** Suppress the non-essential stderr warnings (the
staleness warning, the mixed-currency warning) for scripted use. Errors
still print — quiet silences warnings, not failures (clig.dev). A
standard, low-surface flag.

**5. Richer `--version`.** Inject the build commit and date via `-ldflags`
(goreleaser is already the release tool and supports this), so the output
reads, e.g., `mt5-pnl-cli v1.2.3 (commit abc1234, built 2026-06-16,
schema 1.0)`. This makes bug reports reproducible. (`version --format
json` is a minor follow-on, deferred — see below.)

**6. Show account currency in `pnl` (light).** The `accounts` view shows
a `CURRENCY` column; `pnl` shows none, so the figures are unlabelled. Show
the currency once in the summary footer (single-currency scope only; under
mixed currency the summary is already suppressed by the guard). No
per-row currency column — that is clutter for the common single-currency
case.

### Cleanup

**13. Drop the `--json` alias.** With `--format table|json|csv` in place,
`--json` is pure back-compat and carries the only flag-conflict path in
the code (`--json` disagreeing with `--format`). Removing it leaves one
spelling (`--format json`) and deletes that conflict handling. The CLI is
pre-1.0, so the break is acceptable and preferred over keeping two
spellings.

### Adopt — tied to a later phase

**7. `--sort` (by pnl / trades), with Phase 3.** Sorting rows only earns
its keep once `--by symbol|magic` lands (e.g. "most profitable strategy
first"); for time cuts the natural order is chronological. The reporting
roadmap already lists `--sort` as deferred to that round. Keep it minimal
when built — a single sort key plus direction, no multi-key grammar.

### Adopt — low priority

**8. `-` (stdin) for `--snapshot`.** Accept `--snapshot -` to read the
encrypted snapshot from stdin, per clig.dev's composability convention
(`curl URL | mt5-pnl-cli pnl --snapshot -`). Cheap (a few lines), and the
most optional of the adopts — fine to cut if the surface isn't wanted.

### Deferred — recorded with a trigger

- **9. `--cumulative` running P&L → revisit in Phase 2.** A running
  cumulative-P&L column on time-series rows. Phase 2's max-drawdown
  already requires an ordered-deal cumulative pass, so this is cheapest to
  decide while building that. Don't pre-commit.
- **10. NDJSON (`--format jsonl`) → revisit in Phase 3.** One JSON object
  per line, ideal for large exports piped to `jq`. Only meaningful once
  `trades` / `cash-flows` (the high-row-count views) exist.
- **11. Shell completion → reconsider after Phase 3.** Strong for
  discoverability (flag values; potentially account labels), but the CLI
  uses the standard library's `flag`, which has no completion, so this
  means hand-written completion scripts (a real maintenance cost) or a
  framework move the v1 design deliberately avoided. The current flag
  surface is small enough that the payoff is modest; revisit once Phase 3
  has added `positions` / `trades` / `cash-flows` and `--by symbol|magic`
  and the surface is larger.

### Dropped from this round

- **12. Repeatable `--accounts`.** clig.dev favours repeatable flags, but
  the existing comma-separated form (`--accounts "A,B"`) already covers
  the need cleanly; a second input mode for one flag is surface creep for
  little gain. Trivially addable later if asked for.

## Deliberately left out — the simplicity filter

These were considered and rejected. Recorded so they are not
re-litigated.

| Rejected | Why |
|----------|-----|
| Pager (git/bat-style) | Output is small; adds TTY/pager plumbing for no gain |
| `--limit` / top-N | Don't reimplement `head` — `… --format csv \| head` or `jq` already does it |
| Self-update command | The package manager's / Releases' job; bloats a single-purpose binary |
| Telemetry / analytics | Privacy; never |
| Man-page generation | README + `--help` suffice; low ROI for a modern single binary |
| Fiscal-year flag (`--fy`) | Fiscal years are jurisdiction-specific; the CLI stays neutral — use explicit `--from/--to` |
| Richer exit codes (sysexits) | 0/1 is enough; extra codes add surface scripts won't use |
| `-o` short alias for `--format` | `--format` already exists; a third spelling is clutter (same reasoning that kept `-v` out) |
| Period-over-period / streaks / ROI% | A different, analytical tool; run two ranges or compute downstream |
| Config file / saved views | Already rejected in the roadmap — if ever, a ripgrep-style default-flags file or shell alias, never a schema |

## Already designed in Phase 2/3 (cross-reference, not net-new)

From `2026-06-15-cli-read-views-and-reporting-design.md`:

- P&L component breakdown (trade profit vs commission / swap / fee).
- Deeper summary metrics: expectancy, average / largest win and loss,
  max drawdown.
- Two-group summary block (performance + P&L breakdown).
- `--by symbol|magic`.
- New read-views: `positions`, `trades`, `cash-flows`.

## Suggested sequencing

1. **Ergonomics & polish bundle** (items 1–6 + 13): colour, right-align,
   help examples, `--quiet`, `--version` metadata, currency footer, and
   dropping `--json`. Self-contained and independent of the reporting
   roadmap — can ship on its own.
2. **Within the reporting roadmap**: `--sort` (7) with Phase 3; decide
   `--cumulative` (9) during Phase 2; decide `jsonl` (10) during Phase 3.
3. **Standalone, low priority**: stdin `-` (8) whenever convenient.
4. **Revisit later**: shell completion (11) after Phase 3.

## Notes

- The CLI reports in the account currency and stays jurisdiction-neutral;
  tax-reporting use is one motivation among several and never narrows the
  tool to one regime (consistent with the reporting design).
- Anything built from this review follows the project's standing rules:
  TDD, golden files for table output, British/Commonwealth English, and
  updating `README.md` / `CLAUDE.md` in the same change.
