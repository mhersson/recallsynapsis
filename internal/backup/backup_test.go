package backup

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mhersson/recallsynapsis/internal/config"
	"github.com/mhersson/recallsynapsis/internal/store"
)

var oslo = func() *time.Location {
	zone, err := time.LoadLocation("Europe/Oslo")
	if err != nil {
		panic(err)
	}

	return zone
}()

type fixture struct {
	st   *store.Store
	b    *Backup
	data string // the data directory
	dir  string // the backup directory, absent until the first run

	mu    sync.Mutex
	clock time.Time
}

// newFixture backs up a database with the table rows into a backup directory
// that keep snapshots, on a clock that starts on 2026-11-20 in Oslo.
func newFixture(t *testing.T, keep int) *fixture {
	t.Helper()

	data := t.TempDir()

	st, err := store.Open(t.Context(), filepath.Join(data, "recallsynapsis.db"), []store.Migration{
		{Owner: "test", Version: 1, SQL: "CREATE TABLE rows (id INTEGER PRIMARY KEY, n INTEGER NOT NULL, pad BLOB)"},
	})
	require.NoError(t, err)

	t.Cleanup(func() { _ = st.Close() })

	b, err := New(st.DB, data, &config.Config{Timezone: "Europe/Oslo", Backup: config.Backup{Dir: "unused", At: "03:00", Keep: keep}})
	require.NoError(t, err)

	f := &fixture{st: st, b: b, data: data, dir: filepath.Join(t.TempDir(), "backup"), clock: time.Date(2026, 11, 20, 3, 0, 0, 0, oslo)}
	b.now = f.now

	return f
}

func (f *fixture) now() time.Time {
	f.mu.Lock()
	defer f.mu.Unlock()

	return f.clock
}

func (f *fixture) set(at time.Time) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.clock = at
}

// run backs up into f.dir on the day of at, which must succeed.
func (f *fixture) run(t *testing.T, at time.Time) string {
	t.Helper()

	f.set(at)

	path, err := f.b.Run(t.Context(), f.dir)
	require.NoError(t, err)

	return path
}

func (f *fixture) insert(t *testing.T, id int64) {
	t.Helper()

	_, err := f.st.ExecContext(t.Context(), "INSERT INTO rows (id, n) VALUES (?, ?)", id, id)
	require.NoError(t, err)
}

// names lists the names in dir.
func names(t *testing.T, dir string) []string {
	t.Helper()

	entries, err := os.ReadDir(dir)
	require.NoError(t, err)

	list := make([]string, len(entries))
	for i, e := range entries {
		list[i] = e.Name()
	}

	return list
}

// open opens a copy of the snapshot at path, as a restore would, so the backup
// directory gains no -wal or -shm beside it.
func open(t *testing.T, path string) *store.Store {
	t.Helper()

	restored := filepath.Join(t.TempDir(), "recallsynapsis.db")
	require.NoError(t, copyFile(path, restored))

	st, err := store.Open(t.Context(), restored, nil)
	require.NoError(t, err)

	t.Cleanup(func() { _ = st.Close() })

	return st
}

// ids is the ids of the rows in the snapshot at path, in order.
func ids(t *testing.T, path string) []int64 {
	t.Helper()

	rows, err := open(t, path).QueryContext(t.Context(), "SELECT id FROM rows ORDER BY id")
	require.NoError(t, err)

	defer rows.Close()

	var list []int64

	for rows.Next() {
		var id int64
		require.NoError(t, rows.Scan(&id))

		list = append(list, id)
	}

	require.NoError(t, rows.Err())

	return list
}

func day(d int) time.Time { return time.Date(2026, 11, d, 3, 0, 0, 0, oslo) }

func TestASnapshotTakenWhileWritesGoOnHoldsEveryCommittedRow(t *testing.T) {
	t.Parallel()

	f := newFixture(t, 3)

	// Enough data that the snapshot takes a while.
	_, err := f.st.ExecContext(t.Context(), `WITH RECURSIVE s(i) AS (SELECT 1 UNION ALL SELECT i + 1 FROM s WHERE i < 200)
		INSERT INTO rows (id, n, pad) SELECT -i, 0, randomblob(4000) FROM s`)
	require.NoError(t, err)

	var committed atomic.Int64

	stop, failed := make(chan struct{}), make(chan error, 1)

	go func() {
		defer close(failed)

		for id := int64(1); ; id++ {
			select {
			case <-stop:
				return
			default:
			}

			if _, err := f.st.ExecContext(context.Background(), "INSERT INTO rows (id, n) VALUES (?, ?)", id, id); err != nil {
				failed <- err

				return
			}

			committed.Store(id)
		}
	}()

	require.Eventually(t, func() bool { return committed.Load() > 50 }, 5*time.Second, time.Millisecond)

	// With one CPU the writer may not get to run during a short snapshot, so try again until it has.
	var (
		before, during int64
		path           string
	)

	for range 100 {
		before = committed.Load()
		path = f.run(t, day(20))

		if during = committed.Load(); during > before {
			break
		}
	}

	close(stop)
	require.NoError(t, <-failed)

	assert.Greater(t, during, before, "the writes went on while the snapshot was taken")

	var check string
	require.NoError(t, open(t, path).QueryRowContext(t.Context(), "PRAGMA integrity_check").Scan(&check))
	assert.Equal(t, "ok", check)

	// Each row was its own transaction, so the snapshot holds 1 to n with no gap.
	var got []int64

	for _, id := range ids(t, path) {
		if id > 0 {
			got = append(got, id)
		}
	}

	require.GreaterOrEqual(t, int64(len(got)), before, "every row committed before the backup began")

	for i, id := range got {
		require.Equal(t, int64(i+1), id)
	}
}

