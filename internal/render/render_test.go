package render_test

import (
	"bytes"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tanem/mt5-pnl-cli/internal/aggregate"
	"github.com/tanem/mt5-pnl-cli/internal/render"
	"github.com/tanem/mt5-pnl-cli/internal/snapshot"
)

var update = flag.Bool("update", false, "rewrite golden files")

func ptr[T any](v T) *T { return &v }

var rows = []aggregate.Row{
	{Period: "2026-01-05", Account: ptr(int64(111)), PnL: 5.004, TradeProfit: 6.0, Commission: -0.5, Swap: -0.496, Fee: 0, Trades: 2, Wins: 1, Losses: 1, GrossProfit: 9.0, GrossLoss: -3.996},
	{Period: "2026-01-05", Account: nil, PnL: 5.004, TradeProfit: 6.0, Commission: -0.5, Swap: -0.496, Fee: 0, Trades: 2, Wins: 1, Losses: 1, GrossProfit: 9.0, GrossLoss: -3.996},
}

var sum = aggregate.Summary{
	TotalPnL: 5.004, TotalTrades: 2,
	WinRatePct: ptr(50.0), ProfitFactor: ptr(2.2522522522522523),
	GrossProfit: 9.0, GrossLoss: -3.996,
	Expectancy: ptr(2.502), AvgWin: ptr(9.0), AvgLoss: ptr(-3.996),
	LargestWin: ptr(9.0), LargestLoss: ptr(-3.996), MaxDrawdown: ptr(-3.996),
	TradeProfit: 6.0, Commission: -0.5, Swap: -0.496, Fee: 0,
}

var labels = map[int64]string{111: "Trend EA"}

func checkGolden(t *testing.T, name string, got []byte) {
	t.Helper()
	golden := filepath.Join("testdata", name)
	if *update {
		if err := os.WriteFile(golden, got, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(golden)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("output mismatch:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

func TestPnLTable(t *testing.T) {
	var buf bytes.Buffer
	if err := render.PnLTable(&buf, rows, sum, labels, false, render.TableOpts{}); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	for _, want := range []string{"PERIOD", "Trend EA", "ALL", "5.00", "-4.00", "50.0%", "2.25"} {
		if !bytes.Contains([]byte(out), []byte(want)) {
			t.Errorf("table missing %q:\n%s", want, out)
		}
	}
	checkGolden(t, "pnl_table.golden", buf.Bytes())
}

func TestPnLTableUnknownLabelFallsBackToLogin(t *testing.T) {
	var buf bytes.Buffer
	if err := render.PnLTable(&buf, rows, sum, nil, false, render.TableOpts{}); err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(buf.Bytes(), []byte("111")) {
		t.Errorf("expected login fallback in:\n%s", buf.String())
	}
}

func TestPnLTableNilSummaryFields(t *testing.T) {
	var buf bytes.Buffer
	if err := render.PnLTable(&buf, nil, aggregate.Summary{}, nil, false, render.TableOpts{}); err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(buf.Bytes(), []byte("n/a")) {
		t.Errorf("expected n/a for nil win rate / profit factor:\n%s", buf.String())
	}
}

func TestPnLJSON(t *testing.T) {
	var buf bytes.Buffer
	if err := render.PnLJSON(&buf, rows, sum, false); err != nil {
		t.Fatal(err)
	}
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
	if buf.String() != want {
		t.Errorf("JSON mismatch:\ngot:\n%s\nwant:\n%s", buf.String(), want)
	}
}

var accounts = []snapshot.AccountSnapshot{
	{Login: 111, Label: "Trend EA", Currency: "USD", Balance: 1000, Equity: 1010.5,
		LastSuccessAt: ptr("2026-06-13T00:00:00Z"), LastError: nil},
	{Login: 222, Label: "Scalper EA", Currency: "USD", Balance: 500, Equity: 500,
		LastSuccessAt: nil, LastError: ptr("login failed")},
}

func TestAccountsTable(t *testing.T) {
	var buf bytes.Buffer
	if err := render.AccountsTable(&buf, accounts, "2026-06-13T00:00:00Z", render.TableOpts{}); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	for _, want := range []string{"LOGIN", "Trend EA", "login failed", "Snapshot generated: 2026-06-13T00:00:00Z"} {
		if !bytes.Contains([]byte(out), []byte(want)) {
			t.Errorf("accounts table missing %q:\n%s", want, out)
		}
	}
	checkGolden(t, "accounts_table.golden", buf.Bytes())
}

func TestAccountsJSON(t *testing.T) {
	var buf bytes.Buffer
	if err := render.AccountsJSON(&buf, accounts); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"login": 111`, `"last_error": "login failed"`, `"last_success_at": null`} {
		if !bytes.Contains(buf.Bytes(), []byte(want)) {
			t.Errorf("accounts JSON missing %q:\n%s", want, buf.String())
		}
	}
}

func TestPnLCSV(t *testing.T) {
	var buf bytes.Buffer
	if err := render.PnLCSV(&buf, rows, labels, false); err != nil {
		t.Fatal(err)
	}
	want := "period,account_login,account_label,pnl,trade_profit,commission,swap,fee,trades,wins,losses,gross_profit,gross_loss\n" +
		"2026-01-05,111,Trend EA,5.00,6.00,-0.50,-0.50,0.00,2,1,1,9.00,-4.00\n" +
		"2026-01-05,,ALL,5.00,6.00,-0.50,-0.50,0.00,2,1,1,9.00,-4.00\n"
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

func TestPnLJSONMixedNulls(t *testing.T) {
	var buf bytes.Buffer
	if err := render.PnLJSON(&buf, rows, sum, true); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
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
	if err := render.PnLTable(&buf, rows, sum, labels, true, render.TableOpts{}); err != nil {
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

func TestPnLTableBreakevenNotColoured(t *testing.T) {
	// 0.004 rounds to 0.00 on display, so it is breakeven and must not be
	// tinted even with colour enabled (tone follows the displayed value).
	be := []aggregate.Row{{Period: "2026-01-05", Account: ptr(int64(111)), PnL: 0.004, Trades: 1}}
	var buf bytes.Buffer
	if err := render.PnLTable(&buf, be, aggregate.Summary{}, labels, false, render.TableOpts{Color: true}); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(buf.String(), "\x1b[") {
		t.Errorf("breakeven (displayed 0.00) must not be coloured:\n%q", buf.String())
	}
}
