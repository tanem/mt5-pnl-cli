package snapshot_test

import (
	"os"
	"strings"
	"testing"

	"github.com/tanem/mt5-pnl-cli/internal/snapshot"
	"github.com/tanem/mt5-pnl-cli/internal/snaptest"
)

func TestCheckSchemaVersion(t *testing.T) {
	cases := []struct {
		version string
		wantErr string // "" = accepted
	}{
		{"1.0", ""},
		{"1.1", "unsupported"}, // additive minor newer than this build
		{"0.9", "unsupported"},
		{"2.0", "unsupported"},
		{"garbage", "unsupported"},
		{"1", "unsupported"},
		{"x.0", "unsupported"}, // non-numeric major
		{"1.x", "unsupported"}, // non-numeric minor
		{"", "unsupported"},
	}
	for _, c := range cases {
		err := snapshot.CheckSchemaVersion(c.version)
		if c.wantErr == "" && err != nil {
			t.Errorf("CheckSchemaVersion(%q) = %v, want nil", c.version, err)
		}
		if c.wantErr != "" {
			if err == nil || !strings.Contains(err.Error(), c.wantErr) {
				t.Errorf("CheckSchemaVersion(%q) = %v, want error containing %q", c.version, err, c.wantErr)
			}
		}
	}
}

const minimalJSON = `{
  "schema_version": "1.0",
  "generated_at": "2026-06-13T00:00:00Z",
  "accounts": [
    {"login": 111, "label": "Trend EA", "currency": "USD",
     "balance": 1000.0, "equity": 1010.5,
     "last_success_at": "2026-06-13T00:00:00Z", "last_error": null}
  ],
  "closed_deals": [
    {"account": 111, "ticket": 1, "order": 1, "position_id": 1,
     "time": 1767607200, "time_msc": 1767607200000, "type": 0, "entry": 1,
     "reason": 0, "magic": 7, "volume": 0.1, "price": 1.08,
     "profit": 10.0, "swap": -0.5, "commission": -0.5, "fee": 0.0,
     "symbol": "EURUSD", "comment": "", "external_id": ""}
  ],
  "open_positions": [],
  "cash_flows": []
}`

func TestReadRoundTrip(t *testing.T) {
	path := snaptest.Write(t, minimalJSON, "test-pass")
	snap, err := snapshot.Read(path, "test-pass")
	if err != nil {
		t.Fatal(err)
	}
	if snap.SchemaVersion != "1.0" || len(snap.Accounts) != 1 || len(snap.ClosedDeals) != 1 {
		t.Fatalf("unexpected snapshot: %+v", snap)
	}
	if snap.Accounts[0].Label != "Trend EA" || snap.Accounts[0].LastError != nil {
		t.Errorf("account fields wrong: %+v", snap.Accounts[0])
	}
	d := snap.ClosedDeals[0]
	if d.Account != 111 || d.Profit != 10.0 || d.Commission != -0.5 || d.Time != 1767607200 {
		t.Errorf("deal fields wrong: %+v", d)
	}
}

func TestReadWrongPassphrase(t *testing.T) {
	path := snaptest.Write(t, minimalJSON, "test-pass")
	_, err := snapshot.Read(path, "wrong")
	if err == nil || !strings.Contains(err.Error(), "wrong passphrase") {
		t.Fatalf("err = %v, want wrong-passphrase message", err)
	}
}

func TestReadMissingFile(t *testing.T) {
	_, err := snapshot.Read("/nonexistent/snapshot.json.gz.age", "x")
	if err == nil {
		t.Fatal("want error for missing file")
	}
}

func TestReadCorruptFile(t *testing.T) {
	path := snaptest.Write(t, minimalJSON, "test-pass")
	if err := os.WriteFile(path, []byte("not an age file"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := snapshot.Read(path, "test-pass")
	if err == nil {
		t.Fatal("want error for corrupt file")
	}
}

func TestReadRefusesUnsupportedSchema(t *testing.T) {
	body := strings.Replace(minimalJSON, `"schema_version": "1.0"`, `"schema_version": "2.0"`, 1)
	path := snaptest.Write(t, body, "test-pass")
	_, err := snapshot.Read(path, "test-pass")
	if err == nil || !strings.Contains(err.Error(), "unsupported snapshot schema") {
		t.Fatalf("err = %v, want unsupported-schema message", err)
	}
}

func TestReadCorruptGzip(t *testing.T) {
	// Valid age, but the plaintext is not gzip data.
	path := snaptest.WriteAge(t, []byte("not gzip data"), "test-pass")
	_, err := snapshot.Read(path, "test-pass")
	if err == nil || !strings.Contains(err.Error(), "decompressing") {
		t.Fatalf("err = %v, want decompressing error", err)
	}
}

func TestReadCorruptJSON(t *testing.T) {
	// Valid age and valid gzip, but the decompressed bytes are not JSON.
	path := snaptest.WriteGzip(t, []byte("not json"), "test-pass")
	_, err := snapshot.Read(path, "test-pass")
	if err == nil || !strings.Contains(err.Error(), "parsing") {
		t.Fatalf("err = %v, want parsing error", err)
	}
}
