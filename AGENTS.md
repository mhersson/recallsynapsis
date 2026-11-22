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
  `govulncheck`, `make web-check` and `npm audit`, and on a pull request
  `make scan-commits` against its base. Dependabot bumps the Go modules, the
  web app's npm packages, the pinned actions and the image's base images
  every week.

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
  whose text starts with TEXT, and `type=TEXT` types TEXT into the field
  that has the focus; given more than once, they run in the order given.
  `login=wrong` refuses a sign-in and `login=wait` makes it wait;
  `lapsed=1` ends the access to the page; `empty=1` starts with the
  personal area alone and no token; `modules=notes` switches the named
  modules on, so Search finds the fixed hits; `backup=old` makes the last
  backup three days old and `backup=none` leaves none. What the page
  changes holds until it reloads.
- Read the PNG and say what you see. Fix what looks wrong and shoot again.
- Stop Vite by the pid you recorded, never by port or name.

## Layout

Every package states its one responsibility in its package comment, and
`go doc` on its directory, such as `go doc ./internal/store`, prints it.

- `cmd/recallsynapsis`: the binary. `main.go` holds the verb registry and
  `run(args, stdin, stdout, stderr)`, which the tests call in process; each
  verb registers itself from an `init` in its own file. `datadir.go` holds
  `modules`, the list of the binary's feature modules, empty but for the fake
  the tests add, and every verb that opens the database builds them all, so
  every module's migrations run whether it is on or not. `serve` gives the
  modules the hub of the event streams, whose streams end as a signal begins
  the shutdown, runs the jobs of the modules switched on and the nightly
  backup, and stops them before the store closes.
- `internal/api`: the REST API under `/api/`, served from one route table,
  with the routes of the modules switched on beside the core's. Every refusal
  of a route and every path no route matches is answered as
  `{"error": "one line"}`; a path with `.`, `..` or `//` is first redirected
  to its clean form. Request bodies are decoded strictly. Each route declares
  its access, `open`, `read`, `write` or `owner`, and one check admits a
  bearer token, else the session cookie, refuses a cookie request that
  changes something without the `X-Recallsynapsis-CSRF` header, and puts the
  `module.Caller` in the request's context. At `GET /api/events` one hub,
  which the area routes and the modules' `Deps.Changed` tell, streams each
  change to the open streams of the user whose change it is, and keeps none.
  A stream ends when it falls behind, rather than hold up a writer, when the
  server stops, and after an hour, so its access is checked again when it is
  opened again. At `/mcp` it serves MCP over streamable HTTP, stateless, with
  JSON responses and tools only: the core's `list_areas` and `search`, each
  calling what its REST route calls, and the tools of the modules switched
  on; two tools of one name stop the start. The SDK's bearer check admits an
  API token alone, so a session cookie is no authority there, and one guard
  refuses a call the token's access does not cover before the tool runs, puts
  the token's `module.Caller` in the tool's context, and gives an error as the
  line REST would answer.
- `internal/auth`: signs a user in with a name and an argon2id password,
  slows wrong ones by the name's digest and the address, and keeps the
  sessions and API tokens, both stored only as hashes. It owns the tables
  `users`, `sessions` and `api_tokens`.
- `internal/backup`: backs up the data directory into a private directory,
  once or every night at `backup.at`: a snapshot of the database by
  `VACUUM INTO`, named by its day, the files under `files/` that the backup
  lacks, and no more snapshots than it keeps. Of what it finds there it
  removes only regular files with exactly a snapshot's name. It tells when
  the newest snapshot was made, and owns no table.
- `internal/config`: reads the configuration file strictly: an unknown key, a
  wrong type or an unusable value is refused with the key named. It keeps
  each `[modules.NAME]` table as read, and decodes one as strictly for its
  module: `enabled`, and the rest into the module's own settings. A module
  with no table is off.
- `internal/core`: keeps the areas: each user's one personal area, which keeps
  its name and is never archived, and their projects, each with a name unique
  in any case and a slug made from it. It finds one of a user's areas by its
  id, its slug or its name. It builds the feature modules the binary lists: it
  decodes each one's table, refuses a table no module has, a module route
  that is open and a module tool that asks for neither read nor write, and
  applies every module's migrations, on or off, after auth's and its own.
  Over the modules switched on it searches, grouping the hits by module,
  giving each its area's slug and failing on a hit outside the caller's
  areas, resolves a reference, unavailable rather than an error when no
  module switched on has the item, and finds what links to one. It owns the
  table `areas`.
- `internal/module`: what the core and its feature modules share. The
  contract: the `Module` a feature module's constructor returns, with its
  settings, migrations, routes, MCP tools, jobs, and optionally its search,
  the title of an item for a link and what links to a reference; the `Deps`
  it is built with; and the shapes they exchange, among them the `Area`.
  `NewTool` makes a tool from a typed function, so no module touches the MCP
  SDK. Beside it: the `Caller` an authorised request carries in its context,
  with the user, the source of the request and whether it may write; the
  access levels a route or a tool declares; and the refusal, the JSON reply
  and the strict decoding of a body that every route uses, with the 1 MiB
  cap on a body that `/mcp` keeps too.
  Depguard rules in `.golangci.yaml` let a module package's code import
  `module` and `store` and no other package of this binary, let nothing but
  `api` and `cmd` import a module, and let only tests import the fake; tests
  are otherwise exempt.
- `internal/store`: opens the SQLite database, applies the migrations each
  package that owns tables brings, keyed by that owner and a version, and
  hands every package the one handle. It owns no table but the migration ledger.
- `internal/testkit`: `fake`, a feature module as small as the tests of what
  serves modules need: a table, a route and a write tool that add to it and
  tell of what they add, a search of it, and a job that fails on every tick.
  Only tests register it.
- `web`: the web app's npm package, with its sources in `web/src`, and the Go
  package that embeds its build from `web/dist/app` and serves it in front of
  the API. A binary built without the app serves the committed
  `web/dist/placeholder.html` in its place. Each module of the web app is one
  folder under `web/src/modules/`, listed in `web/src/modules/registry.ts`.
- `scripts/`: the scans and the line count that `make` runs.
- `Dockerfile`: Node builds the web app, Go builds the binary with it
  embedded, and a distroless static image runs it as user 65532. Every base
  image is pinned by digest. `--build-arg VERSION=x.y.z` stamps the version.
