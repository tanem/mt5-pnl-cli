package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/tanem/mt5-pnl-cli/internal/secrets"
	"github.com/tanem/mt5-pnl-cli/internal/snaptest"
)

// Two accounts, four deals (same data as the aggregate tests; week of
// 2026-01-05 plus one deal the following week).
const fixtureJSON = `{
  "schema_version": "1.0",
  "generated_at": "2026-06-13T00:00:00Z",
  "accounts": [
    {"login": 111, "label": "Trend EA", "currency": "USD", "balance": 1000.0,
     "equity": 1010.5, "last_success_at": "2026-06-13T00:00:00Z", "last_error": null},
    {"login": 222, "label": "Scalper EA", "currency": "USD", "balance": 500.0,
     "equity": 500.0, "last_success_at": null, "last_error": "login failed"}
  ],
  "closed_deals": [
    {"account": 111, "time": 1767607200, "profit": 10.0, "swap": -0.5, "commission": -0.5, "fee": 0.0,
     "ticket": 1, "order": 1, "position_id": 1, "time_msc": 0, "type": 0, "entry": 1, "reason": 0,
     "magic": 0, "volume": 0.1, "price": 1.0, "symbol": "EURUSD", "comment": "", "external_id": ""},
    {"account": 111, "time": 1767693600, "profit": -4.0, "swap": 0.0, "commission": 0.0, "fee": 0.0,
     "ticket": 2, "order": 2, "position_id": 2, "time_msc": 0, "type": 1, "entry": 1, "reason": 0,
     "magic": 0, "volume": 0.1, "price": 1.0, "symbol": "EURUSD", "comment": "", "external_id": ""},
    {"account": 222, "time": 1767693600, "profit": 0.7, "swap": 0.0, "commission": -0.7, "fee": 0.0,
     "ticket": 3, "order": 3, "position_id": 3, "time_msc": 0, "type": 0, "entry": 1, "reason": 0,
     "magic": 0, "volume": 0.1, "price": 1.0, "symbol": "XAUUSD", "comment": "", "external_id": ""},
    {"account": 111, "time": 1768212000, "profit": 5.0, "swap": 0.0, "commission": 0.0, "fee": 0.0,
     "ticket": 4, "order": 4, "position_id": 4, "time_msc": 0, "type": 0, "entry": 1, "reason": 0,
     "magic": 0, "volume": 0.1, "price": 1.0, "symbol": "EURUSD", "comment": "", "external_id": ""}
  ],
  "open_positions": [],
  "cash_flows": []
}`

func runCLI(t *testing.T, passphrase string, args ...string) (stdout, stderr string, code int) {
	t.Helper()
	var out, errBuf bytes.Buffer
	code = run(args, &out, &errBuf, func() (string, error) { return passphrase, nil })
	return out.String(), errBuf.String(), code
}

func fixture(t *testing.T) string {
	t.Helper()
	return snaptest.Write(t, fixtureJSON, "test-pass")
}

func TestPnLTableCommand(t *testing.T) {
	path := fixture(t)
	out, errOut, code := runCLI(t, "test-pass",
		"pnl", "--snapshot", path, "--from", "2026-01-01", "--to", "2026-01-31",
		"--stale-after", "876000h")
	if code != 0 {
		t.Fatalf("exit %d, stderr: %s", code, errOut)
	}
	for _, want := range []string{"Trend EA", "Scalper EA", "ALL", "2026-01-05", "2026-01-12", "Total P&L: 10.00"} {
		if !strings.Contains(out, want) {
			t.Errorf("stdout missing %q:\n%s", want, out)
		}
	}
	if errOut != "" {
		t.Errorf("expected silent stderr with huge --stale-after, got %q", errOut)
	}
}

