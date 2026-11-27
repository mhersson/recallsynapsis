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
- `make test`, `make test-race`: the tests, the second with the race detector,
  at `GOMAXPROCS=4` unless `GOMAXPROCS` is given on the command line or in the
  environment, since every sign-in in a test is a memory-hard argon2 verify
  and Go otherwise runs as many packages, and tests in each, at once as there
  are cores. A bare `go test ./...` is not capped, and on a machine with many
  cores it can exhaust the memory.
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
  `make scan-commits` against its base. On a push to `main` or a manual run,
  once those pass, it builds the image on the self-hosted runner labelled
  `recallsynapsis-image` and pushes it to the registry the `REGISTRY`
  variable names, tagged with the short commit and `latest`, or for a manual
  run with its branch and the short commit. A self-hosted runner is safe on
  this public repository only while a pull request from anyone outside it
  waits for approval before its workflows run, so the Actions setting that
  requires approval for all external contributors must stay, and no
  `pull_request_target` workflow, which that setting never holds back, may
  run on it. Dependabot bumps the Go modules, the web app's npm packages,
  the pinned actions and the image's base images every week.

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
  `web/src/dev/fixtures.ts`, whose tasks are on a fixed day, Monday 23
  November 2026. Its `palette` is `copper` or `catppuccin`, its
  `theme` is `dark` or `light`, and its `path` picks the route, for example
  `path=/settings`. `click=TEXT` clicks the first button, link, summary or
  label whose text, or label, starts with TEXT, in the dialog open last when
  one is, `type=TEXT` types TEXT into the field that has the focus, and
  `choose=N` chooses an image of N bytes in the file picker of the note
  being read; given more than once, they run
  in the order given, and `end=1` then scrolls the page, and what is open
  beside a list, to their end.
  `login=wrong` refuses a sign-in and `login=wait` makes it wait;
  `lapsed=1` ends the access to the page; `empty=1` starts with the
  personal area alone, no token, no note and no task; `many=1` adds older
  notes and done tasks, so a list holds more than it shows; `today=none`
  leaves out the tasks Today would show; `save=moved` refuses the first
  save of each note's text, as if a client had saved the note meanwhile;
  `upload=wait` holds every upload, so it stays under way;
  `modules=tasks,notes` switches the named modules on, so Tasks and Search
  find the fixed tasks, and the Inbox, Notes and Search the fixed notes;
  `backup=old` makes the last backup three days old and `backup=none`
  leaves none; with `push` among `modules=`, `push=off`, `on`, `failed`,
  `home`, `refused` or `none` makes this browser one whose notifications
  are off, on, or on with the last one failed, an iPhone's Safari tab, one
  that refuses notifications, or one with no Web Push, with two other
  browsers on but for `off`; with `tasks` and `push` among `modules=`,
  Settings shows the kinds of notification at their first values, and
  `notify=refused` refuses every change of them. What the page changes holds
  until it reloads, and a change of a note, a task, a browser or a kind of
  notification reaches the event stream, as the server's does.
- Read the PNG and say what you see. Fix what looks wrong and shoot again.
- Stop Vite by the pid you recorded, never by port or name.

## Layout

Every package states its one responsibility in its package comment, and
`go doc` on its directory, such as `go doc ./internal/store`, prints it.

- `cmd/recallsynapsis`: the binary. `main.go` holds the verb registry and
  `run(args, stdin, stdout, stderr)`, which the tests call in process; each
  verb registers itself from an `init` in its own file. `datadir.go` holds
  `modules`, the list of the binary's feature modules, which is notes, tasks
  and push, with the fake after it in the tests, and every verb that opens the
  database builds them all, so every module's migrations run whether it is on
  or not, and gives them the configuration's `public_url`.
  `serve` gives the modules the hub of the event streams, whose streams end
  as a signal begins the shutdown, runs the jobs of the modules switched on
  and the nightly backup, and stops them before the store closes.
