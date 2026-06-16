// Package render prints aggregate results as fixed-width tables or JSON.
// All rounding to display precision happens here, not in aggregate.
package render

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"strconv"

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

func numPtr(x float64) *float64 { return &x }

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

func strOr(p *string, fallback string) string {
	if p == nil {
		return fallback
	}
	return *p
}

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

func AccountsJSON(w io.Writer, accounts []snapshot.AccountSnapshot) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(accounts)
}

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
