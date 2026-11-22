// Package notes keeps each user's notes and ideas: each in one of the user's
// areas, with its tags, the inbox flag of a capture that waits to be filed,
// and its history: a revision written by every change, any of which can be
// restored. It searches them through an index of their title, body and tags.
// A note in the trash is found by no search, and kept unchanged until it is
// restored or deleted for good. It owns the tables notes, note_tags,
// note_revisions and notes_fts.
package notes

import (
	"cmp"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/mhersson/recallsynapsis/internal/module"
	"github.com/mhersson/recallsynapsis/internal/store"
)

// migrations make the module's schema. A note's id is AUTOINCREMENT, so a
// note deleted for good never gives its id to another. Nothing writes said or
// links: they wait empty. The index keeps no copy of the text, which a hit
// reads from the note, and its trigrams let a word find part of a compound.
var migrations = []store.Migration{{Version: 1, SQL: `
CREATE TABLE notes (
	id         INTEGER PRIMARY KEY AUTOINCREMENT,
	user_id    INTEGER NOT NULL REFERENCES users (id) ON DELETE CASCADE,
	area_id    INTEGER NOT NULL REFERENCES areas (id) ON DELETE CASCADE,
	kind       TEXT NOT NULL CHECK (kind IN ('note', 'idea')),
	title      TEXT NOT NULL,
	body       TEXT NOT NULL,
	inbox      INTEGER NOT NULL,
	revision   INTEGER NOT NULL,
	source     TEXT NOT NULL,
	created_at INTEGER NOT NULL,
	changed_at INTEGER NOT NULL,
	trashed_at INTEGER
);

CREATE INDEX notes_changed ON notes (user_id, changed_at);

CREATE TABLE note_tags (
	note_id INTEGER NOT NULL REFERENCES notes (id) ON DELETE CASCADE,
	tag     TEXT NOT NULL,
	PRIMARY KEY (note_id, tag)
) WITHOUT ROWID;

CREATE TABLE note_revisions (
	note_id       INTEGER NOT NULL REFERENCES notes (id) ON DELETE CASCADE,
	revision      INTEGER NOT NULL,
	at            INTEGER NOT NULL,
	source        TEXT NOT NULL,
	said          TEXT NOT NULL DEFAULT '',
	restored_from INTEGER,
	title         TEXT NOT NULL,
	body          TEXT NOT NULL,
	kind          TEXT NOT NULL,
	area_id       INTEGER NOT NULL,
	inbox         INTEGER NOT NULL,
	tags          TEXT NOT NULL,
	links         TEXT NOT NULL DEFAULT '[]',
	PRIMARY KEY (note_id, revision)
);
`}, {Version: 2, SQL: `
CREATE VIRTUAL TABLE notes_fts USING fts5 (title, body, tags, tokenize = 'trigram', content = '', contentless_delete = 1);

INSERT INTO notes_fts (rowid, title, body, tags)
	SELECT id, title, body, coalesce((SELECT group_concat(tag, ' ') FROM note_tags WHERE note_id = notes.id), '') FROM notes;

CREATE TRIGGER notes_fts_delete AFTER DELETE ON notes BEGIN
	DELETE FROM notes_fts WHERE rowid = old.id;
END;
`}}

const (
	name = "notes"

	maxTitle     = 200
	maxTag       = 40
	defaultLimit = 50
	maxLimit     = 500
)

var (
	kinds    = []string{"note", "idea"}
	errKind  = fmt.Errorf(`a kind is "note" or "idea": %w`, store.ErrInvalid)
	errLimit = fmt.Errorf("limit is 1 to %d: %w", maxLimit, store.ErrInvalid)

	errTrashed = fmt.Errorf("the note is in the trash, and is restored before it is changed: %w", store.ErrConflict)
)

type notes struct {
	module.Deps
}