- `internal/api`: the REST API under `/api/`, served from one route table,
  with the routes of the modules switched on beside the core's. Every refusal
  of a route and every path no route matches is answered as
  `{"error": "one line"}`, save what `http.ServeContent` refuses itself as it
  serves a note's file, such as a range it cannot serve; a path with `.`, `..`
  or `//` is first redirected to its clean form. A JSON body is decoded
  strictly. Each route declares its access, `open`, `read`, `write` or
  `owner`, and one check admits a bearer token, else the session cookie,
  refuses a cookie request that changes something without the
  `X-Recallsynapsis-CSRF` header, and puts the `module.Caller` in the
  request's context. A change of an area and the restore of its brief ask for
  the session cookie alone, so no token renames, archives or changes the
  brief of one over REST; a `write` token makes a project. At
  `GET /api/events` one hub, which the area routes and the modules'
  `Deps.Changed` tell, streams each change to the open streams of the user
  whose change it is, and keeps none. A stream ends when it falls
  behind, rather than hold up a writer, when the server stops, and after an
  hour, so its access is checked again when it is opened again. At `/mcp` it
  serves MCP over streamable HTTP, stateless, with JSON responses and tools
  only: the core's `list_areas`, which gives each area's brief whole, and
  `search`, each calling what its REST route calls, and the tools of the
  modules switched on; two tools of one name stop the start. The SDK's
  bearer check admits an API token alone, so a session cookie is no authority
  there, and one guard refuses a call the token's access does not cover
  before the tool runs, puts the token's `module.Caller` in the tool's
  context, and gives an error as the line REST would answer.
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
  in any case and a slug made from it. Each area has a brief, Markdown of at
  most 4,000 characters, kept in the column `description`, so an image rolled
  back still reads the areas, and every wording of it is kept as a revision.
  A change of the brief names the revision it was made from and is refused
  once the brief has moved on, and it writes the next revision with what
  changed it, unless it leaves the brief as it is; a change of the name or the
  archived mark writes none. Any revision can be read, and restoring one
  writes a new one that names it, unless the brief already has that wording.
  It finds one of a user's areas by its id, its slug or its name. It builds the
  feature modules the binary lists: it decodes each one's table and checks its
  settings, on or off, refuses a table no module has, a module route that is
  open and a module tool that asks for neither read nor write, and applies
  every module's migrations, on or off, after auth's and its own.
  Over the modules switched on it searches, grouping the hits by module,
  giving each its area's slug and failing on a hit outside the caller's
  areas, resolves a reference, unavailable rather than an error when no
  module switched on has the item, finds what links to one, and hands a
  message for a user to each module that delivers. It owns the tables `areas`
  and `area_revisions`.
- `internal/module`: what the core and its feature modules share. The
  contract: the `Module` a feature module's constructor returns, with its
  settings and their check, migrations, routes, MCP tools, jobs, and
  optionally its search, an item as a link shows it, what links to a
  reference, and the delivery of a message to a user; the `Deps` it is built
  with, among them `Notify`, which sends a message to a user through the
  modules switched on that deliver and returns nothing, and `PublicURL`; and
  the shapes they exchange, among them the `Area`, with its brief, the brief's
  revision, when it changed and what changed it, and the `Message`: a title,
  a text, the path a tap opens and a tag, by which a newer message replaces
  an older one.
  `NewTool` makes a tool from a typed function, so no module touches the MCP
  SDK. Beside it: `Capped`, which refuses a text a tool is given over a module's
  cap, counted in characters; the `Caller` an authorised request carries in its
  context, with the user, the source of the request and whether it may write;
  the access levels a route or a tool declares; the links an item keeps, at most
  100 references, each well formed and kept once, which `Deps.Items` reads as
  the core resolves them; and the refusal, the JSON reply and the strict
  decoding of a JSON body that every route taking one uses, with the 1 MiB cap
  on a body that `/mcp` keeps too.
  Depguard rules in `.golangci.yaml` let a module package's code import
  `module` and `store` and no other package of this binary, let nothing but
  `api` and `cmd` import a module, and let only tests import the fake; tests
  are otherwise exempt.
