package main

import (
	"flag"
	"fmt"
	"io"
	"time"

	"github.com/tanem/mt5-pnl-cli/internal/aggregate"
	"github.com/tanem/mt5-pnl-cli/internal/render"
)

const pnlHelp = `Usage: mt5-pnl-cli pnl [flags]

Show P&L over a date range, grouped per period and account.

Flags:
  --last Nd|Nw|Nm|Ny       relative range ending today (default 30d)
  --from YYYY-MM-DD         start date (UTC)
  --to YYYY-MM-DD           end date (UTC); defaults to today
  --by day|week|month       grouping (default week; weeks start Monday)
  --accounts "A,B"          filter by account label (default: all)
  --format table|json|csv   output format (default table)
  --json                    alias for --format json
  --snapshot PATH           snapshot path (default: $MT5_PNL_SNAPSHOT)
  --stale-after DUR         staleness warning threshold (default 2h)
  -h, --help                show this help
`

func cmdPnL(args []string, stdout, stderr io.Writer, getPassphrase func() (string, error)) int {
	fs := flag.NewFlagSet("pnl", flag.ContinueOnError)
	fs.SetOutput(stderr)
	last := fs.String("last", "", "relative range: Nd, Nw, Nm or Ny (default 30d)")
	from := fs.String("from", "", "start date (YYYY-MM-DD)")
	to := fs.String("to", "", "end date (YYYY-MM-DD); defaults to today")
	by := fs.String("by", "week", "group results by: day, week or month")
	accountsSpec := fs.String("accounts", "", "comma-separated account labels (default: all)")
	asJSON := fs.Bool("json", false, "emit JSON instead of a table")
	snapFlag := fs.String("snapshot", "", "snapshot path (default: $MT5_PNL_SNAPSHOT)")
	staleAfter := fs.Duration("stale-after", 2*time.Hour, "staleness warning threshold")
	if ok, code := parseFlags(fs, args, stdout, pnlHelp); !ok {
		return code
	}

	if *by != "day" && *by != "week" && *by != "month" {
		fmt.Fprintf(stderr, "error: invalid --by %q: use day, week or month\n", *by)
		return 1
	}
	fromD, toD, err := resolveRange(*last, *from, *to, time.Now())
	if err != nil {
		fmt.Fprintln(stderr, "error:", err)
		return 1
	}

	snap, err := loadSnapshot(*snapFlag, *staleAfter, stderr, getPassphrase)
	if err != nil {
		fmt.Fprintln(stderr, "error:", err)
		return 1
	}

	filter, err := resolveAccounts(*accountsSpec, snap.Accounts)
	if err != nil {
		fmt.Fprintln(stderr, "error:", err)
		return 1
	}

	rows, sum := aggregate.Aggregate(snap.ClosedDeals, aggregate.Options{
		From: fromD, To: toD, By: *by, Accounts: filter,
	})

	labels := make(map[int64]string, len(snap.Accounts))
	for _, a := range snap.Accounts {
		labels[a.Login] = a.Label
	}

	if *asJSON {
		err = render.PnLJSON(stdout, rows, sum, false)
	} else {
		err = render.PnLTable(stdout, rows, sum, labels, false)
	}
	if err != nil {
		fmt.Fprintln(stderr, "error:", err)
		return 1
	}
	return 0
}