func TestPnLJSONCommand(t *testing.T) {
	path := fixture(t)
	out, _, code := runCLI(t, "test-pass",
		"pnl", "--snapshot", path, "--from", "2026-01-01", "--to", "2026-01-31", "--json")
	if code != 0 {
		t.Fatalf("exit %d", code)
	}
	var got struct {
		Rows    []map[string]any `json:"rows"`
		Summary struct {
			TotalPnL    float64 `json:"total_pnl"`
			TotalTrades int     `json:"total_trades"`
		} `json:"summary"`
	}
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, out)
	}
	if got.Summary.TotalPnL != 10.0 || got.Summary.TotalTrades != 4 {
		t.Errorf("summary = %+v", got.Summary)
	}
}

func TestPnLAccountsFilter(t *testing.T) {
	path := fixture(t)
	out, _, code := runCLI(t, "test-pass",
		"pnl", "--snapshot", path, "--from", "2026-01-01", "--to", "2026-01-31",
		"--accounts", "scalper ea")
	if code != 0 {
		t.Fatalf("exit %d", code)
	}
	if strings.Contains(out, "Trend EA") {
		t.Errorf("filter leaked other account:\n%s", out)
	}
}

func TestPnLUnknownAccountLabel(t *testing.T) {
	path := fixture(t)
	_, errOut, code := runCLI(t, "test-pass",
		"pnl", "--snapshot", path, "--accounts", "Nope")
	if code != 1 || !strings.Contains(errOut, "Trend EA") {
		t.Errorf("exit %d, stderr %q; want 1 + valid labels listed", code, errOut)
	}
}

func TestPnLStalenessWarning(t *testing.T) {
	path := fixture(t)
	// --stale-after 1ns makes any snapshot stale, so the test never depends
	// on the wall clock's distance from the fixture's generated_at.
	_, errOut, code := runCLI(t, "test-pass",
		"pnl", "--snapshot", path, "--from", "2026-01-01", "--to", "2026-01-31",
		"--stale-after", "1ns")
	if code != 0 {
		t.Fatalf("exit %d", code)
	}
	if !strings.Contains(errOut, "mt5-pnl-exporter export") {
		t.Errorf("want staleness warning on stderr, got %q", errOut)
	}
}

func TestPnLInvalidBy(t *testing.T) {
	_, errOut, code := runCLI(t, "test-pass", "pnl", "--by", "fortnight")
	if code != 1 || !strings.Contains(errOut, "--by") {
		t.Errorf("exit %d, stderr %q", code, errOut)
	}
}

func TestPnLWrongPassphrase(t *testing.T) {
	path := fixture(t)
	_, errOut, code := runCLI(t, "wrong", "pnl", "--snapshot", path)
	if code != 1 || !strings.Contains(errOut, "wrong passphrase") {
		t.Errorf("exit %d, stderr %q", code, errOut)
	}
}

func TestPnLPassphraseMissing(t *testing.T) {
	path := fixture(t)
	var out, errBuf bytes.Buffer
	code := run([]string{"pnl", "--snapshot", path}, &out, &errBuf,
		func() (string, error) { return "", secrets.ErrNotFound })
	if code != 1 || !strings.Contains(errBuf.String(), "set-passphrase") {
		t.Errorf("exit %d, stderr %q", code, errBuf.String())
	}
}

func TestPnLNoSnapshotPath(t *testing.T) {
	t.Setenv("MT5_PNL_SNAPSHOT", "")
	_, errOut, code := runCLI(t, "test-pass", "pnl")
	if code != 1 || !strings.Contains(errOut, "MT5_PNL_SNAPSHOT") {
		t.Errorf("exit %d, stderr %q", code, errOut)
	}
}

func TestPnLEnvFallback(t *testing.T) {
	path := fixture(t)
	t.Setenv("MT5_PNL_SNAPSHOT", path)
	_, errOut, code := runCLI(t, "test-pass",
		"pnl", "--from", "2026-01-01", "--to", "2026-01-31", "--stale-after", "876000h")
	if code != 0 {
		t.Fatalf("exit %d, stderr %q", code, errOut)
	}
}

