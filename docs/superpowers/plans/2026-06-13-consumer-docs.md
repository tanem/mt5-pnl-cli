# Consumer Documentation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Bring mt5-pnl-cli's docs to the family standard — consumer-first README, CONTRIBUTING, SECURITY — plus the agreed tooling riders (SHA-pinned actions, codecov upload, gitleaks hook), per the approved spec at `docs/superpowers/specs/2026-06-13-consumer-docs-design.md`.

**Architecture:** One PR off the existing `docs/consumer-docs` branch. Tooling changes land first (so SECURITY.md's claims are true when it lands), then the three documents, then CLAUDE.md sync and verification. Demo output in the README is regenerated from the test fixture via a throwaway harness — never hand-typed.

**Tech Stack:** Markdown; GitHub Actions; codecov-action v5; pre-commit + gitleaks.

**Conventions:** Working directory is `<repo-root>`, branch `docs/consumer-docs`. British/Commonwealth English. `CODECOV_TOKEN` is already set in repo secrets; Renovate covers the repo (maintainer confirmed both).

---

### Task 1: SHA-pin GitHub Actions and add codecov upload

**Files:**
- Modify: `.github/workflows/ci.yml` (full replacement below)
- Modify: `.github/workflows/release.yml` (full replacement below)

- [ ] **Step 1: Replace `.github/workflows/ci.yml`** with exactly:

```yaml
name: ci

on:
  push:
    branches: [main]
  pull_request:

permissions:
  contents: read

jobs:
  lint:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@34e114876b0b11c390a56381ad16ebd13914f8d5 # v4
      - uses: actions/setup-go@40f1582b2485089dde7abd97c1529aa768e1baff # v5
        with:
          go-version: stable
      - name: gofmt
        run: |
          out=$(gofmt -l .)
          if [ -n "$out" ]; then echo "gofmt needed:"; echo "$out"; exit 1; fi
      - run: go vet ./...
      - uses: golangci/golangci-lint-action@4afd733a84b1f43292c63897423277bb7f4313a9 # v8
        with:
          version: latest

  test:
    strategy:
      matrix:
        os: [ubuntu-latest, macos-latest, windows-latest]
    runs-on: ${{ matrix.os }}
    steps:
      - uses: actions/checkout@34e114876b0b11c390a56381ad16ebd13914f8d5 # v4
      - uses: actions/setup-go@40f1582b2485089dde7abd97c1529aa768e1baff # v5
        with:
          go-version: stable
      - run: go test -race -coverprofile=coverage.out ./...
      - name: Upload coverage
        if: matrix.os == 'ubuntu-latest'
        uses: codecov/codecov-action@0fb7174895f61a3b6b78fc075e0cd60383518dac # v5
        with:
          token: ${{ secrets.CODECOV_TOKEN }}
          files: coverage.out
```

- [ ] **Step 2: Replace `.github/workflows/release.yml`** with exactly:

```yaml
name: release

on:
  push:
    tags: ['v*']

permissions:
  contents: write

jobs:
  goreleaser:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@34e114876b0b11c390a56381ad16ebd13914f8d5 # v4
        with:
          fetch-depth: 0
      - uses: actions/setup-go@40f1582b2485089dde7abd97c1529aa768e1baff # v5
        with:
          go-version: stable
      - uses: goreleaser/goreleaser-action@e435ccd777264be153ace6237001ef4d979d3a7a # v6
        with:
          version: '~> v2'
          args: release --clean
        env:
          GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}
```

- [ ] **Step 3: Sanity-check the YAML parses**

Run: `python3 -c "import yaml,sys; yaml.safe_load(open('.github/workflows/ci.yml')); yaml.safe_load(open('.github/workflows/release.yml')); print('YAML OK')"`
Expected: `YAML OK`

- [ ] **Step 4: Commit**

```bash
git add .github/workflows/
git commit -m "ci: pin actions to commit SHAs; upload coverage to codecov"
```

---

### Task 2: gitleaks pre-commit hook

**Files:**
- Create: `.pre-commit-config.yaml`

- [ ] **Step 1: Create `.pre-commit-config.yaml`** with exactly:

```yaml
repos:
  - repo: https://github.com/gitleaks/gitleaks
    rev: v8.21.2
    hooks:
      - id: gitleaks
```

- [ ] **Step 2: Install and run the hook over the repo**

Run: `pre-commit install && pre-commit run --all-files`
Expected: `gitleaks` reports `Passed`. (If `pre-commit` is not installed: `brew install pre-commit` first.)

- [ ] **Step 3: Commit**

```bash
git add .pre-commit-config.yaml
git commit -m "chore: gitleaks pre-commit hook"
```

---

### Task 3: README rework

**Files:**
- Create (temporary): `demo_gen_test.go` — deleted before commit
- Modify: `README.md` (full replacement below)

- [ ] **Step 1: Regenerate demo output from the fixture.** Create `demo_gen_test.go` with exactly:

```go
package main

// Throwaway generator for README demo output. Deleted after use; never committed.

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/tanem/mt5-pnl-cli/internal/snaptest"
)

func TestDemoGen(t *testing.T) {
	path := snaptest.Write(t, fixtureJSON, "demo-pass")
	pass := func() (string, error) { return "demo-pass", nil }

	for _, args := range [][]string{
		{"pnl", "--snapshot", path, "--from", "2026-01-01", "--to", "2026-01-31", "--stale-after", "876000h"},
		{"pnl", "--snapshot", path, "--from", "2026-01-01", "--to", "2026-01-31", "--by", "month", "--accounts", "Trend EA", "--json", "--stale-after", "876000h"},
		{"accounts", "--snapshot", path, "--stale-after", "876000h"},
	} {
		var out, errBuf bytes.Buffer
		code := run(args, &out, &errBuf, pass)
		fmt.Printf("=== ARGS: %v (exit %d)\n%s\n", args, code, out.String())
	}
}
```

Run: `go test . -run TestDemoGen -v`
Expected: three output blocks, exit 0 each. The `--json` invocation uses `--accounts "Trend EA"` so its rows array is complete (one account row + the combined row) — the README shows it in full, nothing silently elided.

- [ ] **Step 2: Delete the generator**

Run: `rm demo_gen_test.go`

- [ ] **Step 3: Replace `README.md`** with exactly the following, **substituting the three demo blocks with the freshly captured output from Step 1** (the blocks below are from the planning run and should match byte-for-byte; if they differ, trust Step 1's output):

````markdown
# mt5-pnl-cli

[![Licence](https://img.shields.io/github/license/tanem/mt5-pnl-cli)](LICENSE)
[![ci](https://github.com/tanem/mt5-pnl-cli/actions/workflows/ci.yml/badge.svg)](https://github.com/tanem/mt5-pnl-cli/actions/workflows/ci.yml)
[![coverage](https://codecov.io/gh/tanem/mt5-pnl-cli/branch/main/graph/badge.svg)](https://codecov.io/gh/tanem/mt5-pnl-cli)

> Query MT5 P&L from an encrypted [mt5-pnl-exporter](https://github.com/tanem/mt5-pnl-exporter) snapshot. One static binary — no daemon, no database, no third-party service.

```
   ┌──────────────┐  writes   ┌────────────────┐  reads   ┌──────────────┐
   │ mt5-pnl-     │ ────────► │ snapshot.json  │ ───────► │ mt5-pnl-cli  │
   │ exporter     │           │ .gz.age        │          │ (this repo)  │
   │ Windows host │           │ synced via     │          │ macOS, Linux │
   └──────────────┘           │ Dropbox etc.   │          │ or Windows   │
                              └────────────────┘          └──────────────┘
```

The exporter runs on the Windows host where MT5 lives and writes one
encrypted file. This CLI runs wherever you are, decrypts that file in
memory, and prints P&L and account tables — or JSON for scripts and AI
agents.

## Contents

- [Why](#why)
- [Install](#install)
- [Quick start](#quick-start)
- [Demo](#demo)
- [Commands](#commands)
- [How it works](#how-it-works)
- [Schema compatibility](#schema-compatibility)
- [Threat model](#threat-model)
- [Contributing](#contributing)
- [Licence](#licence)

## Why

- **Self-hosted.** Your trading data never touches a third-party
  dashboard. The snapshot is yours; this binary reads it locally.
- **One file in, answers out.** No config file. Point it at the snapshot
  once (env var or flag) and `mt5-pnl-cli pnl` just works.
- **Agent- and script-friendly.** `--json` emits stable machine-readable
  output, and warnings go to stderr so they never corrupt a pipeline. An
  agent like Claude Code can turn *"show me monthly P&L for Q1"* into
  `mt5-pnl-cli pnl --from 2026-01-01 --to 2026-03-31 --by month --json`.
- **Secrets stay in the keychain.** The decryption passphrase lives in
  the OS keychain only — there is deliberately no env var or flag for it.

## Install

Download a binary from
[Releases](https://github.com/tanem/mt5-pnl-cli/releases)
(darwin/linux/windows, amd64/arm64, with `checksums.txt`), or build with
Go:

```sh
go install github.com/tanem/mt5-pnl-cli@latest
```

## Quick start

```sh
# once: store the snapshot passphrase in the OS keychain. Use the SAME
# passphrase you gave mt5-pnl-exporter's set-encryption-passphrase.
mt5-pnl-cli set-passphrase

# once: tell the CLI where the synced snapshot lives
export MT5_PNL_SNAPSHOT=~/snapshots/mt5.json.gz.age

mt5-pnl-cli pnl          # last 30 days, grouped by week
mt5-pnl-cli accounts     # balances, equity, freshness
```

## Demo

Per-account and combined (`ALL`) rows per period, with a summary line:

```
$ mt5-pnl-cli pnl --from 2026-01-01 --to 2026-01-31
PERIOD      ACCOUNT     P&L   TRADES  WINS  LOSSES
2026-01-05  Trend EA    5.00  2       1     1
2026-01-05  Scalper EA  0.00  1       0     0
2026-01-05  ALL         5.00  3       1     1
2026-01-12  Trend EA    5.00  1       1     0
2026-01-12  ALL         5.00  1       1     0

Total P&L: 10.00  Trades: 4  Win rate: 50.0%  Profit factor: 3.50  Gross profit: 14.00  Gross loss: -4.00
```

```
$ mt5-pnl-cli accounts
LOGIN  LABEL       CURRENCY  BALANCE  EQUITY   LAST SUCCESS          LAST ERROR
111    Trend EA    USD       1000.00  1010.50  2026-06-13T00:00:00Z  -
222    Scalper EA  USD       500.00   500.00   -                     login failed

Snapshot generated: 2026-06-13T00:00:00Z
```

`--json` emits the same data for machines (`"account": null` is the
combined row):

```
$ mt5-pnl-cli pnl --from 2026-01-01 --to 2026-01-31 --by month --accounts "Trend EA" --json
{
  "rows": [
    {
      "period": "2026-01-01",
      "account": 111,
      "pnl": 10,
      "trades": 3,
      "wins": 2,
      "losses": 1,
      "gross_profit": 14,
      "gross_loss": -4
    },
    {
      "period": "2026-01-01",
      "account": null,
      "pnl": 10,
      "trades": 3,
      "wins": 2,
      "losses": 1,
      "gross_profit": 14,
      "gross_loss": -4
    }
  ],
  "summary": {
    "total_pnl": 10,
    "total_trades": 3,
    "win_rate_pct": 66.7,
    "profit_factor": 3.5,
    "gross_profit": 14,
    "gross_loss": -4
  }
}
```

## Commands

- `pnl` — P&L over a date range.
  - Range: `--last Nd|Nw|Nm|Ny` (default `30d`; months and years are
    calendar-accurate) or `--from YYYY-MM-DD [--to YYYY-MM-DD]` (`--to`
    defaults to today).
  - `--by day|week|month` (default `week`; weeks start Monday, dates are
    UTC).
  - `--accounts "Trend EA,Scalper EA"` filters by account label
    (case-insensitive; default all).
  - `--json` for machine output.
- `accounts` — balances, equity and freshness per account, plus the
  snapshot's `generated_at`.
- `set-passphrase` — store the snapshot decryption passphrase in the OS
  keychain (macOS Keychain / Windows Credential Manager / Linux Secret
  Service). Prompted twice, never echoed.
- `version` — binary version and supported snapshot schema.

Both query commands accept `--snapshot PATH` (overrides
`MT5_PNL_SNAPSHOT`) and `--stale-after` (default `2h`) — when the
snapshot is older than that, a warning goes to **stderr**, never stdout,
so `--json` pipelines stay clean.

## How it works

```
snapshot.json.gz.age ──► age decrypt ──► gunzip ──► JSON ──► aggregate ──► table / JSON
(ciphertext on disk)     passphrase      (in memory only)
                         from keychain
```

The snapshot is never written to disk decrypted. Per closed deal, net
P&L = `profit + swap + commission + fee`. A deal with net > 0 is a win,
net < 0 a loss; a breakeven deal counts toward trades but neither
bucket. Sums accumulate at full precision and round only for display.
Profit factor = gross profit / |gross loss|.

## Schema compatibility

[`schema/snapshot.schema.json`](schema/snapshot.schema.json) is vendored
from the exporter release this build supports. The CLI accepts the same
major schema version and any minor at or below its own, and refuses
anything else with a message naming both versions and which side to
upgrade. `mt5-pnl-cli version` prints the supported schema version.

## Threat model

The trust boundary is your OS user session — the same as the
exporter's.

### What's protected

- **The snapshot at rest** stays `age`-encrypted; this CLI never writes
  a decrypted copy. Sync services and backups only ever see ciphertext.
- **The passphrase** lives in the OS keychain. There is deliberately no
  env var or flag for it: env vars leak via dotfiles, shell history, and
  child processes, and scripts don't need one — the binary reads the
  keychain itself. (`MT5_PNL_SNAPSHOT` carries only a file path.)

### What's not protected

- **A compromised user session.** Anyone with your OS account can read
  the keychain entry and run this CLI. The exporter's
  [threat model](https://github.com/tanem/mt5-pnl-exporter#threat-model)
  draws the same line.
- **Whatever you do with the output.** Tables and JSON contain balances
  and trade history; treat redirected output accordingly.

See [SECURITY.md](SECURITY.md) for scope and private vulnerability
reporting.

## Contributing

Issues and PRs welcome — see [CONTRIBUTING.md](CONTRIBUTING.md).

## Licence

[MIT](LICENSE)
````

- [ ] **Step 4: Cross-check every flag and command named in the README**

Run: `go run . help` and `go run . pnl -h 2>&1 | head -20`
Expected: every flag the README mentions exists with the documented default (`--by week`, `--stale-after 2h`, `--last` default `30d` behaviour, `MT5_PNL_SNAPSHOT` named in usage).

- [ ] **Step 5: Commit**

```bash
git add README.md
git commit -m "docs: consumer-first README rework"
```

---

### Task 4: CONTRIBUTING.md

**Files:**
- Create: `CONTRIBUTING.md`

- [ ] **Step 1: Create `CONTRIBUTING.md`** with exactly:

````markdown
# Contributing

Thanks for your interest. This is a small project — issues and PRs are
welcome.

## One-time setup

```bash
# Go 1.25+ (go.mod sets the floor; the toolchain auto-downloads if needed)
pre-commit install                   # enable the gitleaks secret-scan hook
```

## Running tests and checks

```bash
go test -race ./...                  # full test suite
gofmt -l .                           # must print nothing
go vet ./...
golangci-lint run                    # same config as CI (.golangci.yml)
pre-commit run --all-files           # run the gitleaks hook manually
```

## Golden files

Table output is asserted byte-for-byte against
`internal/render/testdata/*.golden`. After an intentional rendering
change:

```bash
go test ./internal/render -update    # regenerate the goldens
git diff internal/render/testdata/   # eyeball the change
```

Commit the regenerated goldens together with the rendering change.
`.gitattributes` marks `*.golden -text` so Windows checkouts can't
CRLF-convert them — don't remove that rule.

## Smoke-test against a real snapshot

Before tagging a release, exercise the working tree against a real
exporter snapshot — this walks the same age → gzip → JSON path a
consumer uses:

```bash
go build -o mt5-pnl-cli .
./mt5-pnl-cli set-passphrase         # the exporter's encryption passphrase
./mt5-pnl-cli accounts --snapshot <path-to-snapshot.json.gz.age>
./mt5-pnl-cli pnl --last 3m --by month --snapshot <path-to-snapshot.json.gz.age>
```

Confirm the accounts table matches your accounts and the staleness
warning behaves (it prints to stderr when the snapshot is older than
`--stale-after`).

## Dependency updates

Dependencies are kept current by
[Renovate](https://docs.renovatebot.com/) (config:
[`renovate.json`](renovate.json)), covering Go modules and GitHub
Actions:

- Actions are pinned to commit SHAs (not mutable tags) for supply-chain
  integrity; Renovate keeps the SHA and its version comment current.
- Digest, minor, and patch updates auto-merge once CI passes; majors
  open a PR for review.

Don't hand-bump these versions — let Renovate's PRs flow through.

## Testing a release build

```bash
go run github.com/goreleaser/goreleaser/v2@latest check
go run github.com/goreleaser/goreleaser/v2@latest build --snapshot --clean --single-target
./dist/*/mt5-pnl-cli version         # run the actual release artefact
```

## Releasing

Releases are GitHub Releases with attached binaries — no package index,
no publish approval gate (simpler than mt5-pnl-exporter's PyPI flow on
purpose):

1. `git tag vX.Y.Z && git push origin vX.Y.Z`
2. The tag push triggers
   [`release.yml`](.github/workflows/release.yml): GoReleaser builds
   darwin/linux/windows × amd64/arm64, generates `checksums.txt` and a
   changelog, and publishes the GitHub Release.
3. Check the Release page; binaries and checksums should be attached.

The binary's `version` output is injected from the tag via ldflags.

## Conventions

See [`CLAUDE.md`](CLAUDE.md) — the canonical reference for coding style,
architectural rules, and gotchas (British/Commonwealth English, the no-config-file and
keychain-only invariants, golden-file workflow, doc-sync rule). It's
loaded automatically by Claude Code but reads as a normal project doc.
````

- [ ] **Step 2: Verify the commands in the doc actually run**

Run: `golangci-lint run 2>/dev/null || echo "golangci-lint not installed locally — acceptable, CI covers it"` and `go run github.com/goreleaser/goreleaser/v2@latest check`
Expected: goreleaser config valid; lint either passes or is noted as CI-covered.

- [ ] **Step 3: Commit**

```bash
git add CONTRIBUTING.md
git commit -m "docs: CONTRIBUTING with Go workflows and release process"
```

---

### Task 5: SECURITY.md

**Files:**
- Create: `SECURITY.md`

- [ ] **Step 1: Create `SECURITY.md`** with exactly:

````markdown
# Security policy

## Scope

This tool handles one secret: the **snapshot decryption passphrase**,
stored in the OS keychain (via `go-keyring`: macOS Keychain / Windows
Credential Manager / Linux Secret Service). It is never accepted via
argument, environment variable, or file, and never written to disk or
logs. The decrypted snapshot exists in memory only — the CLI never
writes plaintext to disk.

Vulnerabilities in scope:

- Passphrase disclosure in any output, error message, or crash
- Unsafe keychain read/write behaviour
- The decryption pipeline (`age` + gzip) bypassed, weakened, or made to
  accept tampered input undetected
- Account data (balances, trade history) leaking anywhere other than
  the requested stdout output
- Dependency vulnerabilities with a plausible exploitation path in this
  tool

Out of scope:

- mt5-pnl-exporter and the MT5/broker side (see the
  [exporter's security policy](https://github.com/tanem/mt5-pnl-exporter/blob/main/SECURITY.md))
- A compromised OS user session — the documented trust boundary; anyone
  with the user's session can read the keychain and run the CLI
- Issues only reproducible with a non-current Go toolchain

## Supply-chain controls

- **GitHub Actions are pinned to commit SHAs** (not mutable tags), so a
  compromised or retagged action cannot inject code into CI.
  [Renovate](https://docs.renovatebot.com/) keeps the pins current via
  `helpers:pinGitHubActionDigests`.
- Dependency update PRs (Renovate) must pass the full CI matrix
  (ubuntu/macos/windows) before auto-merging; majors require manual
  review. See [`renovate.json`](renovate.json).
- **Release binaries are static** (`CGO_ENABLED=0`), built in CI by
  GoReleaser from the tagged commit, and published with a
  `checksums.txt` for verification.

## Reporting

**Do not open a public GitHub issue for security vulnerabilities.**
Public issues expose the vulnerability before a fix is available.

Report privately via the Security tab —
[Report a vulnerability](https://github.com/tanem/mt5-pnl-cli/security/advisories/new).
This opens a private workspace visible only to you and the maintainer.

You will receive a response within 7 days. Once a fix is ready, I'll
agree a disclosure date with you before publishing.
````

- [ ] **Step 2: Verify the claims are true in this repo**

Run: `grep -c "# v" .github/workflows/ci.yml .github/workflows/release.yml && grep "CGO_ENABLED" .goreleaser.yaml && grep "pinGitHubActionDigests" renovate.json`
Expected: SHA-comment pins present in both workflows (Task 1 landed them), `CGO_ENABLED=0` in the GoReleaser config, the Renovate preset present.

- [ ] **Step 3: Commit**

```bash
git add SECURITY.md
git commit -m "docs: security policy"
```

---

### Task 6: CLAUDE.md sync

**Files:**
- Modify: `CLAUDE.md` (Commands section only)

- [ ] **Step 1: Update the Commands block.** Replace:

```markdown
```bash
go test ./...                 # all tests (race-enabled in CI)
go test ./internal/render -update   # regenerate golden files after render changes
go build -o mt5-pnl-cli .
go run github.com/goreleaser/goreleaser/v2@latest check   # validate .goreleaser.yaml
```
```

with:

```markdown
```bash
go test ./...                 # all tests (CI runs -race; ubuntu leg uploads coverage to codecov)
go test ./internal/render -update   # regenerate golden files after render changes
go build -o mt5-pnl-cli .
go run github.com/goreleaser/goreleaser/v2@latest check   # validate .goreleaser.yaml
pre-commit install            # gitleaks secret-scan hook (one-time)
pre-commit run --all-files    # run the gitleaks hook manually
```
```

- [ ] **Step 2: Commit**

```bash
git add CLAUDE.md
git commit -m "docs: sync CLAUDE.md commands with pre-commit and coverage"
```

---

### Task 7: Verification and PR

- [ ] **Step 1: Full local verification**

```bash
test -z "$(gofmt -l .)" && go vet ./... && go test -race ./... && pre-commit run --all-files && echo ALL OK
```

Expected: `ALL OK` (gitleaks `Passed` within).

- [ ] **Step 2: Link check.** Confirm each file referenced from the three docs exists in the repo: `LICENSE`, `CONTRIBUTING.md`, `SECURITY.md`, `CLAUDE.md`, `renovate.json`, `schema/snapshot.schema.json`, `.github/workflows/release.yml`. External links (exporter README/threat model/SECURITY, Renovate docs, codecov badge) are eyeballed.

- [ ] **Step 3: Push and open the PR**

```bash
git push -u origin docs/consumer-docs
gh pr create --title "docs: consumer documentation set" --body "Implements the approved consumer-docs spec (docs/superpowers/specs/2026-06-13-consumer-docs-design.md):

- README reworked consumer-first (art-of-readme): pipeline diagram, real fixture-generated demo output, threat model
- CONTRIBUTING and SECURITY mirroring the mt5-pnl-exporter house style, adapted to Go/GoReleaser
- GitHub Actions pinned to commit SHAs (so SECURITY.md's claim is true from day one)
- Coverage upload to codecov on the ubuntu CI leg + badge (CODECOV_TOKEN already configured)
- gitleaks pre-commit hook

🤖 Generated with [Claude Code](https://claude.com/claude-code)"
```

Expected: PR opens; CI goes green including the new coverage upload; codecov badge populates after merge.

---

## Verification checklist (after all tasks)

- [ ] CI green on the PR, coverage upload step succeeded.
- [ ] Every README command/flag exists in the binary's usage output.
- [ ] Demo blocks are byte-identical to fixture-generated output.
- [ ] `pre-commit run --all-files` clean.
- [ ] SECURITY.md claims verified against repo state (pins, CGO, Renovate preset).
