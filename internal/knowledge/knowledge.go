// Package knowledge keeps the decisions that shape each user's projects: each
// what was decided, why, its source, the day it was decided and the decision
// it replaces, with every wording of it. A decision stands until another
// replaces it, and is replaced by one at most. A change is one package of
// decisions added and mended in one project, saved all or none, and only a
// person deletes one. A note or a task may link to a decision, which lists
// what links to it and keeps no links of its own. It searches the decisions
// through an index of their line, why and source. It owns the tables
// decisions, decision_revisions and decisions_fts.
package knowledge

import (
	"cmp"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/mhersson/recallsynapsis/internal/module"
	"github.com/mhersson/recallsynapsis/internal/store"
)

// migrations make the module's schema. A decision's id is AUTOINCREMENT, so
// one that is deleted never gives its id to another, and a link to it never
// leads to another decision. replaces is UNIQUE, so a decision is replaced by
// one at most, and it is set to null as the decision it names is deleted. A
// day is text, YYYY-MM-DD, so days compare as text. The index keeps no copy
// of the text, which a hit reads from the decision.
var migrations = []store.Migration{{Version: 1, SQL: `
CREATE TABLE decisions (
	id         INTEGER PRIMARY KEY AUTOINCREMENT,
	user_id    INTEGER NOT NULL REFERENCES users (id) ON DELETE CASCADE,
	area_id    INTEGER NOT NULL REFERENCES areas (id) ON DELETE CASCADE,
	decision   TEXT NOT NULL,
	why        TEXT NOT NULL,
	source     TEXT NOT NULL,
	decided    TEXT NOT NULL,
	replaces   INTEGER UNIQUE REFERENCES decisions (id) ON DELETE SET NULL,
	revision   INTEGER NOT NULL,
	created_at INTEGER NOT NULL,
	changed_at INTEGER NOT NULL,
	changed_by TEXT NOT NULL
);

CREATE INDEX decisions_area ON decisions (user_id, area_id);

CREATE TABLE decision_revisions (
	decision_id INTEGER NOT NULL REFERENCES decisions (id) ON DELETE CASCADE,
	revision    INTEGER NOT NULL,
	changed_at  INTEGER NOT NULL,
	changed_by  TEXT NOT NULL,
	said        TEXT NOT NULL,
	decision    TEXT NOT NULL,
	why         TEXT NOT NULL,
	source      TEXT NOT NULL,
	decided     TEXT NOT NULL,
	replaces    INTEGER,
	PRIMARY KEY (decision_id, revision)
);
`}, {Version: 2, SQL: `
CREATE VIRTUAL TABLE decisions_fts USING fts5 (decision, why, source, tokenize = 'trigram', content = '', contentless_delete = 1);

INSERT INTO decisions_fts (rowid, decision, why, source) SELECT id, decision, why, source FROM decisions;

CREATE TRIGGER decisions_fts_delete AFTER DELETE ON decisions BEGIN
	DELETE FROM decisions_fts WHERE rowid = old.id;
END;
`}}

const (
	name = "knowledge"

	maxDecision = 200
	maxWhy      = 2000
	maxSource   = 500
	maxPackage  = 20
)

type knowledge struct {
	module.Deps
}

// New builds the knowledge module.
func New(d module.Deps) module.Module {
	k := knowledge{d}

	return module.Module{
		Name:       name,
		Migrations: migrations,
		Resolve:    k.resolve,
		Search:     k.search,
		Routes: []module.Route{
			{Pattern: "GET /api/knowledge", Access: module.Read, Serve: k.serveList},
			{Pattern: "POST /api/knowledge", Access: module.Owner, Serve: k.serveChange},
			{Pattern: "GET /api/knowledge/{id}", Access: module.Read, Serve: k.serveGet},
			{Pattern: "DELETE /api/knowledge/{id}", Access: module.Owner, Serve: k.serveDelete},
			{Pattern: "GET /api/knowledge/{id}/revisions", Access: module.Read, Serve: k.serveHistory},
			{Pattern: "GET /api/knowledge/{id}/revisions/{revision}", Access: module.Read, Serve: k.serveRevision},
		},
	}
}

