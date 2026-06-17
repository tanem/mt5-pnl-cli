# Phase 3 (part 1) — `pnl` grouping rework Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Generalise the `pnl` grouping dimension so `--by` accepts `symbol` and `magic` in addition to `day|week|month`, and emit one uniform JSON/CSV shape (`group`/`group_by` replacing `period`) across every cut.

**Architecture:** `internal/aggregate` renames `Row.Period` to `Row.Group` and forks `Aggregate`: time cuts keep the per-account-plus-combined behaviour; `symbol`/`magic` cuts produce one row per group key (no account, no combined row) aggregated across all in-scope accounts, sorted by key (numeric for `magic`). The whole-range summary is accumulated per-deal so it no longer depends on the combined rows. A new `aggregate.AccountsInScope` exposes the contributing account logins (the basis for the currency guard, which can no longer read it off the now-account-less symbol/magic rows). `internal/render` carries `group`/`group_by` on every row, takes the `groupBy` value as a parameter, drops the `ACCOUNT` column for dimension cuts, and labels the first column `SYMBOL`/`MAGIC`. The command layer accepts the two new `--by` values, passes `groupBy` through, and **refuses** (exit 1) a `symbol`/`magic` cut under mixed currency.

**Tech Stack:** Go 1.25, stdlib `cmp`/`slices`/`strconv`, `encoding/json`, `encoding/csv`. Tests are table-driven; `internal/render` uses golden files plus exact-string matches. The root-level CLI tests live in `package main` and drive `run(...)` through an encrypted `snaptest` fixture. Module path `github.com/tanem/mt5-pnl-cli`.

