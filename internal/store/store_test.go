package store

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"modernc.org/sqlite"
	sqlite3 "modernc.org/sqlite/lib"
)

func dbPath(t *testing.T) string {
	t.Helper()

	return filepath.Join(t.TempDir(), "recallsynapsis.db")
}

func open(t *testing.T, path string, ms ...Migration) *Store {
	t.Helper()

	s, err := Open(t.Context(), path, ms)
	require.NoError(t, err)

	t.Cleanup(func() { _ = s.Close() })

	return s
}

// inspect opens path without migrating, so looking changes nothing.
func inspect(t *testing.T, path string) *Store {
	t.Helper()

	db, err := sql.Open("sqlite", dsn(path))
	require.NoError(t, err)

	t.Cleanup(func() { _ = db.Close() })

	return &Store{db}
}

func column(t *testing.T, s *Store, query string) []string {
	t.Helper()

	rows, err := s.QueryContext(t.Context(), query)
	require.NoError(t, err)

	defer rows.Close()

	out := []string{}

	for rows.Next() {
		var v string
		require.NoError(t, rows.Scan(&v))

		out = append(out, v)
	}

	require.NoError(t, rows.Err())

	return out
}

func ledger(t *testing.T, s *Store) []string {
	t.Helper()

	return column(t, s, `SELECT owner || ' ' || version FROM schema_migrations ORDER BY owner, version`)
}

func tables(t *testing.T, s *Store) []string {
	t.Helper()

	return column(t, s, `SELECT name FROM sqlite_schema WHERE type = 'table' ORDER BY name`)
}

func TestASecondStartAppliesNothing(t *testing.T) {
	t.Parallel()

	path := dbPath(t)
	// Applying either a second time would fail: the table exists.
	ms := []Migration{
		{Owner: "auth", Version: 1, SQL: `CREATE TABLE a (id INTEGER PRIMARY KEY)`},
		{Owner: "auth", Version: 2, SQL: `ALTER TABLE a ADD COLUMN note TEXT`},
	}

	require.NoError(t, open(t, path, ms...).Close())

	s := open(t, path, ms...)
	assert.Equal(t, []string{"auth 1", "auth 2"}, ledger(t, s))
	require.NoError(t, s.Close())

	s = open(t, path, append(ms, Migration{Owner: "auth", Version: 3, SQL: `CREATE TABLE b (id INTEGER PRIMARY KEY)`})...)
	assert.Equal(t, []string{"auth 1", "auth 2", "auth 3"}, ledger(t, s))
}

func TestTwoOwnersEachHaveAVersionOne(t *testing.T) {
	t.Parallel()

	path := dbPath(t)
	auth := Migration{Owner: "auth", Version: 1, SQL: `CREATE TABLE users (id INTEGER PRIMARY KEY)`}
	require.NoError(t, open(t, path, auth).Close())

	s := open(t, path, auth, Migration{Owner: "core", Version: 1, SQL: `CREATE TABLE areas (id INTEGER PRIMARY KEY)`})

	assert.Equal(t, []string{"auth 1", "core 1"}, ledger(t, s))
	assert.Equal(t, []string{"areas", "schema_migrations", "users"}, tables(t, s))
}

func TestAFailedMigrationLeavesTheDatabaseAsItWas(t *testing.T) {
	t.Parallel()

	t.Run("on the first start", func(t *testing.T) {
		t.Parallel()

		path := dbPath(t)
		_, err := Open(t.Context(), path, []Migration{
			{Owner: "auth", Version: 1, SQL: `CREATE TABLE a (id INTEGER PRIMARY KEY)`},
			{Owner: "core", Version: 1, SQL: `CREATE TABLE broken (`},
		})
		require.Error(t, err)

		assert.Empty(t, tables(t, inspect(t, path)), "not even the ledger is left")
	})

	t.Run("on a later start", func(t *testing.T) {
		t.Parallel()

		path := dbPath(t)
		first := Migration{Owner: "auth", Version: 1, SQL: `CREATE TABLE a (id INTEGER PRIMARY KEY)`}
		require.NoError(t, open(t, path, first).Close())

		_, err := Open(t.Context(), path, []Migration{
			first,
			{Owner: "auth", Version: 2, SQL: `CREATE TABLE b (id INTEGER PRIMARY KEY)`},
			{Owner: "core", Version: 1, SQL: `INSERT INTO missing VALUES (1)`},
		})
		require.Error(t, err)

		s := inspect(t, path)
		assert.Equal(t, []string{"auth 1"}, ledger(t, s))
		assert.Equal(t, []string{"a", "schema_migrations"}, tables(t, s))
	})
}