// row is a decision as a list gives it.
type row struct {
	ID         int64      `json:"id"`
	Ref        module.Ref `json:"ref"`
	Area       string     `json:"area"` // its slug
	Decision   string     `json:"decision"`
	Why        string     `json:"why"`
	Source     string     `json:"source"`
	Decided    string     `json:"decided"` // YYYY-MM-DD
	State      string     `json:"state"`   // "replaced" while another decision names it in replaces, else "stands"
	Replaces   *line      `json:"replaces"`
	ReplacedBy *line      `json:"replaced_by"`
	Revision   int64      `json:"revision"`
	CreatedAt  time.Time  `json:"created_at"`
	ChangedAt  time.Time  `json:"changed_at"`
	ChangedBy  string     `json:"changed_by"` // "web" or a token's name
}

// line is a decision as another names it.
type line struct {
	Ref      module.Ref `json:"ref"`
	Decision string     `json:"decision"`
	Decided  string     `json:"decided"`
}

// decision is a decision in full, with the items that link to it.
type decision struct {
	row
	LinkedFrom []module.Item `json:"linked_from"`
}

// list is what a project's list gives: every decision of it, and which day it is.
type list struct {
	Decisions []row  `json:"decisions"`
	Today     string `json:"today"`
}

// entry is a revision as the history lists it.
type entry struct {
	Revision int64     `json:"revision"`
	At       time.Time `json:"at"`
	By       string    `json:"by"`
	Said     string    `json:"said"`
}

// version is a revision in full: the decision as that change left it.
type version struct {
	entry
	Decision string     `json:"decision"`
	Why      string     `json:"why"`
	Source   string     `json:"source"`
	Decided  string     `json:"decided"`
	Replaces module.Ref `json:"replaces"` // what it replaced then, or ""
}

// change is the package of a change: decisions added to the project Area
// names, as a slug or a name, and decisions of it mended, saved all or none.
// Said comes through a tool alone, so REST refuses it.
type change struct {
	Area string  `json:"area"`
	Add  []draft `json:"add,omitempty"`
	Mend []mend  `json:"mend,omitempty"`
	Said string  `json:"-"` // the owner's own words, kept on each revision the package writes
}

// draft is a decision to add. Left out, decided is today and replaces is none.
type draft struct {
	Decision string     `json:"decision"`
	Why      string     `json:"why"`
	Source   string     `json:"source,omitempty"`
	Decided  string     `json:"decided,omitempty"`
	Replaces module.Ref `json:"replaces,omitempty"`
}

// mend changes the decision Ref names from its revision Revision; a nil field
// stays as it is, and a Replaces of "" makes it replace nothing.
type mend struct {
	Ref      module.Ref  `json:"ref"`
	Revision int64       `json:"revision"`
	Decision *string     `json:"decision,omitempty"`
	Why      *string     `json:"why,omitempty"`
	Source   *string     `json:"source,omitempty"`
	Decided  *string     `json:"decided,omitempty"`
	Replaces *module.Ref `json:"replaces,omitempty"`
}

// fields are what a person or a client sets of a decision; replaces is 0 for none.
type fields struct {
	decision, why, source, decided string
	replaces                       int64
}

// check refuses fields no decision may hold.
func (f fields) check() error {
	switch {
	case f.decision == "" || utf8.RuneCountInString(f.decision) > maxDecision || strings.ContainsFunc(f.decision, unicode.IsControl):
		return fmt.Errorf("a decision is one line of 1 to %d characters: %w", maxDecision, store.ErrInvalid)
	case f.why == "" || utf8.RuneCountInString(f.why) > maxWhy || strings.ContainsFunc(f.why, func(r rune) bool {
		return unicode.IsControl(r) && r != '\n' && r != '\t'
	}):
		return fmt.Errorf("a why is 1 to %d characters, with no control characters but newlines and tabs: %w", maxWhy, store.ErrInvalid)
	case utf8.RuneCountInString(f.source) > maxSource || strings.ContainsFunc(f.source, unicode.IsControl):
		return fmt.Errorf("a source is one line of at most %d characters: %w", maxSource, store.ErrInvalid)
	}

	if d, err := time.Parse(time.DateOnly, f.decided); err != nil || d.Format(time.DateOnly) != f.decided {
		return fmt.Errorf("decided %q is not a date, YYYY-MM-DD: %w", f.decided, store.ErrInvalid)
	}

	return nil
}

// state is how a row or a link tells a decision.
func state(replaced bool) string {
	if replaced {
		return "replaced"
	}

	return "stands"
}

