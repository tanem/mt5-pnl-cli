package main

import (
	"runtime/debug"
	"testing"
)

func TestFormatVersion(t *testing.T) {
	tests := []struct {
		name              string
		ver, commit, date string
		want              string
	}{
		{"version only", "1.2.3", "", "", "mt5-pnl-cli 1.2.3"},
		{"all set", "1.2.3", "abc1234", "2026-06-16", "mt5-pnl-cli 1.2.3 (commit abc1234, built 2026-06-16)"},
		{"commit only", "1.2.3", "abc1234", "", "mt5-pnl-cli 1.2.3 (commit abc1234)"},
		{"date only", "1.2.3", "", "2026-06-16", "mt5-pnl-cli 1.2.3 (built 2026-06-16)"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := formatVersion(tt.ver, tt.commit, tt.date); got != tt.want {
				t.Errorf("formatVersion(%q,%q,%q) = %q, want %q", tt.ver, tt.commit, tt.date, got, tt.want)
			}
		})
	}
}

func TestVersionFrom(t *testing.T) {
	withMainVersion := func(v string) func() (*debug.BuildInfo, bool) {
		return func() (*debug.BuildInfo, bool) {
			bi := &debug.BuildInfo{}
			bi.Main.Version = v
			return bi, true
		}
	}
	noBuildInfo := func() (*debug.BuildInfo, bool) { return nil, false }

	tests := []struct {
		name          string
		ldflagVersion string
		readBuildInfo func() (*debug.BuildInfo, bool)
		want          string
	}{
		{"ldflag set wins over build info", "1.0.0", withMainVersion("v2.0.0"), "1.0.0"},
		{"go install resolves the module version", "dev", withMainVersion("v1.0.0"), "1.0.0"},
		{"local go build stays dev", "dev", withMainVersion("(devel)"), "dev"},
		{"empty module version stays dev", "dev", withMainVersion(""), "dev"},
		{"missing build info stays dev", "dev", noBuildInfo, "dev"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := versionFrom(tt.ldflagVersion, tt.readBuildInfo); got != tt.want {
				t.Errorf("versionFrom(%q) = %q, want %q", tt.ldflagVersion, got, tt.want)
			}
		})
	}
}
