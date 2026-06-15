# CLI Ergonomics & Polish Bundle Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Land the self-contained ergonomics & polish bundle from the approved review (`docs/superpowers/specs/2026-06-16-cli-ergonomics-review.md`): drop the `--json` alias, add help examples, richer `--version`, `--quiet`, right-aligned numeric columns, a currency footer, and sign-based colour.

**Architecture:** Most tasks are localised flag/output changes. The one structural move is replacing `text/tabwriter` for the two tables with a small manual column-width writer (`internal/render/table.go`), because `tabwriter` counts ANSI escape bytes as visible width (breaking colour) and cannot right-align individual columns. A `render.TableOpts{Color, Currency}` struct carries the new display options without piling positional bools onto the table signatures. Colour is decided in the `cmd` layer (`resolveColor`, getenv-injectable like `resolveFormat`) and only the ANSI constants/application live in `render`.

**Tech Stack:** Go 1.25, standard library (`flag`, `encoding/json`, `encoding/csv`, `strings`, `strconv`), `golang.org/x/term` (already an indirect dependency — promoted to direct for TTY detection), GoReleaser for release ldflags.

**Conventions:** TDD throughout; golden files for table output (`go test ./internal/render -update`, then eyeball the diff); British/Commonwealth English in comments and docs; no hyperbole. After changing commands/architecture/gotchas, update `README.md` and `CLAUDE.md` in the same change. Implement on a branch based off `feat/cli-read-views-and-reporting` — **not** `main`. The bundle builds directly on the Phase 1 reporting work (`--format`, `resolveFormat`, `currenciesInScope`, `PnLTable(…, mixed)`), which is not yet on `main`; it merges to a release alongside that work (single-release model).

**Invariant guarded across Tasks 5–7:** only the *table* renderers change. `PnLJSON`, `PnLCSV`, `AccountsJSON` and `AccountsCSV` must stay byte-identical — the exact-match JSON test (`TestPnLJSON`) and the CSV tests are the guard. Never route JSON/CSV through `writeTable`.

---

## File Structure

- `cmd_common.go` — gains `resolveColor`; `resolveFormat` is simplified (loses the `--json` reconciliation); `loadSnapshot`'s warning writer is renamed for clarity.
- `cmd_pnl.go` / `cmd_accounts.go` — flag wiring: drop `--json`, add `--quiet`/`-q` (both), add `--color` (pnl only); help text gains Examples; table calls pass `render.TableOpts`.
- `main.go` — `version`/`--version` output uses the new `formatVersion`; top-level usage gains Examples.
- `version.go` — `commit`/`date` ldflag vars + `formatVersion` helper.
- `.goreleaser.yaml` — inject `main.commit` / `main.date`.
- `internal/render/render.go` — `TableOpts`, `signTone`, rewritten `PnLTable`/`AccountsTable`, currency + colour in the footer.
- `internal/render/table.go` (new) — `writeTable`, `colSpec`, `cell`, `tone`, `pad`, `colorise`, ANSI constants.
- Tests: `format_test.go`, `version_test.go`, `cli_test.go`, `internal/render/render_test.go`, new `color_test.go`; goldens `internal/render/testdata/pnl_table.golden`, `accounts_table.golden`.
- Docs: `README.md`, `CLAUDE.md`.

---

## Task 1: Drop the `--json` alias

Removes the back-compat alias so `--format` is the single spelling, deleting the only flag-conflict path.

**Files:**
- Modify: `cmd_common.go` (`resolveFormat`), `cmd_pnl.go`, `cmd_accounts.go`
- Modify: `format_test.go`, `cli_test.go`
- Modify: `README.md`, `CLAUDE.md`

- [ ] **Step 1: Rewrite the `resolveFormat` test for the one-argument signature**

Replace the whole body of `TestResolveFormat` in `format_test.go`:

```go
func TestResolveFormat(t *testing.T) {
	cases := []struct {
		name    string
		format  string
		want    string
		wantErr bool
	}{
		{"default table", "table", "table", false},
		{"json", "json", "json", false},
		{"csv", "csv", "csv", false},
		{"invalid value", "bogus", "", true},
	}
	for _, c := range cases {
		got, err := resolveFormat(c.format)
		if c.wantErr {
			if err == nil {
				t.Errorf("%s: want error, got %q", c.name, got)
			}
			continue
		}
		if err != nil || got != c.want {
			t.Errorf("%s: got %q (err %v), want %q", c.name, got, err, c.want)
		}
	}
}
```

- [ ] **Step 2: Run it to verify it fails to compile**

Run: `go test ./... -run TestResolveFormat`
Expected: build failure — `resolveFormat` still takes four arguments.

- [ ] **Step 3: Simplify `resolveFormat`**

In `cmd_common.go` replace the function (and update its doc comment):