// today is the date in the configured zone.
func (k knowledge) today() string {
	return k.Now().In(k.Zone).Format(time.DateOnly)
}

// columns are a row's, of the decision d, the one o it replaces and the one n
// that replaced it, in the order scanRow reads them, from joined.
const (
	columns = `d.id, d.area_id, d.decision, d.why, d.source, d.decided, d.revision, d.created_at, d.changed_at, d.changed_by,
		o.id, o.decision, o.decided, n.id, n.decision, n.decided`
	joined = `decisions d LEFT JOIN decisions o ON o.id = d.replaces LEFT JOIN decisions n ON n.replaces = d.id`
)

const (
	// snapshot keeps the revision of the decision ?1 as it now stands, with ?2 as its said.
	snapshot = `INSERT INTO decision_revisions (decision_id, revision, changed_at, changed_by, said, decision, why, source, decided, replaces)
		SELECT id, revision, changed_at, changed_by, ?2, decision, why, source, decided, replaces FROM decisions WHERE id = ?1`
	// index writes the decision ?1 into the search index as it now stands.
	index = `REPLACE INTO decisions_fts (rowid, decision, why, source) SELECT id, decision, why, source FROM decisions WHERE id = ?1`
	// purge rewrites the search index whole. A delete only marks a decision's
	// rows in it, whose text a backup would copy until a merge drops them.
	purge = `INSERT INTO decisions_fts (decisions_fts) VALUES ('optimize')`
)

const (
	// entryColumns are an entry's, in the order scanEntry reads them.
	entryColumns = `r.revision, r.changed_at, r.changed_by, r.said`
	// revisions is the FROM and WHERE of one decision's revisions, binding the
	// user's id and then the decision's; a query may add to the WHERE.
	revisions = `decision_revisions r JOIN decisions d ON d.id = r.decision_id WHERE d.user_id = ? AND r.decision_id = ?`
)

// scanRow reads a row of columns, naming its area by its slug in slugs.
func scanRow(sc interface{ Scan(dest ...any) error }, slugs map[int64]string) (row, error) {
	var (
		r                      row
		area, created, changed int64
		old, by                sql.NullInt64
		oldText, oldDay        sql.NullString
		byText, byDay          sql.NullString
	)

	err := sc.Scan(&r.ID, &area, &r.Decision, &r.Why, &r.Source, &r.Decided, &r.Revision, &created, &changed, &r.ChangedBy,
		&old, &oldText, &oldDay, &by, &byText, &byDay)
	if err != nil {
		return row{}, err
	}

	// Each decision is in an area of its user, so one outside them is another user's.
	slug, ok := slugs[area]
	if !ok {
		return row{}, fmt.Errorf("decision %d is in no area of the caller", r.ID)
	}

	r.Ref, r.Area, r.State = module.NewRef(name, r.ID), slug, state(by.Valid)
	r.CreatedAt, r.ChangedAt = time.UnixMilli(created).UTC(), time.UnixMilli(changed).UTC()

	if old.Valid {
		r.Replaces = &line{module.NewRef(name, old.Int64), oldText.String, oldDay.String}
	}

	if by.Valid {
		r.ReplacedBy = &line{module.NewRef(name, by.Int64), byText.String, byDay.String}
	}

	return r, nil
}

// scanEntry reads an entry of entryColumns, and extra after it.
func scanEntry(sc interface{ Scan(dest ...any) error }, extra ...any) (entry, error) {
	var (
		e  entry
		at int64
	)

	if err := sc.Scan(append([]any{&e.Revision, &at, &e.By, &e.Said}, extra...)...); err != nil {
		return entry{}, err
	}

	e.At = time.UnixMilli(at).UTC()

	return e, nil
}

// slugs gives the slug of each area of the user by its id.
func (k knowledge) slugs(ctx context.Context, userID int64) (map[int64]string, error) {
	areas, err := k.Areas(ctx, userID)
	if err != nil {
		return nil, err
	}

	out := make(map[int64]string, len(areas))
	for _, a := range areas {
		out[a.ID] = a.Slug
	}

	return out, nil
}

