# Phase 2 — Richer Reporting Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Decompose `pnl` net P&L into its four components (trade profit / commission / swap / fee) and deepen the summary with expectancy, average and largest win/loss, and max drawdown — surfaced in JSON, CSV and a redesigned two-group table summary block.

**Architecture:** `internal/aggregate` grows the `Row` and `Summary` structs and gains a deal-ordered pass for max drawdown; rounding stays in `internal/render`. `render` adds the new fields to its JSON/CSV emitters and replaces the single summary line with an aligned key/value block (new `writeKV` helper in `table.go`). The command layer is untouched — `PnLTable`/`PnLJSON`/`PnLCSV` keep their signatures, so `cmd_pnl.go` needs no change.

**Tech Stack:** Go 1.25, stdlib `cmp`/`slices` (drawdown sort), `encoding/json`, `encoding/csv`. Tests are table-driven; `internal/render` uses golden files plus exact-string matches. Module path `github.com/tanem/mt5-pnl-cli`.

**Branch:** Work on `feat/cli-read-views-and-reporting` (already checked out and pushed; Phase 1 + the ergonomics bundle live here). Do **not** branch off `main`.

**Spec:** `docs/superpowers/specs/2026-06-15-cli-read-views-and-reporting-design.md` (Phase 2, sections 2.1–2.4).

**Scope notes / deliberate deferrals (do not implement here):**
- `--by symbol|magic`, the uniform `group`/`group_by` JSON/CSV rename, and the `positions`/`trades`/`cash-flows` commands are **Phase 3**. Phase 2 keeps the `period` key in JSON and CSV.
- **This phase deliberately changes JSON, CSV and table output** — the opposite of the ergonomics bundle's byte-identical invariant. The JSON exact-match tests, the CSV exact-match tests, the table golden, and the README Demo are all updated here on purpose. All Phase 1–3 work ships as a **single release**, so these intermediate output changes never reach users.
- **Currency:** `aggregate` stays currency-agnostic — it computes every metric (drawdown included) over the deals it is given. The mixed-currency guard already lives in `render` (the `mixed bool` parameter): under mixed currency the whole summary block and the combined (`account == nil`) rows have their money-valued fields suppressed (`n/a` in tables, `null` in JSON), while counts (`trades`/`wins`/`losses`) and the count-based `win_rate_pct` survive. Phase 2 extends that existing suppression to the new money fields.

**Field reference (used across tasks — keep names consistent):**

`aggregate.Row` gains four signed component subtotals (each a sum of the matching deal field; `PnL` continues to equal `TradeProfit + Commission + Swap + Fee`):

```go
TradeProfit float64 // sum of deal Profit
Commission  float64 // sum of deal Commission
Swap        float64 // sum of deal Swap
Fee         float64 // sum of deal Fee
```

`aggregate.Summary` gains the same four subtotals plus six metrics:

```go
Expectancy  *float64 // TotalPnL / TotalTrades; nil when no trades
AvgWin      *float64 // GrossProfit / wins; nil when no wins
AvgLoss     *float64 // GrossLoss / losses (negative); nil when no losses
LargestWin  *float64 // max single-deal net among wins; nil when no wins
LargestLoss *float64 // min single-deal net among losses (negative); nil when no losses
MaxDrawdown *float64 // realised-P&L drawdown (<= 0); nil when no deals
TradeProfit float64
Commission  float64
Swap        float64
Fee         float64
```

---

### Task 1: P&L component subtotals in `aggregate`

Split net P&L into its four parts on every `Row` and on the `Summary`.

**Files:**
- Modify: `internal/aggregate/aggregate.go` (the `Row` and `Summary` structs and the `Aggregate` fold)
- Test: `internal/aggregate/aggregate_test.go`

- [ ] **Step 1: Make the breakeven fixture deal binary-exact**

In `aggregate_test.go`, deal 3 currently uses `0.7 / -0.7`, which is not exact in float64 and would make the combined-row component sums fragile under `reflect.DeepEqual`. Change it to `0.5 / -0.5` (still net 0, still breakeven) and update its comment.

Replace:

```go
//	acct 222, Tue 2026-01-06: net  0.0 (0.7 - 0.7)          -> breakeven: neither
```
with:
```go
//	acct 222, Tue 2026-01-06: net  0.0 (0.5 - 0.5)          -> breakeven: neither
```

Replace:
```go
	deal(222, 1767693600, 0.7, 0, -0.7, 0),
```
with:
```go
	deal(222, 1767693600, 0.5, 0, -0.5, 0),
```

- [ ] **Step 2: Extend `TestAggregateByWeek`'s expected rows with component subtotals**

Replace the `want` slice in `TestAggregateByWeek` with:

```go
	want := []aggregate.Row{
		{Period: "2026-01-05", Account: ptr(int64(111)), PnL: 5.0, TradeProfit: 6.0, Commission: -0.5, Swap: -0.5, Fee: 0, Trades: 2, Wins: 1, Losses: 1, GrossProfit: 9.0, GrossLoss: -4.0},
		{Period: "2026-01-05", Account: ptr(int64(222)), PnL: 0.0, TradeProfit: 0.5, Commission: -0.5, Swap: 0, Fee: 0, Trades: 1, Wins: 0, Losses: 0, GrossProfit: 0, GrossLoss: 0},
		{Period: "2026-01-05", Account: nil, PnL: 5.0, TradeProfit: 6.5, Commission: -1.0, Swap: -0.5, Fee: 0, Trades: 3, Wins: 1, Losses: 1, GrossProfit: 9.0, GrossLoss: -4.0},
		{Period: "2026-01-12", Account: ptr(int64(111)), PnL: 5.0, TradeProfit: 5.0, Commission: 0, Swap: 0, Fee: 0, Trades: 1, Wins: 1, Losses: 0, GrossProfit: 5.0, GrossLoss: 0},
		{Period: "2026-01-12", Account: nil, PnL: 5.0, TradeProfit: 5.0, Commission: 0, Swap: 0, Fee: 0, Trades: 1, Wins: 1, Losses: 0, GrossProfit: 5.0, GrossLoss: 0},
	}
```

