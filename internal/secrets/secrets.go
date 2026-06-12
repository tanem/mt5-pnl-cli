// Package secrets stores the snapshot decryption passphrase in the OS
// keychain (macOS Keychain / Windows Credential Manager / Linux Secret
// Service). The passphrase is never read from env vars or flags.
package secrets

import (
	"errors"

	"github.com/zalando/go-keyring"
)

const (
	service = "mt5-pnl-cli"
	account = "encryption-passphrase"
)

var ErrNotFound = errors.New("no passphrase in keychain: run 'mt5-pnl-cli set-passphrase' first")

func Get() (string, error) {
	pw, err := keyring.Get(service, account)
	if errors.Is(err, keyring.ErrNotFound) {
		return "", ErrNotFound
	}
	return pw, err
}

func Set(passphrase string) error {
	if passphrase == "" {
		return errors.New("passphrase cannot be empty")
	}
	return keyring.Set(service, account, passphrase)
}
