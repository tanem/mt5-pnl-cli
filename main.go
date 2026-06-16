// mt5-pnl-cli queries MT5 P&L from an encrypted mt5-pnl-exporter snapshot.
package main

import (
	"fmt"
	"io"
	"os"

	"github.com/tanem/mt5-pnl-cli/internal/secrets"
	"github.com/tanem/mt5-pnl-cli/internal/snapshot"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr, secrets.Get))
}

// run is the testable entry point: commands write to the given streams and
// obtain the decryption passphrase via getPassphrase (the real binary wires
// secrets.Get; tests inject a fake).
func run(args []string, stdout, stderr io.Writer, getPassphrase func() (string, error)) int {
	if len(args) == 0 {
		usage(stderr)
		return 1
	}
	switch args[0] {
	case "pnl":
		return cmdPnL(args[1:], stdout, stderr, getPassphrase)
	case "accounts":
		return cmdAccounts(args[1:], stdout, stderr, getPassphrase)
	case "set-passphrase":
		return cmdSetPassphrase(stderr)
	case "version", "--version":
		fmt.Fprintf(stdout, "mt5-pnl-cli %s (schema %d.%d)\n", resolveVersion(), snapshot.SupportedMajor, snapshot.SupportedMinor)
		return 0
	case "help", "-h", "--help":
		usage(stdout)
		return 0
	default:
		fmt.Fprintf(stderr, "unknown command %q\n\n", args[0])
		usage(stderr)
		return 1
	}
}

func usage(w io.Writer) {
	fmt.Fprint(w, `mt5-pnl-cli — query MT5 P&L from an mt5-pnl-exporter snapshot.

Usage:
  mt5-pnl-cli pnl [--last 30d | --from YYYY-MM-DD [--to YYYY-MM-DD]]
                  [--by day|week|month] [--accounts "A,B"]
                  [--format table|json|csv] [--snapshot PATH] [--stale-after 2h]
  mt5-pnl-cli accounts [--format table|json|csv] [--snapshot PATH] [--stale-after 2h]
  mt5-pnl-cli set-passphrase
  mt5-pnl-cli version   (or --version)

The snapshot path comes from --snapshot or the MT5_PNL_SNAPSHOT environment
variable. The decryption passphrase comes from the OS keychain; store it
once with set-passphrase.

Examples:
  mt5-pnl-cli pnl --last 30d
  mt5-pnl-cli accounts --format json
`)
}