// New builds the notes module.
func New(d module.Deps) module.Module {
	n := notes{d}

	return module.Module{
		Name:       name,
		Migrations: migrations,
		Search:     n.search,
		Routes: []module.Route{
			{Pattern: "GET /api/notes", Access: module.Read, Serve: n.serveList},
			{Pattern: "POST /api/notes", Access: module.Write, Serve: n.serveCreate},
			{Pattern: "GET /api/notes/tags", Access: module.Read, Serve: n.serveTags},
			{Pattern: "GET /api/notes/{id}", Access: module.Read, Serve: n.serveGet},
			{Pattern: "PATCH /api/notes/{id}", Access: module.Write, Serve: n.serveChange},
			{Pattern: "POST /api/notes/{id}/trash", Access: module.Write, Serve: n.serveTrash(true)},
			{Pattern: "POST /api/notes/{id}/restore", Access: module.Write, Serve: n.serveTrash(false)},
			{Pattern: "DELETE /api/notes/{id}", Access: module.Owner, Serve: n.serveDelete},
			{Pattern: "DELETE /api/notes/trash", Access: module.Owner, Serve: n.serveEmpty},
			{Pattern: "GET /api/notes/{id}/revisions", Access: module.Read, Serve: n.serveHistory},
			{Pattern: "GET /api/notes/{id}/revisions/{n}", Access: module.Read, Serve: n.serveRevision},
			{Pattern: "POST /api/notes/{id}/revisions/{n}/restore", Access: module.Write, Serve: n.serveRestoreRevision},
		},
	}
}

// row is a note as a list gives it.
type row struct {
	ID        int64      `json:"id"`
	Ref       module.Ref `json:"ref"`
	Area      string     `json:"area"` // its slug
	Kind      string     `json:"kind"`
	Title     string     `json:"title"`
	Excerpt   string     `json:"excerpt"` // the first 200 characters of the body
	Inbox     bool       `json:"inbox"`
	Tags      []string   `json:"tags"`
	Revision  int64      `json:"revision"`
	Source    string     `json:"source"` // that of the last change
	CreatedAt time.Time  `json:"created_at"`
	ChangedAt time.Time  `json:"changed_at"`
	TrashedAt *time.Time `json:"trashed_at"` // null unless the note is in the trash
}

// note is a note in full.
type note struct {
	row
	Body string `json:"body"`
}

// entry is a revision as the history lists it.
type entry struct {
	Revision     int64     `json:"revision"`
	At           time.Time `json:"at"`
	Source       string    `json:"source"`
	Said         string    `json:"said"`
	RestoredFrom *int64    `json:"restored_from"` // the revision a restore brought back, else null
}

// version is a revision in full: the note as that change left it.
type version struct {
	entry
	Title string   `json:"title"`
	Body  string   `json:"body"`
	Kind  string   `json:"kind"`
	Area  string   `json:"area"` // its slug
	Inbox bool     `json:"inbox"`
	Tags  []string `json:"tags"`
}

// draft is a new note: of kind note unless it says, in the personal area
// unless it names another, as a slug or a name.
type draft struct {
	Title string   `json:"title"`
	Body  string   `json:"body"`
	Kind  string   `json:"kind"`
	Area  string   `json:"area"`
	Tags  []string `json:"tags"`
	Inbox bool     `json:"inbox"`
}

// change is what a change sets; a nil field stays as it is. Revision is the
// one the change was made from, which one that only appends need not name.
type change struct {
	Revision *int64    `json:"revision"`
	Title    *string   `json:"title"`
	Body     *string   `json:"body"`
	Append   *string   `json:"append"` // added to the end of the body, as a paragraph of its own
	Kind     *string   `json:"kind"`
	Area     *string   `json:"area"`
	Tags     *[]string `json:"tags"`
	Inbox    *bool     `json:"inbox"`
}

// filter is what a list asks for; a zero field asks for nothing.
type filter struct {
	area    string // a slug or a name
	kind    string
	inbox   *bool
	tag     string
	trashed bool // the trash alone, else none of it
	limit   int  // defaultLimit when 0
}

// page is what a list gives: its notes, and how many it was asked for in all.
type page struct {
	Notes []row `json:"notes"`
	Total int   `json:"total"`
}

type tagCount struct {
	Tag   string `json:"tag"`
	Count int    `json:"count"`
}

