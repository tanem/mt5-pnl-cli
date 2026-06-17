package aggregate_test

import (
	"reflect"
	"testing"
	"time"

	"github.com/tanem/mt5-pnl-cli/internal/aggregate"
	"github.com/tanem/mt5-pnl-cli/internal/snapshot"
)

func deal(account, ts int64, profit, swap, commission, fee float64) snapshot.Deal {
	return snapshot.Deal{Account: account, Time: ts, Profit: profit, Swap: swap, Commission: commission, Fee: fee}
}

func date(y int, m time.Month, d int) time.Time {
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

func ptr[T any](v T) *T { return &v }

// Four deals across two accounts and two ISO weeks:
//
//	acct 111, Mon 2026-01-05: net  9.0 (10.0 - 0.5 - 0.5)  -> win
//	acct 111, Tue 2026-01-06: net -4.0                      -> loss
//	acct 222, Tue 2026-01-06: net  0.0 (0.5 - 0.5)          -> breakeven: neither
//	acct 111, Mon 2026-01-12: net  5.0                      -> win, next week
var deals = []snapshot.Deal{
	deal(111, 1767607200, 10.0, -0.5, -0.5, 0),
	deal(111, 1767693600, -4.0, 0, 0, 0),
	deal(222, 1767693600, 0.5, 0, -0.5, 0),
	deal(111, 1768212000, 5.0, 0, 0, 0),
}

func TestAggregateByWeek(t *testing.T) {
	rows, sum := aggregate.Aggregate(deals, aggregate.Options{
		From: date(2026, 1, 1), To: date(2026, 1, 31), By: "week",
	})
	want := []aggregate.Row{
		{Group: "2026-01-05", Account: ptr(int64(111)), PnL: 5.0, TradeProfit: 6.0, Commission: -0.5, Swap: -0.5, Fee: 0, Trades: 2, Wins: 1, Losses: 1, GrossProfit: 9.0, GrossLoss: -4.0},
		{Group: "2026-01-05", Account: ptr(int64(222)), PnL: 0.0, TradeProfit: 0.5, Commission: -0.5, Swap: 0, Fee: 0, Trades: 1, Wins: 0, Losses: 0, GrossProfit: 0, GrossLoss: 0},
		{Group: "2026-01-05", Account: nil, PnL: 5.0, TradeProfit: 6.5, Commission: -1.0, Swap: -0.5, Fee: 0, Trades: 3, Wins: 1, Losses: 1, GrossProfit: 9.0, GrossLoss: -4.0},
		{Group: "2026-01-12", Account: ptr(int64(111)), PnL: 5.0, TradeProfit: 5.0, Commission: 0, Swap: 0, Fee: 0, Trades: 1, Wins: 1, Losses: 0, GrossProfit: 5.0, GrossLoss: 0},
		{Group: "2026-01-12", Account: nil, PnL: 5.0, TradeProfit: 5.0, Commission: 0, Swap: 0, Fee: 0, Trades: 1, Wins: 1, Losses: 0, GrossProfit: 5.0, GrossLoss: 0},
	}
	if !reflect.DeepEqual(rows, want) {
		t.Errorf("rows:\n got %+v\nwant %+v", rows, want)
	}
	if sum.TotalPnL != 10.0 || sum.TotalTrades != 4 || sum.GrossProfit != 14.0 || sum.GrossLoss != -4.0 {
		t.Errorf("summary totals wrong: %+v", sum)
	}
	if sum.WinRatePct == nil || *sum.WinRatePct != 50.0 {
		t.Errorf("win rate = %v, want 50.0", sum.WinRatePct)
	}
	if sum.ProfitFactor == nil || *sum.ProfitFactor != 3.5 {
		t.Errorf("profit factor = %v, want 3.5", sum.ProfitFactor)
	}
}

func TestAggregateByDayWithDateFilter(t *testing.T) {
	rows, _ := aggregate.Aggregate(deals, aggregate.Options{
		From: date(2026, 1, 6), To: date(2026, 1, 6), By: "day",
	})
	if len(rows) != 3 { // acct 111, acct 222, combined
		t.Fatalf("got %d rows, want 3: %+v", len(rows), rows)
	}
	if rows[0].Group != "2026-01-06" {
		t.Errorf("period = %q, want 2026-01-06", rows[0].Group)
	}
}

func TestAggregateByMonth(t *testing.T) {
	withFeb := append(append([]snapshot.Deal{}, deals...), deal(111, 1770026400, 2.0, 0, 0, 0))
	rows, _ := aggregate.Aggregate(withFeb, aggregate.Options{
		From: date(2026, 1, 1), To: date(2026, 2, 28), By: "month",
	})
	periods := map[string]bool{}
	for _, r := range rows {
		periods[r.Group] = true
	}
	if !periods["2026-01-01"] || !periods["2026-02-01"] || len(periods) != 2 {
		t.Errorf("periods = %v, want 2026-01-01 and 2026-02-01", periods)
	}
}

func TestWeekBoundary(t *testing.T) {
	// Sunday 23:59:59 belongs to the week starting the previous Monday.
	rows, _ := aggregate.Aggregate([]snapshot.Deal{deal(111, 1768175999, 1.0, 0, 0, 0)},
		aggregate.Options{From: date(2026, 1, 1), To: date(2026, 1, 31), By: "week"})
	if rows[0].Group != "2026-01-05" {
		t.Errorf("period = %q, want 2026-01-05", rows[0].Group)
	}
}

func TestAccountFilter(t *testing.T) {
	rows, sum := aggregate.Aggregate(deals, aggregate.Options{
		From: date(2026, 1, 1), To: date(2026, 1, 31), By: "week",
		Accounts: map[int64]bool{222: true},
	})
	if len(rows) != 2 { // acct 222 row + combined
		t.Fatalf("got %d rows, want 2: %+v", len(rows), rows)
	}
	if sum.TotalTrades != 1 {
		t.Errorf("trades = %d, want 1", sum.TotalTrades)
	}
}

func TestEmpty(t *testing.T) {
	rows, sum := aggregate.Aggregate(nil, aggregate.Options{
		From: date(2026, 1, 1), To: date(2026, 1, 31), By: "week",
	})
	if len(rows) != 0 || sum.TotalTrades != 0 {
		t.Errorf("want empty result, got rows=%v sum=%+v", rows, sum)
	}
	if sum.WinRatePct != nil || sum.ProfitFactor != nil {
		t.Errorf("want nil win rate and profit factor, got %+v", sum)
	}
	if sum.Expectancy != nil || sum.AvgWin != nil || sum.AvgLoss != nil ||
		sum.LargestWin != nil || sum.LargestLoss != nil || sum.MaxDrawdown != nil {
		t.Errorf("want nil metrics on empty input, got %+v", sum)
	}
}

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
