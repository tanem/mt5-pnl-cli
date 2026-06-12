package main

import (
	"fmt"
	"io"
	"os"

	"golang.org/x/term"

	"github.com/tanem/mt5-pnl-cli/internal/secrets"
)

// cmdSetPassphrase stores the snapshot decryption passphrase in the OS
// keychain. Input is read without echo and never accepted from arguments
// or the environment.
func cmdSetPassphrase(stderr io.Writer) int {
	fd := int(os.Stdin.Fd())
	if !term.IsTerminal(fd) {
		fmt.Fprintln(stderr, "error: set-passphrase requires an interactive terminal")
		return 1
	}
	fmt.Fprint(stderr, "Encryption passphrase: ")
	p1, err := term.ReadPassword(fd)
	fmt.Fprintln(stderr)
	if err != nil {
		fmt.Fprintln(stderr, "error:", err)
		return 1
	}
	fmt.Fprint(stderr, "Confirm passphrase: ")
	p2, err := term.ReadPassword(fd)
	fmt.Fprintln(stderr)
	if err != nil {
		fmt.Fprintln(stderr, "error:", err)
		return 1
	}
	if string(p1) != string(p2) {
		fmt.Fprintln(stderr, "error: passphrases do not match")
		return 1
	}
	if err := secrets.Set(string(p1)); err != nil {
		fmt.Fprintln(stderr, "error:", err)
		return 1
	}
	fmt.Fprintln(stderr, "Passphrase stored in keychain.")
	return 0
}
