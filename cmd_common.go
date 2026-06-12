package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/tanem/mt5-pnl-cli/internal/snapshot"
)

func resolveSnapshotPath(flagVal string, getenv func(string) string) (string, error) {
	p := flagVal
	if p == "" {
		p = getenv("MT5_PNL_SNAPSHOT")
	}
	if p == "" {
		return "", errors.New("no snapshot path: pass --snapshot or set MT5_PNL_SNAPSHOT")
	}
	return expandTilde(p)
}

func expandTilde(p string) (string, error) {
	if p == "~" || strings.HasPrefix(p, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		return filepath.Join(home, p[1:]), nil
	}
	return p, nil
}

func warnIfStale(w io.Writer, generatedAt string, threshold time.Duration, now time.Time) {
	ts, err := time.Parse(time.RFC3339, generatedAt)
	if err != nil {
		fmt.Fprintln(w, "warning: could not parse snapshot timestamp; staleness unknown")
		return
	}
	if age := now.Sub(ts); age > threshold {
		fmt.Fprintf(w, "warning: snapshot is %.1fh old (threshold %s); run 'mt5-pnl-exporter export' on the host\n",
			age.Hours(), threshold)
	}
}

func resolveAccounts(spec string, accounts []snapshot.AccountSnapshot) (map[int64]bool, error) {
	if strings.TrimSpace(spec) == "" {
		return nil, nil
	}
	byLabel := make(map[string]int64, len(accounts))
	labels := make([]string, 0, len(accounts))
	for _, a := range accounts {
		byLabel[strings.ToLower(a.Label)] = a.Login
		labels = append(labels, a.Label)
	}
	out := map[int64]bool{}
	for _, raw := range strings.Split(spec, ",") {
		name := strings.TrimSpace(raw)
		login, ok := byLabel[strings.ToLower(name)]
		if !ok {
			return nil, fmt.Errorf("unknown account label %q; valid labels: %s", name, strings.Join(labels, ", "))
		}
		out[login] = true
	}
	return out, nil
}

func loadSnapshot(pathFlag string, staleAfter time.Duration, stderr io.Writer, getPassphrase func() (string, error)) (*snapshot.Snapshot, error) {
	path, err := resolveSnapshotPath(pathFlag, os.Getenv)
	if err != nil {
		return nil, err
	}
	pass, err := getPassphrase()
	if err != nil {
		return nil, err
	}
	snap, err := snapshot.Read(path, pass)
	if err != nil {
		return nil, err
	}
	warnIfStale(stderr, snap.GeneratedAt, staleAfter, time.Now())
	return snap, nil
}
