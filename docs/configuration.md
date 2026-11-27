# Configuration

Recall Synapsis keeps what it owns in one data directory and reads its settings
from `recallsynapsis.toml`, which is in that directory unless `--config` names
another file. This document covers both. Building, the password, the tokens, the
image, what stands in front, notifications, backups and upgrading are in
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
that fixes it: the password's hash, the token hashes and push's signing key
live in the database there.

The directory holds:

| Path | What it is |
| --- | --- |
| `recallsynapsis.toml` | the settings, read unless `--config` names another file |
| `recallsynapsis.db` | the database, with its `-wal` and `-shm` files beside it while it is open |
| `files/` | the contents of the attached files, each kept once and named by its SHA-256, and removed when the last attachment that names it goes |
| `tmp/` | an upload while it arrives, renamed into `files/` once it is whole, so it must be on the file system `files/` is on; a file left here by a crash can be removed while the server is stopped |

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
| `timezone` | `UTC` | the time zone the nightly backup's time and a snapshot's day are read in, in which tasks tell which day it is, and at whose midnight a notification stops waiting at a push service, such as `Europe/Oslo` |
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
tags, links, attached files and history, the inbox of captures that wait to be
filed, and the trash.

| Key | Default | Meaning |
| --- | --- | --- |
| `enabled` | `false` | whether notes is on |
| `capture_max_chars` | `4000` | the longest text an MCP tool of notes keeps, in characters, at least 1: `text` and `said` of `create_note`, and `body` and `append` of `update_note`, each on its own; REST, and so the web app, has no such cap; checked whether notes is on or not |
| `file_max_mib` | `15` | the largest file that can be attached, in MiB, at least 1; checked whether notes is on or not |

`[modules.tasks]` is the tasks module: tasks in the areas, each with its notes,
a planned period of days with an optional time on its first, a deadline, the
list Soon or Someday, when it was done, its links and a log of its changes; and
the trash.

| Key | Default | Meaning |
| --- | --- | --- |
| `enabled` | `false` | whether tasks is on |
| `week_start` | `monday` | the first day of the week, which tasks give beside today's date: a weekday's name in lower case, such as `sunday`; checked whether tasks is on or not |
| `capture_max_chars` | `4000` | the longest text an MCP tool of tasks keeps, in characters, at least 1: the `notes` of each task and `said` of `create_tasks`, and `notes` of `update_task`, each on its own; a title has its own cap of 200 characters everywhere; REST, and so the web app, has no such cap; checked whether tasks is on or not |

`[modules.knowledge]` is the knowledge module: the decisions that shape each
project, each with its why, its source, the day it was decided and the
decision it replaces, and every wording of it. It has no key but `enabled`.
Its caps are fixed and are no keys: a decision is one line of at most 200
characters, its why at most 2,000 and its source one line of at most 500, and
one change adds and mends at most 20 decisions. A text over its cap is refused
wherever it comes from, the web app included. The owner's words that the MCP
tool `update_knowledge` keeps, its `said`, are at most 4,000 characters.

| Key | Default | Meaning |
| --- | --- | --- |
| `enabled` | `false` | whether knowledge is on |

`[modules.push]` is the push module: notifications sent to each browser of
the owner's in which they are turned on, as Web Push
([running.md](running.md#notifications)). It has no key but `enabled`, and no
other module's table has a key for it. Push signs what it sends with
`public_url` as its contact, so with push on `public_url` must be `https`;
nothing checks that, and a push service may refuse what an `http` one signs.

| Key | Default | Meaning |
| --- | --- | --- |
| `enabled` | `false` | whether push is on |

A whole file, with `listen`, `backup.at`, `backup.keep`, both
`capture_max_chars`, `file_max_mib` and `week_start` at their defaults:

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
enabled           = true
capture_max_chars = 4000
file_max_mib      = 15

[modules.tasks]
enabled           = true
week_start        = "monday"
capture_max_chars = 4000

[modules.knowledge]
enabled = true

[modules.push]
enabled = true
```
