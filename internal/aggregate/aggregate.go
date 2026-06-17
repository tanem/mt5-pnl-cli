// Package aggregate turns raw closed deals into per-period P&L rows.
//
// The exporter pre-filters closed_deals to closing trades, so every deal
// here counts. Sums accumulate at full float64 precision; rounding happens
// at render time only. A breakeven deal (net == 0) counts toward trades but
// is neither a win nor a loss.
package aggregate

import (
	"cmp"
	"math"
	"slices"
	"time"

	"github.com/tanem/mt5-pnl-cli/internal/snapshot"
)

type Options struct {
	From, To time.Time      // inclusive civil dates at UTC midnight
	By       string         // "day", "week" (Monday-start) or "month"
	Accounts map[int64]bool // nil = all accounts
}

// Row is one group × account bucket. Group holds the period date (time
// cuts) or the symbol/magic key (dimension cuts). Account == nil is the
// combined row across all accounts for a time period, and is also nil for
// every symbol/magic row (those have no per-account dimension).
type Row struct {
	Group       string
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

type Summary struct {
	TotalPnL     float64
	TotalTrades  int
	WinRatePct   *float64 // nil when no trades
	ProfitFactor *float64 // nil when no gross loss
	Expectancy   *float64 // TotalPnL / TotalTrades; nil when no trades
	AvgWin       *float64 // GrossProfit / wins; nil when no wins
	AvgLoss      *float64 // GrossLoss / losses (negative); nil when no losses
	LargestWin   *float64 // max single-deal net among wins; nil when no wins
	LargestLoss  *float64 // min single-deal net among losses (negative); nil when no losses
	MaxDrawdown  *float64 // realised-P&L drawdown (<= 0); nil when no deals; 0 when never retraces
	GrossProfit  float64
	GrossLoss    float64
	TradeProfit  float64
	Commission   float64
	Swap         float64
	Fee          float64
}

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
	if sum.TotalTrades > 0 {
		wr := float64(totalWins) / float64(sum.TotalTrades) * 100
		sum.WinRatePct = &wr
		e := sum.TotalPnL / float64(sum.TotalTrades)
		sum.Expectancy = &e
	}
	if sum.GrossLoss != 0 {
		pf := sum.GrossProfit / math.Abs(sum.GrossLoss)
		sum.ProfitFactor = &pf
	}
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
	return rows, sum
}

func civilDay(unix int64) time.Time {
	t := time.Unix(unix, 0).UTC()
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
}

func periodKey(day time.Time, by string) string {
	switch by {
	case "day":
		return day.Format("2006-01-02")
	case "week":
		monday := day.AddDate(0, 0, -int((day.Weekday()+6)%7))
		return monday.Format("2006-01-02")
	default: // month
		return time.Date(day.Year(), day.Month(), 1, 0, 0, 0, 0, time.UTC).Format("2006-01-02")
	}
}
