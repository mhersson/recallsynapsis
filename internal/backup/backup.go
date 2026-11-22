// Package backup backs up the data directory: a snapshot of the database named
// by its day, the files under files/ that the backup lacks, and no more
// snapshots than it keeps.
package backup

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/mhersson/recallsynapsis/internal/config"
)

// A snapshot is named prefix, its day as layout, then suffix. Nothing else a
// backup finds in its directory is ever removed.
const (
	prefix = "recallsynapsis-"
	layout = "20060102"
	suffix = ".db"
)

// Backup makes the backups of one data directory.
type Backup struct {
	db      *sql.DB
	dataDir string
	keep    int
	zone    *time.Location
	now     func() time.Time // time.Now, or a test's clock
}

// New returns the backups of db, the database of dataDir, as cfg sets them.
func New(db *sql.DB, dataDir string, cfg *config.Config) (*Backup, error) {
	zone, err := time.LoadLocation(cfg.Timezone)
	if err != nil {
		return nil, fmt.Errorf("backup: %w", err)
	}

	return &Backup{db: db, dataDir: dataDir, keep: cfg.Backup.Keep, zone: zone, now: time.Now}, nil
}

// Run makes one backup into dir and returns the path of its snapshot. A backup
// that fails before its snapshot is in place changes no snapshot.
func (b *Backup) Run(ctx context.Context, dir string) (string, error) {
	if err := privateDir(dir); err != nil {
		return "", err
	}

	path := filepath.Join(dir, prefix+b.now().In(b.zone).Format(layout)+suffix)
	if err := b.write(ctx, path); err != nil {
		return "", fmt.Errorf("backup: %w", err)
	}

	if err := prune(dir, b.keep); err != nil {
		return "", fmt.Errorf("backup: %w", err)
	}

	return path, nil
}

// write writes the snapshot under a name no snapshot has, and renames it to
// path once the files are copied too, so only a whole backup is ever taken for one.
func (b *Backup) write(ctx context.Context, path string) (err error) {
	f, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".*.tmp")
	if err != nil {
		return err
	}

	defer func() {
		if err != nil {
			_ = f.Close()
			_ = os.Remove(f.Name())
		}
	}()

	// VACUUM INTO takes an empty file, and refuses to run in a transaction, as
	// every store.Tx is. It syncs nothing; syncing the file through f does.
	if _, err := b.db.ExecContext(ctx, "VACUUM INTO ?", f.Name()); err != nil {
		return fmt.Errorf("snapshot: %w", err)
	}

	if err := f.Sync(); err != nil {
		return err
	}

	if err := f.Close(); err != nil {
		return err
	}

	if err := copyFiles(ctx, filepath.Join(b.dataDir, "files"), filepath.Join(filepath.Dir(path), "files")); err != nil {
		return fmt.Errorf("files: %w", err)
	}

	return os.Rename(f.Name(), path)
}

// copyFiles copies each regular file under src that dst lacks. A file the
// backup has is never copied again, so a module stores a changed file anew.
func copyFiles(ctx context.Context, src, dst string) error {
	// The separator makes WalkDir follow files/ when it is a link, as to another volume.
	root := src + string(filepath.Separator)

	return filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if path == root && errors.Is(err, fs.ErrNotExist) {
			return nil // no module has stored a file yet
		}

		if err != nil {
			return err
		}

		if err := ctx.Err(); err != nil {
			return err
		}

		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}

		to := filepath.Join(dst, rel)

		switch {
		case d.IsDir():
			return os.MkdirAll(to, 0o700)
		case !d.Type().IsRegular():
			return nil
		}

		if _, err := os.Lstat(to); err == nil {
			return nil
		}

		return copyFile(path, to)
	})
}

// copyFile copies from to to through a temporary name, so a file half copied
// is never taken as one the backup has.
func copyFile(from, to string) (err error) {
	in, err := os.Open(from)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.CreateTemp(filepath.Dir(to), filepath.Base(to)+".*.tmp")
	if err != nil {
		return err
	}

	defer func() {
		if err != nil {
			_ = out.Close()
			_ = os.Remove(out.Name())
		}
	}()

	if _, err := io.Copy(out, in); err != nil {
		return err
	}

	if err := out.Sync(); err != nil {
		return err
	}

	if err := out.Close(); err != nil {
		return err
	}

	return os.Rename(out.Name(), to)
}

// prune removes the snapshots in dir beyond the newest keep.
func prune(dir string, keep int) error {
	list, err := snapshots(dir)
	if err != nil {
		return err
	}

	for _, e := range list[min(keep, len(list)):] {
		// Another backup into the same directory may have removed it already.
		if err := os.Remove(filepath.Join(dir, e.Name())); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}
	}

	return nil
}

// snapshots lists the snapshots in dir, newest first: the regular files whose
// name is exactly a snapshot's.
func snapshots(dir string) ([]fs.DirEntry, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	entries = slices.DeleteFunc(entries, func(e fs.DirEntry) bool {
		day, ok := strings.CutPrefix(e.Name(), prefix)
		day, ok2 := strings.CutSuffix(day, suffix)
		_, err := time.Parse(layout, day)

		return !ok || !ok2 || err != nil || !e.Type().IsRegular()
	})
	// ReadDir sorts by name, and a day's name sorts as the day does.
	slices.Reverse(entries)

	return entries, nil
}

// privateDir makes dir 0o700 when absent and refuses one the group or others
// have any permission on, as the data directory is: a snapshot holds the
// password hash, and whoever can write here can plant a snapshot to restore.
func privateDir(dir string) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("backup directory: %w", err)
	}

	info, err := os.Stat(dir)
	if err != nil {
		return fmt.Errorf("backup directory: %w", err)
	}

	if info.Mode().Perm()&0o077 != 0 {
		return fmt.Errorf("the backup directory %s can be read, written or entered by others; run chmod 700 %s", dir, dir)
	}

	return nil
}
