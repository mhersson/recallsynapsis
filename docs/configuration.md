# Configuration

Recall Synapsis keeps what it owns in one data directory and reads its settings
from `recallsynapsis.toml`, which is in that directory unless `--config` names
another file. This document covers both. Building, the password, the tokens, the
image, what stands in front, backups and upgrading are in
[running.md](running.md).

## The data directory

`serve`, `password` and `backup` take their data directory from the first of
these that is set:

1. `--data-dir DIR`
2. the environment variable `RECALLSYNAPSIS_DATA_DIR`
3. `$XDG_DATA_HOME/recallsynapsis`
4. `~/.local/share/recallsynapsis`

The directory is made with mode 700 when it is absent. A verb refuses to work
when the group or others have any permission on it, and prints the `chmod 700`
that fixes it: the password's hash and the token hashes live in the database
there.

The directory holds:

| Path | What it is |
| --- | --- |
| `recallsynapsis.toml` | the settings, read unless `--config` names another file |
| `recallsynapsis.db` | the database, with its `-wal` and `-shm` files beside it while it is open |

The settings are read before the directory is made, so on a first start either
make the directory with mode 700 and write `recallsynapsis.toml` in it, or name
the file with `--config`.

### `--config FILE`

`--config FILE` reads the settings from FILE, which must exist. Nothing writes
to it, so it can be mounted read-only. The database stays in the data
directory. FILE names the owner and where the backups go, so only you should be
able to write it.

## recallsynapsis.toml

The file is read strictly: a key it does not know, a key of the wrong type and a
value it cannot use all stop the verb, with the key named. It is read when a
verb starts, so a change takes effect at the next start.

| Key | Default | Meaning |
| --- | --- | --- |
| `listen` | `127.0.0.1:7119` | the host and port served on, in plain HTTP |
| `public_url` | none, required | the address people reach Recall Synapsis at: `http` or `https`, with a host and no path, query or user, such as `https://recall.example.org` |
| `timezone` | `UTC` | the time zone the nightly backup's time and a snapshot's day are read in, such as `Europe/Oslo` |
| `owner` | none, required | the name the owner signs in with |
| `backup.dir` | none | the directory backups go to; with none there are no nightly backups |
| `backup.at` | `03:00` | the time of day of the nightly backup, as `HH:MM` on a 24-hour clock |
| `backup.keep` | `14` | how many snapshots a backup keeps, at least 1 |

`public_url` tells Recall Synapsis the address in front of it; TLS and that
address belong to what stands in front ([running.md](running.md#what-stands-in-front)).
The session cookie is marked `Secure` when `public_url` is `https`.

There is one owner. A changed `owner` renames them at the next `serve` or
`password`, and the rename keeps their password, sessions and tokens.

Each feature module of the binary has a table of its own, `[modules.NAME]`, and
is off without one. `serve`, `password` and `backup` refuse a table no module of
the binary has. A module that is off serves nothing and keeps what it holds, so
switched on again it has it all back.

`[modules.notes]` is the notes module: notes and ideas in the areas, with their
tags and history, the inbox of captures that wait to be filed, and the trash.

| Key | Default | Meaning |
| --- | --- | --- |
| `enabled` | `false` | whether notes is on |

A whole file, with `listen`, `backup.at` and `backup.keep` at their defaults:

```toml
listen     = "127.0.0.1:7119"
public_url = "https://recall.example.org"
timezone   = "Europe/Oslo"
owner      = "ada"

[backup]
dir  = "/mnt/backup/recallsynapsis"
at   = "03:00"
keep = 14

[modules.notes]
enabled = true
```