// fields are what a person or a client sets of a note.
type fields struct {
	title, body, kind string
	area              int64
	inbox             bool
}

// check refuses fields no note may hold.
func (f fields) check() error {
	switch {
	case utf8.RuneCountInString(f.title) > maxTitle || strings.ContainsFunc(f.title, unicode.IsControl):
		return fmt.Errorf("a title is at most %d characters, with no control characters: %w", maxTitle, store.ErrInvalid)
	case !slices.Contains(kinds, f.kind):
		return errKind
	case f.title == "" && strings.TrimSpace(f.body) == "":
		return fmt.Errorf("a note has a title or a body: %w", store.ErrInvalid)
	}

	return nil
}

// columns are a row's, in the order scanRow reads them.
const columns = `id, area_id, kind, title, substr(body, 1, 200), inbox, revision, source, created_at, changed_at, trashed_at,
	coalesce((SELECT group_concat(tag, ' ' ORDER BY tag) FROM note_tags WHERE note_id = notes.id), '')`

// snapshot writes the revision of the note ?1 as it now stands, restored from
// the revision ?2 or from none when it is null.
const snapshot = `INSERT INTO note_revisions (note_id, revision, at, source, restored_from, title, body, kind, area_id, inbox, tags)
	SELECT id, revision, changed_at, source, ?2, title, body, kind, area_id, inbox,
		(SELECT json_group_array(tag ORDER BY tag) FROM note_tags WHERE note_id = ?1)
	FROM notes WHERE id = ?1`

const (
	// index writes the note ?1 into the search index as it now stands.
	index = `REPLACE INTO notes_fts (rowid, title, body, tags)
		SELECT id, title, body, coalesce((SELECT group_concat(tag, ' ') FROM note_tags WHERE note_id = ?1), '') FROM notes WHERE id = ?1`
	// purge rewrites the search index whole. A delete only marks a note's
	// rows in it, whose text a backup would copy until a merge drops them.
	purge = `INSERT INTO notes_fts (notes_fts) VALUES ('optimize')`
)

const (
	// entryColumns are an entry's, in the order scanEntry reads them.
	entryColumns = `r.revision, r.at, r.source, r.said, r.restored_from`
	// revisions is the FROM and WHERE of one note's revisions, binding the
	// user's id and then the note's; a query may add to the WHERE.
	revisions = `note_revisions r JOIN notes ON notes.id = r.note_id WHERE notes.user_id = ? AND r.note_id = ?`
)

// scanRow reads a row of columns, and extra after it, naming its area by
// its slug in slugs.
func scanRow(sc interface{ Scan(dest ...any) error }, slugs map[int64]string, extra ...any) (row, error) {
	var (
		r                      row
		area, created, changed int64
		trashed                sql.NullInt64
		tags                   string
	)

	err := sc.Scan(append([]any{
		&r.ID, &area, &r.Kind, &r.Title, &r.Excerpt, &r.Inbox, &r.Revision, &r.Source,
		&created, &changed, &trashed, &tags,
	}, extra...)...)
	if err != nil {
		return row{}, err
	}

	// Each note is in an area of its user, so a note outside them is another user's.
	slug, ok := slugs[area]
	if !ok {
		return row{}, fmt.Errorf("note %d is in no area of the caller", r.ID)
	}

	// A tag has no spaces, so the space that joins them parts them again.
	r.Ref, r.Area, r.Tags = module.NewRef(name, r.ID), slug, strings.Fields(tags)
	r.CreatedAt, r.ChangedAt = time.UnixMilli(created).UTC(), time.UnixMilli(changed).UTC()

	if trashed.Valid {
		r.TrashedAt = new(time.UnixMilli(trashed.Int64).UTC())
	}

	return r, nil
}

// scanEntry reads an entry of entryColumns, and extra after it.
func scanEntry(sc interface{ Scan(dest ...any) error }, extra ...any) (entry, error) {
	var (
		e  entry
		at int64
	)

	if err := sc.Scan(append([]any{&e.Revision, &at, &e.Source, &e.Said, &e.RestoredFrom}, extra...)...); err != nil {
		return entry{}, err
	}

	e.At = time.UnixMilli(at).UTC()

	return e, nil
}