func TestTwoOpenersDoNotCollide(t *testing.T) {
	t.Parallel()

	path := dbPath(t)
	ms := []Migration{
		{Owner: "auth", Version: 1, SQL: `CREATE TABLE a (id INTEGER PRIMARY KEY)`},
		{Owner: "core", Version: 1, SQL: `CREATE TABLE b (id INTEGER PRIMARY KEY)`},
	}

	// Two connections switching a new file to WAL at once get SQLITE_BUSY with no wait.
	require.NoError(t, open(t, path).Close())

	var (
		wg     sync.WaitGroup
		stores [2]*Store
		errs   [2]error
	)

	for i := range stores {
		wg.Go(func() { stores[i], errs[i] = Open(context.Background(), path, ms) })
	}

	wg.Wait()

	for i := range stores {
		require.NoError(t, errs[i])
		t.Cleanup(func() { _ = stores[i].Close() })
	}

	assert.Equal(t, []string{"auth 1", "core 1"}, ledger(t, stores[0]))
}

func TestTheSettings(t *testing.T) {
	t.Parallel()

	s := open(t, dbPath(t), Migration{Owner: "core", Version: 1, SQL: `
		CREATE TABLE parent (id INTEGER PRIMARY KEY);
		CREATE TABLE child (parent INTEGER NOT NULL REFERENCES parent(id));
		INSERT INTO parent (id) VALUES (1);
	`})

	assert.Equal(t, []string{"wal"}, column(t, s, `PRAGMA journal_mode`))

	_, err := s.ExecContext(t.Context(), `INSERT INTO child (parent) VALUES (1)`)
	require.NoError(t, err)

	_, err = s.ExecContext(t.Context(), `INSERT INTO child (parent) VALUES (2)`)
	se, ok := errors.AsType[*sqlite.Error](err)
	require.True(t, ok, err)
	assert.Equal(t, sqlite3.SQLITE_CONSTRAINT_FOREIGNKEY, se.Code())

	// A transaction takes the write lock when it begins, so a writer that will not wait is refused.
	conn, err := s.Conn(t.Context())
	require.NoError(t, err)

	defer conn.Close()

	_, err = conn.ExecContext(t.Context(), `PRAGMA busy_timeout = 0`)
	require.NoError(t, err)

	held, release := make(chan struct{}), make(chan struct{})
	done := make(chan error, 1)

	go func() {
		done <- s.Tx(t.Context(), func(*sql.Tx) error {
			close(held)
			<-release

			return nil
		})
	}()

	<-held

	_, err = conn.ExecContext(t.Context(), `INSERT INTO parent (id) VALUES (2)`)

	close(release)
	require.NoError(t, <-done)

	se, ok = errors.AsType[*sqlite.Error](err)
	require.True(t, ok, err)
	assert.Equal(t, sqlite3.SQLITE_BUSY, se.Code())
}

func TestTx(t *testing.T) {
	t.Parallel()

	s := open(t, dbPath(t), Migration{Owner: "core", Version: 1, SQL: `CREATE TABLE a (v TEXT)`})
	insert := func(v string) func(*sql.Tx) error {
		return func(tx *sql.Tx) error {
			_, err := tx.ExecContext(t.Context(), `INSERT INTO a (v) VALUES (?)`, v)

			return err
		}
	}

	require.NoError(t, s.Tx(t.Context(), insert("kept")))

	refused := errors.New("refused")
	err := s.Tx(t.Context(), func(tx *sql.Tx) error {
		require.NoError(t, insert("dropped")(tx))

		return refused
	})
	require.ErrorIs(t, err, refused)

	assert.Equal(t, []string{"kept"}, column(t, s, `SELECT v FROM a`))
}
