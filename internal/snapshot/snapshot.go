// Package snapshot reads the encrypted snapshot written by mt5-pnl-exporter.
//
// The on-disk format is JSON → gzip → age (scrypt passphrase). The struct
// fields match schema/snapshot.schema.json (vendored from the exporter
// v1.0.3 release) field-for-field.
package snapshot

import (
	"compress/gzip"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"

	"filippo.io/age"
)

// Supported schema version: accept the same major and any minor <= SupportedMinor.
const (
	SupportedMajor = 1
	SupportedMinor = 0
)

type Snapshot struct {
	SchemaVersion string            `json:"schema_version"`
	GeneratedAt   string            `json:"generated_at"`
	Accounts      []AccountSnapshot `json:"accounts"`
	ClosedDeals   []Deal            `json:"closed_deals"`
	// OpenPositions and CashFlows are decoded for schema fidelity with the
	// exporter, but intentionally not surfaced: cash flows (deposits and
	// withdrawals) are deliberately excluded from trading P&L, and open
	// positions are reserved for a future command.
	OpenPositions []OpenPosition `json:"open_positions"`
	CashFlows     []Deal         `json:"cash_flows"`
}

type AccountSnapshot struct {
	Login         int64   `json:"login"`
	Label         string  `json:"label"`
	Currency      string  `json:"currency"`
	Balance       float64 `json:"balance"`
	Equity        float64 `json:"equity"`
	LastSuccessAt *string `json:"last_success_at"`
	LastError     *string `json:"last_error"`
}

// Deal is the shape shared by closed_deals and cash_flows (the schema's
// ClosedDeal and CashFlow are field-for-field identical).
type Deal struct {
	Account    int64   `json:"account"`
	Ticket     int64   `json:"ticket"`
	Order      int64   `json:"order"`
	PositionID int64   `json:"position_id"`
	Time       int64   `json:"time"`
	TimeMsc    int64   `json:"time_msc"`
	Type       int     `json:"type"`
	Entry      int     `json:"entry"`
	Reason     int     `json:"reason"`
	Magic      int64   `json:"magic"`
	Volume     float64 `json:"volume"`
	Price      float64 `json:"price"`
	Profit     float64 `json:"profit"`
	Swap       float64 `json:"swap"`
	Commission float64 `json:"commission"`
	Fee        float64 `json:"fee"`
	Symbol     string  `json:"symbol"`
	Comment    string  `json:"comment"`
	ExternalID string  `json:"external_id"`
}

type OpenPosition struct {
	Account       int64   `json:"account"`
	Ticket        int64   `json:"ticket"`
	Identifier    int64   `json:"identifier"`
	Time          int64   `json:"time"`
	TimeMsc       int64   `json:"time_msc"`
	TimeUpdate    int64   `json:"time_update"`
	TimeUpdateMsc int64   `json:"time_update_msc"`
	Type          int     `json:"type"`
	Reason        int     `json:"reason"`
	Magic         int64   `json:"magic"`
	Volume        float64 `json:"volume"`
	PriceOpen     float64 `json:"price_open"`
	PriceCurrent  float64 `json:"price_current"`
	SL            float64 `json:"sl"`
	TP            float64 `json:"tp"`
	Profit        float64 `json:"profit"`
	Swap          float64 `json:"swap"`
	Symbol        string  `json:"symbol"`
	Comment       string  `json:"comment"`
	ExternalID    string  `json:"external_id"`
}

// CheckSchemaVersion enforces the contract: same major, minor <= ours.
func CheckSchemaVersion(v string) error {
	unsupported := func() error {
		return fmt.Errorf(
			"unsupported snapshot schema %q: this build supports %d.0 through %d.%d "+
				"(newer minor: upgrade mt5-pnl-cli; different major: align exporter and CLI releases)",
			v, SupportedMajor, SupportedMajor, SupportedMinor)
	}
	parts := strings.SplitN(v, ".", 2)
	if len(parts) != 2 {
		return unsupported()
	}
	major, err := strconv.Atoi(parts[0])
	if err != nil {
		return unsupported()
	}
	minor, err := strconv.Atoi(parts[1])
	if err != nil {
		return unsupported()
	}
	if major != SupportedMajor || minor > SupportedMinor {
		return unsupported()
	}
	return nil
}

// Read opens, decrypts (age scrypt), decompresses and parses a snapshot,
// then enforces the schema version gate. The pipeline is fully streaming.
func Read(path, passphrase string) (*Snapshot, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	id, err := age.NewScryptIdentity(passphrase)
	if err != nil {
		return nil, err
	}
	dec, err := age.Decrypt(f, id)
	if err != nil {
		return nil, fmt.Errorf("decrypting %s: wrong passphrase, or the file is corrupt (%v)", path, err)
	}
	gz, err := gzip.NewReader(dec)
	if err != nil {
		return nil, fmt.Errorf("decompressing %s: %v", path, err)
	}
	defer gz.Close()

	var snap Snapshot
	if err := json.NewDecoder(gz).Decode(&snap); err != nil {
		return nil, fmt.Errorf("parsing %s: %v", path, err)
	}
	if err := CheckSchemaVersion(snap.SchemaVersion); err != nil {
		return nil, err
	}
	return &snap, nil
}