// places gives the slug of each area of the user by its id, and the id of
// their personal area.
func (n notes) places(ctx context.Context, userID int64) (map[int64]string, int64, error) {
	areas, err := n.Areas(ctx, userID)
	if err != nil {
		return nil, 0, err
	}

	var (
		slugs    = make(map[int64]string, len(areas))
		personal int64
	)

	for _, a := range areas {
		slugs[a.ID] = a.Slug
		if a.Kind == "personal" {
			personal = a.ID
		}
	}

	return slugs, personal, nil
}

// get gives the caller's note with the id.
func (n notes) get(ctx context.Context, id int64) (note, error) {
	c, _ := module.CallerFrom(ctx)

	slugs, _, err := n.places(ctx, c.UserID)
	if err != nil {
		return note{}, err
	}

	return n.read(ctx, slugs, c.UserID, id)
}

func (n notes) read(ctx context.Context, slugs map[int64]string, userID, id int64) (note, error) {
	var (
		out note
		err error
	)

	out.row, err = scanRow(n.Store.QueryRowContext(ctx, `SELECT `+columns+`, body FROM notes WHERE user_id = ? AND id = ?`,
		userID, id), slugs, &out.Body)
	if errors.Is(err, sql.ErrNoRows) {
		return note{}, fmt.Errorf("note %d: %w", id, store.ErrNotFound)
	}

	if err != nil {
		return note{}, fmt.Errorf("notes: read: %w", err)
	}

	return out, nil
}

// list gives the caller's notes that f asks for, the most recently changed
// first or the trash the most recently trashed first, and how many there are
// in all.
func (n notes) list(ctx context.Context, f filter) (page, error) {
	c, _ := module.CallerFrom(ctx)

	if f.limit == 0 {
		f.limit = defaultLimit
	}

	if f.limit < 0 || f.limit > maxLimit {
		return page{}, errLimit
	}

	if f.kind != "" && !slices.Contains(kinds, f.kind) {
		return page{}, errKind
	}

	var area int64

	if f.area != "" {
		a, err := n.FindArea(ctx, c.UserID, f.area)
		if err != nil {
			return page{}, err
		}

		area = a.ID
	}

	if f.tag != "" {
		var err error
		if f.tag, err = cleanTag(f.tag); err != nil {
			return page{}, err
		}
	}

	slugs, _, err := n.places(ctx, c.UserID)
	if err != nil {
		return page{}, err
	}

	// The window counts every row the query matches, before the limit cuts
	// them. Outside the trash trashed_at is null, so only the trash sorts by it.
	rows, err := n.Store.QueryContext(ctx, `SELECT `+columns+`, count(*) OVER () FROM notes
		WHERE user_id = ?1 AND ?2 IN (0, area_id) AND ?3 IN ('', kind) AND (?4 IS NULL OR inbox = ?4)
			AND (?5 = '' OR EXISTS (SELECT 1 FROM note_tags WHERE note_id = notes.id AND tag = ?5))
			AND (trashed_at IS NOT NULL) = ?6
		ORDER BY coalesce(trashed_at, changed_at) DESC, id DESC LIMIT ?7`, c.UserID, area, f.kind, f.inbox, f.tag, f.trashed, f.limit)
	if err != nil {
		return page{}, fmt.Errorf("notes: list: %w", err)
	}
	defer rows.Close()

	out := page{Notes: []row{}}

	for rows.Next() {
		r, err := scanRow(rows, slugs, &out.Total)
		if err != nil {
			return page{}, fmt.Errorf("notes: list: %w", err)
		}

		out.Notes = append(out.Notes, r)
	}

	if err := rows.Err(); err != nil {
		return page{}, fmt.Errorf("notes: list: %w", err)
	}

	return out, nil
}