// list gives every decision of the caller's project area, a slug or a name,
// the newest day first and among those of a day the newest made first.
func (k knowledge) list(ctx context.Context, area string) (list, error) {
	c, _ := module.CallerFrom(ctx)

	if area == "" {
		return list{}, fmt.Errorf("area names the project to list, by its slug or its name: %w", store.ErrInvalid)
	}

	a, err := k.FindArea(ctx, c.UserID, area)
	if err != nil {
		return list{}, err
	}

	slugs := map[int64]string{a.ID: a.Slug}

	rs, err := k.Store.QueryContext(ctx, `SELECT `+columns+` FROM `+joined+` WHERE d.user_id = ? AND d.area_id = ?
		ORDER BY d.decided DESC, d.created_at DESC, d.id DESC`, c.UserID, a.ID)
	if err != nil {
		return list{}, fmt.Errorf("knowledge: list: %w", err)
	}
	defer rs.Close()

	out := list{Decisions: []row{}, Today: k.today()}

	for rs.Next() {
		r, err := scanRow(rs, slugs)
		if err != nil {
			return list{}, fmt.Errorf("knowledge: list: %w", err)
		}

		out.Decisions = append(out.Decisions, r)
	}

	if err := rs.Err(); err != nil {
		return list{}, fmt.Errorf("knowledge: list: %w", err)
	}

	return out, nil
}

// get gives the caller's decision with the id in full, the items that link
// to it each as the core resolves it for the caller.
func (k knowledge) get(ctx context.Context, id int64) (decision, error) {
	c, _ := module.CallerFrom(ctx)

	slugs, err := k.slugs(ctx, c.UserID)
	if err != nil {
		return decision{}, err
	}

	r, err := k.read(ctx, slugs, c.UserID, id)
	if err != nil {
		return decision{}, err
	}

	from, err := k.LinksTo(ctx, r.Ref)
	if err != nil {
		return decision{}, err
	}

	out := decision{row: r}
	if out.LinkedFrom, err = k.Items(ctx, from); err != nil {
		return decision{}, err
	}

	return out, nil
}

// resolve gives the caller's decision with the id as a link shows it: its
// line, the kind decision and its state.
func (k knowledge) resolve(ctx context.Context, id int64) (module.Item, error) {
	c, _ := module.CallerFrom(ctx)

	var (
		it       = module.Item{Kind: "decision"}
		replaced bool
	)

	err := k.Store.QueryRowContext(ctx, `SELECT decision, EXISTS (SELECT 1 FROM decisions WHERE replaces = d.id) FROM decisions d
		WHERE user_id = ? AND id = ?`, c.UserID, id).Scan(&it.Title, &replaced)
	if errors.Is(err, sql.ErrNoRows) {
		return module.Item{}, fmt.Errorf("decision %s: %w", module.NewRef(name, id), store.ErrNotFound)
	}

	if err != nil {
		return module.Item{}, fmt.Errorf("knowledge: resolve: %w", err)
	}

	it.State = state(replaced)

	return it, nil
}

// read gives the user's decision with the id.
func (k knowledge) read(ctx context.Context, slugs map[int64]string, userID, id int64) (row, error) {
	r, err := scanRow(k.Store.QueryRowContext(ctx, `SELECT `+columns+` FROM `+joined+` WHERE d.user_id = ? AND d.id = ?`, userID, id), slugs)
	if errors.Is(err, sql.ErrNoRows) {
		return row{}, fmt.Errorf("decision %s: %w", module.NewRef(name, id), store.ErrNotFound)
	}

	if err != nil {
		return row{}, fmt.Errorf("knowledge: read: %w", err)
	}

	return r, nil
}

// history gives the revisions of the caller's decision with the id, the newest first.
func (k knowledge) history(ctx context.Context, id int64) ([]entry, error) {
	c, _ := module.CallerFrom(ctx)

	rs, err := k.Store.QueryContext(ctx, `SELECT `+entryColumns+` FROM `+revisions+` ORDER BY r.revision DESC`, c.UserID, id)
	if err != nil {
		return nil, fmt.Errorf("knowledge: history: %w", err)
	}
	defer rs.Close()

	out := []entry{}

	for rs.Next() {
		e, err := scanEntry(rs)
		if err != nil {
			return nil, fmt.Errorf("knowledge: history: %w", err)
		}

		out = append(out, e)
	}

	if err := rs.Err(); err != nil {
		return nil, fmt.Errorf("knowledge: history: %w", err)
	}

	// Every decision has its first revision, so a history with none is of no decision of the caller's.
	if len(out) == 0 {
		return nil, fmt.Errorf("decision %s: %w", module.NewRef(name, id), store.ErrNotFound)
	}

	return out, nil
}

