# AGENTS.md

## The bar

The owner's words, which bind every change:

- "I demand super tight, simple and efficient code. We do not do
  over-engineering, we do not write tons of code to remedy far fetched edge
  cases that will never happen under normal usage, however we do not take
  lightly on security. The goal is always the best product possible.""

## Conventions

- Conventional commits with a scope: `type(scope): summary`, with bullet lists
  in bodies.
- No attribution lines, and no session links, task or milestone numbers, or plan
  references in commits, branch names, code comments or documentation.
- No em-dashes anywhere.
- Tests exercise behaviour and never assert on prose.
- One branch per pull request, one commit per logical change. The
  orchestrating session may push feature branches and open pull requests.
  Never push to `main`, and never merge without the owner's word.

## Documentation

- Documentation must always be true: every statement matches the code at HEAD,
  and a change that makes a doc line false updates it in the same commit.
- Document the current state - what exists now and why, not how it got here.
- Comments explain only non-obvious decisions, constraints, safety invariants,
  or workarounds. Do not narrate what code does or record change history; git
  holds history. Keep comments to one or two tight lines unless a longer
  explanation is genuinely necessary. Rewrite or delete if you find comments
  that don't follow this rule.

## Commands

- `make check` runs build-go, fmt-check, vet, lint, test-race, tidy-check, scan
  and size. Run it before a commit. It needs Go, a C compiler for the race
  detector, and golangci-lint, and no Node.
- `make build` and `make build-go` build `bin/recallsynapsis`; `make install`
  installs `recallsynapsis` into `GOBIN`, else `$(go env GOPATH)/bin`.
  `VERSION=x.y.z` on any of them stamps the version `recallsynapsis version`
  prints; without it a build prints the commit.
- `make test`, `make test-race`: the tests, the second with the race detector.
- `make vet`, `make lint`, `make fmt`, `make fmt-check`, `make tidy-check`: the
  static checks and the formatter (`golangci-lint fmt`: gofumpt and goimports).
- `make scan`: fail on an em-dash in any tracked or untracked, unignored text
  file. `make scan-commits`: in the commits from `SCAN_BASE` to `SCAN_HEAD`, by
  default `origin/main` and `HEAD`, fail on task, step or milestone numbers in
  messages, added comments and added Markdown lines, on session links in
  messages and added lines, on attribution lines in messages, on em-dashes in
  messages and added lines, and on blobs over 500 KB.
- `make size`: count the non-test Go lines. `make help` lists the targets.
- CI (`.github/workflows/ci.yaml`) runs the gates of `make check`,
  `govulncheck`, and on a pull request `make scan-commits` against its base.
  Dependabot bumps the Go modules and the pinned actions every week.

## Layout

Every package states its one responsibility in its package comment, and
`go doc` on its directory, such as `go doc ./internal/store`, prints it.

- `cmd/recallsynapsis`: the binary. `main.go` holds the verb registry and
  `run(args, stdin, stdout, stderr)`, which the tests call in process; each
  verb registers itself from an `init` in its own file.
- `internal/store`: opens the SQLite database, applies the migrations each
  package that owns tables brings, keyed by that owner and a version, and
  hands every package the one handle. It owns no table but the migration ledger.
- `scripts/`: the scans and the line count that `make` runs.