func TestASnapshotIsNamedByItsDayInTheZone(t *testing.T) {
	t.Parallel()

	f := newFixture(t, 3)

	// 23:30 in UTC is half past midnight in Oslo, the next day.
	path := f.run(t, time.Date(2026, 11, 22, 23, 30, 0, 0, time.UTC))
	assert.Equal(t, filepath.Join(f.dir, "recallsynapsis-20261123.db"), path)
	assert.Equal(t, []string{"recallsynapsis-20261123.db"}, names(t, f.dir))
}

func TestASecondRunTheSameDayReplacesThatDaysSnapshot(t *testing.T) {
	t.Parallel()

	f := newFixture(t, 3)

	f.insert(t, 1)
	first := f.run(t, day(22))

	f.insert(t, 2)
	second := f.run(t, day(22).Add(20*time.Hour))

	assert.Equal(t, first, second)
	assert.Equal(t, []int64{1, 2}, ids(t, second))
	assert.Equal(t, []string{"recallsynapsis-20261122.db"}, names(t, f.dir))
}

func TestOnlySnapshotsAreEverRemoved(t *testing.T) {
	t.Parallel()

	f := newFixture(t, 3)
	require.NoError(t, os.Mkdir(f.dir, 0o700))

	others := []string{
		"notes.txt",
		"recallsynapsis-2026112.db",
		"recallsynapsis-202611011.db",
		"recallsynapsis-20261301.db",
		"recallsynapsis-20261101.db.bak",
		"recallsynapsis-20261101.db.4711.tmp",
		"recallsynapsis-20261101.db-wal",
		"Recallsynapsis-20261101.db",
		"recallsynapsis-+0261101.db",
	}
	for _, name := range others {
		require.NoError(t, os.WriteFile(filepath.Join(f.dir, name), []byte(name), 0o600))
	}

	// A directory and a link with a snapshot's name are not snapshots.
	require.NoError(t, os.Mkdir(filepath.Join(f.dir, "recallsynapsis-20261102.db"), 0o700))
	require.NoError(t, os.Symlink("notes.txt", filepath.Join(f.dir, "recallsynapsis-20261103.db")))

	// A snapshot an earlier run wrote.
	require.NoError(t, os.WriteFile(filepath.Join(f.dir, "recallsynapsis-20261001.db"), nil, 0o600))

	for d := 20; d <= 24; d++ {
		f.run(t, day(d))
	}

	assert.ElementsMatch(t, append(others,
		"recallsynapsis-20261102.db",
		"recallsynapsis-20261103.db",
		"recallsynapsis-20261122.db",
		"recallsynapsis-20261123.db",
		"recallsynapsis-20261124.db",
	), names(t, f.dir))

	for _, name := range others {
		b, err := os.ReadFile(filepath.Join(f.dir, name))
		require.NoError(t, err)
		assert.Equal(t, name, string(b))
	}
}

func TestAFailedBackupLeavesTheSnapshotsAsTheyWere(t *testing.T) {
	t.Parallel()

	f := newFixture(t, 1)

	f.insert(t, 1)
	earlier := f.run(t, day(21))

	f.insert(t, 2)

	// A snapshot beyond keep, which only a backup that succeeds removes.
	require.NoError(t, os.WriteFile(filepath.Join(f.dir, "recallsynapsis-20261001.db"), nil, 0o600))

	// The same day's snapshot is written, then copying the files fails, since
	// the backup has a file where the data directory has a directory.
	require.NoError(t, os.MkdirAll(filepath.Join(f.data, "files", "sub"), 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(f.dir, "files"), nil, 0o600))

	_, err := f.b.Run(t.Context(), f.dir)
	require.Error(t, err)

	assert.Equal(t, []string{"files", "recallsynapsis-20261001.db", "recallsynapsis-20261121.db"}, names(t, f.dir), "none removed and none half written left")
	assert.Equal(t, []int64{1}, ids(t, earlier), "the day's snapshot is not replaced")

	// A backup ended through its context fails the same way.
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	require.NoError(t, os.Remove(filepath.Join(f.dir, "files")))

	_, err = f.b.Run(ctx, f.dir)
	require.ErrorIs(t, err, context.Canceled)
	assert.Equal(t, []string{"recallsynapsis-20261001.db", "recallsynapsis-20261121.db"}, names(t, f.dir))

	// The next run succeeds, and keeps only its own.
	assert.Equal(t, []int64{1, 2}, ids(t, f.run(t, day(22))))
	assert.Equal(t, []string{"files", "recallsynapsis-20261122.db"}, names(t, f.dir))
}