- `internal/notes`: the notes module. It keeps each user's notes and ideas, each
  in one of the user's areas, named by the area's slug, with its tags,
  lower-cased without any `#` they start with, its links, at most 100, each a
  reference to an item of any module, and the inbox flag of a capture that waits
  to be filed. A change of area, kind or tags files a note unless the same
  change puts it in the inbox, and a note in the inbox is in the personal area.
  A change names the revision it was made from and is refused once the note has
  moved on, unless it only appends, and every change writes a revision with its
  source. Any revision can be read, and restoring one writes a new one with its
  title, body, tags and links, leaving the area, the kind and the inbox flag as
  they are. A note read in full gives its links and the items that link to it,
  each as the core resolves it, and the `said` of its first revision. A link to
  a note in the trash or deleted for good leads nowhere. It searches its notes
  for every word of a query, in title, body and tags and in any case, through an
  FTS5 index with the trigram tokenizer, and looks for a word under three
  characters, which the index cannot find, in the text of the notes themselves.
  Every write keeps the index level, it keeps no copy of the text, and deleting
  a note for good rewrites it, so nothing of the note is left there. A note in
  the trash is in no list but the trash's, is found by no search, counts for no
  tag and cannot be changed. Trashing and restoring it write no revision, and
  only the session cookie deletes it for good, alone or by emptying the trash. A
  note's files, at most `file_max_mib` each, are attached, read and removed with
  the session cookie alone, and attaching and removing one write no revision. An
  upload is written under `tmp/`, hashed as it arrives, and renamed into
  `files/` under its SHA-256, so `files/` holds only whole contents, each once,
  shared by every user; the contents go once the last attachment that names them
  is removed or deleted with its note for good. A file keeps the type it was
  declared as, and is served with `nosniff`, a `sandbox` content policy and to
  its own origin alone: inline as its type only when it is a PNG, JPEG, GIF or
  WebP image, else as a download of `application/octet-stream`, so no page loads
  one as a script or a style. Its MCP tools, `create_note`, `list_notes`,
  `get_note`, `update_note` and `trash_note`, each call what a REST route calls,
  find no note in the trash, and refuse a text over `capture_max_chars`
  characters, which REST does not cap. `create_note` files a capture in the area
  it names, an idea that names none in the personal area and any other in the
  inbox, and keeps its `said` on the first revision. It owns the tables `notes`,
  `note_tags`, `note_revisions`, `note_links`, `notes_fts` and `note_files`.
- `internal/push`: the push module. It delivers a message to every browser of
  its user, one after the other, as Web Push with no push library: encrypted
  for the browser's keys (RFC 8291), its JSON at most 2048 bytes, its text cut
  at a character boundary to fit, and posted with a token signed with the
  server's key (RFC 8292) that names `public_url` as the contact, at high
  urgency, to wait until the day ends in the configured `timezone`, with its
  tag as the topic. The client waits at most 30 seconds and follows no redirect.
  A push service answering 404 or 410 means the browser is gone, and it is
  forgotten; any other failure is logged and kept on the browser's row as one
  line that names the push service's host and never the address, which is a
  capability, until a message goes through. The key is one P-256 key, made the
  first time it is needed and kept in the database. Its routes, all for the
  session cookie alone, give the public key, the count of the caller's browsers
  and one of them by its id, register a browser by its address, unique, which
  makes it the caller's, and turn one or all of the caller's off, each change
  told to the caller's open streams. It owns the tables `push_subscriptions` and
  `push_key`.
- `internal/store`: opens the SQLite database, applies the migrations each
  package that owns tables brings, keyed by that owner and a version, and
  hands every package the one handle. Beside it is what the modules' searches
  share: a search's text parted into its words, each of three characters or
  more quoted as a phrase for an FTS5 index with the trigram tokenizer and
  each shorter one looked for in the text read back, in any case, and a hit's
  snippet, placed by those words. It owns no table but the migration ledger.
