package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/tanem/mt5-pnl-cli/internal/snapshot"
	"golang.org/x/term"
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
	if age := now.Sub(ts).Abs(); age > threshold {
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

func loadSnapshot(pathFlag string, staleAfter time.Duration, warnW io.Writer, getPassphrase func() (string, error)) (*snapshot.Snapshot, error) {
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
	warnIfStale(warnW, snap.GeneratedAt, staleAfter, time.Now())
	return snap, nil
}

// currenciesInScope returns the distinct account currencies in scope, sorted.
// When an explicit --accounts filter is given, scope is those accounts;
// otherwise it is the accounts that actually contributed deals (passed via
// `contributing`). More than one currency means combined totals would sum
// across currencies.
func currenciesInScope(accounts []snapshot.AccountSnapshot, filter map[int64]bool, contributing []int64) []string {
	curBy := make(map[int64]string, len(accounts))
	for _, a := range accounts {
		curBy[a.Login] = a.Currency
	}
	set := map[string]bool{}
	if filter != nil {
		for login := range filter {
			if c := curBy[login]; c != "" {
				set[c] = true
			}
		}
	} else {
		for _, login := range contributing {
			if c := curBy[login]; c != "" {
				set[c] = true
			}
		}
	}
	out := make([]string, 0, len(set))
	for c := range set {
		out = append(out, c)
	}
	sort.Strings(out)
	return out
}

// resolveColor decides whether to emit ANSI colour. always/never are absolute;
// auto enables colour only for an interactive terminal that has not opted out
// via NO_COLOR or TERM=dumb. w is the real output stream: colour is auto-off
// whenever it is not a *os.File TTY (pipes, files, test buffers), which keeps
// machine-consumed output clean.
func resolveColor(mode string, w io.Writer, getenv func(string) string) bool {
	switch mode {
	case "always":
		return true
	case "never":
		return false
	default: // "auto"
		if getenv("NO_COLOR") != "" || getenv("TERM") == "dumb" {
			return false
		}
		f, ok := w.(*os.File)
		return ok && term.IsTerminal(int(f.Fd()))
	}
}

// resolveFormat validates the --format value. The legacy --json alias has been
// removed; --format is the single spelling.
func resolveFormat(format string) (string, error) {
	switch format {
	case "table", "json", "csv":
		return format, nil
	default:
		return "", fmt.Errorf("invalid --format %q: use table, json or csv", format)
	}
}

// parseFlags parses fs, separating the two failure modes flag conflates:
// -h/--help prints the hand-written help to stdout and signals a clean exit
// (ok=false, code 0); any other parse error has already been written to fs's
// output (stderr) by the flag package, so we just signal exit 1. fs.Usage is
// suppressed so flag never dumps its own auto-generated usage.
func parseFlags(fs *flag.FlagSet, args []string, stdout io.Writer, help string) (ok bool, code int) {
	fs.Usage = func() {}
	err := fs.Parse(args)
	switch {
	case err == nil:
		return true, 0
	case errors.Is(err, flag.ErrHelp):
		fmt.Fprint(stdout, help)
		return false, 0
	default:
		return false, 1
	}
}
