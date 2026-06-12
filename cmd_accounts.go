package main

import (
	"flag"
	"fmt"
	"io"
	"time"

	"github.com/tanem/mt5-pnl-cli/internal/render"
)

func cmdAccounts(args []string, stdout, stderr io.Writer, getPassphrase func() (string, error)) int {
	fs := flag.NewFlagSet("accounts", flag.ContinueOnError)
	fs.SetOutput(stderr)
	asJSON := fs.Bool("json", false, "emit JSON instead of a table")
	snapFlag := fs.String("snapshot", "", "snapshot path (default: $MT5_PNL_SNAPSHOT)")
	staleAfter := fs.Duration("stale-after", 2*time.Hour, "staleness warning threshold")
	if err := fs.Parse(args); err != nil {
		return 1
	}

	snap, err := loadSnapshot(*snapFlag, *staleAfter, stderr, getPassphrase)
	if err != nil {
		fmt.Fprintln(stderr, "error:", err)
		return 1
	}

	if *asJSON {
		err = render.AccountsJSON(stdout, snap.Accounts)
	} else {
		err = render.AccountsTable(stdout, snap.Accounts, snap.GeneratedAt)
	}
	if err != nil {
		fmt.Fprintln(stderr, "error:", err)
		return 1
	}
	return 0
}