**Branch:** `feat/cli-phase-3-pnl-grouping`, off `main` (Phases 1–2 merged to `main` via PR #7). Phase 1–3 still ship as a single release — `main` accumulates the phases until the release is tagged.

**Spec:** `docs/superpowers/specs/2026-06-15-cli-read-views-and-reporting-design.md` — this plan implements **3.1** (`--by symbol|magic`) and **3.2** (uniform `group`/`group_by` shape), plus the related parts of 1.2 (CSV `group`/`group_by` columns) and 1.3 (mixed-currency *refusal* for symbol/magic).

**Scope notes / deliberate deferrals (do not implement here):**
- The `positions`, `trades` and `cash-flows` commands (spec 3.3–3.5) are **separate later plans**. This plan touches only the `pnl` command, `aggregate`, `render`, and the shared `currenciesInScope` helper.
- **This part deliberately changes the `pnl` JSON, CSV and table output again** (the `period` → `group`/`group_by` rename, plus the new symbol/magic table shape) — intended, not a regression. The Phase 2 exact-match tests, the table golden and the README Demo are updated here on purpose. The spec calls this break from `period` acceptable pre-1.0 (3.2). All Phase 1–3 work ships as a **single release**.
- `--sort` and human-readable enum mappings are **out of scope** (spec 3.1, Non-goals). Default sort only: symbol alphabetical, magic numeric.
- `--by` remains a **single dimension** — no cross-tabs (no "symbol × month").

**Codebase facts the tasks rely on (verified — do not re-derive):**
- `internal/render/render_test.go` is **`package render_test`** (external): every renderer call is qualified, e.g. `render.PnLTable(&buf, rows, sum, labels, false, render.TableOpts{})`, and uses the inline `if err := render.X(...); err != nil { t.Fatal(err) }` form. Its golden helper is `checkGolden(t, name string, got []byte)` (writes when `-update`, else compares). `ptr[T any]` and the `rows`/`sum`/`labels`/`accounts` fixtures are defined there. It imports `bytes`, `strings`, `os`, `path/filepath`, `aggregate`, `snapshot`, `render`.
- `internal/render/table.go` (`package render`) holds `writeTable`, `colSpec{header string; right bool}`, `cell{text string; tone tone}`, `pad`, `writeKV`, `kv`, `kvGroup`. The table uses a two-space gutter; right-aligned numeric columns are left-padded; the final left-aligned column is not padded.
- `snapshot.Deal` has `Account int64`, `Time int64`, `TimeMsc int64`, `Profit/Swap/Commission/Fee float64`, `Symbol string`, `Magic int64`.
- The root CLI tests (`cli_test.go`, `package main`) use `path := fixture(t)` (a single-currency, two-account USD snapshot with symbols `EURUSD`/`XAUUSD` and all `magic: 0`) and call `runCLI(t, "test-pass", "pnl", "--snapshot", path, ...)`, returning `(stdout, stderr string, code int)`. `--by`/`--color` validation runs **before** the snapshot is loaded, so the existing `TestPnLInvalidBy`/`TestPnLInvalidColor` pass without a snapshot. Tests that reach output pass a huge `--stale-after` (e.g. `"876000h"`) to silence the staleness warning.
- `currenciesInScope` (in `cmd_common.go`) is called from exactly one place (`cmd_pnl.go`) and has no direct unit test, so its signature may change freely. `civilDay` is unexported in `aggregate`.

**Field / signature reference (used across tasks — keep consistent):**

`aggregate.Row.Period` → `aggregate.Row.Group` (`string`: period date for time cuts, symbol for `--by symbol`, stringified magic for `--by magic`). `Account *int64` is the login for a per-account time row, `nil` for a combined time row **and** for every `symbol`/`magic` row.

New / changed signatures:

```go
// aggregate
func AccountsInScope(deals []snapshot.Deal, opts Options) []int64 // sorted contributing logins

// render (each pnl renderer gains a groupBy string carrying the --by value)
func PnLTable(w io.Writer, rows []aggregate.Row, sum aggregate.Summary, labels map[int64]string, groupBy string, mixed bool, opts TableOpts) error
func PnLJSON(w io.Writer, rows []aggregate.Row, sum aggregate.Summary, groupBy string, mixed bool) error
func PnLCSV(w io.Writer, rows []aggregate.Row, labels map[int64]string, groupBy string, mixed bool) error

// main (cmd_common.go) — third param is now the contributing logins, not rows
func currenciesInScope(accounts []snapshot.AccountSnapshot, filter map[int64]bool, contributing []int64) []string
```

JSON row shape becomes (`group`/`group_by` replace `period`; everything else unchanged from Phase 2):

```json
{ "group": "2026-01-05", "group_by": "week", "account": 111, "pnl": 5, "trade_profit": 6, "commission": -0.5, "swap": -0.5, "fee": 0, "trades": 2, "wins": 1, "losses": 1, "gross_profit": 9, "gross_loss": -4 }
```

CSV header: `group, group_by, account_login, account_label, pnl, trade_profit, commission, swap, fee, trades, wins, losses, gross_profit, gross_loss`. Combined **time** row: `account_login` empty, `account_label` = `ALL`. `symbol`/`magic` row: **both** account columns empty.

---

### Task 1: Rename `aggregate.Row.Period` → `Row.Group`

A pure, behaviour-preserving rename. The JSON tag stays `period` for now (it lives on `render.pnlRow`, renamed in Task 3), so output is unchanged and every test stays green.

**Files:**
- Modify: `internal/aggregate/aggregate.go`, `internal/aggregate/aggregate_test.go`
- Modify: `internal/render/render.go`, `internal/render/render_test.go`

- [ ] **Step 1: Rename the struct field and its doc comment**

In `aggregate.go`, in the `Row` struct, change `Period      string` to `Group       string`. Update the comment above `Row` from:

```go
// Row is one period × account bucket. Account == nil is the combined row
// across all accounts for that period.
```
to:
```go
// Row is one group × account bucket. Group holds the period date (time
// cuts) or the symbol/magic key (dimension cuts). Account == nil is the
// combined row across all accounts for a time period, and is also nil for
// every symbol/magic row (those have no per-account dimension).
```

- [ ] **Step 2: Update the two `Period:` field uses in `Aggregate`**

In `aggregate.go` change `b = &Row{Period: k.period, Account: &acct}` to `b = &Row{Group: k.period, Account: &acct}`, and `combined := Row{Period: p}` to `combined := Row{Group: p}`. (Leave the local `key.period` field and the `periodKey`/`periodSet`/`periods` identifiers — they are time-cut-specific and still accurate.)

- [ ] **Step 3: Rename `Period:` → `Group:` in the aggregate tests**

In `aggregate_test.go`, search the whole file for `Period:` and rename each to `Group:` in the `aggregate.Row` literals. The `TestAggregateByWeek` `want` slice becomes:

```go
	want := []aggregate.Row{
		{Group: "2026-01-05", Account: ptr(int64(111)), PnL: 5.0, TradeProfit: 6.0, Commission: -0.5, Swap: -0.5, Fee: 0, Trades: 2, Wins: 1, Losses: 1, GrossProfit: 9.0, GrossLoss: -4.0},
		{Group: "2026-01-05", Account: ptr(int64(222)), PnL: 0.0, TradeProfit: 0.5, Commission: -0.5, Swap: 0, Fee: 0, Trades: 1, Wins: 0, Losses: 0, GrossProfit: 0, GrossLoss: 0},
		{Group: "2026-01-05", Account: nil, PnL: 5.0, TradeProfit: 6.5, Commission: -1.0, Swap: -0.5, Fee: 0, Trades: 3, Wins: 1, Losses: 1, GrossProfit: 9.0, GrossLoss: -4.0},
		{Group: "2026-01-12", Account: ptr(int64(111)), PnL: 5.0, TradeProfit: 5.0, Commission: 0, Swap: 0, Fee: 0, Trades: 1, Wins: 1, Losses: 0, GrossProfit: 5.0, GrossLoss: 0},
		{Group: "2026-01-12", Account: nil, PnL: 5.0, TradeProfit: 5.0, Commission: 0, Swap: 0, Fee: 0, Trades: 1, Wins: 1, Losses: 0, GrossProfit: 5.0, GrossLoss: 0},
	}
```

Rename the remaining `Period:` literals in the other tests (`TestAggregateByDayWithDateFilter`, `TestAggregateByMonth`, `TestWeekBoundary`, `TestAccountFilter`) the same way.

- [ ] **Step 4: Update the `r.Period` reads in render**

In `render.go`, change each `r.Period` read to `r.Group`: the `PnLTable` cell `{r.Period, toneNone}` → `{r.Group, toneNone}`; the `PnLJSON` construction `Period: r.Period,` → `Period: r.Group,` (the `pnlRow.Period` field/tag is unchanged this task — only the right-hand side changes); the `PnLCSV` cell `r.Period,` → `r.Group,`.

- [ ] **Step 5: Rename `Period:` → `Group:` in the render tests**

In `render_test.go`, search the whole file for `Period:` and rename each to `Group:` in the `aggregate.Row` literals — the shared `rows` fixture (top of file) and the inline literals in `TestPnLTableColorBySign` and `TestPnLTableBreakevenNotColoured`. The expected JSON/CSV/golden **strings stay exactly as they are** — output is unchanged because the JSON tag and CSV header still read `period`.

- [ ] **Step 6: Build and run the full suite**

Run: `go build ./... && go test ./...`
Expected: PASS with no output changes. If anything fails to compile, search for a missed `Period` reference.

- [ ] **Step 7: Commit**

```bash
git add internal/aggregate/aggregate.go internal/aggregate/aggregate_test.go internal/render/render.go internal/render/render_test.go
git commit -m "refactor(aggregate): rename Row.Period to Row.Group ahead of symbol/magic cuts"
```

---

### Task 2: `symbol`/`magic` grouping + `AccountsInScope` in `aggregate`

Fork `Aggregate` for dimension cuts (one row per key, `Account` nil, no combined row, key-sorted — numeric for magic); accumulate the summary per-deal so it no longer depends on combined rows; and add `AccountsInScope` for the currency guard.

**Files:**
- Modify: `internal/aggregate/aggregate.go`
- Test: `internal/aggregate/aggregate_test.go`

- [ ] **Step 1: Write the failing tests**

Add to `aggregate_test.go`:

```go
// symDeal builds a deal with a symbol and magic set, time fixed inside the
// standard Jan-2026 test range.
func symDeal(account int64, symbol string, magic int64, profit float64) snapshot.Deal {
	return snapshot.Deal{Account: account, Time: 1767607200, Symbol: symbol, Magic: magic, Profit: profit}
}

func TestAggregateBySymbol(t *testing.T) {
	ds := []snapshot.Deal{
		symDeal(111, "XAUUSD", 0, 10.0),
		symDeal(111, "EURUSD", 0, 4.0),
		symDeal(222, "EURUSD", 0, -1.0), // same symbol, different account -> same row
	}
	rows, sum := aggregate.Aggregate(ds, aggregate.Options{
		From: date(2026, 1, 1), To: date(2026, 1, 31), By: "symbol",
	})
	want := []aggregate.Row{
		{Group: "EURUSD", Account: nil, PnL: 3.0, TradeProfit: 3.0, Trades: 2, Wins: 1, Losses: 1, GrossProfit: 4.0, GrossLoss: -1.0},
		{Group: "XAUUSD", Account: nil, PnL: 10.0, TradeProfit: 10.0, Trades: 1, Wins: 1, Losses: 0, GrossProfit: 10.0, GrossLoss: 0},
	}
	if !reflect.DeepEqual(rows, want) {
		t.Errorf("rows:\n got %+v\nwant %+v", rows, want)
	}
	if sum.TotalPnL != 13.0 || sum.TotalTrades != 3 || sum.GrossProfit != 14.0 || sum.GrossLoss != -1.0 {
		t.Errorf("summary totals wrong: %+v", sum)
	}
}

func TestAggregateByMagicSortsNumerically(t *testing.T) {
	// Numeric order is 20 before 100; a lexical sort of the stringified keys
	// would wrongly put "100" before "20".
	ds := []snapshot.Deal{
		symDeal(111, "EURUSD", 100, 5.0),
		symDeal(111, "EURUSD", 20, 2.0),
	}
	rows, _ := aggregate.Aggregate(ds, aggregate.Options{
		From: date(2026, 1, 1), To: date(2026, 1, 31), By: "magic",
	})
	if len(rows) != 2 {
		t.Fatalf("want 2 rows, got %d: %+v", len(rows), rows)
	}
	if rows[0].Group != "20" || rows[1].Group != "100" {
		t.Errorf("magic order = [%q %q], want [\"20\" \"100\"]", rows[0].Group, rows[1].Group)
	}
	if rows[0].Account != nil || rows[1].Account != nil {
		t.Errorf("magic rows must have nil Account, got %+v", rows)
	}
}

func TestAccountsInScope(t *testing.T) {
	ds := []snapshot.Deal{
		symDeal(111, "EURUSD", 0, 1.0),
		symDeal(222, "EURUSD", 0, 1.0),
		{Account: 333, Time: 1700000000, Symbol: "EURUSD", Profit: 1.0}, // out of range
	}
	got := aggregate.AccountsInScope(ds, aggregate.Options{From: date(2026, 1, 1), To: date(2026, 1, 31)})
	want := []int64{111, 222}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("AccountsInScope = %v, want %v", got, want)
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/aggregate -run 'TestAggregateBySymbol|TestAggregateByMagic|TestAccountsInScope' -v`
Expected: FAIL — `AccountsInScope` is undefined, and symbol/magic cuts currently bucket by `periodKey` (defaulting to month) and emit per-account + combined rows.

- [ ] **Step 3: Add the `strconv` import, `groupKey`, and `AccountsInScope`**

In `aggregate.go`, add `"strconv"` to the import block (sorted: `cmp`, `math`, `slices`, `strconv`, `time`).

Add these at the end of the file (next to `periodKey`):

```go
// groupKey returns the dimension-cut key for a deal: the symbol verbatim, or
// the magic number stringified. Only called for by == "symbol"/"magic".
func groupKey(d snapshot.Deal, by string) string {
	if by == "magic" {
		return strconv.FormatInt(d.Magic, 10)
	}
	return d.Symbol
}

// AccountsInScope returns, sorted, the logins with at least one deal passing
// the account and date filters in opts. The command uses it to decide the
// currency scope independently of how rows are grouped (symbol/magic rows
// carry no account, so the currency guard cannot read it off them).
func AccountsInScope(deals []snapshot.Deal, opts Options) []int64 {
	set := map[int64]bool{}
	for _, d := range deals {
		if opts.Accounts != nil && !opts.Accounts[d.Account] {
			continue
		}
		day := civilDay(d.Time)
		if day.Before(opts.From) || day.After(opts.To) {
			continue
		}
		set[d.Account] = true
	}
	out := make([]int64, 0, len(set))
	for a := range set {
		out = append(out, a)
	}
	slices.Sort(out)
	return out
}
```

- [ ] **Step 4: Add the dimension flag, summary accumulator, and forked bucket key**

In `Aggregate`, replace the opening declarations and the start of the per-deal loop. Change:

```go
func Aggregate(deals []snapshot.Deal, opts Options) ([]Row, Summary) {
	type key struct {
		period  string
		account int64
	}
	buckets := map[key]*Row{}
	accountSet := map[int64]bool{}
	var largestWin, largestLoss *float64

	type timed struct {
		time, timeMsc int64
		net           float64
	}
	var inScope []timed

	for _, d := range deals {
		if opts.Accounts != nil && !opts.Accounts[d.Account] {
			continue
		}
		day := civilDay(d.Time)
		if day.Before(opts.From) || day.After(opts.To) {
			continue
		}
		k := key{periodKey(day, opts.By), d.Account}
		b := buckets[k]
		if b == nil {
			acct := d.Account
			b = &Row{Group: k.period, Account: &acct}
			buckets[k] = b
		}
		net := d.Profit + d.Swap + d.Commission + d.Fee
		b.PnL += net
		b.TradeProfit += d.Profit
		b.Commission += d.Commission
		b.Swap += d.Swap
		b.Fee += d.Fee
		b.Trades++
		switch {
		case net > 0:
			b.Wins++
			b.GrossProfit += net
			if largestWin == nil || net > *largestWin {
				v := net
				largestWin = &v
			}
		case net < 0:
			b.Losses++
			b.GrossLoss += net
			if largestLoss == nil || net < *largestLoss {
				v := net
				largestLoss = &v
			}
		}
		accountSet[d.Account] = true
		inScope = append(inScope, timed{d.Time, d.TimeMsc, net})
	}
```

to:

```go
func Aggregate(deals []snapshot.Deal, opts Options) ([]Row, Summary) {
	dimension := opts.By == "symbol" || opts.By == "magic"

	type key struct {
		group   string
		account int64
	}
	buckets := map[key]*Row{}
	accountSet := map[int64]bool{}
	var largestWin, largestLoss *float64

	type timed struct {
		time, timeMsc int64
		net           float64
	}
	var inScope []timed

	var sum Summary
	totalWins, totalLosses := 0, 0

	for _, d := range deals {
		if opts.Accounts != nil && !opts.Accounts[d.Account] {
			continue
		}
		day := civilDay(d.Time)
		if day.Before(opts.From) || day.After(opts.To) {
			continue
		}
		var k key
		if dimension {
			k = key{groupKey(d, opts.By), 0}
		} else {
			k = key{periodKey(day, opts.By), d.Account}
		}
		b := buckets[k]
		if b == nil {
			b = &Row{Group: k.group}
			if !dimension {
				acct := d.Account
				b.Account = &acct
			}
			buckets[k] = b
		}
		net := d.Profit + d.Swap + d.Commission + d.Fee
		b.PnL += net
		b.TradeProfit += d.Profit
		b.Commission += d.Commission
		b.Swap += d.Swap
		b.Fee += d.Fee
		b.Trades++
		sum.TotalPnL += net
		sum.TradeProfit += d.Profit
		sum.Commission += d.Commission
		sum.Swap += d.Swap
		sum.Fee += d.Fee
		sum.TotalTrades++
		switch {
		case net > 0:
			b.Wins++
			b.GrossProfit += net
			sum.GrossProfit += net
			totalWins++
			if largestWin == nil || net > *largestWin {
				v := net
				largestWin = &v
			}
		case net < 0:
			b.Losses++
			b.GrossLoss += net
			sum.GrossLoss += net
			totalLosses++
			if largestLoss == nil || net < *largestLoss {
				v := net
				largestLoss = &v
			}
		}
		accountSet[d.Account] = true
		inScope = append(inScope, timed{d.Time, d.TimeMsc, net})
	}
```

- [ ] **Step 5: Fork the row-building and drop summary accumulation from the period loop**

Replace the whole block from `periodSet := map[string]bool{}` through the closing `}` of the `for _, p := range periods {` loop (i.e. the existing row builder plus its `var sum Summary` / `totalWins, totalLosses := 0, 0` and per-period summary accumulation). Replace this existing block:

```go
	periodSet := map[string]bool{}
	for k := range buckets {
		periodSet[k.period] = true
	}
	periods := make([]string, 0, len(periodSet))
	for p := range periodSet {
		periods = append(periods, p)
	}
	slices.Sort(periods)
	accounts := make([]int64, 0, len(accountSet))
	for a := range accountSet {
		accounts = append(accounts, a)
	}
	slices.Sort(accounts)

	var rows []Row
	var sum Summary
	totalWins, totalLosses := 0, 0
	for _, p := range periods {
		combined := Row{Group: p}
		for _, a := range accounts {
			b, ok := buckets[key{p, a}]
			if !ok {
				continue
			}
			rows = append(rows, *b)
			combined.PnL += b.PnL
			combined.TradeProfit += b.TradeProfit
			combined.Commission += b.Commission
			combined.Swap += b.Swap
			combined.Fee += b.Fee
			combined.Trades += b.Trades
			combined.Wins += b.Wins
			combined.Losses += b.Losses
			combined.GrossProfit += b.GrossProfit
			combined.GrossLoss += b.GrossLoss
		}
		rows = append(rows, combined)
		sum.TotalPnL += combined.PnL
		sum.TradeProfit += combined.TradeProfit
		sum.Commission += combined.Commission
		sum.Swap += combined.Swap
		sum.Fee += combined.Fee
		sum.TotalTrades += combined.Trades
		totalWins += combined.Wins
		totalLosses += combined.Losses
		sum.GrossProfit += combined.GrossProfit
		sum.GrossLoss += combined.GrossLoss
	}
```

with:

```go
	var rows []Row
	if dimension {
		groups := make([]string, 0, len(buckets))
		for k := range buckets {
			groups = append(groups, k.group)
		}
		slices.SortFunc(groups, func(a, b string) int {
			if opts.By == "magic" {
				ai, _ := strconv.ParseInt(a, 10, 64)
				bi, _ := strconv.ParseInt(b, 10, 64)
				return cmp.Compare(ai, bi)
			}
			return cmp.Compare(a, b)
		})
		for _, g := range groups {
			rows = append(rows, *buckets[key{g, 0}])
		}
	} else {
		periodSet := map[string]bool{}
		for k := range buckets {
			periodSet[k.group] = true
		}
		periods := make([]string, 0, len(periodSet))
		for p := range periodSet {
			periods = append(periods, p)
		}
		slices.Sort(periods)
		accounts := make([]int64, 0, len(accountSet))
		for a := range accountSet {
			accounts = append(accounts, a)
		}
		slices.Sort(accounts)
		for _, p := range periods {
			combined := Row{Group: p}
			for _, a := range accounts {
				b, ok := buckets[key{p, a}]
				if !ok {
					continue
				}
				rows = append(rows, *b)
				combined.PnL += b.PnL
				combined.TradeProfit += b.TradeProfit
				combined.Commission += b.Commission
				combined.Swap += b.Swap
				combined.Fee += b.Fee
				combined.Trades += b.Trades
				combined.Wins += b.Wins
				combined.Losses += b.Losses
				combined.GrossProfit += b.GrossProfit
				combined.GrossLoss += b.GrossLoss
			}
			rows = append(rows, combined)
		}
	}
```

(The post-fold block computing `WinRatePct`/`Expectancy`/`ProfitFactor`/`AvgWin`/`AvgLoss`/`LargestWin`/`LargestLoss`/`MaxDrawdown` is unchanged — it reads the now-per-deal-accumulated `sum`, `totalWins`, `totalLosses`, `largestWin`, `largestLoss`, `inScope`.)

- [ ] **Step 6: Run the new tests, then the whole package**

Run: `go test ./internal/aggregate -run 'TestAggregateBySymbol|TestAggregateByMagic|TestAccountsInScope' -v`
Expected: PASS

Run: `go test ./internal/aggregate`
Expected: PASS — the time-cut tests are unaffected: summary totals are identical (a combined row equals the sum of its per-account buckets, so summing combined rows over periods and summing every in-scope deal give the same totals), and the time-cut row output is byte-identical.

- [ ] **Step 7: Commit**

```bash
git add internal/aggregate/aggregate.go internal/aggregate/aggregate_test.go
git commit -m "feat(aggregate): group pnl by symbol or magic across accounts"
```

---

### Task 3: Uniform `group`/`group_by` JSON shape

Replace `pnlRow.Period` with `group` + `group_by`; thread the `groupBy` value into `PnLJSON`.

**Files:**
- Modify: `internal/render/render.go` (`pnlRow`, `PnLJSON`)
- Modify: `cmd_pnl.go` (the `PnLJSON` call)
- Test: `internal/render/render_test.go`

- [ ] **Step 1: Update `TestPnLJSON`'s call and expected output**

In `render_test.go`, in `TestPnLJSON`, change the call line `if err := render.PnLJSON(&buf, rows, sum, false); err != nil {` to:

```go
	if err := render.PnLJSON(&buf, rows, sum, "week", false); err != nil {
```

and replace the `want` string literal with (each row now leads with `group` + `group_by`):

```go
	want := `{
  "rows": [
    {
      "group": "2026-01-05",
      "group_by": "week",
      "account": 111,
      "pnl": 5,
      "trade_profit": 6,
      "commission": -0.5,
      "swap": -0.5,
      "fee": 0,
      "trades": 2,
      "wins": 1,
      "losses": 1,
      "gross_profit": 9,
      "gross_loss": -4
    },
    {
      "group": "2026-01-05",
      "group_by": "week",
      "account": null,
      "pnl": 5,
      "trade_profit": 6,
      "commission": -0.5,
      "swap": -0.5,
      "fee": 0,
      "trades": 2,
      "wins": 1,
      "losses": 1,
      "gross_profit": 9,
      "gross_loss": -4
    }
  ],
  "summary": {
    "total_pnl": 5,
    "total_trades": 2,
    "win_rate_pct": 50,
    "profit_factor": 2.25,
    "expectancy": 2.5,
    "avg_win": 9,
    "avg_loss": -4,
    "largest_win": 9,
    "largest_loss": -4,
    "max_drawdown": -4,
    "gross_profit": 9,
    "gross_loss": -4,
    "trade_profit": 6,
    "commission": -0.5,
    "swap": -0.5,
    "fee": 0
  }
}
`
```

- [ ] **Step 2: Update `TestPnLJSONMixedNulls`'s call**

In `TestPnLJSONMixedNulls`, change `if err := render.PnLJSON(&buf, rows, sum, true); err != nil {` to:

```go
	if err := render.PnLJSON(&buf, rows, sum, "week", true); err != nil {
```

(The existing `Contains` assertions for the `null` fields and the surviving `pnl`/`win_rate_pct`/`total_trades` stay.)

- [ ] **Step 3: Run the JSON tests to verify they fail (compile error)**

Run: `go test ./internal/render -run 'TestPnLJSON' -v`
Expected: FAIL — `PnLJSON` does not yet take a `groupBy` argument, and `pnlRow` has no `group`/`group_by`.

- [ ] **Step 4: Replace `pnlRow.Period` with `Group` + `GroupBy`**

In `render.go`, change the head of `pnlRow` from:

```go
type pnlRow struct {
	Period      string   `json:"period"`
	Account     *int64   `json:"account"`
```
to:
```go
type pnlRow struct {
	Group       string   `json:"group"`
	GroupBy     string   `json:"group_by"`
	Account     *int64   `json:"account"`
```

- [ ] **Step 5: Thread `groupBy` through `PnLJSON`**

Change the signature `func PnLJSON(w io.Writer, rows []aggregate.Row, sum aggregate.Summary, mixed bool) error {` to:

```go
func PnLJSON(w io.Writer, rows []aggregate.Row, sum aggregate.Summary, groupBy string, mixed bool) error {
```

and in the per-row construction change `Period: r.Group, Account: r.Account,` to:

```go
			Group: r.Group, GroupBy: groupBy, Account: r.Account,
```

(The `if mixed && r.Account == nil { ... }` suppression and the summary construction are unchanged.)

- [ ] **Step 6: Update the `cmd_pnl.go` call site**

In `cmd_pnl.go`, change `err = render.PnLJSON(stdout, rows, sum, mixed)` to:

```go
		err = render.PnLJSON(stdout, rows, sum, *by, mixed)
```

- [ ] **Step 7: Run the JSON tests, then build**

Run: `go test ./internal/render -run 'TestPnLJSON' -v`
Expected: PASS

Run: `go build ./... && go test ./internal/render`
Expected: PASS (CSV/table tests unchanged; `cmd_pnl.go` compiles with the new arity).

- [ ] **Step 8: Commit**

```bash
git add internal/render/render.go internal/render/render_test.go cmd_pnl.go
git commit -m "feat(render): emit uniform group/group_by JSON shape for pnl"
```

---

### Task 4: `group`/`group_by` CSV columns + dimension rows

Replace the CSV `period` column with `group, group_by`; thread `groupBy`; leave both account columns empty for symbol/magic rows.

**Files:**
- Modify: `internal/render/render.go` (`PnLCSV`)
- Modify: `cmd_pnl.go` (the `PnLCSV` call)
- Test: `internal/render/render_test.go`

- [ ] **Step 1: Update `TestPnLCSV` and add a symbol case**

In `render_test.go`, in `TestPnLCSV`, change `if err := render.PnLCSV(&buf, rows, labels, false); err != nil {` to:

```go
	if err := render.PnLCSV(&buf, rows, labels, "week", false); err != nil {
```

and replace its `want` with (header gains `group,group_by`; each data row carries the `week` group_by as the second field):

```go
	want := "group,group_by,account_login,account_label,pnl,trade_profit,commission,swap,fee,trades,wins,losses,gross_profit,gross_loss\n" +
		"2026-01-05,week,111,Trend EA,5.00,6.00,-0.50,-0.50,0.00,2,1,1,9.00,-4.00\n" +
		"2026-01-05,week,,ALL,5.00,6.00,-0.50,-0.50,0.00,2,1,1,9.00,-4.00\n"
```

Then add a symbol-cut test (both account columns empty, `group_by` = `symbol`):

```go
func TestPnLCSVSymbolEmptyAccountColumns(t *testing.T) {
	var buf bytes.Buffer
	symRows := []aggregate.Row{
		{Group: "EURUSD", Account: nil, PnL: 3.0, TradeProfit: 3.0, Trades: 2, Wins: 1, Losses: 1, GrossProfit: 4.0, GrossLoss: -1.0},
		{Group: "XAUUSD", Account: nil, PnL: 10.0, TradeProfit: 10.0, Trades: 1, Wins: 1, Losses: 0, GrossProfit: 10.0, GrossLoss: 0},
	}
	if err := render.PnLCSV(&buf, symRows, map[int64]string{}, "symbol", false); err != nil {
		t.Fatal(err)
	}
	want := "group,group_by,account_login,account_label,pnl,trade_profit,commission,swap,fee,trades,wins,losses,gross_profit,gross_loss\n" +
		"EURUSD,symbol,,,3.00,3.00,0.00,0.00,0.00,2,1,1,4.00,-1.00\n" +
		"XAUUSD,symbol,,,10.00,10.00,0.00,0.00,0.00,1,1,0,10.00,0.00\n"
	if buf.String() != want {
		t.Errorf("symbol CSV:\ngot:\n%q\nwant:\n%q", buf.String(), want)
	}
}
```

- [ ] **Step 2: Update `TestPnLCSVMixedOmitsCombined`**

In `TestPnLCSVMixedOmitsCombined`, change `if err := render.PnLCSV(&buf, rows, labels, true); err != nil {` to:

```go
	if err := render.PnLCSV(&buf, rows, labels, "week", true); err != nil {
```

and update the per-account presence assertion string from `"2026-01-05,111,Trend EA"` to `"2026-01-05,week,111,Trend EA"`. The `strings.Contains(buf.String(), "ALL")` absence assertion stays as-is (the omitted combined row is the only `ALL`).

- [ ] **Step 3: Run the CSV tests to verify they fail**

Run: `go test ./internal/render -run TestPnLCSV -v`
Expected: FAIL — `PnLCSV` does not yet take `groupBy`; header/rows lack `group_by`.

- [ ] **Step 4: Add `groupBy` and the dimension branch to `PnLCSV`**

In `render.go`, change the signature and add the flag:

```go
func PnLCSV(w io.Writer, rows []aggregate.Row, labels map[int64]string, groupBy string, mixed bool) error {
	dimension := groupBy == "symbol" || groupBy == "magic"
```

Change the header `Write` first slice from:

```go
		"period", "account_login", "account_label",
```
to:
```go
		"group", "group_by", "account_login", "account_label",
```

Replace the per-row loop body:

```go
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
			r.Group, login, label,
			money(r.PnL), money(r.TradeProfit), money(r.Commission), money(r.Swap), money(r.Fee),
			strconv.Itoa(r.Trades), strconv.Itoa(r.Wins), strconv.Itoa(r.Losses),
			money(r.GrossProfit), money(r.GrossLoss),
		}); err != nil {
			return err
		}
	}
```

with:

```go
	for _, r := range rows {
		combined := r.Account == nil
		if mixed && combined && !dimension {
			continue
		}
		login, label := "", ""
		switch {
		case dimension:
			// symbol/magic rows have no account dimension: both columns empty.
		case combined:
			label = "ALL"
		default:
			login = strconv.FormatInt(*r.Account, 10)
			label = labels[*r.Account]
			if label == "" {
				label = login
			}
		}
		if err := cw.Write([]string{
			r.Group, groupBy, login, label,
			money(r.PnL), money(r.TradeProfit), money(r.Commission), money(r.Swap), money(r.Fee),
			strconv.Itoa(r.Trades), strconv.Itoa(r.Wins), strconv.Itoa(r.Losses),
			money(r.GrossProfit), money(r.GrossLoss),
		}); err != nil {
			return err
		}
	}
```

- [ ] **Step 5: Update the `cmd_pnl.go` call site**

Change `err = render.PnLCSV(stdout, rows, labels, mixed)` to:

```go
		err = render.PnLCSV(stdout, rows, labels, *by, mixed)
```

- [ ] **Step 6: Run the CSV tests, then build**

Run: `go test ./internal/render -run TestPnLCSV -v`
Expected: PASS (including `TestPnLCSVSymbolEmptyAccountColumns`).

Run: `go build ./... && go test ./internal/render`
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add internal/render/render.go internal/render/render_test.go cmd_pnl.go
git commit -m "feat(render): add group/group_by CSV columns and symbol/magic rows"
```

---

### Task 5: Table dimension cuts (`SYMBOL`/`MAGIC` header, no `ACCOUNT` column)

For symbol/magic cuts the table drops the `ACCOUNT` column and labels the first column `SYMBOL`/`MAGIC`. Time cuts are unchanged. The summary block is unchanged.

**Files:**
- Modify: `internal/render/render.go` (`PnLTable`, add `groupHeader`)
- Modify: `cmd_pnl.go` (the `PnLTable` call)
- Test: `internal/render/render_test.go`
- Golden: regenerate `internal/render/testdata/pnl_table.golden` (call gains `groupBy`, content unchanged); new `internal/render/testdata/pnl_table_symbol.golden`

- [ ] **Step 1: Insert `"week"` into every existing `PnLTable` call**

In `render_test.go`, every `render.PnLTable(...)` call gains a `groupBy` argument **after `labels` and before `mixed`**. Update each (search for `render.PnLTable(`):

- `TestPnLTable`: `render.PnLTable(&buf, rows, sum, labels, false, render.TableOpts{})` → `render.PnLTable(&buf, rows, sum, labels, "week", false, render.TableOpts{})`
- `TestPnLTableUnknownLabelFallsBackToLogin`: `render.PnLTable(&buf, rows, sum, nil, false, render.TableOpts{})` → insert `"week"` before `false`.
- `TestPnLTableNilSummaryFields`: `render.PnLTable(&buf, nil, aggregate.Summary{}, nil, false, render.TableOpts{})` → insert `"week"` before `false`.
- `TestPnLTableMixedNA`: `render.PnLTable(&buf, rows, sum, labels, true, render.TableOpts{})` → insert `"week"` before `true`.
- `TestPnLTableCurrencyFooter`: `render.PnLTable(&buf, rows, sum, labels, false, render.TableOpts{Currency: "USD"})` → insert `"week"` before `false`.
- `TestPnLTableNoCurrencyWhenUnset`: insert `"week"` before `false`.
- `TestPnLTableColorBySign`: both calls (`render.PnLTable(&on, signed, sum, labels, false, render.TableOpts{Color: true})` and `render.PnLTable(&off, signed, sum, labels, false, render.TableOpts{})`) → insert `"week"` before `false` in each.
- `TestPnLTableBreakevenNotColoured`: `render.PnLTable(&buf, be, aggregate.Summary{}, labels, false, render.TableOpts{Color: true})` → insert `"week"` before `false`.

- [ ] **Step 2: Add the failing symbol-table tests**

Add to `render_test.go`:

```go
func TestPnLTableBySymbolDropsAccountColumn(t *testing.T) {
	var buf bytes.Buffer
	symRows := []aggregate.Row{
		{Group: "EURUSD", Account: nil, PnL: 5.0, Trades: 2, Wins: 1, Losses: 1},
		{Group: "XAUUSD", Account: nil, PnL: 10.0, Trades: 1, Wins: 1, Losses: 0},
	}
	if err := render.PnLTable(&buf, symRows, aggregate.Summary{}, map[int64]string{}, "symbol", false, render.TableOpts{}); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	for _, want := range []string{"SYMBOL", "EURUSD", "XAUUSD", "Summary", "Net P&L"} {
		if !strings.Contains(out, want) {
			t.Errorf("symbol table missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "ACCOUNT") {
		t.Errorf("symbol table should not have an ACCOUNT column:\n%s", out)
	}
}

func TestPnLTableBySymbolGolden(t *testing.T) {
	var buf bytes.Buffer
	symRows := []aggregate.Row{
		{Group: "EURUSD", Account: nil, PnL: 5.0, TradeProfit: 5.0, Trades: 2, Wins: 1, Losses: 1, GrossProfit: 9.0, GrossLoss: -4.0},
		{Group: "XAUUSD", Account: nil, PnL: 10.0, TradeProfit: 10.0, Trades: 1, Wins: 1, Losses: 0, GrossProfit: 10.0, GrossLoss: 0},
	}
	symSum := aggregate.Summary{
		TotalPnL: 15.0, TotalTrades: 3,
		WinRatePct: ptr(66.7), ProfitFactor: ptr(4.75),
		Expectancy: ptr(5.0), AvgWin: ptr(9.5), AvgLoss: ptr(-4.0),
		LargestWin: ptr(10.0), LargestLoss: ptr(-4.0), MaxDrawdown: ptr(0.0),
		GrossProfit: 19.0, GrossLoss: -4.0, TradeProfit: 15.0,
	}
	if err := render.PnLTable(&buf, symRows, symSum, map[int64]string{}, "symbol", false, render.TableOpts{}); err != nil {
		t.Fatal(err)
	}
	checkGolden(t, "pnl_table_symbol.golden", buf.Bytes())
}
```

- [ ] **Step 3: Run to verify they fail**

Run: `go test ./internal/render -run 'TestPnLTableBySymbol' -v`
Expected: FAIL — `PnLTable` does not yet take `groupBy`; and the symbol golden does not exist.

- [ ] **Step 4: Add the `groupHeader` helper**

In `render.go`, add near `signTone`:

```go
// groupHeader is the first table column header for a pnl cut.
func groupHeader(groupBy string) string {
	switch groupBy {
	case "symbol":
		return "SYMBOL"
	case "magic":
		return "MAGIC"
	default:
		return "PERIOD"
	}
}
```

- [ ] **Step 5: Thread `groupBy` and fork the columns/rows in `PnLTable`**

Change the signature and the column/row construction. Replace from the `func PnLTable(...)` line through the `if err := writeTable(w, cols, body, opts.Color); err != nil {` guard. Replace:

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
		// Tone from the displayed (rounded) value so a cell that reads 0.00
		// is treated as breakeven (no colour), matching the win/loss rule.
		pnlTone := signTone(round(r.PnL, 2))
		if mixed && combined {
			pnlText, pnlTone = "n/a", toneNone
		}
		body = append(body, []cell{
			{r.Group, toneNone}, {acct, toneNone}, {pnlText, pnlTone},
			{strconv.Itoa(r.Trades), toneNone},
			{strconv.Itoa(r.Wins), toneNone},
			{strconv.Itoa(r.Losses), toneNone},
		})
	}
	if err := writeTable(w, cols, body, opts.Color); err != nil {
		return err
	}
```

with:

```go
func PnLTable(w io.Writer, rows []aggregate.Row, sum aggregate.Summary, labels map[int64]string, groupBy string, mixed bool, opts TableOpts) error {
	dimension := groupBy == "symbol" || groupBy == "magic"
	var cols []colSpec
	if dimension {
		cols = []colSpec{
			{groupHeader(groupBy), false}, {"P&L", true},
			{"TRADES", true}, {"WINS", true}, {"LOSSES", true},
		}
	} else {
		cols = []colSpec{
			{"PERIOD", false}, {"ACCOUNT", false}, {"P&L", true},
			{"TRADES", true}, {"WINS", true}, {"LOSSES", true},
		}
	}
	body := make([][]cell, 0, len(rows))
	for _, r := range rows {
		pnlText := fmt.Sprintf("%.2f", r.PnL)
		// Tone from the displayed (rounded) value so a cell that reads 0.00
		// is treated as breakeven (no colour), matching the win/loss rule.
		pnlTone := signTone(round(r.PnL, 2))
		if dimension {
			// symbol/magic: no account column, and no combined/mixed case
			// (the command refuses a dimension cut under mixed currency).
			body = append(body, []cell{
				{r.Group, toneNone}, {pnlText, pnlTone},
				{strconv.Itoa(r.Trades), toneNone},
				{strconv.Itoa(r.Wins), toneNone},
				{strconv.Itoa(r.Losses), toneNone},
			})
			continue
		}
		acct := "ALL"
		combined := r.Account == nil
		if !combined {
			acct = labels[*r.Account]
			if acct == "" {
				acct = strconv.FormatInt(*r.Account, 10)
			}
		}
		if mixed && combined {
			pnlText, pnlTone = "n/a", toneNone
		}
		body = append(body, []cell{
			{r.Group, toneNone}, {acct, toneNone}, {pnlText, pnlTone},
			{strconv.Itoa(r.Trades), toneNone},
			{strconv.Itoa(r.Wins), toneNone},
			{strconv.Itoa(r.Losses), toneNone},
		})
	}
	if err := writeTable(w, cols, body, opts.Color); err != nil {
		return err
	}
```

(Everything from `na := func(s string) string {` to the end of `PnLTable` — the summary block — is unchanged.)

- [ ] **Step 6: Update the `cmd_pnl.go` call site**

Change `err = render.PnLTable(stdout, rows, sum, labels, mixed, opts)` to:

```go
		err = render.PnLTable(stdout, rows, sum, labels, *by, mixed, opts)
```

- [ ] **Step 7: Regenerate goldens and verify the time-cut golden is unchanged**

Run: `go test ./internal/render -run TestPnLTable -update`

This regenerates `pnl_table.golden` (produced with `groupBy == "week"`, so the `PERIOD` header and `ACCOUNT` column are retained — it must be byte-identical) and writes the new `pnl_table_symbol.golden`.

Run: `git diff --stat internal/render/testdata/pnl_table.golden`
Expected: **no change** (empty). If it changed, the time-cut path regressed — investigate before continuing.

- [ ] **Step 8: Read and confirm the symbol golden**

Read `internal/render/testdata/pnl_table_symbol.golden`. Confirm it matches (a `SYMBOL` first column, **no** `ACCOUNT` column, two symbol rows, then the standard summary block):

```
SYMBOL    P&L  TRADES  WINS  LOSSES
EURUSD   5.00       2     1       1
XAUUSD  10.00       1     1       0

Summary
  Performance
    Trades         3
    Win rate       66.7%
    Profit factor  4.75
    Expectancy     5.00
    Avg win        9.50
    Avg loss       -4.00
    Largest win    10.00
    Largest loss   -4.00
    Max drawdown   0.00
    Gross profit   19.00
    Gross loss     -4.00
  P&L breakdown
    Trade profit   15.00
    Commission     0.00
    Swap           0.00
    Fee            0.00
    Net P&L        15.00
```

If the generated spacing differs, trust the generated file (the manual writer is the source of truth) but verify the invariants: first column `SYMBOL`, no `ACCOUNT` column, two data rows, the summary block present.

- [ ] **Step 9: Run the full render package**

Run: `go test ./internal/render`
Expected: PASS (both goldens, the new symbol tests, all Phase 2 tests).

- [ ] **Step 10: Commit**

```bash
git add internal/render/render.go internal/render/render_test.go internal/render/testdata/pnl_table_symbol.golden cmd_pnl.go
git commit -m "feat(render): symbol/magic pnl table drops account column"
```

---

### Task 6: Wire `--by symbol|magic` into the `pnl` command + currency guard

Accept the two new `--by` values, base the currency guard on the contributing accounts (so it works for account-less symbol/magic rows), refuse a dimension cut under mixed currency, and refresh the help text.

**Files:**
- Modify: `cmd_common.go` (`currenciesInScope` signature), `cmd_pnl.go`
- Test: `cli_test.go`

- [ ] **Step 1: Add the failing CLI tests**

In `cli_test.go`, add these (they follow the existing pattern: `path := fixture(t)`, `--snapshot path`, huge `--stale-after`). The default `fixture` has symbols `EURUSD`/`XAUUSD` and all `magic: 0`:

```go
func TestPnLBySymbol(t *testing.T) {
	path := fixture(t)
	out, errOut, code := runCLI(t, "test-pass",
		"pnl", "--snapshot", path, "--from", "2026-01-01", "--to", "2026-01-31",
		"--by", "symbol", "--stale-after", "876000h")
	if code != 0 {
		t.Fatalf("exit %d, stderr: %s", code, errOut)
	}
	if !strings.Contains(out, "SYMBOL") || !strings.Contains(out, "EURUSD") {
		t.Errorf("by-symbol output should list symbols:\n%s", out)
	}
	if strings.Contains(out, "ACCOUNT") {
		t.Errorf("by-symbol output should not have an ACCOUNT column:\n%s", out)
	}
}

func TestPnLByMagicJSONGroupBy(t *testing.T) {
	path := fixture(t)
	out, _, code := runCLI(t, "test-pass",
		"pnl", "--snapshot", path, "--from", "2026-01-01", "--to", "2026-01-31",
		"--by", "magic", "--format", "json", "--stale-after", "876000h")
	if code != 0 {
		t.Fatalf("exit %d", code)
	}
	if !strings.Contains(out, `"group_by": "magic"`) {
		t.Errorf("by-magic JSON should carry group_by magic:\n%s", out)
	}
}
```

Also add a mixed-currency refusal test. The default fixture is single-currency, so add a dedicated multi-currency fixture and snapshot helper next to the existing `fixtureJSON`/`fixture` in `cli_test.go`:

```go
// Two accounts in different currencies, each with one in-range deal on a
// symbol — for the symbol/magic mixed-currency refusal.
const mixedFixtureJSON = `{
  "schema_version": "1.0",
  "generated_at": "2026-06-13T00:00:00Z",
  "accounts": [
    {"login": 111, "label": "USD EA", "currency": "USD", "balance": 1000.0,
     "equity": 1000.0, "last_success_at": "2026-06-13T00:00:00Z", "last_error": null},
    {"login": 333, "label": "EUR EA", "currency": "EUR", "balance": 1000.0,
     "equity": 1000.0, "last_success_at": "2026-06-13T00:00:00Z", "last_error": null}
  ],
  "closed_deals": [
    {"account": 111, "time": 1767607200, "profit": 5.0, "swap": 0.0, "commission": 0.0, "fee": 0.0,
     "ticket": 1, "order": 1, "position_id": 1, "time_msc": 0, "type": 0, "entry": 1, "reason": 0,
     "magic": 0, "volume": 0.1, "price": 1.0, "symbol": "EURUSD", "comment": "", "external_id": ""},
    {"account": 333, "time": 1767607200, "profit": 3.0, "swap": 0.0, "commission": 0.0, "fee": 0.0,
     "ticket": 2, "order": 2, "position_id": 2, "time_msc": 0, "type": 0, "entry": 1, "reason": 0,
     "magic": 0, "volume": 0.1, "price": 1.0, "symbol": "EURUSD", "comment": "", "external_id": ""}
  ],
  "open_positions": [],
  "cash_flows": []
}`

func mixedFixture(t *testing.T) string {
	t.Helper()
	return snaptest.Write(t, mixedFixtureJSON, "test-pass")
}

func TestPnLBySymbolMixedCurrencyRefuses(t *testing.T) {
	path := mixedFixture(t)
	_, errOut, code := runCLI(t, "test-pass",
		"pnl", "--snapshot", path, "--from", "2026-01-01", "--to", "2026-01-31",
		"--by", "symbol", "--stale-after", "876000h")
	if code != 1 {
		t.Fatalf("exit %d, want 1; stderr: %s", code, errOut)
	}
	if !strings.Contains(errOut, "narrow --accounts") {
		t.Errorf("refusal should guide narrowing accounts:\n%s", errOut)
	}
}
```

(Before adding `mixedFixture`, confirm `snaptest.Write(t, json, pass)` is the helper `fixture` uses — it is, per the existing `fixture` function. Keep the new const/helper next to the existing ones.)

- [ ] **Step 2: Run to verify the new tests fail**

Run: `go test . -run 'TestPnLBySymbol|TestPnLByMagic' -v`
Expected: FAIL — `--by symbol`/`magic` is rejected by the current validation (exit 1 for the success-path tests), and the refusal test fails because the guard does not yet fire for dimension cuts.

- [ ] **Step 3: Accept the new `--by` values**

In `cmd_pnl.go`, replace:

```go
	if *by != "day" && *by != "week" && *by != "month" {
		fmt.Fprintf(stderr, "error: invalid --by %q: use day, week or month\n", *by)
		return 1
	}
```
with:
```go
	switch *by {
	case "day", "week", "month", "symbol", "magic":
	default:
		fmt.Fprintf(stderr, "error: invalid --by %q: use day, week, month, symbol or magic\n", *by)
		return 1
	}
```

- [ ] **Step 4: Change `currenciesInScope` to take the contributing logins**

In `cmd_common.go`, change the signature and the no-filter branch. Replace:

```go
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

with:

```go
func currenciesInScope(accounts []snapshot.AccountSnapshot, filter map[int64]bool, contributing []int64) []string {
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
		for _, login := range contributing {
			if c := curBy[login]; c != "" {
				set[c] = true
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

Also update the doc comment just above it: change "the accounts that actually contributed rows" to "the accounts that actually contributed deals (passed via `contributing`)". If this removes the last use of the `aggregate` import in `cmd_common.go`, drop `aggregate` from that file's imports (the build will tell you: `imported and not used`).

- [ ] **Step 5: Compute `contributing` and refuse dimension cuts under mixed currency**

In `cmd_pnl.go`, replace:

```go
	curs := currenciesInScope(snap.Accounts, filter, rows)
	mixed := len(curs) > 1
	if mixed {
		fmt.Fprintf(warnW,
			"warning: accounts span multiple currencies (%s); combined totals are suppressed — narrow --accounts to one currency\n",
			strings.Join(curs, ", "))
	}
```

with:

```go
	contributing := aggregate.AccountsInScope(snap.ClosedDeals, aggregate.Options{
		From: fromD, To: toD, Accounts: filter,
	})
	curs := currenciesInScope(snap.Accounts, filter, contributing)
	mixed := len(curs) > 1
	if mixed && (*by == "symbol" || *by == "magic") {
		fmt.Fprintf(stderr,
			"error: --by %s aggregates across accounts but they span multiple currencies (%s); narrow --accounts to one currency\n",
			*by, strings.Join(curs, ", "))
		return 1
	}
	if mixed {
		fmt.Fprintf(warnW,
			"warning: accounts span multiple currencies (%s); combined totals are suppressed — narrow --accounts to one currency\n",
			strings.Join(curs, ", "))
	}
```

(The refusal prints to `stderr` and returns 1 — it is a hard failure, so it is not silenced by `--quiet`.)

- [ ] **Step 6: Refresh the `pnl` help text**

In `cmd_pnl.go`, in the `pnlHelp` string, replace the `--by` line:

```
  --by day|week|month         grouping (default week; weeks start Monday)
```
with:
```
  --by day|week|month|symbol|magic
                              grouping (default week; weeks start Monday).
                              symbol/magic aggregate across accounts.
```

and add, after the existing `--by month` example line in the Examples section:

```
  mt5-pnl-cli pnl --from 2026-01-01 --to 2026-12-31 --by symbol
```

- [ ] **Step 7: Confirm `TestPnLInvalidBy` still passes**

`TestPnLInvalidBy` (`--by fortnight`) asserts the error contains `--by`; the new message still does.

Run: `go test . -run TestPnLInvalidBy -v`
Expected: PASS.

- [ ] **Step 8: Run the new tests, then the full suite**

Run: `go test . -run 'TestPnLBySymbol|TestPnLByMagic' -v`
Expected: PASS (success paths exit 0; the mixed-currency cut exits 1 with the guidance).

Run: `go test ./...`
Expected: PASS (all packages).

- [ ] **Step 9: Commit**

```bash
git add cmd_common.go cmd_pnl.go cli_test.go
git commit -m "feat(pnl): accept --by symbol|magic and refuse mixed-currency dimension cuts"
```

---

### Task 7: Documentation

Update README and CLAUDE.md in the same change, per the project's "update in the same change" rule.

**Files:**
- Modify: `README.md`, `CLAUDE.md`

- [ ] **Step 1: Update the README `pnl` JSON Demo to the `group`/`group_by` shape**

In `README.md`, find the JSON Demo block (the `--by month --accounts "Trend EA" --format json` output). In **both** row objects, replace the leading `"period": "...",` line with two lines — `"group": "...",` then `"group_by": "month",` — keeping the date value. For example:

```json
      "period": "2026-01-01",
      "account": 111,
```
becomes:
```json
      "group": "2026-01-01",
      "group_by": "month",
      "account": 111,
```

The `summary` block is unchanged.

- [ ] **Step 2: Update the README `pnl` CSV Demo**

In `README.md`, find the CSV Demo block (the `--format csv` output). Replace its header:

```
period,account_login,account_label,pnl,trade_profit,commission,swap,fee,trades,wins,losses,gross_profit,gross_loss
```
with:
```
group,group_by,account_login,account_label,pnl,trade_profit,commission,swap,fee,trades,wins,losses,gross_profit,gross_loss
```

and insert `month` (matching that Demo's `--by month`) as the second field of each data row:

```
2026-01-01,month,111,Trend EA,10.00,13.00,-2.00,-1.00,0.00,3,2,1,14.00,-4.00
2026-01-01,month,,ALL,10.00,13.00,-2.00,-1.00,0.00,3,2,1,14.00,-4.00
```

- [ ] **Step 3: Document `--by symbol|magic` and add a symbol Demo**

In `README.md`, in the Commands section under `pnl`, replace the existing `--by` documentation (it lists `day|week|month`) with:

```markdown
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
```

Then, in the Demo section, add a short symbol-cut example after the existing `pnl` table Demo (illustrative, not golden-verified — keep numbers internally consistent, 2 + 1 = 3 trades):

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

(Trim the summary to a couple of lines plus `...`; the full block is already shown under the table Demo.)

- [ ] **Step 4: Note the uniform shape under the `--format` bullet**

In `README.md`, append to the existing `pnl` `--format` bullet (the one describing the Summary block / JSON parity / CSV columns):

```markdown
    Every cut emits the same JSON/CSV shape: rows carry `group` (the period
    date, symbol, or magic) and `group_by` (the `--by` value); `account` is
    the login for per-account time rows and `null` for the combined time row
    and for every symbol/magic row.
```

- [ ] **Step 5: Update CLAUDE.md**

In `CLAUDE.md`, replace the `internal/aggregate` architecture bullet:

```
- `internal/aggregate` — deals → period rows + summary. Each row and the
  summary carry net P&L plus its four components (trade_profit / commission
  / swap / fee, summing to net). The summary also carries expectancy,
  average and largest win/loss, and max drawdown (a deal-ordered
  realised-P&L pass, not equity drawdown). Full-precision sums; rounding
  happens in render only. Breakeven (net == 0) is neither win nor loss.
```
with:
```
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
```

Replace the **Mixed-currency guard** gotcha:

```
- **Mixed-currency guard.** `pnl` never sums across currencies: when
  accounts in scope span more than one, combined `ALL` rows and the
  summary are suppressed (`n/a`/`null`/omitted) with a stderr warning.
```
with:
```
- **Mixed-currency guard.** `pnl` never sums across currencies: when
  accounts in scope span more than one, combined `ALL` rows and the summary
  are suppressed (`n/a`/`null`/omitted) with a stderr warning. A
  `--by symbol|magic` cut has no per-account row to fall back to, so it
  **refuses** under mixed currency (stderr, exit 1) — narrow `--accounts`.
  Scope is the accounts that contributed deals (`aggregate.AccountsInScope`),
  not the grouped rows (symbol/magic rows carry no account).
```

Add a new gotcha bullet after the **Summary block is table/JSON only** bullet:

```
- **`group`/`group_by` are uniform across cuts.** Every `pnl` JSON/CSV row
  carries `group` (period date, symbol, or magic) and `group_by` (the
  `--by` value); the Go field is `aggregate.Row.Group`. `account` is the
  login for per-account time rows and `null` for the combined time row and
  for every symbol/magic row. The table labels the first column
  `PERIOD`/`SYMBOL`/`MAGIC` and drops the `ACCOUNT` column for dimension
  cuts.
```

- [ ] **Step 6: Run the full suite**

Run: `go test ./...`
Expected: PASS (docs-only change, but confirm nothing regressed).

- [ ] **Step 7: Commit**

```bash
git add README.md CLAUDE.md
git commit -m "docs: document --by symbol|magic and the uniform group/group_by shape"
```

---

## After all tasks

Dispatch a final whole-branch code review (subagent-driven-development's final step), then use `superpowers:finishing-a-development-branch`. The remaining Phase 3 commands — `positions` (3.3), `trades` (3.4) and `cash-flows` (3.5) — are separate later plans before the single release.
