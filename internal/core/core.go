// Package core keeps the areas things live in: each user's one personal area
// and their projects, and finds one by its id, its slug or its name. It owns
// the table areas.
package core

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/mhersson/recallsynapsis/internal/store"
)

// Migrations are the tables core owns, applied after auth's, whose users
// own the areas. name_fold is the name as fold gives it: SQLite's NOCASE
// folds ASCII alone, and a name may be Norwegian.
var Migrations = []store.Migration{{Owner: "core", Version: 1, SQL: `
CREATE TABLE areas (
	id          INTEGER PRIMARY KEY,
	user_id     INTEGER NOT NULL REFERENCES users (id) ON DELETE CASCADE,
	kind        TEXT NOT NULL CHECK (kind IN ('personal', 'project')),
	name        TEXT NOT NULL,
	name_fold   TEXT NOT NULL,
	slug        TEXT NOT NULL,
	description TEXT NOT NULL DEFAULT '',
	archived    INTEGER NOT NULL DEFAULT 0,
	UNIQUE (user_id, name_fold),
	UNIQUE (user_id, slug)
);

CREATE UNIQUE INDEX areas_personal ON areas (user_id) WHERE kind = 'personal';
`}}

const (
	personal = "personal"
	project  = "project"

	personalName   = "Personal"
	maxName        = 64
	maxDescription = 500
)

// Area is a user's personal area or one of their projects.
type Area struct {
	ID          int64  `json:"id"`
	Kind        string `json:"kind"` // "personal" or "project"
	Name        string `json:"name"`
	Slug        string `json:"slug"`
	Description string `json:"description"`
	Archived    bool   `json:"archived"`
}

// Change is what a change to an area sets; a nil field stays as it is.
type Change struct {
	Name        *string `json:"name"`
	Description *string `json:"description"`
	Archived    *bool   `json:"archived"`
}

// Service keeps the areas. Every query is scoped to the user it is given.
type Service struct {
	store *store.Store
}

// New returns a Service over s.
func New(s *store.Store) *Service {
	return &Service{store: s}
}

const selectArea = `SELECT id, kind, name, slug, description, archived FROM areas `

// EnsurePersonal makes the user's personal area when they have none.
func (c *Service) EnsurePersonal(ctx context.Context, userID int64) error {
	_, err := c.store.ExecContext(ctx, `INSERT INTO areas (user_id, kind, name, name_fold, slug) VALUES (?, ?, ?, ?, ?)
		ON CONFLICT (user_id) WHERE kind = 'personal' DO NOTHING`,
		userID, personal, personalName, fold(personalName), slug(personalName))
	if err != nil {
		return fmt.Errorf("core: make the personal area: %w", err)
	}

	return nil
}

// Areas lists the user's areas, the personal area first, then the projects
// by name in any case, leaving the archived ones out unless archived is true.
func (c *Service) Areas(ctx context.Context, userID int64, archived bool) ([]Area, error) {
	rows, err := c.store.QueryContext(ctx, selectArea+`WHERE user_id = ? AND (? OR NOT archived)
		ORDER BY kind <> 'personal', name_fold`, userID, archived)
	if err != nil {
		return nil, fmt.Errorf("core: list areas: %w", err)
	}
	defer rows.Close()

	out := []Area{}

	for rows.Next() {
		a, err := scan(rows)
		if err != nil {
			return nil, fmt.Errorf("core: list areas: %w", err)
		}

		out = append(out, a)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("core: list areas: %w", err)
	}

	return out, nil
}

// Area returns the user's area with the id; one the user lacks is ErrNotFound.
func (c *Service) Area(ctx context.Context, userID, id int64) (Area, error) {
	return byID(ctx, c.store, userID, id)
}

// Find returns the user's area whose slug or name is ref, in any case. A
// slug is found before another area's name; no such area is ErrNotFound.
func (c *Service) Find(ctx context.Context, userID int64, ref string) (Area, error) {
	key := fold(ref)

	a, err := scan(c.store.QueryRowContext(ctx, selectArea+`WHERE user_id = ?1 AND ?2 IN (slug, name_fold)
		ORDER BY slug = ?2 DESC LIMIT 1`, userID, key))
	if errors.Is(err, sql.ErrNoRows) {
		return Area{}, fmt.Errorf("area %q: %w", ref, store.ErrNotFound)
	}

	if err != nil {
		return Area{}, fmt.Errorf("core: find area: %w", err)
	}

	return a, nil
}

// CreateProject makes a project of the user. A name another of their areas
// has, in any case, is ErrConflict.
func (c *Service) CreateProject(ctx context.Context, userID int64, name, description string) (Area, error) {
	name, err := checkName(name)
	if err != nil {
		return Area{}, err
	}

	description, err = checkDescription(description)
	if err != nil {
		return Area{}, err
	}

	a := Area{Kind: project, Name: name, Description: description}

	err = c.store.Tx(ctx, func(tx *sql.Tx) error {
		var err error
		if a.Slug, err = freeSlug(ctx, tx, userID, 0, name); err != nil {
			return err
		}

		err = tx.QueryRowContext(ctx, `INSERT INTO areas (user_id, kind, name, name_fold, slug, description)
			VALUES (?, ?, ?, ?, ?, ?) RETURNING id`, userID, project, name, fold(name), a.Slug, a.Description).Scan(&a.ID)
		if err != nil {
			return fmt.Errorf("core: create project: %w", err)
		}

		return nil
	})
	if store.IsUniqueViolation(err) {
		return Area{}, fmt.Errorf("an area named %q exists: %w", name, store.ErrConflict)
	}

	if err != nil {
		return Area{}, err
	}

	return a, nil
}