// tags gives each tag of the caller's notes outside the trash, by tag, with
// how many notes have it.
func (n notes) tags(ctx context.Context) ([]tagCount, error) {
	c, _ := module.CallerFrom(ctx)

	rows, err := n.Store.QueryContext(ctx, `SELECT tag, count(*) FROM note_tags JOIN notes ON notes.id = note_id
		WHERE user_id = ? AND trashed_at IS NULL GROUP BY tag ORDER BY tag`, c.UserID)
	if err != nil {
		return nil, fmt.Errorf("notes: tags: %w", err)
	}
	defer rows.Close()

	out := []tagCount{}

	for rows.Next() {
		var t tagCount
		if err := rows.Scan(&t.Tag, &t.Count); err != nil {
			return nil, fmt.Errorf("notes: tags: %w", err)
		}

		out = append(out, t)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("notes: tags: %w", err)
	}

	return out, nil
}

// create makes d a note of the caller, at revision 1. A note put in the
// inbox is in the personal area, whatever area d names.
func (n notes) create(ctx context.Context, d draft) (note, error) {
	c, _ := module.CallerFrom(ctx)

	slugs, personal, err := n.places(ctx, c.UserID)
	if err != nil {
		return note{}, err
	}

	f := fields{title: strings.TrimSpace(d.Title), body: d.Body, kind: cmp.Or(d.Kind, "note"), area: personal, inbox: d.Inbox}

	if d.Area != "" {
		a, err := n.FindArea(ctx, c.UserID, d.Area)
		if err != nil {
			return note{}, err
		}

		f.area = a.ID
	}

	if f.inbox {
		f.area = personal
	}

	tags, err := cleanTags(d.Tags)
	if err != nil {
		return note{}, err
	}

	if err := f.check(); err != nil {
		return note{}, err
	}

	var id int64

	err = n.Store.Tx(ctx, func(tx *sql.Tx) error {
		now := n.Now().UnixMilli()

		err := tx.QueryRowContext(ctx, `INSERT INTO notes (user_id, area_id, kind, title, body, inbox, revision, source, created_at, changed_at)
			VALUES (?, ?, ?, ?, ?, ?, 1, ?, ?, ?) RETURNING id`, c.UserID, f.area, f.kind, f.title, f.body, f.inbox, c.Source, now, now).Scan(&id)
		if err != nil {
			return fmt.Errorf("notes: create: %w", err)
		}

		return keep(ctx, tx, id, &tags, nil)
	})
	if err != nil {
		return note{}, err
	}

	n.Changed(ctx, module.Change{Module: name, Kind: "note", ID: id, Area: f.area})

	return n.read(ctx, slugs, c.UserID, id)
}

