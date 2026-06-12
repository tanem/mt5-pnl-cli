// Package snaptest builds encrypted snapshot fixtures for tests, reversing
// the exporter's pipeline: JSON → gzip → age (scrypt).
package snaptest

import (
	"bytes"
	"compress/gzip"
	"os"
	"path/filepath"
	"testing"

	"filippo.io/age"
)

// Write encrypts jsonBody and writes it under t.TempDir(), returning the path.
func Write(t *testing.T, jsonBody, passphrase string) string {
	t.Helper()
	var buf bytes.Buffer
	r, err := age.NewScryptRecipient(passphrase)
	if err != nil {
		t.Fatal(err)
	}
	// Low work factor keeps tests fast; Read handles any factor from the file.
	r.SetWorkFactor(10)
	aw, err := age.Encrypt(&buf, r)
	if err != nil {
		t.Fatal(err)
	}
	gz := gzip.NewWriter(aw)
	if _, err := gz.Write([]byte(jsonBody)); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	if err := aw.Close(); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "snapshot.json.gz.age")
	if err := os.WriteFile(path, buf.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}
