# Phase 1 — Foundations Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Ship the output/ergonomics foundations from the read-views spec: correct help/exit codes, a `--format table|json|csv` contract (with `--json` as a documented alias), a mixed-currency guard for `pnl`, and a verified-and-documented note on deal-time bucketing.

**Architecture:** Command layer (`main.go`, `cmd_pnl.go`, `cmd_accounts.go`, `cmd_common.go`) gains shared helpers for flag parsing and format resolution; the `internal/render` package gains CSV renderers and a `mixed bool` parameter so it can suppress cross-currency sums. No new commands and no `--by symbol|magic` — those are Phase 3. Logic stays out of `main` (still the only `os.Exit` caller).

**Tech Stack:** Go 1.25, stdlib `flag`, `encoding/csv`, `encoding/json`, `text/tabwriter`. Tests are table-driven; `internal/render` uses golden files plus exact-string matches. Module path `github.com/tanem/mt5-pnl-cli`.

**Branch:** Work on `feat/cli-read-views-and-reporting` (already checked out; the spec commit lives here). Do not branch off `main`.

**Spec:** `docs/superpowers/specs/2026-06-15-cli-read-views-and-reporting-design.md` (sections 1.1–1.4).

**Scope notes / deliberate deferrals (do not implement here):**
- `positions` / `trades` / `cash-flows` commands → Phase 3. So the spec's CSV schemas for those, and `--format` on them, are out of scope. Phase 1 `--format` applies to `pnl` and `accounts` only.
- `--by symbol|magic`, the uniform `group`/`group_by` JSON rename, and the P&L component columns (`trade_profit`/`commission`/`swap`/`fee`) → Phases 2–3. Phase 1 `pnl` CSV therefore keys on `period` (mirroring the unchanged Phase 1 JSON), and carries only the columns that exist today.
- **Schema-evolution decision (record in commit + docs):** Phase 1 `pnl` CSV header is `period,account_login,account_label,pnl,trades,wins,losses,gross_profit,gross_loss`. Phase 2 will insert the component columns; Phase 3 will rename `period`→`group` and add `group_by` in JSON and CSV together. These are intended pre-1.0 evolutions, called out so they are not a surprise. `accounts` CSV is final.
- Mixed-currency guard in Phase 1 applies only to `pnl` time cuts (suppress combined `ALL` rows + summary, warn on stderr). The symbol/magic "refuse" and `positions` suppression are Phase 3.

**Definition of mixed-currency suppression (used throughout):** when accounts in scope span more than one currency, currency-valued figures that would sum across currencies are suppressed — combined (`ALL`) per-period rows and the summary. Counts (`trades`/`wins`/`losses`) and the count-based `win_rate_pct` stay, because they do not sum money. `profit_factor` is suppressed (it is a cross-currency ratio). Per-account rows always print (each is single-currency). Table shows `n/a`; JSON shows `null`; CSV omits the combined rows (CSV has no summary).

---

### Task 1: Top-level `--version` flag

**Files:**
- Modify: `main.go:25-43` (the `run` switch) and `main.go:45-60` (`usage`)
- Test: `cli_test.go` (add), `e2e_test.go:22-29` (extend)

- [ ] **Step 1: Write the failing test**

Add to `cli_test.go`:

```go
func TestVersionFlag(t *testing.T) {
	out, _, code := runCLI(t, "", "--version")
	if code != 0 || !strings.Contains(out, "mt5-pnl-cli") || !strings.Contains(out, "schema 1.0") {
		t.Errorf("exit %d, out %q", code, out)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test . -run TestVersionFlag -v`
Expected: FAIL — `--version` falls through to the `default` case ("unknown command"), exit 1.

- [ ] **Step 3: Add `--version` as an alias of the `version` subcommand**

In `main.go`, change the `version` case to also match the flag form:

```go
	case "version", "--version":
		fmt.Fprintf(stdout, "mt5-pnl-cli %s (schema %d.%d)\n", version, snapshot.SupportedMajor, snapshot.SupportedMinor)
		return 0
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test . -run TestVersionFlag -v`
Expected: PASS

- [ ] **Step 5: Extend the binary smoke test**

In `e2e_test.go`, after the existing `version` check (around line 29), add:

```go
	out, err = exec.Command(bin, "--version").Output()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), "mt5-pnl-cli dev (schema 1.0)") {
		t.Errorf("--version output: %q", out)
	}
```

- [ ] **Step 6: Run the smoke test**

Run: `go test . -run TestBinarySmoke -v`
Expected: PASS

- [ ] **Step 7: Commit**

```bash
git add main.go cli_test.go e2e_test.go
git commit -m "feat: add top-level --version flag as alias of version subcommand"
```

---

### Task 2: Per-command help to stdout (exit 0), parse errors to stderr (exit 1)

Today `cmd_pnl`/`cmd_accounts` do `if err := fs.Parse(args); err != nil { return 1 }`, so `-h`/`--help` (which returns `flag.ErrHelp`) exits 1 and dumps `flag`'s auto-usage to stderr. Fix: a shared `parseFlags` helper that routes `-h`/`--help` to a hand-written help string on **stdout** (exit 0) and genuine parse errors to **stderr** (exit 1).

**Files:**
- Modify: `cmd_common.go` (add `parseFlags`)
- Modify: `cmd_pnl.go:14-26` (use `parseFlags`, add `pnlHelp`)
- Modify: `cmd_accounts.go:13-20` (use `parseFlags`, add `accountsHelp`)
- Test: `cli_test.go` (add)

- [ ] **Step 1: Write the failing tests**

Add to `cli_test.go`:

