package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/tanem/mt5-pnl-cli/internal/aggregate"
	"github.com/tanem/mt5-pnl-cli/internal/render"
)

const pnlHelp = `Usage: mt5-pnl-cli pnl [flags]

Show P&L over a date range, grouped per period and account.

Flags:
  --last Nd|Nw|Nm|Ny         relative range ending today (default 30d)
  --from YYYY-MM-DD           start date (UTC)
  --to YYYY-MM-DD             end date (UTC); defaults to today
  --by day|week|month         grouping (default week; weeks start Monday)
  --accounts "A,B"            filter by account label (default: all)
  --format table|json|csv     output format (default table)
  --color auto|always|never   colourise P&L by sign (default auto)
  --snapshot PATH             snapshot path (default: $MT5_PNL_SNAPSHOT)
  --stale-after DUR           staleness warning threshold (default 2h)
  -q, --quiet                 suppress warnings on stderr
  -h, --help                  show this help

Examples:
  mt5-pnl-cli pnl --last 7d
  mt5-pnl-cli pnl --from 2026-01-01 --to 2026-03-31 --by month
  mt5-pnl-cli pnl --by month --format csv > pnl.csv
`

func cmdPnL(args []string, stdout, stderr io.Writer, getPassphrase func() (string, error)) int {
	fs := flag.NewFlagSet("pnl", flag.ContinueOnError)
	fs.SetOutput(stderr)
	last := fs.String("last", "", "relative range: Nd, Nw, Nm or Ny (default 30d)")
	from := fs.String("from", "", "start date (YYYY-MM-DD)")
	to := fs.String("to", "", "end date (YYYY-MM-DD); defaults to today")
	by := fs.String("by", "week", "group results by: day, week or month")
	accountsSpec := fs.String("accounts", "", "comma-separated account labels (default: all)")
	formatFlag := fs.String("format", "table", "output format: table, json or csv")
	colorMode := fs.String("color", "auto", "colour output: auto, always or never")
	snapFlag := fs.String("snapshot", "", "snapshot path (default: $MT5_PNL_SNAPSHOT)")
	staleAfter := fs.Duration("stale-after", 2*time.Hour, "staleness warning threshold")
	var quiet bool
	fs.BoolVar(&quiet, "quiet", false, "suppress warnings on stderr")
	fs.BoolVar(&quiet, "q", false, "suppress warnings on stderr (shorthand)")
	if ok, code := parseFlags(fs, args, stdout, pnlHelp); !ok {
		return code
	}

	format, err := resolveFormat(*formatFlag)
	if err != nil {
		fmt.Fprintln(stderr, "error:", err)
		return 1
	}

	if *by != "day" && *by != "week" && *by != "month" {
		fmt.Fprintf(stderr, "error: invalid --by %q: use day, week or month\n", *by)
		return 1
	}
	if *colorMode != "auto" && *colorMode != "always" && *colorMode != "never" {
		fmt.Fprintf(stderr, "error: invalid --color %q: use auto, always or never\n", *colorMode)
		return 1
	}
	fromD, toD, err := resolveRange(*last, *from, *to, time.Now())
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

	curs := currenciesInScope(snap.Accounts, filter, rows)
	mixed := len(curs) > 1
	if mixed {
		fmt.Fprintf(warnW,
			"warning: accounts span multiple currencies (%s); combined totals are suppressed — narrow --accounts to one currency\n",
			strings.Join(curs, ", "))
	}

	opts := render.TableOpts{}
	if len(curs) == 1 {
		opts.Currency = curs[0]
	}
	opts.Color = resolveColor(*colorMode, stdout, os.Getenv)

	switch format {
	case "json":
		err = render.PnLJSON(stdout, rows, sum, *by, mixed)
	case "csv":
		err = render.PnLCSV(stdout, rows, labels, *by, mixed)
	default:
		err = render.PnLTable(stdout, rows, sum, labels, mixed, opts)
	}
	if err != nil {
		fmt.Fprintln(stderr, "error:", err)
		return 1
	}
	return 0
}
