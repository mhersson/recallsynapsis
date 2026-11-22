package auth

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/mhersson/recallsynapsis/internal/module"
	"github.com/mhersson/recallsynapsis/internal/store"
)

const tokenPrefix = "recall_"

// Token is an API token as it is listed; its text is readable only when it is made.
type Token struct {
	ID         int64      `json:"id"`
	Name       string     `json:"name"`
	Access     string     `json:"access"` // "read" or "write"
	CreatedAt  time.Time  `json:"created_at"`
	LastUsedAt *time.Time `json:"last_used_at"`
}

// CreateToken stores a token of the user under name and returns it with its
// text, the only time that is readable. A name the user has is ErrConflict.
func (a *Service) CreateToken(ctx context.Context, userID int64, name, access string) (Token, string, error) {
	raw, _, err := newToken()
	if err != nil {
		return Token{}, "", err
	}

	text := tokenPrefix + raw
	// Stored to the millisecond, so made at the time a list will give.
	t := Token{Name: name, Access: access, CreatedAt: time.UnixMilli(a.now().UnixMilli()).UTC()}

	err = a.store.QueryRowContext(ctx, `INSERT INTO api_tokens (user_id, name, access, token_hash, created_at)
		VALUES (?, ?, ?, ?, ?) RETURNING id`, userID, name, access, hashToken(text), t.CreatedAt.UnixMilli()).Scan(&t.ID)
	if store.IsUniqueViolation(err) {
		return Token{}, "", fmt.Errorf("a token named %q exists: %w", name, store.ErrConflict)
	}

	if err != nil {
		return Token{}, "", fmt.Errorf("auth: create token: %w", err)
	}

	return t, text, nil
}

// Tokens lists the user's tokens by name.
func (a *Service) Tokens(ctx context.Context, userID int64) ([]Token, error) {
	rows, err := a.store.QueryContext(ctx, `SELECT id, name, access, created_at, last_used_at FROM api_tokens
		WHERE user_id = ? ORDER BY name`, userID)
	if err != nil {
		return nil, fmt.Errorf("auth: list tokens: %w", err)
	}
	defer rows.Close()

	out := []Token{}

	for rows.Next() {
		var (
			t        Token
			created  int64
			lastUsed sql.NullInt64
		)

		if err := rows.Scan(&t.ID, &t.Name, &t.Access, &created, &lastUsed); err != nil {
			return nil, fmt.Errorf("auth: list tokens: %w", err)
		}

		t.CreatedAt = time.UnixMilli(created).UTC()

		if lastUsed.Valid {
			at := time.UnixMilli(lastUsed.Int64).UTC()
			t.LastUsedAt = &at
		}

		out = append(out, t)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("auth: list tokens: %w", err)
	}

	return out, nil
}

// DeleteToken revokes the user's token with the id; one the user lacks is ErrNotFound.
func (a *Service) DeleteToken(ctx context.Context, userID, id int64) error {
	res, err := a.store.ExecContext(ctx, `DELETE FROM api_tokens WHERE id = ? AND user_id = ?`, id, userID)
	if err != nil {
		return fmt.Errorf("auth: delete token: %w", err)
	}

	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("auth: delete token: %w", err)
	}

	if n == 0 {
		return fmt.Errorf("token %d: %w", id, store.ErrNotFound)
	}

	return nil
}

// Bearer returns the caller the token text stands for and marks the token
// used; false when no token has that text.
func (a *Service) Bearer(ctx context.Context, text string) (module.Caller, bool, error) {
	var (
		c      module.Caller
		access string
	)

	err := a.store.QueryRowContext(ctx, `UPDATE api_tokens SET last_used_at = ? WHERE token_hash = ?
		RETURNING user_id, name, access`, a.now().UnixMilli(), hashToken(text)).Scan(&c.UserID, &c.Source, &access)
	if errors.Is(err, sql.ErrNoRows) {
		return module.Caller{}, false, nil
	}

	if err != nil {
		return module.Caller{}, false, fmt.Errorf("auth: use token: %w", err)
	}

	c.Write = access == "write"

	return c, true, nil
}