- `internal/tasks`: the tasks module. It keeps each user's tasks, each in one of
  the user's areas, named by the area's slug, with its notes, a planned period,
  a deadline, a list, `soon` or `someday`, when it was done, and its links, at
  most 100, each a reference to an item of any module, which a change gives
  whole. A date is text, `YYYY-MM-DD` or `""` for none, so dates compare as
  text, and an answer gives each date's weekday beside it. A from alone plans
  one day, a to before its from and a time without a plan are refused, and a
  from cleared clears the plan whole. A task is `done`, `unsorted` when it is
  open with no list, no plan and no deadline, or `open`. A list gives the open
  tasks unless asked for the done, by area, by list, by a span of dates, which a
  plan overlaps or a deadline lies in, or the unsorted alone, and gives `now`:
  the date, its weekday and the time in the configured `timezone`, and
  `week_start`. A call makes 1 to 50 tasks, all or none, with one `said` kept on
  the making of each. A change gives only its fields and the last one wins. Each
  change writes one entry of the task's log, with its source and what each field
  was and became, and of the notes and the links only that they changed; a
  change that changes nothing writes none. A task in full, wherever one is
  answered, gives its links and the items that link to it, each as the core
  resolves it, and `now`, and a link to a task in the trash or deleted for good
  leads nowhere. It searches its tasks, done and open, for every word of a
  query, in title and notes and in any case, as notes does, through an FTS5
  index with the trigram tokenizer, which every write keeps level, which keeps
  no copy of the text, and which deleting a task for good rewrites. A task in
  the trash is in no list but the trash's, which gives the done and the open
  alike, the most recently trashed first, is found by no search and cannot be
  changed. Trashing and restoring write no entry of the log, and only the
  session cookie deletes a task for good, alone or by emptying the trash. Its
  MCP tools, `create_tasks`, `list_tasks`, `get_task`, `update_task` and
  `trash_task`, each call what a REST route calls, find no task in the trash,
  and refuse notes or a `said` over `capture_max_chars` characters, which REST
  does not cap. A call of `create_tasks` or `update_task` that sets a date is
  refused, and saves nothing, unless its `today` is the date in the configured
  `timezone`, with a line that gives that date, its weekday, the time zone and
  `week_start`; REST takes no `today`. Its job, every 30 seconds, keeps the
  moment it has notified up to and hands `Deps.Notify`, for the user each task
  is of, what came due after that moment and not after now on today's date in
  the configured `timezone`, of the open tasks outside the trash: at a task's
  start time on the first day of its plan, its title over its time and its
  area's name, but the personal area's; at the morning hour, as the Today page
  sorts them, how many are today's, left from earlier and overdue, and their
  titles, those that start at a time today first, by time; at the deadline hour,
  the tasks due tomorrow, and at the planned-today hour, those planned for today
  with no start time today, each kind as one message that opens the task when it
  names one alone. A run after the server was down sends what it missed of
  today, and nothing of a day before, the very first sends nothing, and one the
  stop cuts short keeps no moment. A user's four kinds, start time and morning
  on, at 07:00, and deadline and planned today off, at 18:00 and 09:00, until
  the user changes one, are read and changed at `/api/tasks/notify` with the
  session cookie alone, each change told to the user's open streams. It owns the
  tables `tasks`, `task_changes`, `task_links`, `tasks_fts`, `task_notify` and
  `task_notified`.
- `internal/testkit`: `fake`, a feature module as small as the tests of what
  serves modules need: a table, a route and a write tool that add to it and
  tell of what they add, a search of it, and a job that fails on every tick.
  Only tests register it.
- `web`: the web app's npm package, with its sources in `web/src`, and the Go
  package that embeds its build from `web/dist/app` and serves it in front of
  the API. A binary built without the app serves the committed
  `web/dist/placeholder.html` in its place. Each module of the web app is one
  folder under `web/src/modules/`, listed in `web/src/modules/registry.ts`,
  where a module may also give a part of Settings, shown after Backups, its
  kinds of notification, which the part Notifications shows, and what it
  does before a person signs out, which never keeps the session open. The
  push module gives no page, only the part Notifications, and turns this
  browser's notifications off as a person signs out. The tasks module gives
  its four kinds of notification, each a switch applied as it is flipped,
  three with an hour that waits for Save. The service worker,
  `web/public/sw.js`, shows each notification pushed to the browser, and a
  tap on one takes an open window to its path, or opens one there.
- `scripts/`: the scans and the line count that `make` runs.
- `Dockerfile`: Node builds the web app, Go builds the binary with it
  embedded, and a distroless static image runs it as user 65532. Every base
  image is pinned by digest. `--build-arg VERSION=x.y.z` stamps the version.
- `README.md` and `docs/`: the documents for whoever runs it.
  `configuration.md` holds the data directory and every key; `running.md`
  building, the verbs, the password, the API tokens, the image, what stands
  in front, notifications, backups and restoring, and upgrading.
