package main

import (
	"flag"
	"fmt"
	"io"
	"time"

	"github.com/tanem/mt5-pnl-cli/internal/render"
)

const accountsHelp = `Usage: mt5-pnl-cli accounts [flags]

List accounts with balance, equity and freshness.

Flags:
  --format table|json|csv   output format (default table)
  --snapshot PATH           snapshot path (default: $MT5_PNL_SNAPSHOT)
  --stale-after DUR         staleness warning threshold (default 2h)
  -q, --quiet               suppress warnings on stderr
  -h, --help                show this help

Examples:
  mt5-pnl-cli accounts
  mt5-pnl-cli accounts --format json
`

func cmdAccounts(args []string, stdout, stderr io.Writer, getPassphrase func() (string, error)) int {
	fs := flag.NewFlagSet("accounts", flag.ContinueOnError)
	fs.SetOutput(stderr)
	formatFlag := fs.String("format", "table", "output format: table, json or csv")
	snapFlag := fs.String("snapshot", "", "snapshot path (default: $MT5_PNL_SNAPSHOT)")
	staleAfter := fs.Duration("stale-after", 2*time.Hour, "staleness warning threshold")
	var quiet bool
	fs.BoolVar(&quiet, "quiet", false, "suppress warnings on stderr")
	fs.BoolVar(&quiet, "q", false, "suppress warnings on stderr (shorthand)")
	if ok, code := parseFlags(fs, args, stdout, accountsHelp); !ok {
		return code
	}

	format, err := resolveFormat(*formatFlag)
	if err != nil {
		fmt.Fprintln(stderr, "error:", err)
		return 1
	}

	warnW := io.Writer(stderr)
	if quiet {
		warnW = io.Discard
	}

	snap, err := loadSnapshot(*snapFlag, *staleAfter, warnW, getPassphrase)
	if err != nil {
		fmt.Fprintln(stderr, "error:", err)
		return 1
	}

	switch format {
	case "json":
		err = render.AccountsJSON(stdout, snap.Accounts)
	case "csv":
		err = render.AccountsCSV(stdout, snap.Accounts)
	default:
		err = render.AccountsTable(stdout, snap.Accounts, snap.GeneratedAt, render.TableOpts{})
	}
	if err != nil {
		fmt.Fprintln(stderr, "error:", err)
		return 1
	}
	return 0
}
