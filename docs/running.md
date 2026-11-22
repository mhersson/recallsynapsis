# Running Recall Synapsis

This document covers building Recall Synapsis, the verbs, the password, the API
tokens, running it from the image, what stands in front of it, backups and
upgrading. The data directory and every key of `recallsynapsis.toml` are in
[configuration.md](configuration.md).

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
make or revoke tokens: only a signed-in session can.

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
directory is: a snapshot holds the password hash and the token hashes, and
whoever could write there could plant a snapshot to restore. A share whose
mount sets the modes, as SMB's does, is mounted with
`dir_mode=0700,file_mode=0600` and the server's user as its owner.

The web app asks for its session as it loads and each time its event stream
opens or drops, and the answer reads the backup directory to tell when the last
backup was made. A share that hangs holds the web app's load until the share
answers.

### What a backup holds

- `recallsynapsis-YYYYMMDD.db`: a snapshot of the whole database, taken while
  the server serves, named by its day in `timezone`. A second backup the same
  day replaces that day's snapshot. The newest `keep` snapshots are kept.
- `files/`: the files the data directory's `files/` holds, once a module
  stores files there. A backup copies only those the backup lacks, and never
  removes one, so this one `files/` serves every snapshot.

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
   and copy the backup's in.

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

A restore brings back the password and the tokens the snapshot holds: revoke
again in Settings any token revoked since it was taken, and set the password
again if it was changed since.

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
