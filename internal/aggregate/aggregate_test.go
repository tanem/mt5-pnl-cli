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
//	acct 222, Tue 2026-01-06: net  0.0 (0.7 - 0.7)          -> breakeven: neither
//	acct 111, Mon 2026-01-12: net  5.0                      -> win, next week
var deals = []snapshot.Deal{
	deal(111, 1767607200, 10.0, -0.5, -0.5, 0),
	deal(111, 1767693600, -4.0, 0, 0, 0),
	deal(222, 1767693600, 0.7, 0, -0.7, 0),
	deal(111, 1768212000, 5.0, 0, 0, 0),
}

func TestAggregateByWeek(t *testing.T) {
	rows, sum := aggregate.Aggregate(deals, aggregate.Options{
		From: date(2026, 1, 1), To: date(2026, 1, 31), By: "week",
	})
	want := []aggregate.Row{
		{Period: "2026-01-05", Account: ptr(int64(111)), PnL: 5.0, Trades: 2, Wins: 1, Losses: 1, GrossProfit: 9.0, GrossLoss: -4.0},
		{Period: "2026-01-05", Account: ptr(int64(222)), PnL: 0.0, Trades: 1, Wins: 0, Losses: 0, GrossProfit: 0, GrossLoss: 0},
		{Period: "2026-01-05", Account: nil, PnL: 5.0, Trades: 3, Wins: 1, Losses: 1, GrossProfit: 9.0, GrossLoss: -4.0},
		{Period: "2026-01-12", Account: ptr(int64(111)), PnL: 5.0, Trades: 1, Wins: 1, Losses: 0, GrossProfit: 5.0, GrossLoss: 0},
		{Period: "2026-01-12", Account: nil, PnL: 5.0, Trades: 1, Wins: 1, Losses: 0, GrossProfit: 5.0, GrossLoss: 0},
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
	if rows[0].Period != "2026-01-06" {
		t.Errorf("period = %q, want 2026-01-06", rows[0].Period)
	}
}

func TestAggregateByMonth(t *testing.T) {
	withFeb := append(append([]snapshot.Deal{}, deals...), deal(111, 1770026400, 2.0, 0, 0, 0))
	rows, _ := aggregate.Aggregate(withFeb, aggregate.Options{
		From: date(2026, 1, 1), To: date(2026, 2, 28), By: "month",
	})
	periods := map[string]bool{}
	for _, r := range rows {
		periods[r.Period] = true
	}
	if !periods["2026-01-01"] || !periods["2026-02-01"] || len(periods) != 2 {
		t.Errorf("periods = %v, want 2026-01-01 and 2026-02-01", periods)
	}
}

func TestWeekBoundary(t *testing.T) {
	// Sunday 23:59:59 belongs to the week starting the previous Monday.
	rows, _ := aggregate.Aggregate([]snapshot.Deal{deal(111, 1768175999, 1.0, 0, 0, 0)},
		aggregate.Options{From: date(2026, 1, 1), To: date(2026, 1, 31), By: "week"})
	if rows[0].Period != "2026-01-05" {
		t.Errorf("period = %q, want 2026-01-05", rows[0].Period)
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
}