// change sets what ch gives on the caller's note with the id, one revision
// on. A change of area, kind or tags takes the note out of the inbox unless
// the same change puts it there, and a note in the inbox is in the personal
// area. A change from another revision than the note's is ErrConflict, unless
// it only appends, and so is a change of a note in the trash.
func (n notes) change(ctx context.Context, id int64, ch change) (note, error) {
	c, _ := module.CallerFrom(ctx)
	appends := ch == change{Revision: ch.Revision, Append: ch.Append} && ch.Append != nil

	switch {
	case ch == change{Revision: ch.Revision}:
		return note{}, fmt.Errorf("a change gives at least one field to change: %w", store.ErrInvalid)
	case ch.Body != nil && ch.Append != nil:
		return note{}, fmt.Errorf("a change gives body or append, not both: %w", store.ErrInvalid)
	case ch.Append != nil && *ch.Append == "":
		return note{}, fmt.Errorf("append is the text to add: %w", store.ErrInvalid)
	case ch.Revision == nil && !appends:
		return note{}, fmt.Errorf("a change names the revision it was made from: %w", store.ErrInvalid)
	}

	slugs, personal, err := n.places(ctx, c.UserID)
	if err != nil {
		return note{}, err
	}

	var area int64

	if ch.Area != nil {
		a, err := n.FindArea(ctx, c.UserID, *ch.Area)
		if err != nil {
			return note{}, err
		}

		area = a.ID
	}

	if ch.Tags != nil {
		tags, err := cleanTags(*ch.Tags)
		if err != nil {
			return note{}, err
		}

		ch.Tags = &tags
	}

	var f fields

	err = n.Store.Tx(ctx, func(tx *sql.Tx) error {
		var (
			revision int64
			trashed  bool
		)

		err := tx.QueryRowContext(ctx, `SELECT title, body, kind, area_id, inbox, revision, trashed_at IS NOT NULL FROM notes
			WHERE user_id = ? AND id = ?`, c.UserID, id).Scan(&f.title, &f.body, &f.kind, &f.area, &f.inbox, &revision, &trashed)
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("note %d: %w", id, store.ErrNotFound)
		}

		if err != nil {
			return fmt.Errorf("notes: change: %w", err)
		}

		if trashed {
			return errTrashed
		}

		if !appends && *ch.Revision != revision {
			return fmt.Errorf("note %d is at revision %d, and the change was made from %d: %w", id, revision, *ch.Revision, store.ErrConflict)
		}

		f.apply(ch, area, personal)

		if err := f.check(); err != nil {
			return err
		}

		_, err = tx.ExecContext(ctx, `UPDATE notes SET title = ?, body = ?, kind = ?, area_id = ?, inbox = ?,
			revision = revision + 1, source = ?, changed_at = ? WHERE user_id = ? AND id = ?`,
			f.title, f.body, f.kind, f.area, f.inbox, c.Source, n.Now().UnixMilli(), c.UserID, id)
		if err != nil {
			return fmt.Errorf("notes: change: %w", err)
		}

		return keep(ctx, tx, id, ch.Tags, nil)
	})
	if err != nil {
		return note{}, err
	}

	n.Changed(ctx, module.Change{Module: name, Kind: "note", ID: id, Area: f.area})

	return n.read(ctx, slugs, c.UserID, id)
}

// history gives the revisions of the caller's note with the id, the newest first.
func (n notes) history(ctx context.Context, id int64) ([]entry, error) {
	c, _ := module.CallerFrom(ctx)

	rows, err := n.Store.QueryContext(ctx, `SELECT `+entryColumns+` FROM `+revisions+` ORDER BY r.revision DESC`, c.UserID, id)
	if err != nil {
		return nil, fmt.Errorf("notes: history: %w", err)
	}
	defer rows.Close()

	out := []entry{}

	for rows.Next() {
		e, err := scanEntry(rows)
		if err != nil {
			return nil, fmt.Errorf("notes: history: %w", err)
		}

		out = append(out, e)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("notes: history: %w", err)
	}

	// Every note has its first revision, so a history with none is of no note of the caller's.
	if len(out) == 0 {
		return nil, fmt.Errorf("note %d: %w", id, store.ErrNotFound)
	}

	return out, nil
}

// revision gives the revision rev of the caller's note with the id, in full.
func (n notes) revision(ctx context.Context, id, rev int64) (version, error) {
	c, _ := module.CallerFrom(ctx)

	slugs, _, err := n.places(ctx, c.UserID)
	if err != nil {
		return version{}, err
	}

	var (
		v    version
		area int64
		tags string
	)

	v.entry, err = scanEntry(n.Store.QueryRowContext(ctx, `SELECT `+entryColumns+`, r.title, r.body, r.kind, r.area_id, r.inbox, r.tags
		FROM `+revisions+` AND r.revision = ?`, c.UserID, id, rev), &v.Title, &v.Body, &v.Kind, &area, &v.Inbox, &tags)
	if errors.Is(err, sql.ErrNoRows) {
		return version{}, fmt.Errorf("revision %d of note %d: %w", rev, id, store.ErrNotFound)
	}

	if err != nil {
		return version{}, fmt.Errorf("notes: revision: %w", err)
	}

	slug, ok := slugs[area]
	if !ok {
		return version{}, fmt.Errorf("notes: revision %d of note %d is in no area of the caller", rev, id)
	}

	v.Area = slug

	if err := json.Unmarshal([]byte(tags), &v.Tags); err != nil {
		return version{}, fmt.Errorf("notes: revision: %w", err)
	}

	return v, nil
}