- [ ] **Step 3: Run the test to verify it fails**

Run: `go test ./internal/aggregate -run TestAggregateByWeek -v`
Expected: FAIL — `unknown field 'TradeProfit' in struct literal` (the struct does not yet have the fields).

- [ ] **Step 4: Add the four fields to `Row` and `Summary`**

In `aggregate.go`, change the `Row` struct to:

```go
type Row struct {
	Period      string
	Account     *int64
	PnL         float64
	TradeProfit float64
	Commission  float64
	Swap        float64
	Fee         float64
	Trades      int
	Wins        int
	Losses      int
	GrossProfit float64
	GrossLoss   float64
}
```

and the `Summary` struct to:

```go
type Summary struct {
	TotalPnL     float64
	TotalTrades  int
	WinRatePct   *float64 // nil when no trades
	ProfitFactor *float64 // nil when no gross loss
	GrossProfit  float64
	GrossLoss    float64
	TradeProfit  float64
	Commission   float64
	Swap         float64
	Fee          float64
}
```

- [ ] **Step 5: Accumulate the components in the fold**

In `Aggregate`, in the per-deal loop, right after `b.PnL += net`, add:

```go
		b.TradeProfit += d.Profit
		b.Commission += d.Commission
		b.Swap += d.Swap
		b.Fee += d.Fee
```

Then in the combined/summary loop, after `combined.PnL += b.PnL`, add:

```go
			combined.TradeProfit += b.TradeProfit
			combined.Commission += b.Commission
			combined.Swap += b.Swap
			combined.Fee += b.Fee
```

and after `sum.TotalPnL += combined.PnL`, add:

```go
		sum.TradeProfit += combined.TradeProfit
		sum.Commission += combined.Commission
		sum.Swap += combined.Swap
		sum.Fee += combined.Fee
```

- [ ] **Step 6: Run the test to verify it passes**

Run: `go test ./internal/aggregate -run TestAggregateByWeek -v`
Expected: PASS

- [ ] **Step 7: Run the whole package**

Run: `go test ./internal/aggregate`
Expected: PASS (the other aggregate tests use named fields, so the new fields default to zero and do not break them).

- [ ] **Step 8: Commit**

```bash
git add internal/aggregate/aggregate.go internal/aggregate/aggregate_test.go
git commit -m "feat(aggregate): split net P&L into trade-profit/commission/swap/fee subtotals"
```

---

### Task 2: Expectancy, average and largest win/loss in `Summary`

Fold-derived performance metrics. No deal ordering needed.

**Files:**
- Modify: `internal/aggregate/aggregate.go` (the `Summary` struct and the post-fold computations)
- Test: `internal/aggregate/aggregate_test.go`

- [ ] **Step 1: Write the failing test**

Add to `aggregate_test.go`:

```go
func TestSummaryMetrics(t *testing.T) {
	_, sum := aggregate.Aggregate(deals, aggregate.Options{
		From: date(2026, 1, 1), To: date(2026, 1, 31), By: "week",
	})
	// 4 trades, total P&L 10.0 -> expectancy 2.5
	if sum.Expectancy == nil || *sum.Expectancy != 2.5 {
		t.Errorf("expectancy = %v, want 2.5", sum.Expectancy)
	}
	// gross profit 14.0 over 2 wins -> avg win 7.0
	if sum.AvgWin == nil || *sum.AvgWin != 7.0 {
		t.Errorf("avg win = %v, want 7.0", sum.AvgWin)
	}
	// gross loss -4.0 over 1 loss -> avg loss -4.0
	if sum.AvgLoss == nil || *sum.AvgLoss != -4.0 {
		t.Errorf("avg loss = %v, want -4.0", sum.AvgLoss)
	}
	// winning deals net 9.0 and 5.0 -> largest win 9.0
	if sum.LargestWin == nil || *sum.LargestWin != 9.0 {
		t.Errorf("largest win = %v, want 9.0", sum.LargestWin)
	}
	// only losing deal net -4.0 -> largest loss -4.0
	if sum.LargestLoss == nil || *sum.LargestLoss != -4.0 {
		t.Errorf("largest loss = %v, want -4.0", sum.LargestLoss)
	}
}
```

Also extend `TestEmpty` to assert the new metrics are nil. After the existing `WinRatePct`/`ProfitFactor` nil check, add:

```go
	if sum.Expectancy != nil || sum.AvgWin != nil || sum.AvgLoss != nil ||
		sum.LargestWin != nil || sum.LargestLoss != nil {
		t.Errorf("want nil metrics on empty input, got %+v", sum)
	}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/aggregate -run 'TestSummaryMetrics|TestEmpty' -v`
Expected: FAIL — `sum.Expectancy undefined` (field not on struct yet).

- [ ] **Step 3: Add the five metric fields to `Summary`**

In `aggregate.go`, extend `Summary` (insert after the `ProfitFactor` line):