func TestTheFilesTheBackupLacksAreCopied(t *testing.T) {
	t.Parallel()

	f := newFixture(t, 3)

	f.run(t, day(20))
	assert.NoDirExists(t, filepath.Join(f.dir, "files"), "no module has stored a file")

	// Sources others may read, so the copies' modes are the backup's own.
	files := filepath.Join(f.data, "files")
	require.NoError(t, os.MkdirAll(filepath.Join(files, "sub"), 0o700))
	require.NoError(t, os.Chmod(filepath.Join(files, "sub"), 0o755))                    //nolint:gosec // a source the backup must not take its mode from
	require.NoError(t, os.WriteFile(filepath.Join(files, "a.txt"), []byte("a"), 0o644)) //nolint:gosec // a source the backup must not take its mode from
	require.NoError(t, os.WriteFile(filepath.Join(files, "sub", "b.txt"), []byte("b"), 0o600))
	require.NoError(t, os.Symlink("a.txt", filepath.Join(files, "link")))

	f.run(t, day(21))

	copied := filepath.Join(f.dir, "files")
	assert.Equal(t, []string{"a.txt", "sub"}, names(t, copied), "a link is not copied")
	assertFile(t, filepath.Join(copied, "a.txt"), "a")
	assertFile(t, filepath.Join(copied, "sub", "b.txt"), "b")

	for _, dir := range []string{copied, filepath.Join(copied, "sub")} {
		info, err := os.Stat(dir)
		require.NoError(t, err)
		assert.Equal(t, os.FileMode(0o700), info.Mode().Perm())
	}

	// A file the backup has is not copied again; one a crash left half copied is not had.
	require.NoError(t, os.WriteFile(filepath.Join(files, "a.txt"), []byte("changed"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(files, "c.txt"), []byte("the whole of c"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(copied, "c.txt.4711.tmp"), []byte("the wh"), 0o600))

	f.run(t, day(22))

	assertFile(t, filepath.Join(copied, "a.txt"), "a")
	assertFile(t, filepath.Join(copied, "c.txt"), "the whole of c")
}

func TestFilesMayBeALinkToAnotherVolume(t *testing.T) {
	t.Parallel()

	f := newFixture(t, 3)

	elsewhere := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(elsewhere, "a.txt"), []byte("a"), 0o600))
	require.NoError(t, os.Symlink(elsewhere, filepath.Join(f.data, "files")))

	f.run(t, day(20))

	assertFile(t, filepath.Join(f.dir, "files", "a.txt"), "a")
}

// assertFile asserts the file at path holds content and only its owner may touch it.
func assertFile(t *testing.T, path, content string) {
	t.Helper()

	file, err := os.Open(path)
	require.NoError(t, err)

	defer file.Close()

	b, err := io.ReadAll(file)
	require.NoError(t, err)
	assert.Equal(t, content, string(b))

	info, err := file.Stat()
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
}

func TestTheBackupDirectoryIsPrivate(t *testing.T) {
	t.Parallel()

	f := newFixture(t, 3)
	f.dir = filepath.Join(f.dir, "nested")

	path := f.run(t, day(20))

	info, err := os.Stat(f.dir)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o700), info.Mode().Perm())

	info, err = os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())

	for _, mode := range []os.FileMode{0o740, 0o720, 0o710, 0o704, 0o702, 0o701} {
		require.NoError(t, os.Chmod(f.dir, mode))

		f.set(day(21))

		_, err := f.b.Run(t.Context(), f.dir)
		require.Error(t, err, mode)
		require.NoError(t, os.Chmod(f.dir, 0o700)) //nolint:gosec // a directory, which its owner enters
		assert.Equal(t, []string{"recallsynapsis-20261120.db"}, names(t, f.dir), mode)
	}
}

func TestNewestIsWhenTheNewestSnapshotWasMade(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	for _, none := range []string{"", filepath.Join(dir, "absent"), dir} {
		made, err := Newest(none)
		require.NoError(t, err)
		assert.True(t, made.IsZero(), none)
	}

	made := time.Date(2026, 11, 22, 3, 0, 5, 0, oslo)
	touch := func(name string, at time.Time) {
		path := filepath.Join(dir, name)
		require.NoError(t, os.WriteFile(path, nil, 0o600))
		require.NoError(t, os.Chtimes(path, at, at))
	}

	touch("recallsynapsis-20261121.db", made.Add(-24*time.Hour))
	touch("recallsynapsis-20261122.db", made)
	touch("recallsynapsis-20261123.db.4711.tmp", made.Add(time.Hour))
	touch("recallsynapsis-20261301.db", made.Add(time.Hour))
	touch("notes.txt", made.Add(time.Hour))

	got, err := Newest(dir)
	require.NoError(t, err)
	assert.True(t, made.Equal(got), got)
}
