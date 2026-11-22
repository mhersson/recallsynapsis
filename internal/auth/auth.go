// Package auth signs a user in and knows their sessions and API tokens:
// argon2id passwords, tokens stored only as hashes, and a limiter that slows
// wrong passwords. It owns the tables users, sessions and api_tokens.
package auth

import (
	"cmp"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/mhersson/recallsynapsis/internal/module"
	"github.com/mhersson/recallsynapsis/internal/store"
)

const (
	sessionLife = 30 * 24 * time.Hour
	// A session with more than this left is not written again, so a busy one
	// costs at most one write a day.
	sessionRefreshBelow = 29 * 24 * time.Hour
)

// Migrations are the tables auth owns. An empty password_hash is no password.
var Migrations = []store.Migration{{Owner: "auth", Version: 1, SQL: `
CREATE TABLE users (
	id            INTEGER PRIMARY KEY,
	name          TEXT NOT NULL UNIQUE,
	password_hash TEXT NOT NULL DEFAULT '',
	created_at    INTEGER NOT NULL
);

CREATE TABLE sessions (
	token_hash TEXT PRIMARY KEY,
	user_id    INTEGER NOT NULL REFERENCES users (id) ON DELETE CASCADE,
	created_at INTEGER NOT NULL,
	expires_at INTEGER NOT NULL
);

CREATE INDEX sessions_user ON sessions (user_id);

CREATE TABLE api_tokens (
	id           INTEGER PRIMARY KEY AUTOINCREMENT,
	user_id      INTEGER NOT NULL REFERENCES users (id) ON DELETE CASCADE,
	name         TEXT NOT NULL,
	access       TEXT NOT NULL CHECK (access IN ('read', 'write')),
	token_hash   TEXT NOT NULL UNIQUE,
	created_at   INTEGER NOT NULL,
	last_used_at INTEGER,
	UNIQUE (user_id, name)
);
`}}

// ErrBadLogin is returned for a wrong name and for a wrong password alike.
var ErrBadLogin = errors.New("wrong name or password")

// LimitedError is returned while too many sign-ins failed for a name from an address.
type LimitedError struct{ RetryAfter time.Duration }

func (e *LimitedError) Error() string {
	return fmt.Sprintf("too many failed sign-ins; retry in %s", e.RetryAfter.Round(time.Second))
}

// Service signs users in and knows their sessions and API tokens.
type Service struct {
	store   *store.Store
	now     func() time.Time
	limiter *limiter
	mu      sync.Mutex
}

// New returns a Service over s that reads time from now.
func New(s *store.Store, now func() time.Time) *Service {
	return &Service{store: s, now: now, limiter: newLimiter(now)}
}

// EnsureUser returns the id of the user named name, renaming the one user
// when it has another name and making the user when there is none.
func (a *Service) EnsureUser(ctx context.Context, name string) (int64, error) {
	var id int64

	err := a.store.Tx(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `UPDATE users SET name = ? WHERE name <> ? AND (SELECT count(*) FROM users) = 1`, name, name); err != nil {
			return err
		}

		if _, err := tx.ExecContext(ctx, `INSERT INTO users (name, created_at) VALUES (?, ?) ON CONFLICT (name) DO NOTHING`, name, a.now().UnixMilli()); err != nil {
			return err
		}

		return tx.QueryRowContext(ctx, `SELECT id FROM users WHERE name = ?`, name).Scan(&id)
	})
	if err != nil {
		return 0, fmt.Errorf("auth: ensure user: %w", err)
	}

	return id, nil
}

// UserName returns the name of the user with the id.
func (a *Service) UserName(ctx context.Context, id int64) (string, error) {
	var name string
	if err := a.store.QueryRowContext(ctx, `SELECT name FROM users WHERE id = ?`, id).Scan(&name); err != nil {
		return "", fmt.Errorf("auth: read user: %w", err)
	}

	return name, nil
}

// SetPasswordHash sets the user's password from an argon2id hash. The hash
// already stored changes nothing; a new one ends every session of the user.
func (a *Service) SetPasswordHash(ctx context.Context, userID int64, encoded string) error {
	if _, _, err := parsePHC(encoded); err != nil {
		return err
	}

	err := a.store.Tx(ctx, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx, `UPDATE users SET password_hash = ? WHERE id = ? AND password_hash <> ?`, encoded, userID, encoded)
		if err != nil {
			return err
		}

		if n, err := res.RowsAffected(); err != nil || n == 0 {
			return err
		}

		_, err = tx.ExecContext(ctx, `DELETE FROM sessions WHERE user_id = ?`, userID)

		return err
	})
	if err != nil {
		return fmt.Errorf("auth: set password: %w", err)
	}

	return nil
}

