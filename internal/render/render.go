// Package render prints aggregate results as tabwriter tables or JSON.
// All rounding to display precision happens here, not in aggregate.
package render

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
	"strconv"
	"text/tabwriter"

	"github.com/tanem/mt5-pnl-cli/internal/aggregate"
	"github.com/tanem/mt5-pnl-cli/internal/snapshot"
)

func round(x float64, places int) float64 {
	scale := math.Pow(10, float64(places))
	return math.Round(x*scale) / scale
}

func roundPtr(p *float64, places int) *float64 {
	if p == nil {
		return nil
	}
	r := round(*p, places)
	return &r
}

func fmtPtr(p *float64, format string) string {
	if p == nil {
		return "n/a"
	}
	return fmt.Sprintf(format, *p)
}

func PnLTable(w io.Writer, rows []aggregate.Row, sum aggregate.Summary, labels map[int64]string) error {
	tw := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
	fmt.Fprintln(tw, "PERIOD\tACCOUNT\tP&L\tTRADES\tWINS\tLOSSES")
	for _, r := range rows {
		acct := "ALL"
		if r.Account != nil {
			acct = labels[*r.Account]
			if acct == "" {
				acct = strconv.FormatInt(*r.Account, 10)
			}
		}
		fmt.Fprintf(tw, "%s\t%s\t%.2f\t%d\t%d\t%d\n", r.Period, acct, r.PnL, r.Trades, r.Wins, r.Losses)
	}
	if err := tw.Flush(); err != nil {
		return err
	}
	_, err := fmt.Fprintf(w,
		"\nTotal P&L: %.2f  Trades: %d  Win rate: %s  Profit factor: %s  Gross profit: %.2f  Gross loss: %.2f\n",
		sum.TotalPnL, sum.TotalTrades,
		fmtPtr(sum.WinRatePct, "%.1f%%"), fmtPtr(sum.ProfitFactor, "%.2f"),
		sum.GrossProfit, sum.GrossLoss)
	return err
}

type pnlRow struct {
	Period      string  `json:"period"`
	Account     *int64  `json:"account"`
	PnL         float64 `json:"pnl"`
	Trades      int     `json:"trades"`
	Wins        int     `json:"wins"`
	Losses      int     `json:"losses"`
	GrossProfit float64 `json:"gross_profit"`
	GrossLoss   float64 `json:"gross_loss"`
}

type pnlSummary struct {
	TotalPnL     float64  `json:"total_pnl"`
	TotalTrades  int      `json:"total_trades"`
	WinRatePct   *float64 `json:"win_rate_pct"`
	ProfitFactor *float64 `json:"profit_factor"`
	GrossProfit  float64  `json:"gross_profit"`
	GrossLoss    float64  `json:"gross_loss"`
}

func PnLJSON(w io.Writer, rows []aggregate.Row, sum aggregate.Summary) error {
	out := struct {
		Rows    []pnlRow   `json:"rows"`
		Summary pnlSummary `json:"summary"`
	}{Rows: make([]pnlRow, 0, len(rows))}
	for _, r := range rows {
		out.Rows = append(out.Rows, pnlRow{
			Period: r.Period, Account: r.Account,
			PnL: round(r.PnL, 2), Trades: r.Trades, Wins: r.Wins, Losses: r.Losses,
			GrossProfit: round(r.GrossProfit, 2), GrossLoss: round(r.GrossLoss, 2),
		})
	}
	out.Summary = pnlSummary{
		TotalPnL: round(sum.TotalPnL, 2), TotalTrades: sum.TotalTrades,
		WinRatePct: roundPtr(sum.WinRatePct, 1), ProfitFactor: roundPtr(sum.ProfitFactor, 2),
		GrossProfit: round(sum.GrossProfit, 2), GrossLoss: round(sum.GrossLoss, 2),
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}

func strOr(p *string, fallback string) string {
	if p == nil {
		return fallback
	}
	return *p
}

func AccountsTable(w io.Writer, accounts []snapshot.AccountSnapshot, generatedAt string) error {
	tw := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
	fmt.Fprintln(tw, "LOGIN\tLABEL\tCURRENCY\tBALANCE\tEQUITY\tLAST SUCCESS\tLAST ERROR")
	for _, a := range accounts {
		fmt.Fprintf(tw, "%d\t%s\t%s\t%.2f\t%.2f\t%s\t%s\n",
			a.Login, a.Label, a.Currency, a.Balance, a.Equity,
			strOr(a.LastSuccessAt, "-"), strOr(a.LastError, "-"))
	}
	if err := tw.Flush(); err != nil {
		return err
	}
	_, err := fmt.Fprintf(w, "\nSnapshot generated: %s\n", generatedAt)
	return err
}

func AccountsJSON(w io.Writer, accounts []snapshot.AccountSnapshot) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(accounts)
}
