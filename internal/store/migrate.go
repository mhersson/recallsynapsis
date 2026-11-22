package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// Migration is one versioned schema change of the package that owns the
// tables it touches. Versions count per owner, and a version is never edited.
type Migration struct {
	Owner   string
	Version int
	SQL     string
}

// Migrate applies, in order, the migrations not yet in the ledger, all with
// their ledger rows in one transaction so a failure leaves nothing behind.
// The transaction is immediate (see dsn), so a concurrent opener of a database
// already in WAL mode waits for it, then finds every version recorded.
func (s *Store) Migrate(ctx context.Context, ms []Migration) error {
	return s.Tx(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (
			owner      TEXT NOT NULL,
			version    INTEGER NOT NULL,
			applied_at INTEGER NOT NULL,
			PRIMARY KEY (owner, version)
		)`); err != nil {
			return fmt.Errorf("store: schema_migrations table: %w", err)
		}

		for _, m := range ms {
			// The ledger row goes first, and one already there means the migration ran.
			res, err := tx.ExecContext(ctx,
				`INSERT INTO schema_migrations (owner, version, applied_at) VALUES (?, ?, ?) ON CONFLICT DO NOTHING`,
				m.Owner, m.Version, time.Now().UnixMilli())
			if err != nil {
				return fmt.Errorf("store: migration %s v%d: record: %w", m.Owner, m.Version, err)
			}

			n, err := res.RowsAffected()
			if err != nil {
				return fmt.Errorf("store: migration %s v%d: record: %w", m.Owner, m.Version, err)
			}

			if n == 0 {
				continue
			}

			if _, err := tx.ExecContext(ctx, m.SQL); err != nil {
				return fmt.Errorf("store: migration %s v%d: %w", m.Owner, m.Version, err)
			}
		}

		return nil
	})
}
