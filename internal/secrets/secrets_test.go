package secrets_test

import (
	"errors"
	"testing"

	"github.com/zalando/go-keyring"

	"github.com/tanem/mt5-pnl-cli/internal/secrets"
)

func TestSetAndGet(t *testing.T) {
	keyring.MockInit() // in-memory store; no real keychain touched
	if err := secrets.Set("hunter2"); err != nil {
		t.Fatal(err)
	}
	got, err := secrets.Get()
	if err != nil {
		t.Fatal(err)
	}
	if got != "hunter2" {
		t.Errorf("Get() = %q, want %q", got, "hunter2")
	}
}

func TestGetMissing(t *testing.T) {
	keyring.MockInit()
	_, err := secrets.Get()
	if !errors.Is(err, secrets.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestSetEmpty(t *testing.T) {
	keyring.MockInit()
	if err := secrets.Set(""); err == nil {
		t.Fatal("want error for empty passphrase")
	}
}
