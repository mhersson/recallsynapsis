# Running Recall Synapsis

## Backups

### Switching them on

Backups are on when `backup.dir` is set in the configuration:

```toml
[backup]
dir  = "/var/lib/recallsynapsis/backup"   # no dir, no backups
at   = "03:00"                            # the default
keep = 14                                 # the default, at least 1
```

The server then backs up every day at `at`, in the configured `timezone`. With
no `backup.dir` there are no nightly backups, and the server says so as it
starts.

Put the backup directory on a volume other than the data's own, such as a
share on a NAS. On the same volume a backup guards against a mistake, not
against losing the disk.

The backup directory is made with mode 0700 when it is absent, and a backup
into one the group or others have any permission on is refused, as the data
directory is: a snapshot holds the password hash and the token hashes, and
whoever could write there could plant a snapshot to restore. A share whose
mount sets the modes, as SMB's does, is mounted with
`dir_mode=0700,file_mode=0600` and the server's user as its owner.

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
so the backup stays the server's own.

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

A restore brings back the password and the tokens the snapshot holds: revoke
again in Settings any token revoked since it was taken, and set the password
again if it was changed since.