// Login returns a new session's cookie value and when it expires, counting a
// failure against the name from remote, the address the request came from.
func (a *Service) Login(ctx context.Context, name, password, remote string) (cookie string, expires time.Time, err error) {
	// The limiter keeps failed keys in memory, so it holds the name's digest, not the name.
	userID, hash, err := a.check(ctx, name, password, hashToken(name)+"|"+remote)
	if err != nil {
		return "", time.Time{}, err
	}

	return a.startSession(ctx, userID, hash)
}

// startSession makes a session of the user only while hash, the one the
// password matched, is still the user's: recallsynapsis password runs in
// another process, and a session made after its change would outlive it.
func (a *Service) startSession(ctx context.Context, userID int64, hash string) (cookie string, expires time.Time, err error) {
	now := a.now()
	if _, err := a.store.ExecContext(ctx, `DELETE FROM sessions WHERE expires_at <= ?`, now.UnixMilli()); err != nil {
		return "", time.Time{}, fmt.Errorf("auth: delete sessions: %w", err)
	}

	raw, tokenHash, err := newToken()
	if err != nil {
		return "", time.Time{}, err
	}

	expires = now.Add(sessionLife)

	res, err := a.store.ExecContext(ctx, `INSERT INTO sessions (token_hash, user_id, created_at, expires_at)
		SELECT ?, id, ?, ? FROM users WHERE id = ? AND password_hash = ?`,
		tokenHash, now.UnixMilli(), expires.UnixMilli(), userID, hash)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("auth: create session: %w", err)
	}

	if n, err := res.RowsAffected(); err != nil || n == 0 {
		return "", time.Time{}, cmp.Or(err, ErrBadLogin)
	}

	return raw, expires, nil
}

// check returns the user and the hash the password matched. It holds the lock
// so the limiter counts each attempt before the next is checked, and only one
// 64 MiB argon2 verify runs at a time.
func (a *Service) check(ctx context.Context, name, password, key string) (int64, string, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if wait, ok := a.limiter.allow(key); !ok {
		return 0, "", &LimitedError{RetryAfter: wait}
	}

	var (
		id   int64
		hash string
	)

	err := a.store.QueryRowContext(ctx, `SELECT id, password_hash FROM users WHERE name = ?`, name).Scan(&id, &hash)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return 0, "", fmt.Errorf("auth: read user: %w", err)
	}

	match, err := VerifyPassword(password, cmp.Or(hash, absent))
	if err != nil {
		return 0, "", err
	}

	if !match || hash == "" {
		a.limiter.failure(key)

		return 0, "", ErrBadLogin
	}

	a.limiter.reset(key)

	return id, hash, nil
}

// Session returns the caller of the live session the cookie value names, and
// keeps it alive: renewed is the session's new expiry when it moved, else zero.
func (a *Service) Session(ctx context.Context, cookie string) (c module.Caller, renewed time.Time, ok bool, err error) {
	hash := hashToken(cookie)

	var userID, ms int64

	err = a.store.QueryRowContext(ctx, `SELECT user_id, expires_at FROM sessions WHERE token_hash = ?`, hash).Scan(&userID, &ms)
	if errors.Is(err, sql.ErrNoRows) {
		return c, renewed, false, nil
	}

	if err != nil {
		return c, renewed, false, fmt.Errorf("auth: read session: %w", err)
	}

	now := a.now()

	expiry := time.UnixMilli(ms)
	if !expiry.After(now) {
		return c, renewed, false, nil
	}

	if expiry.Sub(now) < sessionRefreshBelow {
		renewed = now.Add(sessionLife)
		if _, err := a.store.ExecContext(ctx, `UPDATE sessions SET expires_at = ? WHERE token_hash = ?`, renewed.UnixMilli(), hash); err != nil {
			return c, time.Time{}, false, fmt.Errorf("auth: extend session: %w", err)
		}
	}

	return module.Caller{UserID: userID, Source: "web", Write: true}, renewed, true, nil
}

// Logout ends the session the cookie value names; an unknown one is a no-op.
func (a *Service) Logout(ctx context.Context, cookie string) error {
	if _, err := a.store.ExecContext(ctx, `DELETE FROM sessions WHERE token_hash = ?`, hashToken(cookie)); err != nil {
		return fmt.Errorf("auth: delete session: %w", err)
	}

	return nil
}