```go
// resolveFormat validates the --format value. The legacy --json alias has been
// removed; --format is the single spelling.
func resolveFormat(format string) (string, error) {
	switch format {
	case "table", "json", "csv":
		return format, nil
	default:
		return "", fmt.Errorf("invalid --format %q: use table, json or csv", format)
	}
}
```

- [ ] **Step 4: Drop the `--json` flag and `Visit` plumbing from both commands**

In `cmd_pnl.go`, delete the `asJSON` flag line and replace the format-resolution block:

```go
	formatFlag := fs.String("format", "table", "output format: table, json or csv")
```
(remove `asJSON := fs.Bool("json", ...)`)

and replace lines 47–53 (the `set := map[string]bool{}` / `fs.Visit` / `resolveFormat` block) with:

```go
	format, err := resolveFormat(*formatFlag)
	if err != nil {
		fmt.Fprintln(stderr, "error:", err)
		return 1
	}
```

Apply the identical change in `cmd_accounts.go` (delete `asJSON`, replace the `set`/`Visit`/`resolveFormat` block with the four-line version above).

- [ ] **Step 5: Remove `--json` from both help strings**

Delete this line from `pnlHelp` (in `cmd_pnl.go`) and `accountsHelp` (in `cmd_accounts.go`):

```
  --json                    alias for --format json
```

- [ ] **Step 6: Update every test call site, delete the alias/conflict tests**

Find all remaining `--json` usages: `grep -n -- '--json' *.go`. Expected hits in `cli_test.go` only. For each, replace the single arg `"--json"` with the two args `"--format", "json"`. Specifically:
- `TestPnLJSONCommand` (≈ line 75): `…"--to", "2026-01-31", "--json"` → `…"--to", "2026-01-31", "--format", "json"`.
- `TestAccountsJSONCommand` (≈ line 408): `…"--snapshot", path, "--json", …` → `…"--snapshot", path, "--format", "json", …`.
- The mixed-currency JSON test (≈ line 363, `--by month --json`): `"--by", "month", "--json"` → `"--by", "month", "--format", "json"`.

Delete these three now-obsolete tests entirely (alias equivalence and conflict no longer exist): `TestPnLFormatJSONMatchesJSONAlias`, `TestPnLFormatJSONConflict`, `TestAccountsFormatJSONConflict`.

- [ ] **Step 7: Run the suite**

Run: `go test ./...`
Expected: PASS. Then `grep -rn -- '--json' *.go` returns nothing.

- [ ] **Step 8: Update the docs**

In `README.md`:
- Line ≈42: `` `--format json` (or the alias `--json`) `` → `` `--format json` ``.
- Line ≈100: `` `--format json` (or the alias `--json`) emits the same data for machines `` → `` `--format json` emits the same data for machines ``.
- pnl flag bullet (≈167–169): replace with
  `` - `--format table|json|csv` (default `table`). CSV is header + rows only (no summary block) — the spreadsheet/`mlr` import path. ``
- accounts flag bullet (≈177–178): replace with
  `` - `--format table|json|csv` (default `table`). ``

In `CLAUDE.md`, replace the gotcha (lines ≈53–56):

```markdown
- **`--format`.** `pnl`/`accounts` take `--format table|json|csv`
  (default `table`). CSV is rows-only (no summary).
```

Also grep `CLAUDE.md` for any other `--json` mention (`grep -n -- '--json' CLAUDE.md`) and remove the "documented alias" wording from the render bullet if present.

- [ ] **Step 9: Commit**

```bash
git add -A
git commit -m "feat: drop the --json alias; --format is the single spelling"
```

---

## Task 2: Examples in `--help`

clig.dev: lead with examples. Text-only.

**Files:**
- Modify: `cmd_pnl.go` (`pnlHelp`), `cmd_accounts.go` (`accountsHelp`), `main.go` (`usage`)
- Test: `cli_test.go`

- [ ] **Step 1: Write the failing test**

Add to `cli_test.go`:

```go
func TestPnLHelpShowsExamples(t *testing.T) {
	out, _, code := runCLI(t, "", "pnl", "-h")
	if code != 0 || !strings.Contains(out, "Examples:") {
		t.Errorf("exit %d; pnl help should show Examples:\n%s", code, out)
	}
	if !strings.Contains(out, "mt5-pnl-cli pnl --from") {
		t.Errorf("pnl help should show a worked example:\n%s", out)
	}
}

func TestAccountsHelpShowsExamples(t *testing.T) {
	out, _, code := runCLI(t, "", "accounts", "--help")
	if code != 0 || !strings.Contains(out, "Examples:") {
		t.Errorf("exit %d; accounts help should show Examples:\n%s", code, out)
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./... -run 'HelpShowsExamples'`
Expected: FAIL — no `Examples:` in help.

- [ ] **Step 3: Add Examples to the help strings**

Append to `pnlHelp` (before the closing backtick):

