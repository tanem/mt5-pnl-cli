# mt5-pnl-cli consumer documentation design

Date: 2026-06-13
Status: awaiting review

## Purpose

Bring this repo's documentation up to the family standard set by
mt5-pnl-exporter: a consumer-first README in the
[art-of-readme](https://github.com/hackergrrl/art-of-readme) style, plus
CONTRIBUTING.md and SECURITY.md mirroring the exporter's structure —
adapted honestly to what this repo actually does (Go, GoReleaser, no
package index, no coverage gate).

Two tooling additions ride along, both agreed during design: codecov
coverage reporting (badge parity with the exporter) and the gitleaks
pre-commit hook (secret-scan parity).

## Deliverables

1. `README.md` — full rework (current one is a 66-line skeleton).
2. `CONTRIBUTING.md` — new.
3. `SECURITY.md` — new.
4. `.pre-commit-config.yaml` — gitleaks hook, copied from the exporter.
5. `.github/workflows/ci.yml` — coverage upload added to the ubuntu test
   leg; codecov action.
6. `CLAUDE.md` — doc-sync touch-ups only (mention pre-commit and the
   coverage step in Commands; nothing else changes).

One PR off branch `docs/consumer-docs`.

## README design

Section order (art-of-readme: identity → picture → why → install → use →
internals → trust):

1. **Header**: title; one-liner blockquote ("Query MT5 P&L from an
   encrypted mt5-pnl-exporter snapshot. One static binary — no daemon, no
   database, no third-party service."); badges: licence, ci, codecov.
2. **ASCII pipeline diagram**, consumer's side, visually rhyming with the
   exporter's: exporter (Windows host) → `snapshot.json.gz.age` (synced
   via Dropbox/Syncthing/etc.) → mt5-pnl-cli (macOS/Linux/Windows). One
   short paragraph beneath: exporter writes one encrypted file; this CLI
   decrypts it in memory and prints tables or JSON.
3. **Contents** ToC.
4. **Why**: four bullets — self-hosted (data never touches a third-party
   dashboard); no config file (point at the snapshot once, `pnl` just
   works); agent- and script-friendly (`--json`, warnings to stderr,
   example: an agent turns *"show me monthly P&L for Q1"* into
   `mt5-pnl-cli pnl --from 2026-01-01 --to 2026-03-31 --by month --json`);
   secrets stay in the OS keychain (no env var or flag for the
   passphrase, deliberately).
5. **Install**: GitHub Releases binaries (darwin/linux/windows,
   amd64/arm64, `checksums.txt`) and `go install
   github.com/tanem/mt5-pnl-cli@latest`.
6. **Quick start**: `set-passphrase` — stating explicitly it must be **the
   same passphrase given to the exporter's `set-encryption-passphrase`**
   (the one cross-repo gotcha every new consumer hits); `export
   MT5_PNL_SNAPSHOT=...`; bare `mt5-pnl-cli pnl`; `mt5-pnl-cli accounts`.
7. **Demo**: real output generated from the test fixture (already
   captured): the `pnl` table for `--from 2026-01-01 --to 2026-01-31`,
   the `accounts` table, and a `--json` sample (rows truncated to the
   first element for brevity, summary in full). Commands shown exactly as
   run; no hand-typed output.
8. **Commands**: `pnl` (range: `--last Nd|Nw|Nm|Ny`, default `30d`,
   calendar-accurate; or `--from [--to]`, `--to` defaulting to today;
   `--by day|week|month`, default week, Monday weeks, UTC dates;
   `--accounts` label filter, case-insensitive; `--json`); `accounts`;
   `set-passphrase` (prompted twice, never echoed); `version` (binary +
   supported schema version). Shared flags: `--snapshot` overrides
   `MT5_PNL_SNAPSHOT`; `--stale-after` (default 2h) staleness warning to
   stderr only, so `--json` pipelines stay clean.
9. **How it works**: one-line pipeline diagram (`age decrypt → gunzip →
   JSON → aggregate`, ciphertext on disk, plaintext in memory only) +
   aggregation semantics: net per deal = profit + swap + commission +
   fee; win is net > 0, loss net < 0, breakeven counts toward trades
   only; full-precision sums rounded at display; profit factor = gross
   profit / |gross loss|.
10. **Schema compatibility**: vendored `schema/snapshot.schema.json`;
    accepts same major, minor ≤ supported; refusal names both versions;
    `version` prints the supported schema.
11. **Threat model**: trust boundary = OS user session, same as the
    exporter. *What's protected*: snapshot stays encrypted at rest (no
    decrypted copy ever written; sync/backup sees ciphertext);
    passphrase keychain-only with the env-var-leakage rationale;
    `MT5_PNL_SNAPSHOT` carries only a path. *What's not protected*:
    compromised user session (can read keychain and run the CLI);
    redirected output (tables/JSON contain balances and history). Links
    to the exporter's threat model and SECURITY.md.
12. **Contributing** pointer and **Licence**.

## CONTRIBUTING.md design

Exporter skeleton, Go content:

- Intro: small project, issues and PRs welcome.
- **One-time setup**: Go ≥1.25; `pre-commit install` (gitleaks hook).
- **Running tests and checks**: `go test -race ./...`, gofmt check,
  `go vet ./...`, `golangci-lint run`, `pre-commit run --all-files`.
- **Golden files**: render table output is asserted byte-for-byte against
  `internal/render/testdata/*.golden`; after intentional output changes
  run `go test ./internal/render -update`, eyeball the diff, commit the
  goldens with the change. Note the `.gitattributes` `-text` rule that
  protects them from EOL conversion.
- **Smoke-test against a real snapshot**: build from the working tree
  (`go build -o mt5-pnl-cli .`), `./mt5-pnl-cli set-passphrase` (exporter
  passphrase), run `accounts` and `pnl` against the real synced snapshot
  — the consumer-side mirror of the exporter's "smoke-test a real
  export".
- **Dependency updates**: Renovate manages Go modules and GitHub Actions
  (SHA-pinned via `helpers:pinGitHubActionDigests`); digest/minor/patch
  auto-merge on green CI, majors open a PR; don't hand-bump.
- **Testing a release build**: `goreleaser check` and
  `goreleaser build --snapshot --clean --single-target` (via
  `go run github.com/goreleaser/goreleaser/v2@latest`), then run the
  binary from `dist/`.
- **Releasing**: tag `vX.Y.Z` → `git push origin vX.Y.Z` → `release.yml`
  runs GoReleaser, which builds all platform binaries, generates
  `checksums.txt` and the changelog, and publishes the GitHub Release.
  Explicitly note this is simpler than the exporter's flow: no package
  index, no OIDC, no manual approval gate; the tag is the trigger and
  the GitHub Release is the artefact.
- **Conventions**: pointer to CLAUDE.md as canonical (NZ English, TDD,
  doc-sync rule, no-config-file and keychain-only invariants).

## SECURITY.md design

Exporter's three-part skeleton:

- **Scope**: this tool handles one secret — the snapshot decryption
  passphrase, stored in the OS keychain (go-keyring: macOS Keychain /
  Windows Credential Manager / Linux Secret Service), never accepted via
  argument, env var, file, or written to disk/logs. The decrypted
  snapshot exists in memory only. In scope: passphrase disclosure in any
  output; unsafe keychain read/write behaviour; the decrypt pipeline
  (age + gzip) bypassed or weakened; account data leaking anywhere it
  shouldn't; dependency vulnerabilities with a plausible exploitation
  path here. Out of scope: the exporter and MT5 infrastructure (exporter
  has its own policy); a compromised OS user session (the documented
  trust boundary); issues only reproducible on non-current Go.
- **Supply-chain controls**: GitHub Actions pinned to commit SHAs, kept
  current by Renovate; Renovate update PRs gated on the full CI matrix
  before auto-merge; release binaries are static (`CGO_ENABLED=0`),
  built in CI by GoReleaser from the tagged commit, with `checksums.txt`
  published alongside. No claims beyond what the repo actually does.
- **Reporting**: no public issues for vulnerabilities; private reporting
  via this repo's Security tab (advisories/new link); response within 7
  days; coordinated disclosure date agreed before publishing.

## Tooling riders

- **`.pre-commit-config.yaml`**: gitleaks hook, same pin/style as the
  exporter's config (copy and verify hook revision is current-ish; let
  Renovate manage it thereafter if supported, else note manual bumps).
- **ci.yml coverage**: on the ubuntu test leg only, run
  `go test -race -coverprofile=coverage.out ./...` and upload via the
  codecov action with `CODECOV_TOKEN`. Reporting only — **no fail-under
  gate**, and the README badge must not imply one; unlike the exporter,
  this repo has not adopted a coverage policy.
- **Pin GitHub Actions to commit SHAs in this PR** (exporter style: SHA
  with trailing version comment). The workflows currently reference
  mutable tags and no Renovate pin PR has landed, so SECURITY.md's
  pinning claim would otherwise be false on day one. Renovate keeps the
  pins current thereafter.
- **Manual steps for the maintainer**: (1) enable the repo on codecov.io
  and add `CODECOV_TOKEN` to the repo's Actions secrets (required while
  the repo is private — CI's coverage upload fails without it);
  (2) confirm the Renovate app is enabled for this repo (it was set up
  for mt5-pnl-exporter; new repos aren't necessarily covered).

## Verification

Docs are prose; verification is mechanical where possible:

- Every command and flag mentioned in README/CONTRIBUTING cross-checked
  against the binary's actual `--help`/usage output.
- Demo blocks byte-identical to output generated from the test fixture
  (regenerate with a temporary harness; never hand-edit).
- `pre-commit run --all-files` clean; CI green including the new
  coverage upload; codecov badge renders.
- Markdown link check by hand (six internal links, four external).

## Out of scope

- Coverage gates or thresholds.
- Restructuring CLAUDE.md beyond the doc-sync touch-ups listed.
- Public-repo rollout steps (release badge, pkg.go.dev links) — deferred
  until the repo flips public.
