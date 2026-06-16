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
	{Period: "2026-01-05", Account: ptr(int64(111)), PnL: 5.004, Trades: 2, Wins: 1, Losses: 1, GrossProfit: 9.0, GrossLoss: -3.996},
	{Period: "2026-01-05", Account: nil, PnL: 5.004, Trades: 2, Wins: 1, Losses: 1, GrossProfit: 9.0, GrossLoss: -3.996},
}

var sum = aggregate.Summary{
	TotalPnL: 5.004, TotalTrades: 2,
	WinRatePct: ptr(50.0), ProfitFactor: ptr(2.2522522522522523),
	GrossProfit: 9.0, GrossLoss: -3.996,
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
    "gross_profit": 9,
    "gross_loss": -4
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
