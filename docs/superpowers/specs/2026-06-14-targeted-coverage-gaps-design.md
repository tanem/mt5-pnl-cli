# Targeted test-coverage improvements

## Goal

Close genuine, user-facing coverage gaps with characterization tests, without
altering production code (beyond test infrastructure) or testing deliberately
untestable seams. Coverage is the diagnostic, not the target: each test pins a
behaviour a user can actually trigger.

## Background

The repo sits at ~85% statement coverage. The core P&L logic
(`internal/aggregate`) is already at 100%, and the existing CLI tests cover most
error paths. Tracing every uncovered block to a line showed the remaining gaps
fall into three buckets: genuine user-facing gaps, real-but-rarer corruption
modes, and code that is untestable by design (os.Exit wiring, keychain writes,
`os.UserHomeDir` failure).

This change picks up the first two buckets and deliberately leaves the third.

## In scope — pure test additions

| Gap | Location | Test |
|---|---|---|
| `expandTilde` `~` / `~/` expansion | `cmd_common.go:27-32` | unit test, `t.Setenv("HOME", …)`, assert `filepath.Join` result |
| `accounts --json` branch | `cmd_accounts.go:28-30` | CLI test asserting JSON shape |
| `CheckSchemaVersion` non-numeric major/minor | `snapshot.go:108-114` | extend existing table with `"x.0"`, `"1.x"` |
| `resolveRange` invalid `--to` | `args.go:58-60` | `--from 2026-01-01 --to junk` → error |
| `pnl` / `accounts` flag-parse error | `cmd_pnl.go:24-26`, `cmd_accounts.go:18-20` | `pnl --nope`, `accounts --nope` → exit 1 |
| `pnl` invalid `--from` | `cmd_pnl.go:33-36` | `pnl --from junk` → exit 1 |
| `set-passphrase` non-terminal guard | `cmd_setpassphrase.go:18-21`, `main.go:30-31` | `run("set-passphrase")` under test (stdin not a TTY) → exit 1 + message |
| `help` / `--help` | `main.go:35-37` | `run("--help")` → usage on stdout, exit 0 |

## In scope — test infrastructure + corruption branches

`snapshot.Read`'s gzip-error and JSON-parse-error branches need a fixture that is
*valid age wrapping a bad payload*; the current corrupt-file test fails earlier
at the age layer.

- Refactor `snaptest.Write` to share an age-encrypt core, then add:
  - `WriteAge(t, raw []byte, passphrase)` — age-encrypt arbitrary bytes (no
    gzip), for the gzip-error path.
  - `WriteGzip(t, raw []byte, passphrase)` — gzip then age-encrypt arbitrary
    bytes, for the JSON-parse-error path.
- `snapshot.Read` gzip-error: `WriteAge([]byte("not gzip"))` → expect error from
  `snapshot.go:139-141`.
- `snapshot.Read` JSON-parse-error: `WriteGzip([]byte("not json"))` → expect
  error from `snapshot.go:145-147`.

## Out of scope — deliberately untested

- `main()` os.Exit wiring (`run()` is the testable seam, per CLAUDE.md).
- `set-passphrase` happy path: reads a real TTY and writes the keychain; CLAUDE.md
  says don't test the keychain and the binary smoke test only covers pre-keychain
  failure paths.
- `os.UserHomeDir()` failure, tabwriter flush errors, `age.NewScryptIdentity`
  error — can't be triggered portably without contorting production code.

## Approach

These are characterization tests for existing code, so not classic red-green TDD.
Each test asserts specific behaviour (error text, output, exit code) so it fails
on regression rather than merely executing a line. After writing, confirm each
test closes its target block via the coverage profile.

New tests follow existing patterns: the `runCLI` helper in `cli_test.go` for
command-level cases, table tests in `snapshot_test.go`, and a focused unit test
file for `expandTilde`.

The `set-passphrase` guard test depends on the test process's stdin not being a
TTY, which is the normal `go test` situation.

## Verification

- `go test ./... -race` green.
- `go tool cover -func` confirms the targeted blocks are now covered.
- Report the total-coverage delta.

## Docs

No CLAUDE.md or README change — no commands, architecture or gotchas change.