// restoreRevision gives the caller's note with the id the title, the body
// and the tags of its revision rev, one revision on, which names rev. The
// area, the kind and the inbox flag stay as they are. A note in the trash is
// ErrConflict.
func (n notes) restoreRevision(ctx context.Context, id, rev int64) (note, error) {
	c, _ := module.CallerFrom(ctx)

	slugs, _, err := n.places(ctx, c.UserID)
	if err != nil {
		return note{}, err
	}

	var area int64

	err = n.Store.Tx(ctx, func(tx *sql.Tx) error {
		var (
			trashed           bool
			title, body, tags string
		)

		err := tx.QueryRowContext(ctx, `SELECT notes.area_id, notes.trashed_at IS NOT NULL, r.title, r.body, r.tags
			FROM `+revisions+` AND r.revision = ?`, c.UserID, id, rev).Scan(&area, &trashed, &title, &body, &tags)

		switch {
		case errors.Is(err, sql.ErrNoRows):
			return fmt.Errorf("revision %d of note %d: %w", rev, id, store.ErrNotFound)
		case err != nil:
			return fmt.Errorf("notes: restore: %w", err)
		case trashed:
			return errTrashed
		}

		var restored []string
		if err := json.Unmarshal([]byte(tags), &restored); err != nil {
			return fmt.Errorf("notes: restore: %w", err)
		}

		_, err = tx.ExecContext(ctx, `UPDATE notes SET title = ?, body = ?, revision = revision + 1, source = ?, changed_at = ?
			WHERE user_id = ? AND id = ?`, title, body, c.Source, n.Now().UnixMilli(), c.UserID, id)
		if err != nil {
			return fmt.Errorf("notes: restore: %w", err)
		}

		return keep(ctx, tx, id, &restored, &rev)
	})
	if err != nil {
		return note{}, err
	}

	n.Changed(ctx, module.Change{Module: name, Kind: "note", ID: id, Area: area})

	return n.read(ctx, slugs, c.UserID, id)
}

// trash puts the caller's note with the id in the trash, or takes it out
// when trashed is false. Neither writes a revision or moves the note in its
// list, and finding the note where it is asked to be changes nothing.
func (n notes) trash(ctx context.Context, id int64, trashed bool) (note, error) {
	c, _ := module.CallerFrom(ctx)

	slugs, _, err := n.places(ctx, c.UserID)
	if err != nil {
		return note{}, err
	}

	var (
		at   any // null takes the note out
		area int64
	)

	if trashed {
		at = n.Now().UnixMilli()
	}

	err = n.Store.QueryRowContext(ctx, `UPDATE notes SET trashed_at = ?1 WHERE user_id = ?2 AND id = ?3 AND (trashed_at IS NULL) = ?4
		RETURNING area_id`, at, c.UserID, id, trashed).Scan(&area)

	switch {
	case err == nil:
		n.Changed(ctx, module.Change{Module: name, Kind: "note", ID: id, Area: area})
	case !errors.Is(err, sql.ErrNoRows):
		return note{}, fmt.Errorf("notes: trash: %w", err)
	}

	return n.read(ctx, slugs, c.UserID, id)
}

// remove deletes the caller's note with the id for good, with its tags, its
// revisions and its text in the search index. A note outside the trash is
// ErrConflict.
func (n notes) remove(ctx context.Context, id int64) error {
	c, _ := module.CallerFrom(ctx)

	var area int64

	err := n.Store.Tx(ctx, func(tx *sql.Tx) error {
		var trashed bool

		err := tx.QueryRowContext(ctx, `SELECT area_id, trashed_at IS NOT NULL FROM notes WHERE user_id = ? AND id = ?`,
			c.UserID, id).Scan(&area, &trashed)

		switch {
		case errors.Is(err, sql.ErrNoRows):
			return fmt.Errorf("note %d: %w", id, store.ErrNotFound)
		case err != nil:
			return fmt.Errorf("notes: delete: %w", err)
		case !trashed:
			return fmt.Errorf("note %d is not in the trash, and only a note in the trash is deleted: %w", id, store.ErrConflict)
		}

		if _, err := tx.ExecContext(ctx, `DELETE FROM notes WHERE user_id = ? AND id = ?`, c.UserID, id); err != nil {
			return fmt.Errorf("notes: delete: %w", err)
		}

		if _, err := tx.ExecContext(ctx, purge); err != nil {
			return fmt.Errorf("notes: delete: %w", err)
		}

		return nil
	})
	if err != nil {
		return err
	}

	n.Changed(ctx, module.Change{Module: name, Kind: "note", ID: id, Area: area})

	return nil
}

