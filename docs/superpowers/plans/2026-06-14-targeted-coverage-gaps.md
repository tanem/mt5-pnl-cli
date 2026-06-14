# Targeted test-coverage gaps Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Close genuine, user-facing coverage gaps in mt5-pnl-cli with characterization tests, without changing production code.

**Architecture:** Add tests for existing behaviour, following established patterns (`runCLI` helper in `cli_test.go`, table tests in `*_test.go`, unit tests in `args_test.go`). One small refactor to `internal/snaptest` adds two fixture helpers so the snapshot decode-error branches become reachable. Spec: `docs/superpowers/specs/2026-06-14-targeted-coverage-gaps-design.md`.

**Tech Stack:** Go, standard `testing` package, `filippo.io/age`, existing `snaptest` fixtures.

---

## Note on TDD for this plan

These are characterization tests for code that already exists, so the classic
red-green "write a failing test first" does not apply: the tests pass the moment
they compile against working code. Verification is therefore two-pronged:

1. The new test **passes** (`go test`).
2. The test **exercises its target branch** — confirmed via the coverage
   profile (`go tool cover -func`), and the test asserts specific behaviour
   (exit code, error text, output) so it would fail if that behaviour regressed.

The one exception is Task 1 (a refactor of test infrastructure): there the
existing tests must stay green, which is the regression guard.

## File structure

- `internal/snaptest/snaptest.go` — **modify.** Refactor `Write` to share an
  age-encrypt core; add exported `WriteAge` and `WriteGzip` helpers.
- `internal/snapshot/snapshot_test.go` — **modify.** Extend the
  `CheckSchemaVersion` table; add corrupt-gzip and corrupt-JSON read tests.
- `args_test.go` — **modify.** Add `TestExpandTilde`; add an invalid-`--to`
  case to the existing `TestResolveRange` error table.
- `cli_test.go` — **modify.** Add command-level tests: `accounts --json`,
  flag-parse errors, invalid `pnl` range, `set-passphrase` terminal guard,
  `--help`.

No production files change.

---

## Task 1: snaptest fixture helpers

**Files:**
- Modify: `internal/snaptest/snaptest.go`

The current `Write` does JSON → gzip → age in one function. Refactor so the
age-encrypt and file-write steps are shared, then expose two helpers that stop
short of the full pipeline: `WriteAge` (raw bytes → age, skips gzip) feeds the
gzip-decompress error path; `WriteGzip` (raw bytes → gzip → age, skips JSON)
feeds the JSON-parse error path. `Write` keeps its exact existing behaviour by
delegating through `WriteGzip`.

- [ ] **Step 1: Replace the file contents**

Overwrite `internal/snaptest/snaptest.go` with:

```go
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
```

- [ ] **Step 2: Run the existing suite to confirm the refactor is behaviour-preserving**

Run: `go test ./...`
Expected: PASS for all packages — every existing test still uses `Write`, which
now routes through `WriteGzip`/`WriteAge` but produces the identical pipeline.

- [ ] **Step 3: Commit**

```bash
git add internal/snaptest/snaptest.go
git commit -m "test: add raw snaptest fixture helpers for decode-error paths"
```

---

## Task 2: snapshot decode-error and malformed-version tests

**Files:**
- Modify: `internal/snapshot/snapshot_test.go`

Covers `snapshot.go:108-114` (non-numeric major/minor in `CheckSchemaVersion`)
and `snapshot.go:139-141` / `145-147` (gzip and JSON decode errors in `Read`).

- [ ] **Step 1: Add two cases to the `CheckSchemaVersion` table**

In `TestCheckSchemaVersion`, add these two rows to the `cases` slice (after the
existing `{"1", "unsupported"}` line is fine):

```go
		{"x.0", "unsupported"}, // non-numeric major
		{"1.x", "unsupported"}, // non-numeric minor
```

- [ ] **Step 2: Append the corrupt-payload read tests**

Add to the end of `internal/snapshot/snapshot_test.go`:

```go
func TestReadCorruptGzip(t *testing.T) {
	// Valid age, but the plaintext is not gzip data.
	path := snaptest.WriteAge(t, []byte("not gzip data"), "test-pass")
	_, err := snapshot.Read(path, "test-pass")
	if err == nil || !strings.Contains(err.Error(), "decompressing") {
		t.Fatalf("err = %v, want decompressing error", err)
	}
}

func TestReadCorruptJSON(t *testing.T) {
	// Valid age and valid gzip, but the decompressed bytes are not JSON.
	path := snaptest.WriteGzip(t, []byte("not json"), "test-pass")
	_, err := snapshot.Read(path, "test-pass")
	if err == nil || !strings.Contains(err.Error(), "parsing") {
		t.Fatalf("err = %v, want parsing error", err)
	}
}
```

- [ ] **Step 3: Run the snapshot tests**

Run: `go test ./internal/snapshot/ -run 'TestCheckSchemaVersion|TestReadCorrupt' -v`
Expected: PASS for `TestCheckSchemaVersion`, `TestReadCorruptGzip`,
`TestReadCorruptJSON`.

- [ ] **Step 4: Confirm the target branches are now covered**

Run: `go test ./internal/snapshot/ -coverprofile=/tmp/c.out && go tool cover -func=/tmp/c.out | grep -E 'CheckSchemaVersion|Read\b'`
Expected: `CheckSchemaVersion` and `Read` both report higher coverage than the
pre-change 85.7% / 85.0% (both should be at or near 100% now).

- [ ] **Step 5: Commit**

```bash
git add internal/snapshot/snapshot_test.go
git commit -m "test: cover snapshot decode errors and malformed versions"
```

---

## Task 3: expandTilde unit test and invalid --to range case

**Files:**
- Modify: `args_test.go`

