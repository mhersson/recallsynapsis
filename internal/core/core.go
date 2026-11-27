// Package core keeps the areas things live in: each user's one personal area
// and their projects, each with its brief and every wording of it, and finds
// one by its id, its slug or its name. It builds the feature modules, and asks
// the ones switched on to search, to resolve a reference and to find what
// links to one. It owns the tables areas and area_revisions.
package core

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/mhersson/recallsynapsis/internal/module"
	"github.com/mhersson/recallsynapsis/internal/store"
)

// Migrations are the tables core owns, applied after auth's, whose users
// own the areas. name_fold is the name as fold gives it: SQLite's NOCASE
// folds ASCII alone, and a name may be Norwegian. The brief is kept in
// description, so an image rolled back still reads the areas.
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
`}, {Owner: "core", Version: 2, SQL: `
ALTER TABLE areas ADD COLUMN brief_revision INTEGER NOT NULL DEFAULT 0;
ALTER TABLE areas ADD COLUMN brief_changed_at INTEGER;
ALTER TABLE areas ADD COLUMN brief_changed_by TEXT NOT NULL DEFAULT '';

CREATE TABLE area_revisions (
	area_id       INTEGER NOT NULL REFERENCES areas (id) ON DELETE CASCADE,
	revision      INTEGER NOT NULL,
	changed_at    INTEGER NOT NULL,
	changed_by    TEXT NOT NULL,
	said          TEXT NOT NULL DEFAULT '',
	restored_from INTEGER,
	brief         TEXT NOT NULL,
	PRIMARY KEY (area_id, revision)
);

UPDATE areas SET brief_revision = 1, brief_changed_at = CAST(unixepoch('subsec') * 1000 AS INTEGER), brief_changed_by = 'web'
	WHERE description <> '';

INSERT INTO area_revisions (area_id, revision, changed_at, changed_by, brief)
	SELECT id, 1, brief_changed_at, brief_changed_by, description FROM areas WHERE brief_revision = 1;
