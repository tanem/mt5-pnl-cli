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

// Write encrypts jsonBody the way the exporter does (JSON → gzip → age) and
// writes it under t.TempDir(), returning the path.
func Write(t *testing.T, jsonBody, passphrase string) string {
	t.Helper()
	return WriteGzip(t, []byte(jsonBody), passphrase)
}

// WriteGzip gzips raw, then age-encrypts it, skipping any JSON-validity step.
// Use it to exercise the snapshot JSON-parse error path with a payload that
// decompresses cleanly but is not valid JSON.
func WriteGzip(t *testing.T, raw []byte, passphrase string) string {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	if _, err := gz.Write(raw); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return WriteAge(t, buf.Bytes(), passphrase)
}

// WriteAge age-encrypts raw bytes directly, skipping gzip. Use it to exercise
// the snapshot gzip-decompress error path with a payload that decrypts cleanly
// but is not gzip data.
func WriteAge(t *testing.T, raw []byte, passphrase string) string {
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
	if _, err := aw.Write(raw); err != nil {
		t.Fatal(err)
	}
	if err := aw.Close(); err != nil {
		t.Fatal(err)
	}
	return writeFixture(t, buf.Bytes())
}

func writeFixture(t *testing.T, b []byte) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "snapshot.json.gz.age")
	if err := os.WriteFile(path, b, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}