```go
	Expectancy   *float64 // TotalPnL / TotalTrades; nil when no trades
	AvgWin       *float64 // GrossProfit / wins; nil when no wins
	AvgLoss      *float64 // GrossLoss / losses (negative); nil when no losses
	LargestWin   *float64 // max single-deal net among wins; nil when no wins
	LargestLoss  *float64 // min single-deal net among losses (negative); nil when no losses
```

- [ ] **Step 4: Track largest win/loss in the fold and count losses**

In `Aggregate`, declare trackers before the per-deal loop, next to the existing `accountSet` declaration:

```go
	var largestWin, largestLoss *float64
```

In the per-deal loop's `switch`, extend the win and loss cases:

```go
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
```

Add a `totalLosses` counter alongside the existing `totalWins`. Change:

```go
	totalWins := 0
```
to:
```go
	totalWins, totalLosses := 0, 0
```

and in the summary loop, after `totalWins += combined.Wins`, add:

```go
		totalLosses += combined.Losses
```

- [ ] **Step 5: Compute the derived metrics after the fold**

In `Aggregate`, extend the post-loop block. Inside the existing `if sum.TotalTrades > 0 {` block, after setting `sum.WinRatePct`, add expectancy:

```go
		e := sum.TotalPnL / float64(sum.TotalTrades)
		sum.Expectancy = &e
```

After the existing `if sum.GrossLoss != 0 { ... }` profit-factor block, add:

```go
	if totalWins > 0 {
		aw := sum.GrossProfit / float64(totalWins)
		sum.AvgWin = &aw
	}
	if totalLosses > 0 {
		al := sum.GrossLoss / float64(totalLosses)
		sum.AvgLoss = &al
	}
	sum.LargestWin = largestWin
	sum.LargestLoss = largestLoss
```

- [ ] **Step 6: Run the tests to verify they pass**

Run: `go test ./internal/aggregate -run 'TestSummaryMetrics|TestEmpty' -v`
Expected: PASS

- [ ] **Step 7: Run the whole package**

Run: `go test ./internal/aggregate`
Expected: PASS

- [ ] **Step 8: Commit**

```bash
git add internal/aggregate/aggregate.go internal/aggregate/aggregate_test.go
git commit -m "feat(aggregate): add expectancy, average and largest win/loss to summary"
```

---

### Task 3: Max drawdown in `aggregate`

Realised-P&L drawdown over the in-scope deals, ordered by `time` then `time_msc`, cumulative from 0, largest peak-to-trough decline (signed-negative). See spec 2.2.

**Files:**
- Modify: `internal/aggregate/aggregate.go` (imports, the fold, the post-fold block)
- Test: `internal/aggregate/aggregate_test.go`

- [ ] **Step 1: Write the failing tests**

Add to `aggregate_test.go`:

```go
var ddOpts = aggregate.Options{From: date(2026, 1, 1), To: date(2026, 1, 31), By: "week"}

const ddBase = int64(1767607200) // 2026-01-05 UTC, inside ddOpts range

func TestMaxDrawdownMonotoneUp(t *testing.T) {
	// Cumulative curve only ever rises -> no retrace -> 0.00 (not nil).
	ds := []snapshot.Deal{
		deal(1, ddBase, 1.0, 0, 0, 0),
		deal(1, ddBase+60, 2.0, 0, 0, 0),
		deal(1, ddBase+120, 3.0, 0, 0, 0),
	}
	_, sum := aggregate.Aggregate(ds, ddOpts)
	if sum.MaxDrawdown == nil || *sum.MaxDrawdown != 0 {
		t.Errorf("max drawdown = %v, want 0", sum.MaxDrawdown)
	}
}

func TestMaxDrawdownSingleTrough(t *testing.T) {
	// cum: 10, 6, 7 ; peak 10 ; deepest decline 6-10 = -4
	ds := []snapshot.Deal{
		deal(1, ddBase, 10.0, 0, 0, 0),
		deal(1, ddBase+60, -4.0, 0, 0, 0),
		deal(1, ddBase+120, 1.0, 0, 0, 0),
	}
	_, sum := aggregate.Aggregate(ds, ddOpts)
	if sum.MaxDrawdown == nil || *sum.MaxDrawdown != -4.0 {
		t.Errorf("max drawdown = %v, want -4.0", sum.MaxDrawdown)
	}
}

func TestMaxDrawdownMultipleTroughsUnordered(t *testing.T) {
	// Time order of nets: +5, -2, +3, -6, +1
	// cum:  5,  3,  6,  0,  1 ; peak: 5, 5, 6, 6, 6 ; deepest 0-6 = -6.
	// Input is deliberately shuffled to prove Aggregate sorts by time.
	ds := []snapshot.Deal{
		deal(1, ddBase+180, -6.0, 0, 0, 0),
		deal(1, ddBase, 5.0, 0, 0, 0),
		deal(1, ddBase+240, 1.0, 0, 0, 0),
		deal(1, ddBase+60, -2.0, 0, 0, 0),
		deal(1, ddBase+120, 3.0, 0, 0, 0),
	}
	_, sum := aggregate.Aggregate(ds, ddOpts)
	if sum.MaxDrawdown == nil || *sum.MaxDrawdown != -6.0 {
		t.Errorf("max drawdown = %v, want -6.0", sum.MaxDrawdown)
	}
}

func TestMaxDrawdownTiesBrokenByTimeMsc(t *testing.T) {
	// Three deals share Time; ordering is decided by TimeMsc.
	// time_msc order of nets: +10, -4, -3 -> cum 10, 6, 3 -> deepest 3-10 = -7.
	ds := []snapshot.Deal{
		{Account: 1, Time: ddBase, TimeMsc: ddBase*1000 + 300, Profit: -3.0},
		{Account: 1, Time: ddBase, TimeMsc: ddBase*1000 + 100, Profit: 10.0},
		{Account: 1, Time: ddBase, TimeMsc: ddBase*1000 + 200, Profit: -4.0},
	}
	_, sum := aggregate.Aggregate(ds, ddOpts)
	if sum.MaxDrawdown == nil || *sum.MaxDrawdown != -7.0 {
		t.Errorf("max drawdown = %v, want -7.0", sum.MaxDrawdown)
	}
}
```

