package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/tanem/mt5-pnl-cli/internal/snapshot"
)

var now = time.Date(2026, 6, 13, 10, 30, 0, 0, time.UTC)

func d(y int, m time.Month, day int) time.Time {
	return time.Date(y, m, day, 0, 0, 0, 0, time.UTC)
}

func TestParseLast(t *testing.T) {
	cases := []struct {
		in       string
		from, to time.Time
	}{
		{"30d", d(2026, 5, 14), d(2026, 6, 13)},
		{"2w", d(2026, 5, 30), d(2026, 6, 13)},
		{"6m", d(2025, 12, 13), d(2026, 6, 13)}, // calendar-accurate, not 180 days
		{"1y", d(2025, 6, 13), d(2026, 6, 13)},
	}
	for _, c := range cases {
		from, to, err := parseLast(c.in, now)
		if err != nil {
			t.Errorf("parseLast(%q): %v", c.in, err)
			continue
		}
		if !from.Equal(c.from) || !to.Equal(c.to) {
			t.Errorf("parseLast(%q) = %v..%v, want %v..%v", c.in, from, to, c.from, c.to)
		}
	}
	for _, bad := range []string{"", "30", "d30", "30x", "-5d"} {
		if _, _, err := parseLast(bad, now); err == nil {
			t.Errorf("parseLast(%q): want error", bad)
		}
	}
}

func TestResolveRange(t *testing.T) {
	// default when nothing given: --last 30d
	from, to, err := resolveRange("", "", "", now)
	if err != nil || !from.Equal(d(2026, 5, 14)) || !to.Equal(d(2026, 6, 13)) {
		t.Errorf("default = %v..%v (%v), want 30d window", from, to, err)
	}
	// --from alone runs to today
	from, to, err = resolveRange("", "2026-01-01", "", now)
	if err != nil || !from.Equal(d(2026, 1, 1)) || !to.Equal(d(2026, 6, 13)) {
		t.Errorf("from-only = %v..%v (%v)", from, to, err)
	}
	// explicit range
	from, to, err = resolveRange("", "2026-01-01", "2026-03-31", now)
	if err != nil || !from.Equal(d(2026, 1, 1)) || !to.Equal(d(2026, 3, 31)) {
		t.Errorf("explicit = %v..%v (%v)", from, to, err)
	}
	// errors
	for _, c := range [][3]string{
		{"30d", "2026-01-01", ""},        // --last with --from
		{"", "", "2026-03-31"},           // --to without --from
		{"", "2026-03-31", "2026-01-01"}, // to before from
		{"", "not-a-date", ""},
		{"", "2026-01-01", "not-a-date"}, // valid --from, invalid --to
	} {
		if _, _, err := resolveRange(c[0], c[1], c[2], now); err == nil {
			t.Errorf("resolveRange(%q,%q,%q): want error", c[0], c[1], c[2])
		}
	}
}

func TestResolveSnapshotPath(t *testing.T) {
	env := func(vars map[string]string) func(string) string {
		return func(k string) string { return vars[k] }
	}
	if p, err := resolveSnapshotPath("/flag/path", env(map[string]string{"MT5_PNL_SNAPSHOT": "/env/path"})); err != nil || p != "/flag/path" {
		t.Errorf("flag should win: %q %v", p, err)
	}
	if p, err := resolveSnapshotPath("", env(map[string]string{"MT5_PNL_SNAPSHOT": "/env/path"})); err != nil || p != "/env/path" {
		t.Errorf("env fallback: %q %v", p, err)
	}
	if _, err := resolveSnapshotPath("", env(nil)); err == nil || !strings.Contains(err.Error(), "MT5_PNL_SNAPSHOT") {
		t.Errorf("missing both: %v, want guidance naming the env var", err)
	}
}

func TestWarnIfStale(t *testing.T) {
	var buf bytes.Buffer
	warnIfStale(&buf, "2026-06-13T00:00:00Z", 2*time.Hour, now) // 10.5h old
	if !strings.Contains(buf.String(), "stale") && !strings.Contains(buf.String(), "old") {
		t.Errorf("want staleness warning, got %q", buf.String())
	}
	buf.Reset()
	warnIfStale(&buf, "2026-06-13T10:00:00Z", 2*time.Hour, now) // 0.5h old
	if buf.Len() != 0 {
		t.Errorf("want no warning, got %q", buf.String())
	}
	buf.Reset()
	warnIfStale(&buf, "garbage", 2*time.Hour, now)
	if !strings.Contains(buf.String(), "staleness unknown") {
		t.Errorf("want unparseable-timestamp warning, got %q", buf.String())
	}
}

func TestResolveAccounts(t *testing.T) {
	accts := []snapshot.AccountSnapshot{
		{Login: 111, Label: "Trend EA"},
		{Login: 222, Label: "Scalper EA"},
	}
	if got, err := resolveAccounts("", accts); err != nil || got != nil {
		t.Errorf("empty spec = %v, %v; want nil, nil", got, err)
	}
	got, err := resolveAccounts("trend ea, Scalper EA", accts)
	if err != nil || !got[111] || !got[222] || len(got) != 2 {
		t.Errorf("case-insensitive resolve = %v, %v", got, err)
	}
	_, err = resolveAccounts("Nope", accts)
	if err == nil || !strings.Contains(err.Error(), "Trend EA") {
		t.Errorf("unknown label error should list valid labels: %v", err)
	}
}

func TestExpandTilde(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skipf("no home dir on this platform: %v", err)
	}
	cases := []struct{ in, want string }{
		{"~", home},
		{"~/snap.age", filepath.Join(home, "snap.age")},
		{"~/a/b", filepath.Join(home, "a", "b")},
		{"/abs/path", "/abs/path"}, // no leading ~, returned unchanged
		{"relative", "relative"},
	}
	for _, c := range cases {
		got, err := expandTilde(c.in)
		if err != nil {
			t.Errorf("expandTilde(%q): %v", c.in, err)
			continue
		}
		if got != c.want {
			t.Errorf("expandTilde(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
