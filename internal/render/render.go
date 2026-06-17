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
}

type pnlRow struct {
	Group       string   `json:"group"`
	GroupBy     string   `json:"group_by"`
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

// PnLJSON emits the rows and summary as JSON. Under mixed currency the
// combined (account == nil) rows and the summary have their currency-valued
// fields set to null (they would sum across currencies); counts and the
// count-based win rate are kept.
func PnLJSON(w io.Writer, rows []aggregate.Row, sum aggregate.Summary, groupBy string, mixed bool) error {
	out := struct {
		Rows    []pnlRow   `json:"rows"`
		Summary pnlSummary `json:"summary"`
	}{Rows: make([]pnlRow, 0, len(rows))}
	for _, r := range rows {
		row := pnlRow{
			Group: r.Group, GroupBy: groupBy, Account: r.Account,
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
		out.Rows = append(out.Rows, row)
	}
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

// PnLCSV writes rows as CSV (header + data rows, no summary). groupBy is
// written into every row's group_by column (e.g. "week", "symbol",
// "magic"). For dimension cuts (symbol/magic) both account columns are
// empty. Under mixed currency the combined ALL rows for time cuts are
// omitted, since they would sum across currencies; per-account rows (each
// single-currency) still print.
func PnLCSV(w io.Writer, rows []aggregate.Row, labels map[int64]string, groupBy string, mixed bool) error {
	dimension := groupBy == "symbol" || groupBy == "magic"
	cw := csv.NewWriter(w)
	if err := cw.Write([]string{
		"group", "group_by", "account_login", "account_label",
		"pnl", "trade_profit", "commission", "swap", "fee",
		"trades", "wins", "losses", "gross_profit", "gross_loss",
	}); err != nil {
		return err
	}
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