Also extend `TestEmpty`'s nil-metrics assertion to include `MaxDrawdown`:

```go
	if sum.Expectancy != nil || sum.AvgWin != nil || sum.AvgLoss != nil ||
		sum.LargestWin != nil || sum.LargestLoss != nil || sum.MaxDrawdown != nil {
		t.Errorf("want nil metrics on empty input, got %+v", sum)
	}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/aggregate -run TestMaxDrawdown -v`
Expected: FAIL — `sum.MaxDrawdown undefined`.

- [ ] **Step 3: Add the `MaxDrawdown` field and the `cmp` import**

In `aggregate.go`, add `"cmp"` to the import block (keep imports sorted: `cmp`, `math`, `slices`, `time`).

Extend `Summary` (after `LargestLoss`):

```go
	MaxDrawdown  *float64 // realised-P&L drawdown (<= 0); nil when no deals; 0 when never retraces
```

- [ ] **Step 4: Collect in-scope deals during the fold**

In `Aggregate`, declare the collector before the per-deal loop (next to `largestWin, largestLoss`):

```go
	type timed struct {
		time, timeMsc int64
		net           float64
	}
	var inScope []timed
```

In the per-deal loop, after `accountSet[d.Account] = true` (the deal has passed both the account and date filters by this point), add:

```go
		inScope = append(inScope, timed{d.Time, d.TimeMsc, net})
```

- [ ] **Step 5: Compute drawdown after the fold**

In `Aggregate`, after `sum.LargestLoss = largestLoss` (from Task 2) and before `return rows, sum`, add:

```go
	if len(inScope) > 0 {
		slices.SortFunc(inScope, func(a, b timed) int {
			if a.time != b.time {
				return cmp.Compare(a.time, b.time)
			}
			return cmp.Compare(a.timeMsc, b.timeMsc)
		})
		var cum, peak, maxDD float64 // all start at 0
		for _, d := range inScope {
			cum += d.net
			if cum > peak {
				peak = cum
			}
			if dd := cum - peak; dd < maxDD {
				maxDD = dd
			}
		}
		sum.MaxDrawdown = &maxDD
	}
```

- [ ] **Step 6: Run the tests to verify they pass**

Run: `go test ./internal/aggregate -run 'TestMaxDrawdown|TestEmpty' -v`
Expected: PASS

- [ ] **Step 7: Run the whole package**

Run: `go test ./internal/aggregate`
Expected: PASS

- [ ] **Step 8: Commit**

```bash
git add internal/aggregate/aggregate.go internal/aggregate/aggregate_test.go
git commit -m "feat(aggregate): compute realised-P&L max drawdown over ordered deals"
```

---

### Task 4: Component subtotals and metrics in JSON

Carry the four components on every JSON row and add the components plus the six metrics to the JSON summary. Honour the existing mixed-currency suppression.

**Files:**
- Modify: `internal/render/render.go` (`pnlRow`, `pnlSummary`, `PnLJSON`)
- Test: `internal/render/render_test.go`

- [ ] **Step 1: Update the shared render fixtures with the new fields**

In `render_test.go`, replace the `rows` fixture with:

```go
var rows = []aggregate.Row{
	{Period: "2026-01-05", Account: ptr(int64(111)), PnL: 5.004, TradeProfit: 6.0, Commission: -0.5, Swap: -0.496, Fee: 0, Trades: 2, Wins: 1, Losses: 1, GrossProfit: 9.0, GrossLoss: -3.996},
	{Period: "2026-01-05", Account: nil, PnL: 5.004, TradeProfit: 6.0, Commission: -0.5, Swap: -0.496, Fee: 0, Trades: 2, Wins: 1, Losses: 1, GrossProfit: 9.0, GrossLoss: -3.996},
}
```

and replace the `sum` fixture with:

```go
var sum = aggregate.Summary{
	TotalPnL: 5.004, TotalTrades: 2,
	WinRatePct: ptr(50.0), ProfitFactor: ptr(2.2522522522522523),
	GrossProfit: 9.0, GrossLoss: -3.996,
	Expectancy: ptr(2.502), AvgWin: ptr(9.0), AvgLoss: ptr(-3.996),
	LargestWin: ptr(9.0), LargestLoss: ptr(-3.996), MaxDrawdown: ptr(-3.996),
	TradeProfit: 6.0, Commission: -0.5, Swap: -0.496, Fee: 0,
}
```

(`6.0 - 0.5 - 0.496 = 5.004 = PnL`. Rounded to 2 dp the components read 6.00 / -0.50 / -0.50 / 0.00.)

- [ ] **Step 2: Rewrite `TestPnLJSON`'s expected output**

Replace the `want` string literal in `TestPnLJSON` with:

```go
	want := `{
  "rows": [
    {
      "period": "2026-01-05",
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
      "period": "2026-01-05",
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

- [ ] **Step 3: Extend `TestPnLJSONMixedNulls`**

In `TestPnLJSONMixedNulls`, replace the `want` slice in the suppression check with the full set of suppressed summary fields:

```go
	for _, want := range []string{
		`"total_pnl": null`, `"profit_factor": null`,
		`"expectancy": null`, `"avg_win": null`, `"avg_loss": null`,
		`"largest_win": null`, `"largest_loss": null`, `"max_drawdown": null`,
		`"gross_profit": null`, `"gross_loss": null`,
		`"trade_profit": null`, `"commission": null`, `"swap": null`, `"fee": null`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("mixed summary missing %q:\n%s", want, out)
		}
	}
```

The existing assertions in that test (`"pnl": 5` present, `"win_rate_pct": 50` present, `"total_trades": 2` present) stay as they are — those survive mixed currency.

- [ ] **Step 4: Run the JSON tests to verify they fail**

Run: `go test ./internal/render -run 'TestPnLJSON' -v`
Expected: FAIL — output is missing the new keys (`pnlRow`/`pnlSummary` not yet extended).

- [ ] **Step 5: Add the component fields to `pnlRow`**

In `render.go`, change `pnlRow` to (insert the four after `PnL`):

```go
type pnlRow struct {
	Period      string   `json:"period"`
	Account     *int64   `json:"account"`
	PnL         *float64 `json:"pnl"`
	TradeProfit *float64 `json:"trade_profit"`
	Commission  *float64 `json:"commission"`
	Swap        *float64 `json:"swap"`
	Fee         *float64 `json:"fee"`
	Trades      int      `json:"trades"`
	Wins        int      `json:"wins"`
	Losses      int      `json:"losses"`
	GrossProfit *float64 `json:"gross_profit"`
	GrossLoss   *float64 `json:"gross_loss"`
}
```

- [ ] **Step 6: Add the metrics and components to `pnlSummary`**

Change `pnlSummary` to:

```go
type pnlSummary struct {
	TotalPnL     *float64 `json:"total_pnl"`
	TotalTrades  int      `json:"total_trades"`
	WinRatePct   *float64 `json:"win_rate_pct"`
	ProfitFactor *float64 `json:"profit_factor"`
	Expectancy   *float64 `json:"expectancy"`
	AvgWin       *float64 `json:"avg_win"`
	AvgLoss      *float64 `json:"avg_loss"`
	LargestWin   *float64 `json:"largest_win"`
	LargestLoss  *float64 `json:"largest_loss"`
	MaxDrawdown  *float64 `json:"max_drawdown"`
	GrossProfit  *float64 `json:"gross_profit"`
	GrossLoss    *float64 `json:"gross_loss"`
	TradeProfit  *float64 `json:"trade_profit"`
	Commission   *float64 `json:"commission"`
	Swap         *float64 `json:"swap"`
	Fee          *float64 `json:"fee"`
}
```

- [ ] **Step 7: Populate the new fields in `PnLJSON`**

In `PnLJSON`, in the per-row loop, extend the `row` construction to set the components (rounded to 2 dp):

```go
		row := pnlRow{
			Period: r.Period, Account: r.Account,
			PnL:         numPtr(round(r.PnL, 2)),
			TradeProfit: numPtr(round(r.TradeProfit, 2)),
			Commission:  numPtr(round(r.Commission, 2)),
			Swap:        numPtr(round(r.Swap, 2)),
			Fee:         numPtr(round(r.Fee, 2)),
			Trades:      r.Trades, Wins: r.Wins, Losses: r.Losses,
			GrossProfit: numPtr(round(r.GrossProfit, 2)), GrossLoss: numPtr(round(r.GrossLoss, 2)),
		}
		if mixed && r.Account == nil {
			row.PnL, row.GrossProfit, row.GrossLoss = nil, nil, nil
			row.TradeProfit, row.Commission, row.Swap, row.Fee = nil, nil, nil, nil
		}
```

Extend the summary construction:

```go
	out.Summary = pnlSummary{
		TotalPnL: numPtr(round(sum.TotalPnL, 2)), TotalTrades: sum.TotalTrades,
		WinRatePct: roundPtr(sum.WinRatePct, 1), ProfitFactor: roundPtr(sum.ProfitFactor, 2),
		Expectancy:  roundPtr(sum.Expectancy, 2),
		AvgWin:      roundPtr(sum.AvgWin, 2),
		AvgLoss:     roundPtr(sum.AvgLoss, 2),
		LargestWin:  roundPtr(sum.LargestWin, 2),
		LargestLoss: roundPtr(sum.LargestLoss, 2),
		MaxDrawdown: roundPtr(sum.MaxDrawdown, 2),
		GrossProfit: numPtr(round(sum.GrossProfit, 2)), GrossLoss: numPtr(round(sum.GrossLoss, 2)),
		TradeProfit: numPtr(round(sum.TradeProfit, 2)),
		Commission:  numPtr(round(sum.Commission, 2)),
		Swap:        numPtr(round(sum.Swap, 2)),
		Fee:         numPtr(round(sum.Fee, 2)),
	}
	if mixed {
		out.Summary.TotalPnL, out.Summary.GrossProfit, out.Summary.GrossLoss, out.Summary.ProfitFactor = nil, nil, nil, nil
		out.Summary.Expectancy, out.Summary.AvgWin, out.Summary.AvgLoss = nil, nil, nil
		out.Summary.LargestWin, out.Summary.LargestLoss, out.Summary.MaxDrawdown = nil, nil, nil
		out.Summary.TradeProfit, out.Summary.Commission, out.Summary.Swap, out.Summary.Fee = nil, nil, nil, nil
	}
```

- [ ] **Step 8: Run the JSON tests to verify they pass**

Run: `go test ./internal/render -run 'TestPnLJSON' -v`
Expected: PASS

- [ ] **Step 9: Run the whole render package**

Run: `go test ./internal/render`
Expected: PASS. (The table golden is unchanged — the single-line summary only shows fields that did not change values; the CSV test is unchanged — `PnLCSV` does not yet emit components.)

- [ ] **Step 10: Commit**

```bash
git add internal/render/render.go internal/render/render_test.go
git commit -m "feat(render): add P&L components and performance metrics to JSON"
```

---

### Task 5: Component columns in CSV

Insert the four component columns into the `pnl` CSV, after `pnl` and before `trades` (matching spec 1.2 column order).

**Files:**
- Modify: `internal/render/render.go` (`PnLCSV`)
- Test: `internal/render/render_test.go`

- [ ] **Step 1: Rewrite `TestPnLCSV`'s expected output**

Replace the `want` string in `TestPnLCSV` with:

```go
	want := "period,account_login,account_label,pnl,trade_profit,commission,swap,fee,trades,wins,losses,gross_profit,gross_loss\n" +
		"2026-01-05,111,Trend EA,5.00,6.00,-0.50,-0.50,0.00,2,1,1,9.00,-4.00\n" +
		"2026-01-05,,ALL,5.00,6.00,-0.50,-0.50,0.00,2,1,1,9.00,-4.00\n"
```

(`TestPnLCSVMixedOmitsCombined` needs no change — it asserts the `ALL` row is absent and the per-account row prefix `2026-01-05,111,Trend EA` is present, both still true.)

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/render -run TestPnLCSV -v`
Expected: FAIL — header and rows lack the component columns.

- [ ] **Step 3: Add the columns in `PnLCSV`**

In `render.go`, change the header `Write` in `PnLCSV` to:

```go
	if err := cw.Write([]string{
		"period", "account_login", "account_label",
		"pnl", "trade_profit", "commission", "swap", "fee",
		"trades", "wins", "losses", "gross_profit", "gross_loss",
	}); err != nil {
		return err
	}
```

and the per-row `Write` to:

```go
		if err := cw.Write([]string{
			r.Period, login, label,
			money(r.PnL), money(r.TradeProfit), money(r.Commission), money(r.Swap), money(r.Fee),
			strconv.Itoa(r.Trades), strconv.Itoa(r.Wins), strconv.Itoa(r.Losses),
			money(r.GrossProfit), money(r.GrossLoss),
		}); err != nil {
			return err
		}
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test ./internal/render -run TestPnLCSV -v`
Expected: PASS

- [ ] **Step 5: Run the whole render package**

Run: `go test ./internal/render`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add internal/render/render.go internal/render/render_test.go
git commit -m "feat(render): add P&L component columns to pnl CSV"
```

---

### Task 6: Two-group table summary block

Replace the single summary line with an aligned "Summary" block in two groups (Performance, P&L breakdown). The headline **Net P&L** keeps the sign colour and the currency suffix from the ergonomics bundle; the rest of the block is plain. See spec 2.3.

**Design decisions (carried into the code below):**
- The block is rendered by a new `writeKV` helper in `table.go`, mirroring `writeTable`: widths from plain text, colour applied to the value only and after padding, so alignment never skews.
- Only the **Net P&L** value is sign-coloured (the analogue of the old coloured total) and only it carries the currency suffix; this keeps the block readable and the change minimal. Per-row P&L cells keep their existing colouring.
- Under mixed currency, every money line shows `n/a` (no colour, no currency); **Trades** and **Win rate** survive (they do not sum money). `null` metrics (e.g. no trades) also render `n/a`.

**Files:**
- Modify: `internal/render/table.go` (add `kv`, `kvGroup`, `writeKV`)
- Modify: `internal/render/render.go` (`PnLTable` summary section)
- Test: `internal/render/table_test.go`, `internal/render/render_test.go`, `cli_test.go`
- Golden: `internal/render/testdata/pnl_table.golden`

- [ ] **Step 1: Write the failing white-box test for `writeKV`**

Add to `internal/render/table_test.go`:

```go
func TestWriteKVAligns(t *testing.T) {
	var buf bytes.Buffer
	err := writeKV(&buf, []kvGroup{
		{"Performance", []kv{
			{"Trades", "4", toneNone},
			{"Profit factor", "3.50", toneNone},
		}},
		{"P&L breakdown", []kv{
			{"Net P&L", "10.00", tonePos},
		}},
	}, false)
	if err != nil {
		t.Fatal(err)
	}
	want := "Summary\n" +
		"  Performance\n" +
		"    Trades         4\n" +
		"    Profit factor  3.50\n" +
		"  P&L breakdown\n" +
		"    Net P&L        10.00\n"
	if buf.String() != want {
		t.Errorf("writeKV output:\ngot:\n%q\nwant:\n%q", buf.String(), want)
	}
}
```

(`table_test.go` is already `package render` and already imports `bytes`.)

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/render -run TestWriteKVAligns -v`
Expected: FAIL — `undefined: writeKV` / `undefined: kv`.

- [ ] **Step 3: Add `kv`, `kvGroup` and `writeKV` to `table.go`**

Append to `internal/render/table.go`:

```go
type kv struct {
	label string
	value string
	tone  tone
}

type kvGroup struct {
	title string
	items []kv
}

// writeKV renders grouped label/value pairs beneath a "Summary" heading. Group
// titles are indented two spaces and their items four; values align in one
// column across all groups (width from the widest label). Tone colour, when
// color is true, wraps the value only and is applied after the label is padded,
// so it never skews the label column.
func writeKV(w io.Writer, groups []kvGroup, color bool) error {
	width := 0
	for _, g := range groups {
		for _, it := range g.items {
			if len(it.label) > width {
				width = len(it.label)
			}
		}
	}
	var b strings.Builder
	b.WriteString("Summary\n")
	for _, g := range groups {
		b.WriteString("  ")
		b.WriteString(g.title)
		b.WriteByte('\n')
		for _, it := range g.items {
			b.WriteString("    ")
			b.WriteString(pad(it.label, width, false))
			b.WriteString("  ")
			b.WriteString(colorise(it.value, it.tone, color))
			b.WriteByte('\n')
		}
	}
	_, err := io.WriteString(w, b.String())
	return err
}
```

- [ ] **Step 4: Run the white-box test to verify it passes**

Run: `go test ./internal/render -run TestWriteKVAligns -v`
Expected: PASS

- [ ] **Step 5: Replace the summary section of `PnLTable`**

In `render.go`, replace everything in `PnLTable` from `totalPnL := fmt.Sprintf(...)` through the final `return err` (the whole single-line `fmt.Fprintf` block) with:

```go
	na := func(s string) string {
		if mixed {
			return "n/a"
		}
		return s
	}
	netStr := na(fmt.Sprintf("%.2f", sum.TotalPnL))
	netTone := signTone(round(sum.TotalPnL, 2))
	if mixed {
		netTone = toneNone
	} else if opts.Currency != "" {
		netStr += " " + opts.Currency
	}
	perf := []kv{
		{"Trades", strconv.Itoa(sum.TotalTrades), toneNone},
		{"Win rate", fmtPtr(sum.WinRatePct, "%.1f%%"), toneNone},
		{"Profit factor", na(fmtPtr(sum.ProfitFactor, "%.2f")), toneNone},
		{"Expectancy", na(fmtPtr(sum.Expectancy, "%.2f")), toneNone},
		{"Avg win", na(fmtPtr(sum.AvgWin, "%.2f")), toneNone},
		{"Avg loss", na(fmtPtr(sum.AvgLoss, "%.2f")), toneNone},
		{"Largest win", na(fmtPtr(sum.LargestWin, "%.2f")), toneNone},
		{"Largest loss", na(fmtPtr(sum.LargestLoss, "%.2f")), toneNone},
		{"Max drawdown", na(fmtPtr(sum.MaxDrawdown, "%.2f")), toneNone},
		{"Gross profit", na(fmt.Sprintf("%.2f", sum.GrossProfit)), toneNone},
		{"Gross loss", na(fmt.Sprintf("%.2f", sum.GrossLoss)), toneNone},
	}
	breakdown := []kv{
		{"Trade profit", na(fmt.Sprintf("%.2f", sum.TradeProfit)), toneNone},
		{"Commission", na(fmt.Sprintf("%.2f", sum.Commission)), toneNone},
		{"Swap", na(fmt.Sprintf("%.2f", sum.Swap)), toneNone},
		{"Fee", na(fmt.Sprintf("%.2f", sum.Fee)), toneNone},
		{"Net P&L", netStr, netTone},
	}
	if _, err := io.WriteString(w, "\n"); err != nil {
		return err
	}
	return writeKV(w, []kvGroup{
		{"Performance", perf},
		{"P&L breakdown", breakdown},
	}, opts.Color)
```

Note: `fmtPtr` already returns `"n/a"` for a nil pointer, so a nil metric reads `n/a` even when not mixed. `Win rate` is intentionally **not** wrapped in `na` — it survives mixed currency.

- [ ] **Step 6: Regenerate the golden and eyeball it**

Run: `go test ./internal/render -run TestPnLTable -update`
Then read `internal/render/testdata/pnl_table.golden` and confirm it matches exactly:

```
PERIOD      ACCOUNT    P&L  TRADES  WINS  LOSSES
2026-01-05  Trend EA  5.00       2     1       1
2026-01-05  ALL       5.00       2     1       1

Summary
  Performance
    Trades         2
    Win rate       50.0%
    Profit factor  2.25
    Expectancy     2.50
    Avg win        9.00
    Avg loss       -4.00
    Largest win    9.00
    Largest loss   -4.00
    Max drawdown   -4.00
    Gross profit   9.00
    Gross loss     -4.00
  P&L breakdown
    Trade profit   6.00
    Commission     -0.50
    Swap           -0.50
    Fee            0.00
    Net P&L        5.00
```

- [ ] **Step 7: Update the affected render tests**

In `render_test.go`:

`TestPnLTable` — broaden the contains-list to assert block labels:

```go
	for _, want := range []string{"PERIOD", "Trend EA", "ALL", "Summary", "Net P&L", "Max drawdown", "5.00", "-4.00", "50.0%", "2.25"} {
```

`TestPnLTableMixedNA` — replace its body assertions with:

```go
	out := buf.String()
	if !strings.Contains(out, "Net P&L        n/a") {
		t.Errorf("mixed summary should show n/a net P&L:\n%s", out)
	}
	if !strings.Contains(out, "Trade profit   n/a") {
		t.Errorf("mixed summary should suppress the P&L breakdown:\n%s", out)
	}
	if !strings.Contains(out, "Win rate       50.0%") {
		t.Errorf("mixed summary should keep the count-based win rate:\n%s", out)
	}
```

`TestPnLTableCurrencyFooter` — change the assertion to:

```go
	if !strings.Contains(buf.String(), "Net P&L        5.00 USD") {
		t.Errorf("summary should show the currency on the net line:\n%s", buf.String())
	}
```

`TestPnLTableNoCurrencyWhenUnset`, `TestPnLTableColorBySign`, `TestPnLTableBreakevenNotColoured`, `TestPnLTableNilSummaryFields` need **no change** — re-run them to confirm (no-currency still has no `USD`; colour still emits codes for the signed per-row cells and the green net; breakeven and empty summaries still emit no ANSI / still show `n/a`).

- [ ] **Step 8: Update the CLI command test**

In `cli_test.go`, in `TestPnLTableCommand`, replace `"Total P&L: 10.00"` in the contains-list with `"Net P&L"`:

```go
	for _, want := range []string{"Trend EA", "Scalper EA", "ALL", "2026-01-05", "2026-01-12", "Net P&L"} {
```

- [ ] **Step 9: Run the full suite**

Run: `go test ./...`
Expected: PASS (all packages, including `cli_test.go` and the e2e smoke test).

- [ ] **Step 10: Commit**

```bash
git add internal/render/table.go internal/render/render.go internal/render/render_test.go internal/render/table_test.go internal/render/testdata/pnl_table.golden cli_test.go
git commit -m "feat(render): replace summary line with two-group performance/breakdown block"
```

---

### Task 7: Documentation

Update README and CLAUDE.md in the same change, per the project's "update in the same change" rule and the spec's "Docs to update" list.

**Files:**
- Modify: `README.md`
- Modify: `CLAUDE.md`

- [ ] **Step 1: Update the README pnl table Demo**

In `README.md`, in the Demo section, replace the single summary line under the first `pnl` example:

```
Total P&L: 10.00 USD  Trades: 4  Win rate: 50.0%  Profit factor: 3.50  Gross profit: 14.00  Gross loss: -4.00
```

with the block:

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
    Net P&L        10.00 USD
```

- [ ] **Step 2: Update the README JSON Demo**

Replace the JSON Demo block (the `--by month --accounts "Trend EA" --format json` output) with one carrying the new fields:

```json
{
  "rows": [
    {
      "period": "2026-01-01",
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
      "period": "2026-01-01",
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

- [ ] **Step 3: Update the README CSV Demo**

Replace the CSV Demo block (the `--format csv` output) with the component columns:

```
period,account_login,account_label,pnl,trade_profit,commission,swap,fee,trades,wins,losses,gross_profit,gross_loss
2026-01-01,111,Trend EA,10.00,13.00,-2.00,-1.00,0.00,3,2,1,14.00,-4.00
2026-01-01,,ALL,10.00,13.00,-2.00,-1.00,0.00,3,2,1,14.00,-4.00
```

- [ ] **Step 4: Document the breakdown, summary and drawdown under the `pnl` command**

In `README.md`, in the Commands section under `pnl`, update the `--format` bullet to mention the component columns and the summary block, and add a drawdown bullet. Replace the existing `--format table|json|csv` bullet with:

```markdown
  - `--format table|json|csv` (default `table`). The table footer is a
    **Summary** block in two groups — *Performance* (trades, win rate,
    profit factor, expectancy, average and largest win/loss, max drawdown,
    gross profit/loss) and *P&L breakdown* (trade profit, commission, swap,
    fee, and the net). JSON carries the same fields per row and in the
    summary; CSV is header + rows only (no summary), with the component
    columns `pnl,trade_profit,commission,swap,fee` so a `--by month` export
    drops straight into a spreadsheet or tax register. The summary footer
    shows the account currency when all in-scope accounts share one
    (e.g. `Net P&L  10.00 USD`).
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
```

- [ ] **Step 5: Update CLAUDE.md**

In `CLAUDE.md`, update the `internal/aggregate` architecture bullet to mention the components and metrics. Replace:

```
- `internal/aggregate` — deals → period rows + summary. Full-precision
  sums; rounding happens in render only. Breakeven (net == 0) is neither
  win nor loss.
```

with:

```
- `internal/aggregate` — deals → period rows + summary. Each row and the
  summary carry net P&L plus its four components (trade_profit / commission
  / swap / fee, summing to net). The summary also carries expectancy,
  average and largest win/loss, and max drawdown (a deal-ordered
  realised-P&L pass, not equity drawdown). Full-precision sums; rounding
  happens in render only. Breakeven (net == 0) is neither win nor loss.
```

Then add a bullet to the Gotchas section:

```
- **Summary block is table/JSON only.** The two-group performance/breakdown
  summary appears in `--format table` (an aligned key/value block) and
  `--format json`; CSV is rows-only by design. Max drawdown is realised-P&L
  drawdown over the ordered in-scope deals, deliberately distinct from
  broker equity drawdown.
```

- [ ] **Step 6: Run the full suite**

Run: `go test ./...`
Expected: PASS (docs-only change, but confirm nothing regressed).

- [ ] **Step 7: Commit**

```bash
git add README.md CLAUDE.md
git commit -m "docs: document P&L component breakdown, richer summary and drawdown"
```

---

## After all tasks

Dispatch a final whole-branch code review (subagent-driven-development's final step), then use `superpowers:finishing-a-development-branch`. Phase 3 (`--by symbol|magic`, the `group`/`group_by` rename, and the `positions`/`trades`/`cash-flows` commands) remains before the single release.
