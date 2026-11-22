// Package store opens the SQLite database, applies the migrations the packages
// that own tables bring, and hands every package the one handle. It owns no
// table but the migration ledger.
package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"modernc.org/sqlite"
	sqlite3 "modernc.org/sqlite/lib"
)

// The API gives the caller each of these errors with all the text that wraps
// it, so wrap them only with what the caller may see, never with a driver error.
var (
	// ErrNotFound is returned when a row does not exist.
	ErrNotFound = errors.New("not found")
	// ErrConflict is returned when a write loses against existing state.
	ErrConflict = errors.New("conflict")
	// ErrInvalid is returned when a write breaks a rule of the data it changes.
	ErrInvalid = errors.New("invalid")
)

// Store is the database handle every package shares. It is safe for concurrent use.
type Store struct {
	*sql.DB
}

// dsn builds a file: URI by concatenation because url.URL puts a relative
// path in the authority, which the driver rejects. _txlock=immediate makes
// every transaction take the write lock up front, so read-then-write
// sequences hold across processes.
func dsn(path string) string {
	return "file:" + path +
		"?_txlock=immediate" +
		"&_pragma=journal_mode(WAL)" +
		"&_pragma=synchronous(NORMAL)" +
		"&_pragma=busy_timeout(5000)" +
		"&_pragma=foreign_keys(1)"
}

// Open opens the database at path, whose directory must exist, and applies
// the migrations in ms that its ledger lacks.
func Open(ctx context.Context, path string, ms []Migration) (*Store, error) {
	db, err := sql.Open("sqlite", dsn(path))
	if err != nil {
		return nil, fmt.Errorf("store: open: %w", err)
	}

	// SQLite has one writer; extra connections let readers proceed under WAL.
	db.SetMaxOpenConns(5)
	db.SetMaxIdleConns(2)

	s := &Store{db}
	if err := s.migrate(ctx, ms); err != nil {
		_ = db.Close()

		return nil, err
	}

	return s, nil
}

// Tx runs fn in one transaction, committed when fn returns nil and rolled
// back otherwise.
func (s *Store) Tx(ctx context.Context, fn func(*sql.Tx) error) error {
	tx, err := s.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store: begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if err := fn(tx); err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("store: commit: %w", err)
	}

	return nil
}

// IsUniqueViolation reports whether err is a write refused by a UNIQUE
// constraint, which the package that owns the table answers as ErrConflict.
func IsUniqueViolation(err error) bool {
	se, ok := errors.AsType[*sqlite.Error](err)

	return ok && se.Code() == sqlite3.SQLITE_CONSTRAINT_UNIQUE
}