`}}

const (
	personal = "personal"
	project  = "project"

	personalName = "Personal"
	maxName      = 64
	maxBrief     = 4000
)

// Change is what a change to an area sets; a nil field stays as it is. A
// change of the brief names the revision it was made from, and no other does.
type Change struct {
	Name          *string `json:"name"`
	Brief         *string `json:"brief"`
	BriefRevision *int64  `json:"brief_revision"`
	Archived      *bool   `json:"archived"`
}

// Entry is a wording of a brief as the history lists it.
type Entry struct {
	Revision     int64     `json:"revision"`
	At           time.Time `json:"at"`
	By           string    `json:"by"`
	Said         string    `json:"said"`
	RestoredFrom *int64    `json:"restored_from"` // the revision a restore brought back, else null
}

// Version is a wording of a brief in full.
type Version struct {
	Entry
	Brief string `json:"brief"`
}

// Service keeps the areas. Every query is scoped to the user it is given.
type Service struct {
	store *store.Store
}

// New returns a Service over s.
func New(s *store.Store) *Service {
	return &Service{store: s}
}

const selectArea = `SELECT id, kind, name, slug, description, brief_revision, brief_changed_at, brief_changed_by, archived FROM areas `

const (
	// entryColumns are an entry's, in the order scanEntry reads them.
	entryColumns = `r.revision, r.changed_at, r.changed_by, r.said, r.restored_from`
	// revisions is the FROM and WHERE of one area's revisions, binding the
	// user's id and then the area's; a query may add to the WHERE.
	revisions = `area_revisions r JOIN areas ON areas.id = r.area_id WHERE areas.user_id = ? AND r.area_id = ?`
)

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
func (c *Service) Areas(ctx context.Context, userID int64, archived bool) ([]module.Area, error) {
	rows, err := c.store.QueryContext(ctx, selectArea+`WHERE user_id = ? AND (? OR NOT archived)
		ORDER BY kind <> 'personal', name_fold`, userID, archived)
	if err != nil {
		return nil, fmt.Errorf("core: list areas: %w", err)
	}
	defer rows.Close()

	out := []module.Area{}

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
func (c *Service) Area(ctx context.Context, userID, id int64) (module.Area, error) {
	return byID(ctx, c.store, userID, id)
}

// Find returns the user's area whose slug or name is ref, in any case. A
// slug is found before another area's name; no such area is ErrNotFound.
func (c *Service) Find(ctx context.Context, userID int64, ref string) (module.Area, error) {
	key := fold(ref)

	a, err := scan(c.store.QueryRowContext(ctx, selectArea+`WHERE user_id = ?1 AND ?2 IN (slug, name_fold)
		ORDER BY slug = ?2 DESC LIMIT 1`, userID, key))
	if errors.Is(err, sql.ErrNoRows) {
		return module.Area{}, fmt.Errorf("area %q: %w", ref, store.ErrNotFound)
	}

	if err != nil {
		return module.Area{}, fmt.Errorf("core: find area: %w", err)
	}

	return a, nil
}

// CreateProject makes a project of the caller, with its brief as revision 1
// when it has one. A name another of their areas has, in any case, is
// ErrConflict.
func (c *Service) CreateProject(ctx context.Context, caller module.Caller, name, brief string) (module.Area, error) {
	name, err := checkName(name)
	if err != nil {
		return module.Area{}, err
	}

	brief, err = checkBrief(brief)
	if err != nil {
		return module.Area{}, err
	}

	var a module.Area

	err = c.store.Tx(ctx, func(tx *sql.Tx) error {
		s, err := freeSlug(ctx, tx, caller.UserID, 0, name)
		if err != nil {
			return err
		}

		var id int64

		err = tx.QueryRowContext(ctx, `INSERT INTO areas (user_id, kind, name, name_fold, slug)
			VALUES (?, ?, ?, ?, ?) RETURNING id`, caller.UserID, project, name, fold(name), s).Scan(&id)
		if err != nil {
			return fmt.Errorf("core: create project: %w", err)
		}

		if brief != "" {
			if err := writeBrief(ctx, tx, caller, id, brief, nil); err != nil {
				return err
			}
		}

		a, err = byID(ctx, tx, caller.UserID, id)

		return err
	})
	if store.IsUniqueViolation(err) {
		return module.Area{}, fmt.Errorf("an area named %q exists: %w", name, store.ErrConflict)
	}

	if err != nil {
		return module.Area{}, err
	}

	return a, nil
}

// ChangeArea sets what ch holds on the caller's area with the id and returns
// the area. The personal area keeps its name and is never archived, a name
// another of the caller's areas has, in any case, is ErrConflict, and so is a
// brief changed from another revision than the area's.
func (c *Service) ChangeArea(ctx context.Context, caller module.Caller, id int64, ch Change) (module.Area, error) {
	if ch.Name != nil {
		name, err := checkName(*ch.Name)
		if err != nil {
			return module.Area{}, err
		}

		ch.Name = &name
	}

	if (ch.Brief == nil) != (ch.BriefRevision == nil) {
		return module.Area{}, fmt.Errorf("a change of the brief names the brief_revision it was made from, and no other change names one: %w",
			store.ErrInvalid)
	}

	if ch.Brief != nil {
		brief, err := checkBrief(*ch.Brief)
		if err != nil {
			return module.Area{}, err
		}

		ch.Brief = &brief
	}

	var a module.Area

	err := c.store.Tx(ctx, func(tx *sql.Tx) error {
		var err error
		if a, err = byID(ctx, tx, caller.UserID, id); err != nil {
			return err
		}

		if ch.Name != nil && *ch.Name != a.Name {
			if a.Kind == personal {
				return fmt.Errorf("the personal area keeps its name: %w", store.ErrInvalid)
			}

			// A slug moved by a change of case alone would break what refers to it.
			if fold(*ch.Name) != fold(a.Name) {
				if a.Slug, err = freeSlug(ctx, tx, caller.UserID, id, *ch.Name); err != nil {
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

		if ch.Brief != nil && *ch.BriefRevision != a.BriefRevision {
			return fmt.Errorf("the brief is at revision %d, and the change was made from %d: %w",
				a.BriefRevision, *ch.BriefRevision, store.ErrConflict)
		}

		_, err = tx.ExecContext(ctx, `UPDATE areas SET name = ?, name_fold = ?, slug = ?, archived = ? WHERE id = ? AND user_id = ?`,
			a.Name, fold(a.Name), a.Slug, a.Archived, id, caller.UserID)
		if err != nil {
			return fmt.Errorf("core: change area: %w", err)
		}

		if ch.Brief != nil {
			if err := writeBrief(ctx, tx, caller, id, *ch.Brief, nil); err != nil {
				return err
			}
		}

		a, err = byID(ctx, tx, caller.UserID, id)

		return err
	})
	if store.IsUniqueViolation(err) {
		return module.Area{}, fmt.Errorf("an area named %q exists: %w", a.Name, store.ErrConflict)
	}

	if err != nil {
		return module.Area{}, err
	}

	return a, nil
}

// History gives the wordings of the brief of the user's area with the id,
// the newest first; none while no brief was written.
func (c *Service) History(ctx context.Context, userID, id int64) ([]Entry, error) {
	if _, err := byID(ctx, c.store, userID, id); err != nil {
		return nil, err
	}

	rows, err := c.store.QueryContext(ctx, `SELECT `+entryColumns+` FROM `+revisions+` ORDER BY r.revision DESC`, userID, id)
	if err != nil {
		return nil, fmt.Errorf("core: history: %w", err)
	}
	defer rows.Close()

	out := []Entry{}

	for rows.Next() {
		e, err := scanEntry(rows)
		if err != nil {
			return nil, fmt.Errorf("core: history: %w", err)
		}

		out = append(out, e)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("core: history: %w", err)
	}

	return out, nil
}

// Revision gives the revision rev of the brief of the user's area with the id, in full.
func (c *Service) Revision(ctx context.Context, userID, id, rev int64) (Version, error) {
	var (
		v   Version
		err error
	)

	v.Entry, err = scanEntry(c.store.QueryRowContext(ctx, `SELECT `+entryColumns+`, r.brief FROM `+revisions+` AND r.revision = ?`,
		userID, id, rev), &v.Brief)
	if errors.Is(err, sql.ErrNoRows) {
		return Version{}, fmt.Errorf("revision %d of the brief of area %d: %w", rev, id, store.ErrNotFound)
	}

	if err != nil {
		return Version{}, fmt.Errorf("core: revision: %w", err)
	}

	return v, nil
}

// Restore gives the caller's area with the id the brief of its revision rev,
// one revision on, which names rev, and returns the area.
func (c *Service) Restore(ctx context.Context, caller module.Caller, id, rev int64) (module.Area, error) {
	var a module.Area

	err := c.store.Tx(ctx, func(tx *sql.Tx) error {
		var brief string

		err := tx.QueryRowContext(ctx, `SELECT r.brief FROM `+revisions+` AND r.revision = ?`, caller.UserID, id, rev).Scan(&brief)
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("revision %d of the brief of area %d: %w", rev, id, store.ErrNotFound)
		}

		if err != nil {
			return fmt.Errorf("core: restore: %w", err)
		}

		if err := writeBrief(ctx, tx, caller, id, brief, &rev); err != nil {
			return err
		}

		a, err = byID(ctx, tx, caller.UserID, id)

		return err
	})
	if err != nil {
		return module.Area{}, err
	}

	return a, nil
}

// writeBrief sets the brief of the caller's area with the id, one revision on,
// and keeps that revision, restored from the revision from unless it is nil.
func writeBrief(ctx context.Context, tx *sql.Tx, caller module.Caller, id int64, brief string, from *int64) error {
	_, err := tx.ExecContext(ctx, `UPDATE areas SET description = ?, brief_revision = brief_revision + 1, brief_changed_at = ?,
		brief_changed_by = ? WHERE id = ? AND user_id = ?`, brief, time.Now().UnixMilli(), caller.Source, id, caller.UserID)
	if err != nil {
		return fmt.Errorf("core: write the brief: %w", err)
	}

	_, err = tx.ExecContext(ctx, `INSERT INTO area_revisions (area_id, revision, changed_at, changed_by, restored_from, brief)
		SELECT id, brief_revision, brief_changed_at, brief_changed_by, ?, description FROM areas WHERE id = ? AND user_id = ?`,
		from, id, caller.UserID)
	if err != nil {
		return fmt.Errorf("core: keep the revision of the brief: %w", err)
	}

	return nil
}

// querier is the store or a transaction on it.
type querier interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

func byID(ctx context.Context, q querier, userID, id int64) (module.Area, error) {
	a, err := scan(q.QueryRowContext(ctx, selectArea+`WHERE user_id = ? AND id = ?`, userID, id))
	if errors.Is(err, sql.ErrNoRows) {
		return module.Area{}, fmt.Errorf("area %d: %w", id, store.ErrNotFound)
	}

	if err != nil {
		return module.Area{}, fmt.Errorf("core: read area: %w", err)
	}

	return a, nil
}

func scan(row interface{ Scan(dest ...any) error }) (module.Area, error) {
	var (
		a       module.Area
		changed sql.NullInt64
	)

	if err := row.Scan(&a.ID, &a.Kind, &a.Name, &a.Slug, &a.Brief, &a.BriefRevision, &changed, &a.BriefChangedBy, &a.Archived); err != nil {
		return module.Area{}, err
	}

	if changed.Valid {
		a.BriefChangedAt = new(time.UnixMilli(changed.Int64).UTC())
	}

	return a, nil
}

// scanEntry reads an entry of entryColumns, and extra after it.
func scanEntry(row interface{ Scan(dest ...any) error }, extra ...any) (Entry, error) {
	var (
		e  Entry
		at int64
	)

	if err := row.Scan(append([]any{&e.Revision, &at, &e.By, &e.Said, &e.RestoredFrom}, extra...)...); err != nil {
		return Entry{}, err
	}

	e.At = time.UnixMilli(at).UTC()

	return e, nil
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

func checkBrief(brief string) (string, error) {
	brief = strings.TrimSpace(brief)
	if utf8.RuneCountInString(brief) > maxBrief || strings.ContainsFunc(brief, func(r rune) bool {
		return unicode.IsControl(r) && r != '\n' && r != '\t'
	}) {
		return "", fmt.Errorf("a brief is at most %d characters, with no control characters but newlines and tabs: %w",
			maxBrief, store.ErrInvalid)
	}

	return brief, nil
}