// empty deletes every note in the caller's trash for good, as remove does.
func (n notes) empty(ctx context.Context) error {
	c, _ := module.CallerFrom(ctx)

	err := n.Store.Tx(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `DELETE FROM notes WHERE user_id = ? AND trashed_at IS NOT NULL`, c.UserID); err != nil {
			return fmt.Errorf("notes: empty the trash: %w", err)
		}

		if _, err := tx.ExecContext(ctx, purge); err != nil {
			return fmt.Errorf("notes: empty the trash: %w", err)
		}

		return nil
	})
	if err != nil {
		return err
	}

	// One change for them all, since a stream that falls behind is ended.
	n.Changed(ctx, module.Change{Module: name, Kind: "trash"})

	return nil
}

// apply sets on f what ch gives, with area the id of the area it names and
// personal that of the personal area.
func (f *fields) apply(ch change, area, personal int64) {
	if ch.Title != nil {
		f.title = strings.TrimSpace(*ch.Title)
	}

	if ch.Body != nil {
		f.body = *ch.Body
	}

	if ch.Append != nil {
		if f.body != "" {
			f.body += "\n\n"
		}

		f.body += *ch.Append
	}

	if ch.Kind != nil {
		f.kind = *ch.Kind
	}

	if ch.Area != nil {
		f.area = area
	}

	switch {
	case ch.Inbox != nil:
		f.inbox = *ch.Inbox
	case ch.Area != nil || ch.Kind != nil || ch.Tags != nil:
		f.inbox = false
	}

	if f.inbox {
		f.area = personal
	}
}

// keep replaces the note's tags with tags, unless tags is nil, and writes
// it into the search index and its revision as it now stands, restored from
// the revision from unless it is nil.
func keep(ctx context.Context, tx *sql.Tx, id int64, tags *[]string, from *int64) error {
	if tags != nil {
		if _, err := tx.ExecContext(ctx, `DELETE FROM note_tags WHERE note_id = ?`, id); err != nil {
			return fmt.Errorf("notes: tags: %w", err)
		}

		for _, t := range *tags {
			if _, err := tx.ExecContext(ctx, `INSERT INTO note_tags (note_id, tag) VALUES (?, ?)`, id, t); err != nil {
				return fmt.Errorf("notes: tags: %w", err)
			}
		}
	}

	if _, err := tx.ExecContext(ctx, index, id); err != nil {
		return fmt.Errorf("notes: index: %w", err)
	}

	if _, err := tx.ExecContext(ctx, snapshot, id, from); err != nil {
		return fmt.Errorf("notes: revision: %w", err)
	}

	return nil
}

// cleanTags gives each of tags as cleanTag does, each once and sorted.
func cleanTags(tags []string) ([]string, error) {
	out := make([]string, 0, len(tags))

	for _, t := range tags {
		t, err := cleanTag(t)
		if err != nil {
			return nil, err
		}

		out = append(out, t)
	}

	slices.Sort(out)

	return slices.Compact(out), nil
}

// cleanTag gives t in lower case without its leading #s, and refuses a tag
// that is empty, has a space or is longer than maxTag. A stored tag then
// never starts with #, so a filter by it cleans to itself.
func cleanTag(t string) (string, error) {
	t = strings.ToLower(strings.TrimLeft(t, "#"))
	if t == "" || utf8.RuneCountInString(t) > maxTag || strings.ContainsFunc(t, func(r rune) bool { return unicode.IsSpace(r) || unicode.IsControl(r) }) {
		return "", fmt.Errorf("the tag %q is not 1 to %d characters with no spaces: %w", t, maxTag, store.ErrInvalid)
	}

	return t, nil
}
