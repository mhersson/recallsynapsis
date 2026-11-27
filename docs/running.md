# Running Recall Synapsis

This document covers building Recall Synapsis, the verbs, the password, the API
tokens, running it from the image, what stands in front of it, notifications,
backups and upgrading. The data directory and every key of
`recallsynapsis.toml` are in [configuration.md](configuration.md).

## Building

Building needs the Go version in `go.mod` and the Node version in `.nvmrc`:

```sh
make web-install      # once: the web app's npm packages
make build            # writes bin/recallsynapsis with the web app embedded
make install          # installs recallsynapsis into GOBIN, else the GOPATH bin directory
bin/recallsynapsis version
```

`VERSION=1.2.3 make build` stamps the version `recallsynapsis version` prints
and Settings shows; without it the version is the commit the binary was built
from. `make build-go` builds the binary without Node, with the web app as it was
last built, or with a page saying it was built without one. `make help` lists
the other targets.

## The verbs

| Verb | What it does |
| --- | --- |
| `serve` | serves the web app, the API and `/mcp` on `listen` until it is stopped |
| `password` | sets the owner's password from standard input, or prints its hash with `--hash` |
| `backup` | makes one backup now |
| `version` | prints the version |

`serve`, `password` and `backup` take `--data-dir DIR` and `--config FILE`
([configuration.md](configuration.md#the-data-directory)).

`serve` stops on SIGINT or SIGTERM, and on SIGHUP unless `nohup` started it,
and gives requests that are still open five seconds. It logs to standard error:
`listening` once, with the address, `public_url`, the data directory and the
version, then one `request` line per request with its method, path, status and
how long it took. The log holds no query, body or header. It also logs each
nightly backup, as `backed up` or `the backup failed`, and says as it starts
when the owner has no password, when there is no `backup.dir`, and when every
backup into it will fail.

## The password

The owner signs in with the `owner` name and a password of at least 12
characters. There is no default: until a password is set nobody can sign in,
and `serve` says so in its log as it starts. Set it with:

```sh
sh -c 'umask 077 && cat > password.txt'
recallsynapsis password < password.txt
rm password.txt
```

Type the password, press Enter, then Ctrl-D. The `umask 077` makes the file
readable by you alone.

`recallsynapsis password` reads the password from standard input only. It
refuses an argument and a terminal, because an argument shows in the process
list and the shell's history. One line ending at the end is dropped. It works in
the data directory, as `serve` does, and also while `serve` runs. Setting a
password ends every session, so the web app asks you to sign in again.

`recallsynapsis password --hash` prints the password's argon2id hash and touches
no data directory:

```sh
recallsynapsis password --hash < password.txt
```

Put that hash in the environment variable `RECALLSYNAPSIS_PASSWORD_HASH` of the
process that runs `recallsynapsis serve`. Each start with the variable set makes
its hash the password: the hash already stored changes nothing, and a new one is
stored and ends every session. A value that is not a hash `--hash` makes stops
the start, and the message names the variable, never its value. The hash is a
secret: give it from a file only you can read, such as Docker's `--env-file`,
not on a command line. This is how a container receives the password.

## API tokens

An AI client or a script reaches Recall Synapsis with an API token. Make one in
the web app, signed in: Settings, Tokens, New token. Give it a name no other
token has and one of two accesses:

| Access | What the token may do |
| --- | --- |
| `read` | search and read; every route and tool that changes something refuses it |
| `write` | also change things |

The token starts with `recall_` and is shown once, as it is made: only its hash
is kept. A client sends it as `Authorization: Bearer TOKEN`, to `/mcp` or to
`/api/`. Settings lists each token with its access, when it was made and when it
was last used. Revoking one refuses it from its next request. No token can list,
make or revoke tokens, change an area's brief over REST, restore one, rename or
archive a project, delete a note or a task for good, or attach, read or remove a
note's file: only a signed-in session can. A `write` token can make a project,
and replace a brief through the MCP tool `update_area` alone.

### The MCP tools

At `/mcp` a client with a token calls the core's three tools and those of each
module switched on. These are the core's, the tools of notes and those of tasks,
with the mark suggested for each in an MCP gateway such as Portal, which keeps
the marks in its own configuration: a free tool runs at once, and one that needs
a tap waits for the owner to allow it.

| Tool | Access | What it does | Mark |
| --- | --- | --- | --- |
| `list_areas` | `read` | lists the areas that are not archived, each with the first paragraph of its brief, or gives one area, an archived project too, with its brief whole, its revision and when and by what it changed | free |
| `search` | `read` | searches every area or one, the hits grouped by module | free |
| `update_area` | `write` | replaces an area's brief whole, from the revision it names, with the owner's words kept on the revision | needs a tap |
| `create_note` | `write` | saves a note or an idea: in the area it names, an idea that names none in the personal area, and any other in the inbox | free |
| `list_notes` | `read` | lists notes and ideas by area, kind, inbox flag and tag, the most recently changed first | free |
| `get_note` | `read` | reads one note in full, with its links, what links to it, and the id, name, type and size of each file | free |
| `update_note` | `write` | changes a note's title, body, kind, area, tags, links or inbox flag, or adds text to its body | free |
| `trash_note` | `write` | moves a note to the trash | needs a tap |
| `create_tasks` | `write` | saves 1 to 50 tasks, all or none, each in the area it names or else the personal area, with the owner's words kept on each | free |
| `list_tasks` | `read` | lists the open or the done tasks by area, list, a span of dates or what waits to be sorted, and tells which day it is | free |
| `get_task` | `read` | reads one task in full, with its notes, links, what links to it, and its log | free |
| `update_task` | `write` | changes a task's title, notes, area, plan, deadline, list or links, or marks it done or opens it again | free |
| `trash_task` | `write` | moves a task to the trash | needs a tap |

`update_area` carries Claude Code's marker:
`"_meta": {"anthropic/requiresUserInteraction": true}` on its entry in
`tools/list`. Claude Code then asks its user before every call of it, in auto
mode and under `bypassPermissions` too, with no "don't ask again", and a run
with nobody to answer is refused the call. A gateway such as Portal does not
pass the marker on, so through one the mark set there for the tool is what
holds.
`update_area` refuses a `said` of more than 4,000 characters, and a brief of
more than 4,000 is refused wherever it comes from.

No tool finds a note or a task in the trash, restores one or deletes one for
good, and no tool reads, attaches or removes a note's file. A text through
`create_note`, `update_note`, `create_tasks` or `update_task` longer than its
module's `capture_max_chars`
([configuration.md](configuration.md#recallsynapsistoml)) is refused, and
nothing is saved. A call of `create_tasks` or `update_task` that sets a date
gives `today`, the date the client takes for today; unless it is the date in
the configured `timezone`, the call is refused with that date, its weekday, the
time zone and the first day of the week, and nothing is saved. A write through
a tool records the token's name as its source.

As a client connects, the server's instructions tell it in one line, the same
with any module on or off, to read a project's brief first when a discussion of
the project begins, with `list_areas` and the project's slug or name; Claude
Code reads them as a session starts. Portal relays neither annotations nor
instructions to the client, so each tool's description tells a client what it
does and when to use it, and that of `list_areas` says too to read a project's
brief as a discussion of it begins.

## From the image

```sh
docker build --build-arg VERSION=1.2.3 -t recallsynapsis .
docker run --rm recallsynapsis version
```

The image holds one binary, `recallsynapsis`, which is its entrypoint, and runs
as user and group 65532. The verb and its flags follow the image's name. Without
`--build-arg VERSION` the version is `dev`.

What the container needs from whatever runs it:

- **The listen address.** The default `listen`, `127.0.0.1:7119`, cannot be
  reached from outside the container. Set `listen = "0.0.0.0:7119"` and publish
  the port only where what stands in front and the MCP clients reach it.
- **Two volumes, each with a directory the server makes.** The data directory
  and `backup.dir` are refused when the group or others have any permission on
  them, and a volume's root often has group permissions. So name a directory
  beneath each volume's root, which the server makes with mode 700, such as
  `/var/lib/recallsynapsis/data` for the data and `/mnt/backup/recallsynapsis`
  for the backups. User 65532 must be able to make it: the root is owned by user
  65532, or by group 65532 with group write. A new Docker volume's root belongs
  to root, so give it to 65532 once, as below. In Kubernetes, `fsGroup: 65532`
  gives the root of a volume that supports it the group, and
  `fsGroupChangePolicy: OnRootMismatch` is needed with it: without it every
  start gives the group permissions on every directory in the volume, and the
  server refuses its data directory.
- **Nothing else writable.** The server writes only to the data directory and
  `backup.dir`, so the root filesystem can be read-only, with no other writable
  mount.
- **The settings.** `--config FILE`, with FILE mounted read-only. User 65532
  must be able to read FILE and enter its directory.
- **The password hash.** `RECALLSYNAPSIS_PASSWORD_HASH` comes from the
  container's environment. Make the hash with the image itself; standard input
  must not be a terminal, so use `-i` and a redirected file:

  ```sh
  docker run --rm -i recallsynapsis password --hash < password.txt
  ```

- **A probe.** `GET /api/health` answers 200 without a sign-in.

A whole run with Docker, from a directory that holds `password.txt`:

```sh
docker volume create recallsynapsis-data
docker volume create recallsynapsis-backup
docker run --rm -v recallsynapsis-data:/data -v recallsynapsis-backup:/backup \
  busybox chown 65532:65532 /data /backup

mkdir etc
cat > etc/recallsynapsis.toml <<'EOF'
listen     = "0.0.0.0:7119"
public_url = "https://recall.example.org"
owner      = "ada"

[backup]
dir = "/mnt/backup/recallsynapsis"
EOF

sh -c 'umask 077 && printf "RECALLSYNAPSIS_PASSWORD_HASH=%s\n" \
  "$(docker run --rm -i recallsynapsis password --hash < password.txt)" > recallsynapsis.env'
rm password.txt

docker run -d --name recallsynapsis --restart unless-stopped --read-only \
  -p 127.0.0.1:7119:7119 \
  -v recallsynapsis-data:/var/lib/recallsynapsis \
  -v recallsynapsis-backup:/mnt/backup \
  -v "$PWD/etc:/etc/recallsynapsis:ro" \
  -e RECALLSYNAPSIS_DATA_DIR=/var/lib/recallsynapsis/data \
  --env-file recallsynapsis.env \
  recallsynapsis serve --config /etc/recallsynapsis/recallsynapsis.toml
```

It publishes the port on the host's loopback address, for a proxy on the same
host. A container keeps the environment it was made with, so a new hash takes
effect in a container made with the new value.

## What stands in front

Recall Synapsis serves plain HTTP on its one `listen` address and does no TLS.
TLS and the public name belong to a proxy or tunnel in front, which passes the
requests to `listen`. Set `public_url` to that https address: the session cookie
is marked `Secure` only when `public_url` is `https`.

| Path | Who it is for |
| --- | --- |
| `/api/` | the web app, with its session cookie, and clients with an API token |
| `/mcp` | MCP clients with an API token; the session cookie is no authority there |
| every other path | the web app, for a person in a browser |

`/mcp` is meant for an MCP gateway such as Portal and for agents on the home
network, which reach the port directly, so the public name does not need to
pass it.

Only what stands in front and those clients should reach the port: whatever else
reaches it has only the password and the tokens between it and the data.

### The event stream

Each open tab of the web app keeps one request open: `GET /api/events`, a
server-sent event stream of what changes, so the page shows a change without a
reload. It is long-lived by design. It sends a comment line every 15 seconds and
ends by itself after an hour. A proxy in front must pass each event on as it is
written: the stream sends `X-Accel-Buffering: no`, which turns nginx's buffering
off, and a proxy that buffers it holds the changes back from the open pages. A
request timeout shorter than an hour, or an idle timeout shorter than 15
seconds, ends the stream early. The page then opens it again and reads afresh
what it shows, so an early end costs a reload of that, not data.

### Uploads

A file is attached as the whole body of one request, of up to `file_max_mib`.
A proxy in front must pass a body that large: nginx's `client_max_body_size` is
1 MiB unless set, and nginx answers a larger file with its own 413 before it
reaches the server. The server sets no timeout on reading a body, so a slow
upload from a phone is not cut, but a proxy in front may set one.

### The sign-in limiter

Wrong sign-ins are counted by the name and the connection's address; no
forwarded header is read. From the third wrong password in a row for a name from
an address, sign-ins with that name from that address are refused, with 429 and
`Retry-After`, for one second, and twice as long after each further one, up to
five minutes; until then a right password is refused too. A right password after
the wait clears the count, and so does a restart: it is kept in memory. Behind a
proxy every connection has the proxy's address, so whoever reaches the sign-in
can make the owner wait by trying the owner's name. That is why the web app
belongs behind an access layer, such as Cloudflare Access over the whole
hostname.

## Notifications

With `[modules.push]` on ([configuration.md](configuration.md#recallsynapsistoml)),
the server sends notifications to the owner's browsers as Web Push, with
`public_url`, which must be `https`, as its contact. They are turned on in each
browser, for that browser, in Settings under Notifications, with the session
cookie alone: no API token turns a browser on or off. On an iPhone or iPad only
the app added to the Home Screen can turn them on, since a Safari tab there has
no Web Push. A message goes to every browser that has them on.

Tasks sends four kinds of notification, each to the user whose open tasks,
outside the trash, it names:

- at a task's start time, on the first day of its plan, its title over its
  time and its area, but the personal one;
- at a morning hour, what the day holds: how many tasks are today's, left
  from earlier and overdue, and their titles;
- at a deadline hour, the tasks due tomorrow;
- at a planned-today hour, the tasks planned for today with no start time
  today.

A kind with no task to name sends nothing. Its hour is read in `timezone`. At
first the start time and the morning, at 07:00, are on, and the deadline, at
18:00, and the planned today, at 09:00, are off. Each kind is switched on or
off, and its hour set, in Settings under Notifications, for every browser of
the user at once, with the session cookie alone: `GET /api/tasks/notify` reads
the four and their hours, and `PATCH /api/tasks/notify` changes those it is
given.

The server looks every 30 seconds and keeps the moment it has notified up to,
so each notification is sent once. One whose time passed while the server was
down is sent late as it starts again, if it is still the same day in
`timezone`, and else never. One whose time passed while push was off is not
sent when push is switched on. A server stopped while it sends may send that
notification again as it starts, and the second replaces the first.

The server must be able to reach, on port 443, the push service of each
browser in use, which the browser names as it turns notifications on:
`web.push.apple.com` for Safari, `updates.push.services.mozilla.com` for
Firefox, and `fcm.googleapis.com` for Chrome, Brave and Android, as their
makers run them today.

A message is encrypted for the browser alone. Its push service sees when it is
sent, its size, the server's address and `public_url`, and its tag, which
names what it is about so that a newer one replaces it; never its words. It
waits there until the day ends in `timezone`.

A browser whose push service says it no longer knows it is forgotten. Any
other failure is logged and kept on the browser, with the push service's host
and when, until a message to it goes through; a failed message is not sent
again.

The server makes its signing key the first time push needs it and keeps it in
the database, so a snapshot holds it, and a restore keeps every browser that
had notifications on when the snapshot was taken.

## Backups

### Switching them on

Backups are on when `backup.dir` is set in the configuration:

```toml
[backup]
dir  = "/mnt/backup/recallsynapsis"       # no dir, no backups
at   = "03:00"                            # the default
keep = 14                                 # the default, at least 1
```

The server then backs up every day at `at`, in the configured `timezone`. With
no `backup.dir` there are no nightly backups, and the server says so as it
starts.

Put the backup directory on a volume other than the data's own, such as a
share on a NAS. On the same volume a backup guards against a mistake, not
against losing the disk. In the image, that is the second volume, with
`backup.dir` a directory beneath its root ([From the image](#from-the-image)).

The backup directory is made with mode 0700 when it is absent, and a backup
into one the group or others have any permission on is refused, as the data
directory is: a snapshot holds the password hash, the token hashes and push's
signing key, and whoever could write there could plant a snapshot to restore.
A share whose mount sets the modes, as SMB's does, is mounted with
`dir_mode=0700,file_mode=0600` and the server's user as its owner.

The web app asks for its session as it loads and each time its event stream
opens or drops, and the answer reads the backup directory to tell when the last
backup was made. A share that hangs holds the web app's load until the share
answers.

### What a backup holds

- `recallsynapsis-YYYYMMDD.db`: a snapshot of the whole database, push's
  signing key with it, taken while the server serves, named by its day in
  `timezone`. A second backup the same day replaces that day's snapshot. The
  newest `keep` snapshots are kept.
- `files/`: the contents of the attached files that the data directory's
  `files/` holds. A backup copies only those the backup lacks, and never
  removes one, so this one `files/` serves every snapshot, and the contents
  of an attachment removed since stay here. An upload under `tmp/` is never
  copied.

A snapshot appears only once it and the files are written in whole. A backup
that fails before then is logged and changes no snapshot, and the next night's
runs as usual. Settings in the web app shows when the newest snapshot was made,
and marks it late once it is older than a day and a half.

Of what a backup finds in the directory, it removes only the snapshots beyond
`keep`: regular files named exactly as a snapshot. A file ending in `.tmp` is
one a backup was writing when its process was killed; it is never taken for a
backup, and can be removed when no backup is running.

### Making one now

```sh
recallsynapsis backup [--to DIR] [--data-dir DIR] [--config FILE]
```

It makes one backup into `backup.dir`, or into the directory `--to` names,
which it keeps to `keep` snapshots in the same way, and prints the snapshot's
path. It may run while the server serves the same data directory. It refuses a
data directory that holds no database. Run it as the user the server runs as,
so the backup stays the server's own. In the image, run it in the server's
container, which runs it as that user:

```sh
docker exec recallsynapsis recallsynapsis backup --config /etc/recallsynapsis/recallsynapsis.toml
```

### Restoring

Carry the steps out as the user the server runs as, so that what is restored
is its own.

1. Stop the server.
2. In the data directory, move the database out of the way, with its `-wal`
   and `-shm` files when they are there. SQLite would replay a `-wal` left
   there onto the restored database, and break it.

   ```sh
   cd DATA_DIR
   mkdir replaced
   mv recallsynapsis.db* replaced/
   ```

3. Copy the snapshot in as the database.

   ```sh
   cp BACKUP_DIR/recallsynapsis-YYYYMMDD.db recallsynapsis.db
   ```

4. When the backup has `files/`, move the data directory's own out of the way
   and copy the backup's in. It holds the contents of attachments removed
   since the snapshot too, which nothing removes again; they take room and
   nothing else.

   ```sh
   [ -d files ] && mv files replaced/
   cp -R BACKUP_DIR/files files
   ```

5. Start the server. Once it serves as it should, remove `replaced/`.

The image has no shell. Stop the server's container, then move the database
aside and copy the snapshot and the files in, as above, from a shell in another
image, run as user 65532 with the volumes mounted where the server's container
mounts them. There `DATA_DIR` is
`/var/lib/recallsynapsis/data` and `BACKUP_DIR` is `/mnt/backup/recallsynapsis`:

```sh
docker stop recallsynapsis
docker run --rm -it -u 65532:65532 \
  -v recallsynapsis-data:/var/lib/recallsynapsis \
  -v recallsynapsis-backup:/mnt/backup \
  busybox sh
docker start recallsynapsis
```

A restore brings back the password, the tokens, push's signing key and the
browsers with notifications on that the snapshot holds: revoke again in
Settings any token revoked since it was taken, set the password again if it
was changed since, turn notifications on again in a browser that turned them
on since, and off again in one that turned them off since.

## Upgrading

Every verb that opens the data directory, `serve`, `password` and `backup`,
first applies the database migrations the new version brings. A migration that
fails is rolled back and stops the verb. Nothing migrates a database back.

So take the backup before an upgrade with the version that runs now, in the
image with `docker exec` into the running container. The new version's `backup`
would migrate the database before it took the snapshot, which would then hold
no copy of the database as the running version left it.

Then build the new version, or pull the image CI built, and restart the server
on it; with Docker, remove the container and run the new image with the same
command. To go back, restore that snapshot and start the version before.
