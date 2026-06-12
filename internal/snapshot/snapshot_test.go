package snapshot_test

import (
	"strings"
	"testing"

	"github.com/tanem/mt5-pnl-cli/internal/snapshot"
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