```go
func TestPnLHelpToStdout(t *testing.T) {
	out, errOut, code := runCLI(t, "", "pnl", "-h")
	if code != 0 {
		t.Fatalf("exit %d", code)
	}
	if !strings.Contains(out, "Usage: mt5-pnl-cli pnl") {
		t.Errorf("help should be on stdout, got %q", out)
	}
	if errOut != "" {
		t.Errorf("help should not write stderr, got %q", errOut)
	}
}

func TestPnLParseErrorToStderr(t *testing.T) {
	out, errOut, code := runCLI(t, "", "pnl", "--nope")
	if code != 1 {
		t.Fatalf("exit %d", code)
	}
	if out != "" {
		t.Errorf("parse error should not write stdout, got %q", out)
	}
	if !strings.Contains(errOut, "not defined") {
		t.Errorf("want flag error on stderr, got %q", errOut)
	}
}

func TestAccountsHelpToStdout(t *testing.T) {
	out, _, code := runCLI(t, "", "accounts", "--help")
	if code != 0 || !strings.Contains(out, "Usage: mt5-pnl-cli accounts") {
		t.Errorf("exit %d, out %q", code, out)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test . -run 'TestPnLHelpToStdout|TestPnLParseErrorToStderr|TestAccountsHelpToStdout' -v`
Expected: FAIL — help currently exits 1 with empty stdout.

- [ ] **Step 3: Add the `parseFlags` helper**

In `cmd_common.go`, add these imports if missing (`errors`, `flag`, `fmt`, `io` — `errors`, `fmt`, `io` are already imported; add `flag`) and the function:

```go
// parseFlags parses fs, separating the two failure modes flag conflates:
// -h/--help prints the hand-written help to stdout and signals a clean exit
// (ok=false, code 0); any other parse error has already been written to fs's
// output (stderr) by the flag package, so we just signal exit 1. fs.Usage is
// suppressed so flag never dumps its own auto-generated usage.
func parseFlags(fs *flag.FlagSet, args []string, stdout io.Writer, help string) (ok bool, code int) {
	fs.Usage = func() {}
	err := fs.Parse(args)
	switch {
	case err == nil:
		return true, 0
	case errors.Is(err, flag.ErrHelp):
		fmt.Fprint(stdout, help)
		return false, 0
	default:
		return false, 1
	}
}
```

- [ ] **Step 4: Wire `pnl` to use it**

In `cmd_pnl.go`, add the help constant at the top of the file (after the imports):

```go
const pnlHelp = `Usage: mt5-pnl-cli pnl [flags]

Show P&L over a date range, grouped per period and account.

Flags:
  --last Nd|Nw|Nm|Ny       relative range ending today (default 30d)
  --from YYYY-MM-DD         start date (UTC)
  --to YYYY-MM-DD           end date (UTC); defaults to today
  --by day|week|month       grouping (default week; weeks start Monday)
  --accounts "A,B"          filter by account label (default: all)
  --format table|json|csv   output format (default table)
  --json                    alias for --format json
  --snapshot PATH           snapshot path (default: $MT5_PNL_SNAPSHOT)
  --stale-after DUR         staleness warning threshold (default 2h)
  -h, --help                show this help
`
```

Replace the parse block (`cmd_pnl.go:24-26`):

```go
	if err := fs.Parse(args); err != nil {
		return 1
	}
```

with:

```go
	if ok, code := parseFlags(fs, args, stdout, pnlHelp); !ok {
		return code
	}
```

- [ ] **Step 5: Wire `accounts` to use it**

In `cmd_accounts.go`, add the help constant after the imports:

```go
const accountsHelp = `Usage: mt5-pnl-cli accounts [flags]

List accounts with balance, equity and freshness.

Flags:
  --format table|json|csv   output format (default table)
  --json                    alias for --format json
  --snapshot PATH           snapshot path (default: $MT5_PNL_SNAPSHOT)
  --stale-after DUR         staleness warning threshold (default 2h)
  -h, --help                show this help
`
```

Replace the parse block (`cmd_accounts.go:18-20`):

```go
	if err := fs.Parse(args); err != nil {
		return 1
	}
```

with:

```go
	if ok, code := parseFlags(fs, args, stdout, accountsHelp); !ok {
		return code
	}
```

- [ ] **Step 6: Run the tests to verify they pass**

Run: `go test . -run 'TestPnLHelpToStdout|TestPnLParseErrorToStderr|TestAccountsHelpToStdout' -v`
Expected: PASS

- [ ] **Step 7: Run the full package to check no regressions**

Run: `go test .`
Expected: PASS (existing `TestPnLInvalidBy`, `TestUnknownCommand` etc. unaffected).

- [ ] **Step 8: Commit**

```bash
git add cmd_common.go cmd_pnl.go cmd_accounts.go cli_test.go
git commit -m "feat: route -h/--help to stdout (exit 0), parse errors to stderr (exit 1)"
```

---

### Task 3: Refresh top-level usage text

The top-level `usage` in `main.go` still advertises `[--json]`. Update it to `--format` and mention the `--version` flag, so `mt5-pnl-cli help` lists the current surface.

**Files:**
- Modify: `main.go:45-60`
- Test: `cli_test.go` (add)

- [ ] **Step 1: Write the failing test**

Add to `cli_test.go`:

```go
func TestTopLevelHelpMentionsFormat(t *testing.T) {
	out, _, code := runCLI(t, "", "help")
	if code != 0 {
		t.Fatalf("exit %d", code)
	}
	if !strings.Contains(out, "--format") {
		t.Errorf("top-level help should mention --format:\n%s", out)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test . -run TestTopLevelHelpMentionsFormat -v`