`args_test.go` already houses tests for `cmd_common.go` helpers
(`resolveSnapshotPath`, `warnIfStale`, `resolveAccounts`), so `expandTilde`
belongs here too. Covers `cmd_common.go:27-32` and `args.go:58-60`.

- [ ] **Step 1: Add the missing imports**

The file currently imports `bytes`, `strings`, `testing`, `time`, and the
`snapshot` package. Add `os` and `path/filepath` to the import block:

```go
import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/tanem/mt5-pnl-cli/internal/snapshot"
)
```

- [ ] **Step 2: Add the invalid-`--to` case to `TestResolveRange`**

In `TestResolveRange`, add this row to the error-cases slice (the
`[][3]string{...}` literal), e.g. after the `{"", "not-a-date", ""}` line:

```go
		{"", "2026-01-01", "not-a-date"}, // valid --from, invalid --to
```

- [ ] **Step 3: Add `TestExpandTilde`**

Append to `args_test.go`:

```go
func TestExpandTilde(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skipf("no home dir on this platform: %v", err)
	}
	cases := []struct{ in, want string }{
		{"~", home},
		{"~/snap.age", filepath.Join(home, "snap.age")},
		{"~/a/b", filepath.Join(home, "a", "b")},
		{"/abs/path", "/abs/path"}, // no leading ~, returned unchanged
		{"relative", "relative"},
	}
	for _, c := range cases {
		got, err := expandTilde(c.in)
		if err != nil {
			t.Errorf("expandTilde(%q): %v", c.in, err)
			continue
		}
		if got != c.want {
			t.Errorf("expandTilde(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
```

- [ ] **Step 4: Run the affected tests**

Run: `go test . -run 'TestResolveRange|TestExpandTilde' -v`
Expected: PASS for both.

- [ ] **Step 5: Commit**

```bash
git add args_test.go
git commit -m "test: cover expandTilde and invalid --to range"
```

---

## Task 4: command-level CLI tests

**Files:**
- Modify: `cli_test.go`

Covers `cmd_accounts.go:18-20,28-30`, `cmd_pnl.go:24-26,33-36`,
`cmd_setpassphrase.go:18-21`, and `main.go:30-31,35-37`. All use the existing
`runCLI` / `fixture` helpers; no new imports are needed (`json`, `strings`,
`bytes` are already imported).

- [ ] **Step 1: Append the new tests**

Add to the end of `cli_test.go`:

```go
func TestAccountsJSONCommand(t *testing.T) {
	path := fixture(t)
	out, _, code := runCLI(t, "test-pass",
		"accounts", "--snapshot", path, "--json", "--stale-after", "876000h")
	if code != 0 {
		t.Fatalf("exit %d", code)
	}
	var accts []map[string]any
	if err := json.Unmarshal([]byte(out), &accts); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, out)
	}
	if len(accts) != 2 {
		t.Fatalf("got %d accounts, want 2:\n%s", len(accts), out)
	}
	if !strings.Contains(out, "Trend EA") {
		t.Errorf("JSON missing account label:\n%s", out)
	}
}

func TestPnLFlagParseError(t *testing.T) {
	_, errOut, code := runCLI(t, "test-pass", "pnl", "--nope")
	if code != 1 || errOut == "" {
		t.Errorf("exit %d, stderr %q; want 1 + flag error", code, errOut)
	}
}

func TestAccountsFlagParseError(t *testing.T) {
	_, errOut, code := runCLI(t, "test-pass", "accounts", "--nope")
	if code != 1 || errOut == "" {
		t.Errorf("exit %d, stderr %q; want 1 + flag error", code, errOut)
	}
}

func TestPnLInvalidRange(t *testing.T) {
	_, errOut, code := runCLI(t, "test-pass", "pnl", "--from", "not-a-date")
	if code != 1 || !strings.Contains(errOut, "--from") {
		t.Errorf("exit %d, stderr %q; want 1 + --from guidance", code, errOut)
	}
}

func TestSetPassphraseRequiresTerminal(t *testing.T) {
	// Under `go test` stdin is not a TTY, so the interactive guard fires
	// before any keychain access (which CI cannot reach anyway).
	_, errOut, code := runCLI(t, "", "set-passphrase")
	if code != 1 || !strings.Contains(errOut, "interactive terminal") {
		t.Errorf("exit %d, stderr %q; want 1 + terminal guard", code, errOut)
	}
}

func TestHelpCommand(t *testing.T) {
	out, _, code := runCLI(t, "", "--help")
	if code != 0 || !strings.Contains(out, "Usage") {
		t.Errorf("exit %d, stdout %q; want 0 + usage", code, out)
	}
}
```

- [ ] **Step 2: Run the new tests**

Run: `go test . -run 'TestAccountsJSONCommand|FlagParseError|TestPnLInvalidRange|TestSetPassphraseRequiresTerminal|TestHelpCommand' -v`
Expected: PASS for all six.

- [ ] **Step 3: Commit**

```bash
git add cli_test.go
git commit -m "test: cover accounts --json, flag errors, set-passphrase guard, help"
```

---

## Final verification

- [ ] **Step 1: Full suite with race detector**

Run: `go test ./... -race`
Expected: PASS for every package.

- [ ] **Step 2: Report the coverage delta**

Run: `go test ./... -coverprofile=/tmp/cover.out && go tool cover -func=/tmp/cover.out | tail -1`
Expected: total coverage above the pre-change 85.1% baseline (target low-90s%).
Record the before/after numbers in the completion summary.

- [ ] **Step 3: Confirm scope — no production files changed**

Run: `git diff --stat main -- '*.go' ':!*_test.go' ':!internal/snaptest/*'`
Expected: empty output (only `_test.go` files and `internal/snaptest` changed).
