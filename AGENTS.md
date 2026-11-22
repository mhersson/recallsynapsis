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
- `make build` builds the web app, then `bin/recallsynapsis` with it embedded;
  `make build-go` builds `bin/recallsynapsis` alone, with the app as built
  last, or the placeholder page when none is; `make install` builds the web
  app, then installs `recallsynapsis` into `GOBIN`, else
  `$(go env GOPATH)/bin`. `VERSION=x.y.z` on any of them stamps the version
  `recallsynapsis version` prints; without it a build prints the commit.
- `make web-install` installs the web app's npm packages once (`npm ci` in
  `web/`, with the Node of `.nvmrc`); `make web-build` builds the app into
  `web/dist/app`; `make web-check` lints, tests and builds it. Run
  `make web-check` when `web/` changes.
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

## Looking at the web app

After a change to `web/`, look at the screen before you finish.

- Run `make web-install` once, then start Vite in `web/` on a loopback port in
  the background and record its pid:
  `./node_modules/.bin/vite --host 127.0.0.1 --port 5197 --strictPort > /tmp/vite.log 2>&1 & echo $!`.
  Started through `npx`, the pid would be npm's, and stopping it would leave
  Vite running.
- Take the shot from `web/` with headless Firefox, which opens no window:
  `node shoot.mjs 1280 900 "http://127.0.0.1:5197/harness.html?palette=copper&theme=dark" /tmp/shot.png`.
  Use `390 844` for a phone.
- `web/harness.html` shows the app on the fixed data of
  `web/src/dev/fixtures.ts`. Its `palette` is `copper` or `catppuccin`, its
  `theme` is `dark` or `light`, and its `path` picks the route, for example
  `path=/settings`. `click=TEXT` clicks the first button, link or summary
  whose text starts with TEXT; `login=wrong` refuses a sign-in and
  `login=wait` makes it wait; `lapsed=1` ends the access to the page.
- Read the PNG and say what you see. Fix what looks wrong and shoot again.
- Stop Vite by the pid you recorded, never by port or name.

## Layout

Every package states its one responsibility in its package comment, and
`go doc` on its directory, such as `go doc ./internal/store`, prints it.

- `cmd/recallsynapsis`: the binary. `main.go` holds the verb registry and
  `run(args, stdin, stdout, stderr)`, which the tests call in process; each
  verb registers itself from an `init` in its own file.
- `internal/api`: the REST API under `/api/`, served from one route table.
  Every refusal and every path no route matches is answered as
  `{"error": "one line"}`; a path with `.`, `..` or `//` is first redirected
  to its clean form. Request bodies are decoded strictly.
- `internal/config`: reads the configuration file strictly: an unknown key, a
  wrong type or an unusable value is refused with the key named. It keeps
  each `[modules.NAME]` table as read.
- `internal/store`: opens the SQLite database, applies the migrations each
  package that owns tables brings, keyed by that owner and a version, and
  hands every package the one handle. It owns no table but the migration ledger.
- `web`: the web app's npm package, with its sources in `web/src`, and the Go
  package that embeds its build from `web/dist/app` and serves it in front of
  the API. A binary built without the app serves the committed
  `web/dist/placeholder.html` in its place. Each module of the web app is one
  folder under `web/src/modules/`, listed in `web/src/modules/registry.ts`.
- `scripts/`: the scans and the line count that `make` runs.