func TestUnsupportedSchema(t *testing.T) {
	body := strings.Replace(fixtureJSON, `"schema_version": "1.0"`, `"schema_version": "2.0"`, 1)
	path := snaptest.Write(t, body, "test-pass")
	_, errOut, code := runCLI(t, "test-pass", "pnl", "--snapshot", path)
	if code != 1 || !strings.Contains(errOut, "unsupported snapshot schema") {
		t.Errorf("exit %d, stderr %q", code, errOut)
	}
}

func TestAccountsCommand(t *testing.T) {
	path := fixture(t)
	out, _, code := runCLI(t, "test-pass",
		"accounts", "--snapshot", path, "--stale-after", "876000h")
	if code != 0 {
		t.Fatalf("exit %d", code)
	}
	for _, want := range []string{"111", "Trend EA", "login failed", "Snapshot generated: 2026-06-13T00:00:00Z"} {
		if !strings.Contains(out, want) {
			t.Errorf("stdout missing %q:\n%s", want, out)
		}
	}
}

func TestAccountsMissingSnapshotFile(t *testing.T) {
	_, errOut, code := runCLI(t, "test-pass", "accounts", "--snapshot", "/nonexistent/snap.age")
	if code != 1 || errOut == "" {
		t.Errorf("exit %d, stderr %q; want 1 + error", code, errOut)
	}
}

func TestVersionCommand(t *testing.T) {
	out, _, code := runCLI(t, "", "version")
	if code != 0 || !strings.Contains(out, "mt5-pnl-cli") || !strings.Contains(out, "schema 1.0") {
		t.Errorf("exit %d, out %q", code, out)
	}
}

func TestVersionFlag(t *testing.T) {
	out, _, code := runCLI(t, "", "--version")
	if code != 0 || !strings.Contains(out, "mt5-pnl-cli") || !strings.Contains(out, "schema 1.0") {
		t.Errorf("exit %d, out %q", code, out)
	}
}

func TestUnknownCommand(t *testing.T) {
	_, errOut, code := runCLI(t, "", "bogus")
	if code != 1 || !strings.Contains(errOut, "Usage") {
		t.Errorf("exit %d, stderr %q", code, errOut)
	}
}

func TestNoCommand(t *testing.T) {
	_, errOut, code := runCLI(t, "")
	if code != 1 || !strings.Contains(errOut, "Usage") {
		t.Errorf("exit %d, stderr %q", code, errOut)
	}
}

func TestPnLHelpToStdout(t *testing.T) {
	out, errOut, code := runCLI(t, "", "pnl", "-h")
	if code != 0 {
		t.Fatalf("exit %d", code)
	}
	if !strings.Contains(out, "Usage: mt5-pnl-cli pnl") {
		t.Errorf("help should be on stdout, got %q", out)
	}
	if errOut != "" {
		t.Errorf("help should not write stderr, got %q", errOut)
	}
}

func TestPnLParseErrorToStderr(t *testing.T) {
	out, errOut, code := runCLI(t, "", "pnl", "--nope")
	if code != 1 {
		t.Fatalf("exit %d", code)
	}
	if out != "" {
		t.Errorf("parse error should not write stdout, got %q", out)
	}
	if !strings.Contains(errOut, "not defined") {
		t.Errorf("want flag error on stderr, got %q", errOut)
	}
}

func TestAccountsHelpToStdout(t *testing.T) {
	out, _, code := runCLI(t, "", "accounts", "--help")
	if code != 0 || !strings.Contains(out, "Usage: mt5-pnl-cli accounts") {
		t.Errorf("exit %d, out %q", code, out)
	}
}

func TestTopLevelHelpMentionsFormat(t *testing.T) {
	out, _, code := runCLI(t, "", "help")
	if code != 0 {
		t.Fatalf("exit %d", code)
	}
	if !strings.Contains(out, "--format") {
		t.Errorf("top-level help should mention --format:\n%s", out)
	}
}