// ChangeArea sets what ch holds on the user's area with the id and returns
// the area. The personal area keeps its name and is never archived, and a
// name another of the user's areas has, in any case, is ErrConflict.
func (c *Service) ChangeArea(ctx context.Context, userID, id int64, ch Change) (Area, error) {
	if ch.Name != nil {
		name, err := checkName(*ch.Name)
		if err != nil {
			return Area{}, err
		}

		ch.Name = &name
	}

	if ch.Description != nil {
		description, err := checkDescription(*ch.Description)
		if err != nil {
			return Area{}, err
		}

		ch.Description = &description
	}

	var a Area

	err := c.store.Tx(ctx, func(tx *sql.Tx) error {
		var err error
		if a, err = byID(ctx, tx, userID, id); err != nil {
			return err
		}

		if ch.Name != nil && *ch.Name != a.Name {
			if a.Kind == personal {
				return fmt.Errorf("the personal area keeps its name: %w", store.ErrInvalid)
			}

			// A slug moved by a change of case alone would break what refers to it.
			if fold(*ch.Name) != fold(a.Name) {
				if a.Slug, err = freeSlug(ctx, tx, userID, id, *ch.Name); err != nil {
					return err
				}
			}

			a.Name = *ch.Name
		}

		if ch.Archived != nil && *ch.Archived != a.Archived {
			if a.Kind == personal {
				return fmt.Errorf("the personal area is never archived: %w", store.ErrInvalid)
			}

			a.Archived = *ch.Archived
		}

		if ch.Description != nil {
			a.Description = *ch.Description
		}

		_, err = tx.ExecContext(ctx, `UPDATE areas SET name = ?, name_fold = ?, slug = ?, description = ?, archived = ?
			WHERE id = ? AND user_id = ?`, a.Name, fold(a.Name), a.Slug, a.Description, a.Archived, id, userID)
		if err != nil {
			return fmt.Errorf("core: change area: %w", err)
		}

		return nil
	})
	if store.IsUniqueViolation(err) {
		return Area{}, fmt.Errorf("an area named %q exists: %w", a.Name, store.ErrConflict)
	}

	if err != nil {
		return Area{}, err
	}

	return a, nil
}

// querier is the store or a transaction on it.
type querier interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

func byID(ctx context.Context, q querier, userID, id int64) (Area, error) {
	a, err := scan(q.QueryRowContext(ctx, selectArea+`WHERE user_id = ? AND id = ?`, userID, id))
	if errors.Is(err, sql.ErrNoRows) {
		return Area{}, fmt.Errorf("area %d: %w", id, store.ErrNotFound)
	}

	if err != nil {
		return Area{}, fmt.Errorf("core: read area: %w", err)
	}

	return a, nil
}

func scan(row interface{ Scan(dest ...any) error }) (Area, error) {
	var a Area

	err := row.Scan(&a.ID, &a.Kind, &a.Name, &a.Slug, &a.Description, &a.Archived)

	return a, err
}

// freeSlug is name's slug, or the first of it with -2, -3 and on that no
// other area of the user has; self is the area being renamed, else 0.
func freeSlug(ctx context.Context, tx *sql.Tx, userID, self int64, name string) (string, error) {
	base := slug(name)

	for n := 1; ; n++ {
		s := base
		if n > 1 {
			s += "-" + strconv.Itoa(n)
		}

		var taken bool
		if err := tx.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM areas WHERE user_id = ? AND slug = ? AND id <> ?)`,
			userID, s, self).Scan(&taken); err != nil {
			return "", fmt.Errorf("core: find a free slug: %w", err)
		}

		if !taken {
			return s, nil
		}
	}
}

// slug is name in lower case with its letters and digits kept and each run of
// anything else between them made one hyphen; "project" when none is left.
func slug(name string) string {
	var b strings.Builder

	gap := false

	for _, r := range strings.ToLower(name) {
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) {
			gap = true

			continue
		}

		if gap && b.Len() > 0 {
			b.WriteByte('-')
		}

		b.WriteRune(r)

		gap = false
	}

	if b.Len() == 0 {
		return project
	}

	return b.String()
}

// fold is s as names are compared: trimmed and in lower case.
func fold(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}

func checkName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" || utf8.RuneCountInString(name) > maxName || strings.ContainsFunc(name, unicode.IsControl) {
		return "", fmt.Errorf("a name is 1 to %d characters, with no control characters: %w", maxName, store.ErrInvalid)
	}

	return name, nil
}

func checkDescription(description string) (string, error) {
	description = strings.TrimSpace(description)
	if utf8.RuneCountInString(description) > maxDescription || strings.ContainsFunc(description, func(r rune) bool {
		return unicode.IsControl(r) && r != '\n' && r != '\t'
	}) {
		return "", fmt.Errorf("a description is at most %d characters, with no control characters but newlines and tabs: %w",
			maxDescription, store.ErrInvalid)
	}

	return description, nil
}
