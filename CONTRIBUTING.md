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