Expected: FAIL — usage text still says `[--json]`.

- [ ] **Step 3: Update the usage text**

Replace the body string in `usage` (`main.go:46-59`) with:

```go
	fmt.Fprint(w, `mt5-pnl-cli — query MT5 P&L from an mt5-pnl-exporter snapshot.

Usage:
  mt5-pnl-cli pnl [--last 30d | --from YYYY-MM-DD [--to YYYY-MM-DD]]
                  [--by day|week|month] [--accounts "A,B"]
                  [--format table|json|csv] [--snapshot PATH] [--stale-after 2h]
  mt5-pnl-cli accounts [--format table|json|csv] [--snapshot PATH] [--stale-after 2h]
  mt5-pnl-cli set-passphrase
  mt5-pnl-cli version   (or --version)

The snapshot path comes from --snapshot or the MT5_PNL_SNAPSHOT environment
variable. The decryption passphrase comes from the OS keychain; store it
once with set-passphrase.
`)
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test . -run TestTopLevelHelpMentionsFormat -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add main.go cli_test.go
git commit -m "docs: refresh top-level usage for --format and --version"
```

---

### Task 4: `resolveFormat` helper

A pure function that reconciles `--format` with the legacy `--json` alias. Unit-tested in isolation before wiring.

**Files:**
- Modify: `cmd_common.go` (add `resolveFormat`)
- Test: `format_test.go` (create)

- [ ] **Step 1: Write the failing test**

Create `format_test.go`:

```go
package main

import "testing"

func TestResolveFormat(t *testing.T) {
	cases := []struct {
		name                        string
		format                      string
		formatSet, jsonSet, jsonVal bool
		want                        string
		wantErr                     bool
	}{
		{"default table", "table", false, false, false, "table", false},
		{"json alias alone", "table", false, true, true, "json", false},
		{"json=false ignored", "table", false, true, false, "table", false},
		{"csv explicit", "csv", true, false, false, "csv", false},
		{"json and --format json agree", "json", true, true, true, "json", false},
		{"json conflicts with csv", "csv", true, true, true, "", true},
		{"invalid value", "bogus", true, false, false, "", true},
	}
	for _, c := range cases {
		got, err := resolveFormat(c.format, c.formatSet, c.jsonSet, c.jsonVal)
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

- [ ] **Step 2: Run test to verify it fails**

Run: `go test . -run TestResolveFormat -v`
Expected: FAIL — `resolveFormat` undefined.

- [ ] **Step 3: Implement `resolveFormat`**

In `cmd_common.go`, add:

```go
// resolveFormat reconciles --format with the legacy --json alias. format is
// the --format value; formatSet/jsonSet report whether each flag was given;
// jsonVal is the --json bool. --json is treated as --format json; if both are
// set and disagree it is an error.
func resolveFormat(format string, formatSet, jsonSet, jsonVal bool) (string, error) {
	f := format
	if jsonSet && jsonVal {
		if formatSet && f != "json" {
			return "", fmt.Errorf("--json conflicts with --format %s; use one or the other", f)
		}
		f = "json"
	}
	switch f {
	case "table", "json", "csv":
		return f, nil
	default:
		return "", fmt.Errorf("invalid --format %q: use table, json or csv", f)
	}
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test . -run TestResolveFormat -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add cmd_common.go format_test.go
git commit -m "feat: add resolveFormat reconciling --format and the --json alias"
```

---

### Task 5: CSV renderers (`PnLCSV`, `AccountsCSV`)

Additive render functions. `PnLCSV` takes `mixed bool` from the start (so no later signature churn) and omits combined rows when mixed.

**Files:**
- Modify: `internal/render/render.go` (add `encoding/csv` import, `money`, `PnLCSV`, `AccountsCSV`)
- Test: `internal/render/render_test.go` (add; add `"strings"` import)

- [ ] **Step 1: Write the failing tests**

In `internal/render/render_test.go`, ensure `"strings"` is imported, then add:

```go
func TestPnLCSV(t *testing.T) {
	var buf bytes.Buffer
	if err := render.PnLCSV(&buf, rows, labels, false); err != nil {
		t.Fatal(err)
	}
	want := "period,account_login,account_label,pnl,trades,wins,losses,gross_profit,gross_loss\n" +
		"2026-01-05,111,Trend EA,5.00,2,1,1,9.00,-4.00\n" +
		"2026-01-05,,ALL,5.00,2,1,1,9.00,-4.00\n"
	if buf.String() != want {
		t.Errorf("CSV mismatch:\ngot:\n%s\nwant:\n%s", buf.String(), want)
	}
}

func TestPnLCSVMixedOmitsCombined(t *testing.T) {
	var buf bytes.Buffer
	if err := render.PnLCSV(&buf, rows, labels, true); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(buf.String(), "ALL") {
		t.Errorf("mixed CSV should omit the combined ALL row:\n%s", buf.String())
	}
	if !strings.Contains(buf.String(), "2026-01-05,111,Trend EA") {
		t.Errorf("mixed CSV should keep per-account rows:\n%s", buf.String())
	}
}

func TestAccountsCSV(t *testing.T) {
	var buf bytes.Buffer
	if err := render.AccountsCSV(&buf, accounts); err != nil {
		t.Fatal(err)
	}
	want := "login,label,currency,balance,equity,last_success_at,last_error\n" +
		"111,Trend EA,USD,1000.00,1010.50,2026-06-13T00:00:00Z,\n" +
		"222,Scalper EA,USD,500.00,500.00,,login failed\n"
	if buf.String() != want {
		t.Errorf("CSV mismatch:\ngot:\n%s\nwant:\n%s", buf.String(), want)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/render -run 'CSV' -v`
Expected: FAIL — `render.PnLCSV` / `render.AccountsCSV` undefined.

- [ ] **Step 3: Implement the CSV renderers**

In `internal/render/render.go`, add `"encoding/csv"` to the import block, then add:

```go
// money formats a value at display precision (2 dp) for CSV cells.
func money(x float64) string {
	return strconv.FormatFloat(round(x, 2), 'f', 2, 64)
}

// PnLCSV writes per-period rows as CSV (header + rows, no summary). Under
// mixed currency the combined ALL rows are omitted, since they would sum
// across currencies; per-account rows (each single-currency) still print.
func PnLCSV(w io.Writer, rows []aggregate.Row, labels map[int64]string, mixed bool) error {
	cw := csv.NewWriter(w)
	if err := cw.Write([]string{
		"period", "account_login", "account_label",
		"pnl", "trades", "wins", "losses", "gross_profit", "gross_loss",
	}); err != nil {
		return err
	}
	for _, r := range rows {
		combined := r.Account == nil
		if mixed && combined {
			continue
		}
		login, label := "", "ALL"
		if !combined {
			login = strconv.FormatInt(*r.Account, 10)
			label = labels[*r.Account]
			if label == "" {
				label = login
			}
		}
		if err := cw.Write([]string{
			r.Period, login, label,
			money(r.PnL), strconv.Itoa(r.Trades), strconv.Itoa(r.Wins), strconv.Itoa(r.Losses),
			money(r.GrossProfit), money(r.GrossLoss),
		}); err != nil {
			return err
		}
	}
	cw.Flush()
	return cw.Error()
}

// AccountsCSV writes one row per account (header + rows). Nullable timestamps
// and errors render as empty cells.
func AccountsCSV(w io.Writer, accounts []snapshot.AccountSnapshot) error {
	cw := csv.NewWriter(w)
	if err := cw.Write([]string{
		"login", "label", "currency", "balance", "equity", "last_success_at", "last_error",
	}); err != nil {
		return err
	}
	for _, a := range accounts {
		if err := cw.Write([]string{
			strconv.FormatInt(a.Login, 10), a.Label, a.Currency,
			money(a.Balance), money(a.Equity),
			strOr(a.LastSuccessAt, ""), strOr(a.LastError, ""),
		}); err != nil {
			return err
		}
	}
	cw.Flush()
	return cw.Error()
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/render -run 'CSV' -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/render/render.go internal/render/render_test.go
git commit -m "feat: add CSV renderers for pnl and accounts"
```

---

### Task 6: Render nullability + `mixed` parameter for `PnLJSON` and `PnLTable`

Change the JSON DTO currency fields to `*float64` so they can render `null`, and add a `mixed bool` parameter that suppresses cross-currency sums in both renderers. Non-mixed output is byte-identical (a `*float64` pointing at a value marshals exactly like the value, and the table reformats the same numbers), so existing golden files and the exact-match JSON test stay green. This task also updates the existing callers and tests so the build stays green; the real `mixed` value is computed in Task 7.

**Files:**
- Modify: `internal/render/render.go:37-101` (`PnLTable`, `pnlRow`, `pnlSummary`, `PnLJSON`)
- Modify: `internal/render/render_test.go` (update existing calls; add mixed tests)
- Modify: `cmd_pnl.go:59-63` (pass `false` to the renderers — temporary until Task 7)

- [ ] **Step 1: Write the failing mixed-render tests**

Add to `internal/render/render_test.go`:

```go
func TestPnLJSONMixedNulls(t *testing.T) {
	var buf bytes.Buffer
	if err := render.PnLJSON(&buf, rows, sum, true); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	for _, want := range []string{`"total_pnl": null`, `"profit_factor": null`, `"gross_profit": null`, `"gross_loss": null`} {
		if !strings.Contains(out, want) {
			t.Errorf("mixed summary missing %q:\n%s", want, out)
		}
	}
	if !strings.Contains(out, `"pnl": 5`) {
		t.Errorf("per-account pnl should remain a number:\n%s", out)
	}
	if !strings.Contains(out, `"win_rate_pct": 50`) {
		t.Errorf("count-based win rate should remain:\n%s", out)
	}
	if !strings.Contains(out, `"total_trades": 2`) {
		t.Errorf("trade count should remain:\n%s", out)
	}
}

func TestPnLTableMixedNA(t *testing.T) {
	var buf bytes.Buffer
	if err := render.PnLTable(&buf, rows, sum, labels, true); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, "Total P&L: n/a") {
		t.Errorf("mixed table should show n/a total:\n%s", out)
	}
	if !strings.Contains(out, "Trades: 2") {
		t.Errorf("mixed table should keep trade count:\n%s", out)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail (compile error)**

Run: `go test ./internal/render -run 'Mixed' -v`
Expected: FAIL — `PnLJSON`/`PnLTable` take fewer arguments (compile error).

- [ ] **Step 3: Add a value→pointer helper**

In `internal/render/render.go`, near `roundPtr` (around line 28), add:

```go
func numPtr(x float64) *float64 { return &x }
```

- [ ] **Step 4: Update `PnLTable` to take `mixed` and suppress sums**

Replace `PnLTable` (`internal/render/render.go:37-59`) with:

```go
func PnLTable(w io.Writer, rows []aggregate.Row, sum aggregate.Summary, labels map[int64]string, mixed bool) error {
	tw := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
	fmt.Fprintln(tw, "PERIOD\tACCOUNT\tP&L\tTRADES\tWINS\tLOSSES")
	for _, r := range rows {
		acct := "ALL"
		combined := r.Account == nil
		if !combined {
			acct = labels[*r.Account]
			if acct == "" {
				acct = strconv.FormatInt(*r.Account, 10)
			}
		}
		pnl := fmt.Sprintf("%.2f", r.PnL)
		if mixed && combined {
			pnl = "n/a"
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%d\t%d\t%d\n", r.Period, acct, pnl, r.Trades, r.Wins, r.Losses)
	}
	if err := tw.Flush(); err != nil {
		return err
	}
	totalPnL := fmt.Sprintf("%.2f", sum.TotalPnL)
	grossProfit := fmt.Sprintf("%.2f", sum.GrossProfit)
	grossLoss := fmt.Sprintf("%.2f", sum.GrossLoss)
	profitFactor := fmtPtr(sum.ProfitFactor, "%.2f")
	if mixed {
		totalPnL, grossProfit, grossLoss, profitFactor = "n/a", "n/a", "n/a", "n/a"
	}
	_, err := fmt.Fprintf(w,
		"\nTotal P&L: %s  Trades: %d  Win rate: %s  Profit factor: %s  Gross profit: %s  Gross loss: %s\n",
		totalPnL, sum.TotalTrades,
		fmtPtr(sum.WinRatePct, "%.1f%%"), profitFactor,
		grossProfit, grossLoss)
	return err
}
```

- [ ] **Step 5: Make the JSON DTO nullable and update `PnLJSON`**

Replace the `pnlRow`/`pnlSummary` types and `PnLJSON` (`internal/render/render.go:61-101`) with:

```go
type pnlRow struct {
	Period      string   `json:"period"`
	Account     *int64   `json:"account"`
	PnL         *float64 `json:"pnl"`
	Trades      int      `json:"trades"`
	Wins        int      `json:"wins"`
	Losses      int      `json:"losses"`
	GrossProfit *float64 `json:"gross_profit"`
	GrossLoss   *float64 `json:"gross_loss"`
}

type pnlSummary struct {
	TotalPnL     *float64 `json:"total_pnl"`
	TotalTrades  int      `json:"total_trades"`
	WinRatePct   *float64 `json:"win_rate_pct"`
	ProfitFactor *float64 `json:"profit_factor"`
	GrossProfit  *float64 `json:"gross_profit"`
	GrossLoss    *float64 `json:"gross_loss"`
}

// PnLJSON emits the rows and summary as JSON. Under mixed currency the
// combined (account == nil) rows and the summary have their currency-valued
// fields set to null (they would sum across currencies); counts and the
// count-based win rate are kept.
func PnLJSON(w io.Writer, rows []aggregate.Row, sum aggregate.Summary, mixed bool) error {
	out := struct {
		Rows    []pnlRow   `json:"rows"`
		Summary pnlSummary `json:"summary"`
	}{Rows: make([]pnlRow, 0, len(rows))}
	for _, r := range rows {
		row := pnlRow{
			Period: r.Period, Account: r.Account,
			PnL: numPtr(round(r.PnL, 2)), Trades: r.Trades, Wins: r.Wins, Losses: r.Losses,
			GrossProfit: numPtr(round(r.GrossProfit, 2)), GrossLoss: numPtr(round(r.GrossLoss, 2)),
		}
		if mixed && r.Account == nil {
			row.PnL, row.GrossProfit, row.GrossLoss = nil, nil, nil
		}
		out.Rows = append(out.Rows, row)
	}
	out.Summary = pnlSummary{
		TotalPnL: numPtr(round(sum.TotalPnL, 2)), TotalTrades: sum.TotalTrades,
		WinRatePct: roundPtr(sum.WinRatePct, 1), ProfitFactor: roundPtr(sum.ProfitFactor, 2),
		GrossProfit: numPtr(round(sum.GrossProfit, 2)), GrossLoss: numPtr(round(sum.GrossLoss, 2)),
	}
	if mixed {
		out.Summary.TotalPnL, out.Summary.GrossProfit, out.Summary.GrossLoss, out.Summary.ProfitFactor = nil, nil, nil, nil
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}
```

- [ ] **Step 6: Update existing render tests to the new signatures**

In `internal/render/render_test.go`:
- `TestPnLTable`: change `render.PnLTable(&buf, rows, sum, labels)` → `render.PnLTable(&buf, rows, sum, labels, false)`.
- `TestPnLTableUnknownLabelFallsBackToLogin`: change `render.PnLTable(&buf, rows, sum, nil)` → `render.PnLTable(&buf, rows, sum, nil, false)`.
- `TestPnLTableNilSummaryFields`: change `render.PnLTable(&buf, nil, aggregate.Summary{}, nil)` → `render.PnLTable(&buf, nil, aggregate.Summary{}, nil, false)`.
- `TestPnLJSON`: change `render.PnLJSON(&buf, rows, sum)` → `render.PnLJSON(&buf, rows, sum, false)`. The expected `want` string is unchanged.

- [ ] **Step 7: Keep the command building by passing `false` (temporary)**

In `cmd_pnl.go`, update the dispatch (`cmd_pnl.go:59-63`) so it compiles against the new signatures:

```go
	if *asJSON {
		err = render.PnLJSON(stdout, rows, sum, false)
	} else {
		err = render.PnLTable(stdout, rows, sum, labels, false)
	}
```

(Task 7 replaces this dispatch entirely.)

- [ ] **Step 8: Run the render package and the main package**

Run: `go test ./internal/render -v`
Expected: PASS — including the unchanged `TestPnLJSON` exact-match and golden `TestPnLTable` (output byte-identical for `mixed=false`).

Run: `go test .`
Expected: PASS.

- [ ] **Step 9: Commit**

```bash
git add internal/render/render.go internal/render/render_test.go cmd_pnl.go
git commit -m "feat: nullable pnl render DTO and mixed-currency suppression in render"
```

---

### Task 7: Wire `--format` and the mixed-currency guard into the command layer

Replace the `--json`-only dispatch in `pnl` and `accounts` with `--format` resolution (keeping `--json` as an alias), and compute the mixed-currency state for `pnl`, warning on stderr and passing `mixed` to the renderers.

**Files:**
- Modify: `cmd_common.go` (add `currenciesInScope`; add `sort` and `aggregate` imports)
- Modify: `cmd_pnl.go` (add `--format` flag, format dispatch, mixed detection; add `strings` import)
- Modify: `cmd_accounts.go` (add `--format` flag, format dispatch)
- Test: `cli_test.go` (add format + mixed-currency integration tests, with a multi-currency fixture)

- [ ] **Step 1: Write the failing integration tests**

Add to `cli_test.go` (note the new `mixedFixtureJSON` constant):

```go
const mixedFixtureJSON = `{
  "schema_version": "1.0",
  "generated_at": "2026-06-13T00:00:00Z",
  "accounts": [
    {"login": 111, "label": "USD Acct", "currency": "USD", "balance": 1000.0,
     "equity": 1000.0, "last_success_at": "2026-06-13T00:00:00Z", "last_error": null},
    {"login": 333, "label": "EUR Acct", "currency": "EUR", "balance": 800.0,
     "equity": 800.0, "last_success_at": "2026-06-13T00:00:00Z", "last_error": null}
  ],
  "closed_deals": [
    {"account": 111, "time": 1767607200, "profit": 10.0, "swap": 0.0, "commission": 0.0, "fee": 0.0,
     "ticket": 1, "order": 1, "position_id": 1, "time_msc": 0, "type": 0, "entry": 1, "reason": 0,
     "magic": 0, "volume": 0.1, "price": 1.0, "symbol": "EURUSD", "comment": "", "external_id": ""},
    {"account": 333, "time": 1767607200, "profit": 7.0, "swap": 0.0, "commission": 0.0, "fee": 0.0,
     "ticket": 2, "order": 2, "position_id": 2, "time_msc": 0, "type": 0, "entry": 1, "reason": 0,
     "magic": 0, "volume": 0.1, "price": 1.0, "symbol": "EURUSD", "comment": "", "external_id": ""}
  ],
  "open_positions": [],
  "cash_flows": []
}`

func TestPnLFormatCSV(t *testing.T) {
	path := fixture(t)
	out, _, code := runCLI(t, "test-pass",
		"pnl", "--snapshot", path, "--from", "2026-01-01", "--to", "2026-01-31",
		"--by", "month", "--format", "csv", "--stale-after", "876000h")
	if code != 0 {
		t.Fatalf("exit %d", code)
	}
	if !strings.HasPrefix(out, "period,account_login,account_label,pnl,") {
		t.Errorf("want CSV header first, got:\n%s", out)
	}
}

func TestPnLFormatJSONMatchesJSONAlias(t *testing.T) {
	path := fixture(t)
	a, _, _ := runCLI(t, "test-pass", "pnl", "--snapshot", path,
		"--from", "2026-01-01", "--to", "2026-01-31", "--format", "json", "--stale-after", "876000h")
	b, _, _ := runCLI(t, "test-pass", "pnl", "--snapshot", path,
		"--from", "2026-01-01", "--to", "2026-01-31", "--json", "--stale-after", "876000h")
	if a != b || a == "" {
		t.Errorf("--format json and --json should match;\nformat:\n%s\njson:\n%s", a, b)
	}
}

func TestPnLInvalidFormat(t *testing.T) {
	_, errOut, code := runCLI(t, "test-pass", "pnl", "--format", "yaml")
	if code != 1 || !strings.Contains(errOut, "invalid --format") {
		t.Errorf("exit %d, stderr %q", code, errOut)
	}
}

func TestPnLFormatJSONConflict(t *testing.T) {
	_, errOut, code := runCLI(t, "test-pass", "pnl", "--json", "--format", "csv")
	if code != 1 || !strings.Contains(errOut, "conflicts") {
		t.Errorf("exit %d, stderr %q", code, errOut)
	}
}

func TestAccountsFormatCSV(t *testing.T) {
	path := fixture(t)
	out, _, code := runCLI(t, "test-pass", "accounts", "--snapshot", path,
		"--format", "csv", "--stale-after", "876000h")
	if code != 0 || !strings.HasPrefix(out, "login,label,currency,") {
		t.Errorf("exit %d, want accounts CSV header, got:\n%s", code, out)
	}
}

func TestPnLMixedCurrencyTableSuppresses(t *testing.T) {
	path := snaptest.Write(t, mixedFixtureJSON, "test-pass")
	out, errOut, code := runCLI(t, "test-pass",
		"pnl", "--snapshot", path, "--from", "2026-01-01", "--to", "2026-01-31",
		"--by", "month", "--stale-after", "876000h")
	if code != 0 {
		t.Fatalf("exit %d, stderr %s", code, errOut)
	}
	if !strings.Contains(errOut, "multiple currencies") {
		t.Errorf("want mixed-currency warning on stderr, got %q", errOut)
	}
	if !strings.Contains(out, "n/a") {
		t.Errorf("want suppressed combined total (n/a) in table, got:\n%s", out)
	}
}

func TestPnLMixedCurrencyJSONNull(t *testing.T) {
	path := snaptest.Write(t, mixedFixtureJSON, "test-pass")
	out, _, code := runCLI(t, "test-pass",
		"pnl", "--snapshot", path, "--from", "2026-01-01", "--to", "2026-01-31",
		"--by", "month", "--json", "--stale-after", "876000h")
	if code != 0 {
		t.Fatalf("exit %d", code)
	}
	if !strings.Contains(out, `"total_pnl": null`) {
		t.Errorf("want null total under mixed currency, got:\n%s", out)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test . -run 'Format|MixedCurrency' -v`
Expected: FAIL — `--format` is not a defined flag yet (parse error), and no mixed warning is emitted.

- [ ] **Step 3: Add `currenciesInScope` to `cmd_common.go`**

Add `"sort"` and the `aggregate` package to `cmd_common.go` imports:

```go
	"sort"

	"github.com/tanem/mt5-pnl-cli/internal/aggregate"
```

Then add:

```go
// currenciesInScope returns the distinct account currencies in scope, sorted.
// When an explicit --accounts filter is given, scope is those accounts;
// otherwise it is the accounts that actually contributed rows. More than one
// currency means combined totals would sum across currencies.
func currenciesInScope(accounts []snapshot.AccountSnapshot, filter map[int64]bool, rows []aggregate.Row) []string {
	curBy := make(map[int64]string, len(accounts))
	for _, a := range accounts {
		curBy[a.Login] = a.Currency
	}
	set := map[string]bool{}
	if filter != nil {
		for login := range filter {
			if c := curBy[login]; c != "" {
				set[c] = true
			}
		}
	} else {
		for _, r := range rows {
			if r.Account != nil {
				if c := curBy[*r.Account]; c != "" {
					set[c] = true
				}
			}
		}
	}
	out := make([]string, 0, len(set))
	for c := range set {
		out = append(out, c)
	}
	sort.Strings(out)
	return out
}
```

- [ ] **Step 4: Rewire `cmd_pnl.go`**

Add `"strings"` to the imports. Replace the `--json` flag declaration (`cmd_pnl.go:21`) with both flags:

```go
	asJSON := fs.Bool("json", false, "alias for --format json")
	formatFlag := fs.String("format", "table", "output format: table, json or csv")
```

After the `parseFlags` block (added in Task 2), resolve the format:

```go
	set := map[string]bool{}
	fs.Visit(func(f *flag.Flag) { set[f.Name] = true })
	format, err := resolveFormat(*formatFlag, set["format"], set["json"], *asJSON)
	if err != nil {
		fmt.Fprintln(stderr, "error:", err)
		return 1
	}
```

Then replace the render dispatch (the `if *asJSON { ... } else { ... }` block from Task 6, `cmd_pnl.go:59-63`) with mixed detection and a format switch:

```go
	curs := currenciesInScope(snap.Accounts, filter, rows)
	mixed := len(curs) > 1
	if mixed {
		fmt.Fprintf(stderr,
			"warning: accounts span multiple currencies (%s); combined totals are suppressed — narrow --accounts to one currency\n",
			strings.Join(curs, ", "))
	}

	switch format {
	case "json":
		err = render.PnLJSON(stdout, rows, sum, mixed)
	case "csv":
		err = render.PnLCSV(stdout, rows, labels, mixed)
	default:
		err = render.PnLTable(stdout, rows, sum, labels, mixed)
	}
```

(`err` is already declared earlier in the function; keep using `=`.)

- [ ] **Step 5: Rewire `cmd_accounts.go`**

Replace the `--json` flag declaration (`cmd_accounts.go:15`) with:

```go
	asJSON := fs.Bool("json", false, "alias for --format json")
	formatFlag := fs.String("format", "table", "output format: table, json or csv")
```

After the `parseFlags` block, resolve the format:

```go
	set := map[string]bool{}
	fs.Visit(func(f *flag.Flag) { set[f.Name] = true })
	format, err := resolveFormat(*formatFlag, set["format"], set["json"], *asJSON)
	if err != nil {
		fmt.Fprintln(stderr, "error:", err)
		return 1
	}
```

Replace the dispatch (`cmd_accounts.go:28-32`) with:

```go
	switch format {
	case "json":
		err = render.AccountsJSON(stdout, snap.Accounts)
	case "csv":
		err = render.AccountsCSV(stdout, snap.Accounts)
	default:
		err = render.AccountsTable(stdout, snap.Accounts, snap.GeneratedAt)
	}
```

Note: `cmd_accounts.go` currently assigns `snap, err := loadSnapshot(...)`. The new `format, err := resolveFormat(...)` must come before that `:=` so `err` is first declared by `resolveFormat`; the `loadSnapshot` line then becomes `snap, err = loadSnapshot(...)`. Order the format resolution immediately after `parseFlags`, before `loadSnapshot`.

- [ ] **Step 6: Run the targeted tests**

Run: `go test . -run 'Format|MixedCurrency' -v`
Expected: PASS

- [ ] **Step 7: Run the whole suite**

Run: `go test ./...`
Expected: PASS — existing `TestPnLJSONCommand` (uses `--json`) still passes via the alias.

- [ ] **Step 8: Commit**

```bash
git add cmd_common.go cmd_pnl.go cmd_accounts.go cli_test.go
git commit -m "feat: --format table|json|csv with --json alias and pnl mixed-currency guard"
```

---

### Task 8: Verify and document deal-time bucketing (spec 1.4)

Determine whether the exporter stores a deal's Unix `time` as MT5 server-time or true UTC, then record the answer in the README so monthly/weekly figures can be trusted. No production code changes; no `--tz` flag.

**Files:**
- Modify: `README.md` (Commands section, near the existing UTC note around lines 141-152)

- [ ] **Step 1: Determine how the exporter populates `time`**

Inspect `mt5-pnl-exporter` (the source the vendored `schema/snapshot.schema.json` came from). Look at where it reads each closed deal's time from MT5 (the MetaTrader5 Python `Deal.time` / `HistoryDealGetInteger(..., DEAL_TIME)` value) and whether it converts that to UTC before writing the snapshot. MT5's `DEAL_TIME` is documented as the trade-server's clock as a Unix timestamp; the expected finding is that the exporter writes it unchanged (so it is server-time-as-epoch, Variant A below). Confirm against the actual exporter code before writing — do not assume.

- [ ] **Step 2: Add the matching note to the README**

Add one of the following paragraphs to the `pnl` Commands subsection in `README.md`, immediately after the existing "interpreted in **UTC**" bullet. Use **Variant A** if the exporter writes the MT5 value unchanged (server-time), or **Variant B** if it converts to true UTC.

Variant A (server-time stored as epoch — buckets line up with broker statements):

```markdown
  - **Deal times and broker months.** Each deal's `time` is the value MT5
    records — on most brokers the server's local clock stored as a Unix
    timestamp. The CLI buckets by the UTC day/week/month of that value, so
    monthly and weekly figures line up with what your broker statement
    shows; there is no timezone skew to correct for.
```

Variant B (true UTC — month boundaries can differ from the broker):

```markdown
  - **Deal times and broker months.** Each deal's `time` is a true UTC
    timestamp, and the CLI buckets by UTC. A deal closed in the first or
    last hours of a month in the broker's server timezone can therefore
    fall in a different month here than on the broker statement; check
    trades near month boundaries when reconciling.
```

- [ ] **Step 3: Verify the README renders and the link/anchor list is intact**

Run: `go test ./...`
Expected: PASS (no code touched; this confirms nothing else broke).

- [ ] **Step 4: Commit**

```bash
git add README.md
git commit -m "docs: document deal-time bucketing vs broker months"
```

---

### Task 9: Update README and CLAUDE.md for the new surface

Bring the user-facing docs in line with `--format`, `--version`, and the mixed-currency behaviour, per CLAUDE.md's "update docs in the same change" rule.

**Files:**
- Modify: `README.md` (Why, Demo, Commands sections)
- Modify: `CLAUDE.md` (Commands + Gotchas)

- [ ] **Step 1: Update the README Commands section**

In `README.md`, in the `pnl` and `accounts` command descriptions, replace the `--json` bullet/mentions with `--format`:

- For `pnl`, replace the `- \`--json\` for machine output.` bullet with:

```markdown
  - `--format table|json|csv` (default `table`). `--json` is a documented
    alias for `--format json`. CSV is header + rows only (no summary
    block) — the spreadsheet/import path.
```

- For `accounts`, add the same `--format` line.
- Add a short bullet under `pnl`:

```markdown
  - **Mixed currencies.** If the accounts in scope span more than one
    currency, combined `ALL` rows and the summary are suppressed (`n/a` in
    tables, `null` in JSON, omitted from CSV) and a warning goes to
    stderr — the tool never silently sums across currencies. Narrow
    `--accounts` to one currency for combined totals.
```

- Update the `version` bullet to note the flag form:

```markdown
- `version` — binary version and supported snapshot schema (also
  available as `mt5-pnl-cli --version`).
```

- [ ] **Step 2: Update the README "Why" and Demo**

In the "Why" section, change the agent-friendly bullet's example from `--json` to `--format json` (or leave `--json` and add a parenthetical that `--format json` is equivalent). In the Demo section, add a one-line CSV example after the JSON block:

```markdown
`--format csv` emits header + rows for spreadsheets (no summary):

```
$ mt5-pnl-cli pnl --from 2026-01-01 --to 2026-01-31 --by month --format csv
period,account_login,account_label,pnl,trades,wins,losses,gross_profit,gross_loss
2026-01-01,111,Trend EA,10.00,3,2,1,14.00,-4.00
2026-01-01,,ALL,10.00,3,2,1,14.00,-4.00
```
```

- [ ] **Step 3: Update CLAUDE.md**

In `CLAUDE.md`, under Commands/Architecture, note that `pnl`/`accounts` take `--format table|json|csv` (with `--json` an alias). Add a Gotchas bullet:

```markdown
- **`--format` and the `--json` alias.** `pnl`/`accounts` take
  `--format table|json|csv`; `--json` is kept as a documented alias and
  errors if it disagrees with `--format`. CSV is rows-only (no summary).
- **Mixed-currency guard.** `pnl` never sums across currencies: when
  accounts in scope span more than one, combined `ALL` rows and the
  summary are suppressed (`n/a`/`null`/omitted) with a stderr warning.
```

- [ ] **Step 4: Verify the build and tests are green**

Run: `go test ./...`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add README.md CLAUDE.md
git commit -m "docs: README and CLAUDE.md for --format, --version and mixed-currency guard"
```

---

## Final verification

- [ ] Run the full suite with race detection (mirrors CI):

Run: `go test -race ./...`
Expected: PASS

- [ ] Confirm the golden files still match without regeneration (output is byte-identical for the non-mixed path):

Run: `go test ./internal/render`
Expected: PASS (no `-update` needed). If a golden genuinely changed, regenerate with `go test ./internal/render -update` and eyeball the diff before committing.

- [ ] Confirm the branch:

Run: `git branch --show-current`
Expected: `feat/cli-read-views-and-reporting`