// revision gives the revision rev of the caller's decision with the id, in full.
func (k knowledge) revision(ctx context.Context, id, rev int64) (version, error) {
	c, _ := module.CallerFrom(ctx)

	var (
		v        version
		replaces sql.NullInt64
		err      error
	)

	v.entry, err = scanEntry(k.Store.QueryRowContext(ctx, `SELECT `+entryColumns+`, r.decision, r.why, r.source, r.decided, r.replaces
		FROM `+revisions+` AND r.revision = ?`, c.UserID, id, rev), &v.Decision, &v.Why, &v.Source, &v.Decided, &replaces)
	if errors.Is(err, sql.ErrNoRows) {
		return version{}, fmt.Errorf("revision %d of decision %s: %w", rev, module.NewRef(name, id), store.ErrNotFound)
	}

	if err != nil {
		return version{}, fmt.Errorf("knowledge: revision: %w", err)
	}

	if replaces.Valid {
		v.Replaces = module.NewRef(name, replaces.Int64)
	}

	return v, nil
}

// apply saves the package ch for the caller: its mends first, then what it
// adds, all of it or, when one entry is refused, none, with a line that names
// the entry. It gives what it mended, then what it added. A mend that leaves
// a decision as it is writes no revision and tells no page.
func (k knowledge) apply(ctx context.Context, ch change) ([]row, error) {
	c, _ := module.CallerFrom(ctx)

	switch n := len(ch.Add) + len(ch.Mend); {
	case n < 1 || n > maxPackage:
		return nil, fmt.Errorf("a package adds and mends 1 to %d decisions in all: %w", maxPackage, store.ErrInvalid)
	case ch.Area == "":
		return nil, fmt.Errorf("area names the project of the package, by its slug or its name: %w", store.ErrInvalid)
	}

	a, err := k.FindArea(ctx, c.UserID, ch.Area)
	if err != nil {
		return nil, err
	}

	if a.Kind == "personal" {
		return nil, fmt.Errorf("the personal area has no decisions, and a package is of a project: %w", store.ErrInvalid)
	}

	slugs, err := k.slugs(ctx, c.UserID)
	if err != nil {
		return nil, err
	}

	var (
		ids     []int64 // what the package mended, then what it added
		changed []int64 // what it wrote
	)

	err = k.Store.Tx(ctx, func(tx *sql.Tx) error {
		w := writer{
			knowledge: k, tx: tx, caller: c, area: a, slugs: slugs, said: ch.Said, now: k.Now().UnixMilli(),
			mended: map[int64]string{}, claimed: map[int64]string{},
		}

		for i, m := range ch.Mend {
			id, wrote, err := w.mend(ctx, fmt.Sprintf("entry %d of mend", i+1), m)
			if err != nil {
				return err
			}

			ids = append(ids, id)
			if wrote {
				changed = append(changed, id)
			}
		}

		for i, d := range ch.Add {
			id, err := w.add(ctx, fmt.Sprintf("entry %d of add", i+1), d)
			if err != nil {
				return err
			}

			ids, changed = append(ids, id), append(changed, id)
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	for _, id := range changed {
		k.Changed(ctx, module.Change{Module: name, Kind: "decision", ID: id, Area: a.ID})
	}

	out := make([]row, len(ids))
	for i, id := range ids {
		if out[i], err = k.read(ctx, slugs, c.UserID, id); err != nil {
			return nil, err
		}
	}

	return out, nil
}

// writer writes one package in its transaction, for the caller, to the area.
type writer struct {
	knowledge
	tx      *sql.Tx
	caller  module.Caller
	area    module.Area
	slugs   map[int64]string
	said    string
	now     int64
	mended  map[int64]string // what an entry has mended, by the entry
	claimed map[int64]string // what an entry has named in replaces, by the entry
}

// mend applies m, the entry called label, and gives the id of its decision
// and whether it wrote a revision.
func (w writer) mend(ctx context.Context, label string, m mend) (int64, bool, error) {
	id, err := open(m.Ref)
	ref := module.NewRef(name, id)

	switch {
	case err != nil:
		return 0, false, fmt.Errorf("%s: %w", label, err)
	case m.Revision == 0:
		return 0, false, fmt.Errorf("%s: a mend names the revision it was made from: %w", label, store.ErrInvalid)
	case m == mend{Ref: m.Ref, Revision: m.Revision}:
		return 0, false, fmt.Errorf("%s: a mend gives at least one field to change: %w", label, store.ErrInvalid)
	case w.mended[id] != "":
		return 0, false, fmt.Errorf("%s and %s both mend %s, and one mend gives every field it changes: %w", w.mended[id], label, ref,
			store.ErrInvalid)
	}

	w.mended[id] = label

	var (
		f              fields
		area, revision int64
	)

	err = w.tx.QueryRowContext(ctx, `SELECT area_id, decision, why, source, decided, coalesce(replaces, 0), revision FROM decisions
		WHERE user_id = ? AND id = ?`, w.caller.UserID, id).Scan(&area, &f.decision, &f.why, &f.source, &f.decided, &f.replaces, &revision)

	switch {
	case errors.Is(err, sql.ErrNoRows):
		return 0, false, fmt.Errorf("%s: decision %s: %w", label, ref, store.ErrNotFound)
	case err != nil:
		return 0, false, fmt.Errorf("knowledge: mend: %w", err)
	case area != w.area.ID:
		return 0, false, fmt.Errorf("%s: %s is a decision of %s, and the package is of %s: %w", label, ref, w.slugs[area], w.area.Slug,
			store.ErrInvalid)
	case m.Revision != revision:
		return 0, false, fmt.Errorf("%s: %s is at revision %d, and the mend was made from %d: %w", label, ref, revision, m.Revision,
			store.ErrConflict)
	}

	was := f

	set := func(field *string, to *string) {
		if to != nil {
			*field = strings.TrimSpace(*to)
		}
	}

	set(&f.decision, m.Decision)
	set(&f.why, m.Why)
	set(&f.source, m.Source)

	if m.Decided != nil {
		f.decided = *m.Decided
	}

	if m.Replaces != nil {
		f.replaces = 0

		if *m.Replaces != "" {
			if f.replaces, err = open(*m.Replaces); err != nil {
				return 0, false, fmt.Errorf("%s: %w", label, err)
			}
		}
	}

	if err := f.check(); err != nil {
		return 0, false, fmt.Errorf("%s: %w", label, err)
	}

	if err := w.claim(ctx, label, id, f.replaces, was.replaces); err != nil {
		return 0, false, err
	}

	if f == was {
		return id, false, nil
	}

	_, err = w.tx.ExecContext(ctx, `UPDATE decisions SET decision = ?, why = ?, source = ?, decided = ?, replaces = nullif(?, 0),
		revision = revision + 1, changed_at = ?, changed_by = ? WHERE user_id = ? AND id = ?`,
		f.decision, f.why, f.source, f.decided, f.replaces, w.now, w.caller.Source, w.caller.UserID, id)
	if err != nil {
		return 0, false, fmt.Errorf("knowledge: mend: %w", err)
	}

	if err := w.keep(ctx, id); err != nil {
		return 0, false, err
	}

	return id, true, nil
}

// add makes d, the entry called label, a decision of the area at revision 1,
// and gives its id.
func (w writer) add(ctx context.Context, label string, d draft) (int64, error) {
	f := fields{
		decision: strings.TrimSpace(d.Decision), why: strings.TrimSpace(d.Why), source: strings.TrimSpace(d.Source),
		decided: cmp.Or(d.Decided, w.today()),
	}

	if d.Replaces != "" {
		var err error
		if f.replaces, err = open(d.Replaces); err != nil {
			return 0, fmt.Errorf("%s: %w", label, err)
		}
	}

	if err := f.check(); err != nil {
		return 0, fmt.Errorf("%s: %w", label, err)
	}

	if err := w.claim(ctx, label, 0, f.replaces, 0); err != nil {
		return 0, err
	}

	var id int64

	err := w.tx.QueryRowContext(ctx, `INSERT INTO decisions (user_id, area_id, decision, why, source, decided, replaces, revision,
		created_at, changed_at, changed_by) VALUES (?, ?, ?, ?, ?, ?, nullif(?, 0), 1, ?, ?, ?) RETURNING id`,
		w.caller.UserID, w.area.ID, f.decision, f.why, f.source, f.decided, f.replaces, w.now, w.now, w.caller.Source).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("knowledge: add: %w", err)
	}

	if err := w.keep(ctx, id); err != nil {
		return 0, err
	}

	return id, nil
}

// keep writes the revision of the decision with the id as it now stands, and
// its text into the search index.
func (w writer) keep(ctx context.Context, id int64) error {
	if _, err := w.tx.ExecContext(ctx, snapshot, id, w.said); err != nil {
		return fmt.Errorf("knowledge: revision: %w", err)
	}

	if _, err := w.tx.ExecContext(ctx, index, id); err != nil {
		return fmt.Errorf("knowledge: index: %w", err)
	}

	return nil
}

// claim refuses the entry label making self, 0 when new, replace target unless
// target is 0 or was: target is of the package's project, stands, is named by
// no other entry, and leads not back to self.
func (w writer) claim(ctx context.Context, label string, self, target, was int64) error {
	if target == 0 || target == was {
		return nil
	}

	ref := module.NewRef(name, target)

	if other, ok := w.claimed[target]; ok {
		return fmt.Errorf("%s and %s both replace %s, which one decision replaces at most: %w", other, label, ref, store.ErrInvalid)
	}

	w.claimed[target] = label

	if target == self {
		return fmt.Errorf("%s: a decision does not replace itself: %w", label, store.ErrInvalid)
	}

	var (
		area     int64
		by       sql.NullInt64
		circular bool
	)

	// The chain from target follows what each decision replaces, and reaches self when self replacing target closes a circle.
	err := w.tx.QueryRowContext(ctx, `SELECT area_id, (SELECT id FROM decisions WHERE replaces = ?2),
		EXISTS (WITH RECURSIVE chain (id) AS (SELECT ?2 UNION SELECT d.replaces FROM decisions d JOIN chain ON d.id = chain.id)
			SELECT 1 FROM chain WHERE id = ?3)
		FROM decisions WHERE user_id = ?1 AND id = ?2`, w.caller.UserID, target, self).Scan(&area, &by, &circular)

	switch {
	case errors.Is(err, sql.ErrNoRows):
		return fmt.Errorf("%s: decision %s: %w", label, ref, store.ErrNotFound)
	case err != nil:
		return fmt.Errorf("knowledge: replaces: %w", err)
	case area != w.area.ID:
		return fmt.Errorf("%s: %s is a decision of %s, and a decision replaces one of its own project, %s: %w", label, ref, w.slugs[area],
			w.area.Slug, store.ErrInvalid)
	case by.Valid:
		return fmt.Errorf("%s: %s is replaced already, by %s: %w", label, ref, module.NewRef(name, by.Int64), store.ErrInvalid)
	case circular:
		return fmt.Errorf("%s: %s replaces %s, directly or through what it replaces, so %[3]s cannot replace it: %w", label, ref,
			module.NewRef(name, self), store.ErrInvalid)
	}

	return nil
}

// remove deletes the caller's decision with the id, with its revisions and
// its text in the search index. The one it replaced then stands, and the one
// that replaced it replaces nothing, with no revision written for that.
func (k knowledge) remove(ctx context.Context, id int64) error {
	c, _ := module.CallerFrom(ctx)

	var area int64

	err := k.Store.Tx(ctx, func(tx *sql.Tx) error {
		err := tx.QueryRowContext(ctx, `DELETE FROM decisions WHERE user_id = ? AND id = ? RETURNING area_id`, c.UserID, id).Scan(&area)
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("decision %s: %w", module.NewRef(name, id), store.ErrNotFound)
		}

		if err != nil {
			return fmt.Errorf("knowledge: delete: %w", err)
		}

		if _, err := tx.ExecContext(ctx, purge); err != nil {
			return fmt.Errorf("knowledge: delete: %w", err)
		}

		return nil
	})
	if err != nil {
		return err
	}

	k.Changed(ctx, module.Change{Module: name, Kind: "decision", ID: id, Area: area})

	return nil
}

// open gives the id of the decision ref names.
func open(ref module.Ref) (int64, error) {
	mod, id, ok := ref.Split()
	if !ok || mod != name || id < 1 {
		return 0, fmt.Errorf("the ref %q is not a decision's, which is knowledge:ID: %w", ref, store.ErrInvalid)
	}

	return id, nil
}
