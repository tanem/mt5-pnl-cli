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
  -h, --help                show this help
`

func cmdAccounts(args []string, stdout, stderr io.Writer, getPassphrase func() (string, error)) int {
	fs := flag.NewFlagSet("accounts", flag.ContinueOnError)
	fs.SetOutput(stderr)
	formatFlag := fs.String("format", "table", "output format: table, json or csv")
	snapFlag := fs.String("snapshot", "", "snapshot path (default: $MT5_PNL_SNAPSHOT)")
	staleAfter := fs.Duration("stale-after", 2*time.Hour, "staleness warning threshold")
	if ok, code := parseFlags(fs, args, stdout, accountsHelp); !ok {
		return code
	}

	format, err := resolveFormat(*formatFlag)
	if err != nil {
		fmt.Fprintln(stderr, "error:", err)
		return 1
	}

	snap, err := loadSnapshot(*snapFlag, *staleAfter, stderr, getPassphrase)
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
		err = render.AccountsTable(stdout, snap.Accounts, snap.GeneratedAt)
	}
	if err != nil {
		fmt.Fprintln(stderr, "error:", err)
		return 1
	}
	return 0
}