```

Examples:
  mt5-pnl-cli pnl --last 7d
  mt5-pnl-cli pnl --from 2026-01-01 --to 2026-03-31 --by month
  mt5-pnl-cli pnl --by month --format csv > pnl.csv
```

Append to `accountsHelp`:

```

Examples:
  mt5-pnl-cli accounts
  mt5-pnl-cli accounts --format json
```

In `main.go` `usage`, add before the closing backtick of the existing string:

```

Examples:
  mt5-pnl-cli pnl --last 30d
  mt5-pnl-cli accounts --format json
```

- [ ] **Step 4: Run to verify it passes**

Run: `go test ./... -run 'HelpShowsExamples'`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add -A
git commit -m "docs: add Examples sections to command help"
```

---

## Task 3: Richer `--version` (commit + build date)

**Files:**
- Modify: `version.go`, `main.go`, `.goreleaser.yaml`
- Test: `version_test.go`

- [ ] **Step 1: Write the failing test for `formatVersion`**

Add to `version_test.go`:

```go
func TestFormatVersion(t *testing.T) {
	tests := []struct {
		name                string
		ver, commit, date   string
		want                string
	}{
		{"version only", "1.2.3", "", "", "mt5-pnl-cli 1.2.3"},
		{"all set", "1.2.3", "abc1234", "2026-06-16", "mt5-pnl-cli 1.2.3 (commit abc1234, built 2026-06-16)"},
		{"commit only", "1.2.3", "abc1234", "", "mt5-pnl-cli 1.2.3 (commit abc1234)"},
		{"date only", "1.2.3", "", "2026-06-16", "mt5-pnl-cli 1.2.3 (built 2026-06-16)"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := formatVersion(tt.ver, tt.commit, tt.date); got != tt.want {
				t.Errorf("formatVersion(%q,%q,%q) = %q, want %q", tt.ver, tt.commit, tt.date, got, tt.want)
			}
		})
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./... -run TestFormatVersion`
Expected: build failure — `formatVersion` undefined.

- [ ] **Step 3: Add the ldflag vars and `formatVersion`**

In `version.go`, add alongside `var version = "dev"`:

```go
// commit and date are injected by GoReleaser via -ldflags; empty for local
// builds and `go install`, where they are simply omitted from the output.
var (
	commit = ""
	date   = ""
)

// formatVersion renders the binary identity line (without the schema suffix,
// which the caller appends). commit/date are shown only when set.
func formatVersion(ver, commit, date string) string {
	s := "mt5-pnl-cli " + ver
	var extra []string
	if commit != "" {
		extra = append(extra, "commit "+commit)
	}
	if date != "" {
		extra = append(extra, "built "+date)
	}
	if len(extra) > 0 {
		s += " (" + strings.Join(extra, ", ") + ")"
	}
	return s
}
```

(`strings` is already imported in `version.go`.)

- [ ] **Step 4: Wire it into `main.go`**

Replace the `version`/`--version` case body:

```go
	case "version", "--version":
		fmt.Fprintf(stdout, "%s (schema %d.%d)\n", formatVersion(resolveVersion(), commit, date), snapshot.SupportedMajor, snapshot.SupportedMinor)
		return 0
```

- [ ] **Step 5: Run the suite**

Run: `go test ./...`
Expected: PASS. (In tests `commit`/`date` are empty, so existing `version`/`--version` assertions — which expect `mt5-pnl-cli dev (schema 1.0)` and the tolerant e2e prefix check — still hold.)

- [ ] **Step 6: Inject commit/date at release time**

In `.goreleaser.yaml`, replace the ldflags line:

```yaml
    ldflags:
      - -s -w -X main.version={{.Version}} -X main.commit={{.Commit}} -X main.date={{.Date}}
```

Validate: `go run github.com/goreleaser/goreleaser/v2@latest check`
Expected: config is valid.

- [ ] **Step 7: Commit**

```bash
git add -A
git commit -m "feat: include commit and build date in version output"
```

---

## Task 4: `--quiet` / `-q`

Suppress the staleness and mixed-currency *warnings* (stderr) for clean pipelines. Errors still print.

**Files:**
- Modify: `cmd_pnl.go`, `cmd_accounts.go`, `cmd_common.go` (rename `loadSnapshot` writer param)
- Test: `cli_test.go`

- [ ] **Step 1: Write the failing test**

Add to `cli_test.go` (uses the same `fixture`/`runCLI` helpers; the fixture is stale because `--stale-after` is tiny):

```go
func TestPnLQuietSuppressesWarnings(t *testing.T) {
	path := fixture(t)
	// Without --quiet the staleness warning fires (default 2h vs old fixture).
	_, errOut, code := runCLI(t, "test-pass", "pnl", "--snapshot", path,
		"--from", "2026-01-01", "--to", "2026-01-31")
	if code != 0 || !strings.Contains(errOut, "warning") {
		t.Fatalf("precondition: expected a staleness warning, exit %d stderr %q", code, errOut)
	}
	// With --quiet stderr is clean.
	_, errOut, code = runCLI(t, "test-pass", "pnl", "--snapshot", path,
		"--from", "2026-01-01", "--to", "2026-01-31", "--quiet")
	if code != 0 || errOut != "" {
		t.Errorf("--quiet should silence warnings; exit %d stderr %q", code, errOut)
	}
}

func TestPnLQuietStillPrintsErrors(t *testing.T) {
	// A bad --from is an error, not a warning: --quiet must not hide it.
	_, errOut, code := runCLI(t, "test-pass", "pnl", "--from", "not-a-date", "--quiet")
	if code != 1 || !strings.Contains(errOut, "--from") {
		t.Errorf("error must still print under --quiet; exit %d stderr %q", code, errOut)
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./... -run 'PnLQuiet'`
Expected: FAIL — `flag provided but not defined: -quiet`.

- [ ] **Step 3: Add the flag (both spellings → one var) and a warning writer**

In `cmd_pnl.go`, after the existing flag declarations add:

```go
	var quiet bool
	fs.BoolVar(&quiet, "quiet", false, "suppress warnings on stderr")
	fs.BoolVar(&quiet, "q", false, "suppress warnings on stderr (shorthand)")
```

After flags are parsed and validated, introduce the warning writer and use it for both warning sites:

```go
	warnW := io.Writer(stderr)
	if quiet {
		warnW = io.Discard
	}
```

Change the `loadSnapshot` call to pass `warnW` instead of `stderr`, and change the mixed-currency `fmt.Fprintf(stderr, …)` to `fmt.Fprintf(warnW, …)`. Leave every `error:` print on `stderr`.

In `cmd_accounts.go`, add the same two `fs.BoolVar` lines and the `warnW` block, and pass `warnW` to `loadSnapshot` (accounts has no mixed-currency warning).

- [ ] **Step 4: Rename `loadSnapshot`'s writer param for clarity**

In `cmd_common.go`, rename the `stderr io.Writer` parameter of `loadSnapshot` to `warnW io.Writer` and update its single use (`warnIfStale(warnW, …)`). This documents that the writer carries warnings only; errors are returned.

- [ ] **Step 5: Run to verify it passes**

Run: `go test ./... -run 'PnLQuiet|Staleness'`
Expected: PASS. Then `go test ./...` for the full suite.

- [ ] **Step 6: Add help + docs**

Add to `pnlHelp` and `accountsHelp` (in the Flags block, before `-h, --help`):

```
  -q, --quiet               suppress warnings on stderr
```

In `README.md`, under the shared-flags paragraph (the `--snapshot`/`--stale-after` note), add a sentence: `Pass --quiet (-q) to silence warnings for scripted use; errors still print.` Add a one-line gotcha to `CLAUDE.md`: `- **--quiet/-q** silences stderr warnings (staleness, mixed-currency); errors still print.`

- [ ] **Step 7: Commit**

```bash
git add -A
git commit -m "feat: add --quiet/-q to suppress warnings"
```

---

## Task 5: Right-align numeric columns (manual table writer)

Introduces `internal/render/table.go` and routes both tables through it with per-column alignment. This is the structural task colour builds on.

**Files:**
- Create: `internal/render/table.go`
- Modify: `internal/render/render.go` (`PnLTable`, `AccountsTable`, add `TableOpts`, `signTone`)
- Modify: `cmd_pnl.go`, `cmd_accounts.go` (pass `render.TableOpts{}`)
- Modify: `internal/render/render_test.go` (signature updates), regenerate goldens

- [ ] **Step 1: Write the failing test for `writeTable` alignment**

Create `internal/render/table_test.go`:

```go
package render

import (
	"bytes"
	"testing"
)

func TestWriteTableRightAligns(t *testing.T) {
	cols := []colSpec{{"NAME", false}, {"AMOUNT", true}}
	rows := [][]cell{
		{{"a", toneNone}, {"5.00", toneNone}},
		{{"bb", toneNone}, {"100.00", toneNone}},
	}
	var buf bytes.Buffer
	if err := writeTable(&buf, cols, rows, false); err != nil {
		t.Fatal(err)
	}
	got := buf.String()
	// AMOUNT column is right-aligned to width 6 ("100.00"); "5.00" gets two
	// leading spaces. Two-space gutter after the NAME column (width 4: "NAME").
	want := "NAME  AMOUNT\n" +
		"a       5.00\n" +
		"bb    100.00\n"
	if got != want {
		t.Errorf("got:\n%q\nwant:\n%q", got, want)
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./internal/render -run TestWriteTableRightAligns`
Expected: build failure — `writeTable`/`colSpec`/`cell`/`toneNone` undefined.

- [ ] **Step 3: Implement the table writer**

Create `internal/render/table.go`:

```go
package render

import (
	"io"
	"strings"
)

type tone int

const (
	toneNone tone = iota
	tonePos
	toneNeg
)

const (
	ansiGreen = "\x1b[32m"
	ansiRed   = "\x1b[31m"
	ansiReset = "\x1b[0m"
)

type colSpec struct {
	header string
	right  bool
}

type cell struct {
	text string
	tone tone
}

// writeTable renders a header and body rows as a fixed-width table with
// per-column alignment. Widths are computed from the plain cell text, so ANSI
// colour (applied after padding when color is true) never skews alignment.
// Columns are separated by two spaces (matching the old tabwriter gutter); a
// left-aligned final column is not right-padded, so there is no trailing space.
func writeTable(w io.Writer, cols []colSpec, rows [][]cell, color bool) error {
	widths := make([]int, len(cols))
	for i, c := range cols {
		widths[i] = len(c.header)
	}
	for _, row := range rows {
		for i, cl := range row {
			if len(cl.text) > widths[i] {
				widths[i] = len(cl.text)
			}
		}
	}
	var b strings.Builder
	writeRow := func(get func(i int) (string, tone)) {
		for i := range cols {
			if i > 0 {
				b.WriteString("  ")
			}
			text, tn := get(i)
			last := i == len(cols)-1
			if !(last && !cols[i].right) {
				text = pad(text, widths[i], cols[i].right)
			}
			b.WriteString(colorise(text, tn, color))
		}
		b.WriteByte('\n')
	}
	writeRow(func(i int) (string, tone) { return cols[i].header, toneNone })
	for _, row := range rows {
		writeRow(func(i int) (string, tone) { return row[i].text, row[i].tone })
	}
	_, err := io.WriteString(w, b.String())
	return err
}

func pad(s string, width int, right bool) string {
	gap := width - len(s)
	if gap <= 0 {
		return s
	}
	if right {
		return strings.Repeat(" ", gap) + s
	}
	return s + strings.Repeat(" ", gap)
}

func colorise(s string, t tone, color bool) string {
	if !color || t == toneNone {
		return s
	}
	code := ansiGreen
	if t == toneNeg {
		code = ansiRed
	}
	return code + s + ansiReset
}
```

- [ ] **Step 4: Run to verify it passes**

Run: `go test ./internal/render -run TestWriteTableRightAligns`
Expected: PASS.

- [ ] **Step 5: Add `TableOpts` + `signTone` and rewrite `PnLTable`**

In `internal/render/render.go` add:

```go
// TableOpts carries display-only options for the table renderers. Color enables
// ANSI sign colouring; Currency, when set, is shown once in the pnl summary
// footer. Both are ignored by JSON/CSV.
type TableOpts struct {
	Color    bool
	Currency string
}

func signTone(x float64) tone {
	switch {
	case x > 0:
		return tonePos
	case x < 0:
		return toneNeg
	default:
		return toneNone
	}
}
```

Replace `PnLTable` with (alignment: PERIOD/ACCOUNT left, P&L/TRADES/WINS/LOSSES right; colour and currency are wired in Tasks 6–7 but the parameter is added now):

```go
func PnLTable(w io.Writer, rows []aggregate.Row, sum aggregate.Summary, labels map[int64]string, mixed bool, opts TableOpts) error {
	cols := []colSpec{
		{"PERIOD", false}, {"ACCOUNT", false}, {"P&L", true},
		{"TRADES", true}, {"WINS", true}, {"LOSSES", true},
	}
	body := make([][]cell, 0, len(rows))
	for _, r := range rows {
		acct := "ALL"
		combined := r.Account == nil
		if !combined {
			acct = labels[*r.Account]
			if acct == "" {
				acct = strconv.FormatInt(*r.Account, 10)
			}
		}
		pnlText := fmt.Sprintf("%.2f", r.PnL)
		pnlTone := signTone(r.PnL)
		if mixed && combined {
			pnlText, pnlTone = "n/a", toneNone
		}
		body = append(body, []cell{
			{r.Period, toneNone}, {acct, toneNone}, {pnlText, pnlTone},
			{strconv.Itoa(r.Trades), toneNone},
			{strconv.Itoa(r.Wins), toneNone},
			{strconv.Itoa(r.Losses), toneNone},
		})
	}
	if err := writeTable(w, cols, body, opts.Color); err != nil {
		return err
	}

	totalPnL := fmt.Sprintf("%.2f", sum.TotalPnL)
	grossProfit := fmt.Sprintf("%.2f", sum.GrossProfit)
	grossLoss := fmt.Sprintf("%.2f", sum.GrossLoss)
	profitFactor := fmtPtr(sum.ProfitFactor, "%.2f")
	totalTone := signTone(sum.TotalPnL)
	if mixed {
		totalPnL, grossProfit, grossLoss, profitFactor = "n/a", "n/a", "n/a", "n/a"
		totalTone = toneNone
	}
	cur := ""
	if opts.Currency != "" {
		cur = " " + opts.Currency
	}
	_, err := fmt.Fprintf(w,
		"\nTotal P&L: %s%s  Trades: %d  Win rate: %s  Profit factor: %s  Gross profit: %s  Gross loss: %s\n",
		colorise(totalPnL, totalTone, opts.Color), cur, sum.TotalTrades,
		fmtPtr(sum.WinRatePct, "%.1f%%"), profitFactor, grossProfit, grossLoss)
	return err
}
```

Remove the now-unused `text/tabwriter` import from `render.go` *only if* `AccountsTable` (next step) also stops using it.

- [ ] **Step 6: Rewrite `AccountsTable` through `writeTable`**

Replace `AccountsTable` (BALANCE/EQUITY right, the rest left; no colour flag for accounts so `opts.Color` stays false in practice):

```go
func AccountsTable(w io.Writer, accounts []snapshot.AccountSnapshot, generatedAt string, opts TableOpts) error {
	cols := []colSpec{
		{"LOGIN", false}, {"LABEL", false}, {"CURRENCY", false},
		{"BALANCE", true}, {"EQUITY", true},
		{"LAST SUCCESS", false}, {"LAST ERROR", false},
	}
	body := make([][]cell, 0, len(accounts))
	for _, a := range accounts {
		body = append(body, []cell{
			{strconv.FormatInt(a.Login, 10), toneNone},
			{a.Label, toneNone},
			{a.Currency, toneNone},
			{fmt.Sprintf("%.2f", a.Balance), toneNone},
			{fmt.Sprintf("%.2f", a.Equity), toneNone},
			{strOr(a.LastSuccessAt, "-"), toneNone},
			{strOr(a.LastError, "-"), toneNone},
		})
	}
	if err := writeTable(w, cols, body, opts.Color); err != nil {
		return err
	}
	_, err := fmt.Fprintf(w, "\nSnapshot generated: %s\n", generatedAt)
	return err
}
```

Now remove the `"text/tabwriter"` import from `render.go`.

- [ ] **Step 7: Update render-test call sites and other callers**

In `internal/render/render_test.go`, update every `PnLTable(...)` call to pass a final `render.TableOpts{}` (the tests are `package render_test`): the calls in `TestPnLTable`, `TestPnLTableUnknownLabelFallsBackToLogin`, `TestPnLTableNilSummaryFields`, `TestPnLTableMixedNA`. Update every `AccountsTable(...)` call (`TestAccountsTable`) to pass a final `render.TableOpts{}`.

Wait — `render_test.go` is `package render_test`, but `table_test.go` (Step 1) is `package render` (it touches unexported `writeTable`). Both compile together; keep `table_test.go` internal and `render_test.go` external.

In `cmd_pnl.go` change the table call: `render.PnLTable(stdout, rows, sum, labels, mixed, render.TableOpts{})`.
In `cmd_accounts.go` change the table call: `render.AccountsTable(stdout, snap.Accounts, snap.GeneratedAt, render.TableOpts{})`.

- [ ] **Step 8: Regenerate goldens and eyeball**

Run: `go test ./internal/render -update`
Then inspect: `git diff internal/render/testdata/`
Expected: numeric columns now right-aligned; otherwise structurally the same (header row, two-space gutters, summary footer unchanged for the no-opts case). Confirm the diff is alignment-only.

- [ ] **Step 9: Run the full suite (invariant check)**

Run: `go test ./...`
Expected: PASS — crucially `TestPnLJSON` (exact-match) and the CSV tests are unchanged, proving JSON/CSV output is byte-identical.

- [ ] **Step 10: Update README Demo + commit**

Update the two table blocks in the `README.md` Demo to match the regenerated right-aligned output (copy from the goldens / a local run). Then:

```bash
git add -A
git commit -m "feat: right-align numeric table columns via a manual table writer"
```

---

## Task 6: Currency in the pnl summary footer

**Files:**
- Modify: `cmd_pnl.go` (set `TableOpts.Currency`)
- Test: `internal/render/render_test.go`
- Modify: `README.md`

- [ ] **Step 1: Write the failing test**

Add to `internal/render/render_test.go`:

```go
func TestPnLTableCurrencyFooter(t *testing.T) {
	var buf bytes.Buffer
	if err := render.PnLTable(&buf, rows, sum, labels, false, render.TableOpts{Currency: "USD"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "Total P&L: 5.00 USD") {
		t.Errorf("summary should show the currency:\n%s", buf.String())
	}
}

func TestPnLTableNoCurrencyWhenUnset(t *testing.T) {
	var buf bytes.Buffer
	if err := render.PnLTable(&buf, rows, sum, labels, false, render.TableOpts{}); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(buf.String(), "USD") {
		t.Errorf("no currency expected when unset:\n%s", buf.String())
	}
}
```

- [ ] **Step 2: Run to verify the first passes, second passes**

Run: `go test ./internal/render -run 'CurrencyFooter|NoCurrencyWhenUnset'`
Expected: PASS — the footer logic already added in Task 5 handles `opts.Currency`. (If `TestPnLTableCurrencyFooter` fails, the Task 5 footer wiring is wrong; fix there.)

This task is wiring, so the render side needs no new code; the test simply locks the behaviour.

- [ ] **Step 3: Wire the single-currency value in `cmd_pnl.go`**

Where `cmd_pnl.go` builds the table call, replace `render.TableOpts{}` with:

```go
	opts := render.TableOpts{}
	if len(curs) == 1 {
		opts.Currency = curs[0]
	}
	...
	err = render.PnLTable(stdout, rows, sum, labels, mixed, opts)
```

(`curs` is the existing `currenciesInScope(...)` result; under mixed currency `len(curs) > 1`, so `Currency` stays empty and the footer is already `n/a`.)

- [ ] **Step 4: Run the suite**

Run: `go test ./...`
Expected: PASS.

- [ ] **Step 5: Update README Demo + commit**

Update the pnl table Demo footer in `README.md` to show the currency (e.g. `Total P&L: 10.00 USD  Trades: …`). Note in the pnl flag docs that the summary footer shows the account currency for single-currency scope.

```bash
git add -A
git commit -m "feat: show account currency in the pnl summary footer"
```

---

## Task 7: Colour output (`--color`, sign-based)

Colour scoped to `pnl` only: per-row P&L cells and the summary total, by sign. `accounts` is unchanged (no `--color` flag; `AccountsTable` keeps `Color:false`). Precedence: `--color=always|never` win outright; only `auto` consults `NO_COLOR`, `TERM=dumb` and TTY detection. Colour needs an ANSI-capable terminal (modern Windows Terminal included); no VT-enabling syscalls are added (YAGNI).

**Files:**
- Modify: `go.mod` (promote `golang.org/x/term` to direct), `cmd_common.go` (`resolveColor`), `cmd_pnl.go` (`--color` flag + wire `TableOpts.Color`)
- Create: `color_test.go`
- Test: `internal/render/render_test.go`, `cli_test.go`
- Modify: `README.md`, `CLAUDE.md`

- [ ] **Step 1: Write the failing test for `resolveColor`**

Create `color_test.go`:

```go
package main

import (
	"bytes"
	"testing"
)

func TestResolveColor(t *testing.T) {
	env := func(m map[string]string) func(string) string {
		return func(k string) string { return m[k] }
	}
	var buf bytes.Buffer // not an *os.File, so isatty is false

	cases := []struct {
		name string
		mode string
		env  map[string]string
		want bool
	}{
		{"always overrides everything", "always", map[string]string{"NO_COLOR": "1"}, true},
		{"never overrides everything", "never", nil, false},
		{"auto to a buffer is off", "auto", nil, false},
		{"auto honours NO_COLOR", "auto", map[string]string{"NO_COLOR": "1"}, false},
		{"auto honours TERM=dumb", "auto", map[string]string{"TERM": "dumb"}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := resolveColor(c.mode, &buf, env(c.env)); got != c.want {
				t.Errorf("resolveColor(%q) = %v, want %v", c.mode, got, c.want)
			}
		})
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./... -run TestResolveColor`
Expected: build failure — `resolveColor` undefined.

- [ ] **Step 3: Implement `resolveColor`**

In `cmd_common.go` add the import `"golang.org/x/term"` (and ensure `"os"` is imported — it is) and:

```go
// resolveColor decides whether to emit ANSI colour. always/never are absolute;
// auto enables colour only for an interactive terminal that has not opted out
// via NO_COLOR or TERM=dumb. w is the real output stream: colour is auto-off
// whenever it is not a *os.File TTY (pipes, files, test buffers), which keeps
// machine-consumed output clean.
func resolveColor(mode string, w io.Writer, getenv func(string) string) bool {
	switch mode {
	case "always":
		return true
	case "never":
		return false
	default: // "auto"
		if getenv("NO_COLOR") != "" || getenv("TERM") == "dumb" {
			return false
		}
		f, ok := w.(*os.File)
		return ok && term.IsTerminal(int(f.Fd()))
	}
}
```

- [ ] **Step 4: Run to verify it passes; promote the dependency**

Run: `go test ./... -run TestResolveColor`
Expected: PASS.
Run: `go mod tidy` — moves `golang.org/x/term` from the indirect block to a direct require. Confirm with `git diff go.mod`.

- [ ] **Step 5: Write the render colour test**

Add to `internal/render/render_test.go`:

```go
func TestPnLTableColorBySign(t *testing.T) {
	signed := []aggregate.Row{
		{Period: "2026-01-05", Account: ptr(int64(111)), PnL: 5.0, Trades: 1, Wins: 1},
		{Period: "2026-01-12", Account: ptr(int64(111)), PnL: -3.0, Trades: 1, Losses: 1},
	}
	var on, off bytes.Buffer
	if err := render.PnLTable(&on, signed, sum, labels, false, render.TableOpts{Color: true}); err != nil {
		t.Fatal(err)
	}
	if err := render.PnLTable(&off, signed, sum, labels, false, render.TableOpts{}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(on.String(), "\x1b[32m") || !strings.Contains(on.String(), "\x1b[31m") {
		t.Errorf("colour on: expected green and red codes:\n%q", on.String())
	}
	if strings.Contains(off.String(), "\x1b[") {
		t.Errorf("colour off: expected no ANSI codes:\n%q", off.String())
	}
}
```

- [ ] **Step 6: Run to verify it passes**

Run: `go test ./internal/render -run TestPnLTableColorBySign`
Expected: PASS — the colour application already exists in `writeTable`/the footer from Task 5; this test confirms the per-row and code paths.

- [ ] **Step 7: Add the `--color` flag and wire it (pnl only)**

In `cmd_pnl.go` add a flag:

```go
	colorMode := fs.String("color", "auto", "colour output: auto, always or never")
```

After parsing, validate it (mirroring `--by`):

```go
	if *colorMode != "auto" && *colorMode != "always" && *colorMode != "never" {
		fmt.Fprintf(stderr, "error: invalid --color %q: use auto, always or never\n", *colorMode)
		return 1
	}
```

When building `opts`, set `opts.Color = resolveColor(*colorMode, stdout, os.Getenv)`. Add `--color` to `pnlHelp` Flags:

```
  --color auto|always|never colourise P&L by sign (default auto)
```

(`os` must be imported in `cmd_pnl.go`.)

- [ ] **Step 8: Write the end-to-end wiring test**

Add to `cli_test.go`:

```go
func TestPnLColorAlwaysForcesAnsi(t *testing.T) {
	path := fixture(t)
	out, _, code := runCLI(t, "test-pass", "pnl", "--snapshot", path,
		"--from", "2026-01-01", "--to", "2026-01-31", "--stale-after", "876000h", "--color=always")
	if code != 0 || !strings.Contains(out, "\x1b[") {
		t.Errorf("--color=always should emit ANSI; exit %d out %q", code, out)
	}
}

func TestPnLDefaultNoColorToBuffer(t *testing.T) {
	path := fixture(t)
	out, _, code := runCLI(t, "test-pass", "pnl", "--snapshot", path,
		"--from", "2026-01-01", "--to", "2026-01-31", "--stale-after", "876000h")
	if code != 0 || strings.Contains(out, "\x1b[") {
		t.Errorf("default (non-TTY buffer) should be uncoloured; exit %d out %q", code, out)
	}
}
```

- [ ] **Step 9: Run to verify it passes; full suite**

Run: `go test ./... -run 'PnLColor|DefaultNoColor'`
Expected: PASS.
Run: `go test ./...`
Expected: PASS — goldens are unaffected (generated with `Color:false`).

- [ ] **Step 10: Docs + commit**

In `README.md`: under the pnl flags, document `--color auto|always|never` (default `auto`; off when output is piped/redirected; honours `NO_COLOR` and `TERM=dumb`). Add a short note to the "Why"/Agent-friendly section that colour is auto-disabled for non-terminals so pipelines stay clean.
In `CLAUDE.md`: add a gotcha — `- **--color** (pnl only): auto/always/never; auto needs a *os.File TTY and honours NO_COLOR/TERM=dumb. Colour is applied after width padding (manual table writer) so ANSI never skews alignment.`

```bash
git add -A
git commit -m "feat: add sign-based colour to pnl with --color and auto TTY detection"
```

---

## Self-review notes

- **Spec coverage (review items):** colour (Task 7), right-align (Task 5), help examples (Task 2), `--quiet` (Task 4), `--version` metadata (Task 3), currency footer (Task 6), drop `--json` (Task 1). All seven bundle items covered.
- **Invariant:** Tasks 5–7 change only the table renderers; `TestPnLJSON` (exact-match) and the CSV tests stand unchanged as the byte-identical guard (Task 5 Step 9).
- **Type consistency:** `TableOpts{Color bool; Currency string}` introduced in Task 5 and used unchanged in Tasks 6–7; `writeTable`/`colSpec`/`cell`/`tone`/`colorise`/`pad`/`signTone` defined once in Task 5; `resolveColor` and `formatVersion` are pure, getenv/arg-injected helpers tested in isolation.
- **Deferred (not in this plan, by design):** `--sort` (with Phase 3), `--cumulative` (Phase 2), `jsonl`, stdin `-`, shell completion, repeatable `--accounts` — see the review doc's sequencing.
