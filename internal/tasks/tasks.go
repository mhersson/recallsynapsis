// Package tasks keeps each user's tasks: each in one of the user's areas,
// with its notes, a planned period of days with an optional time on its
// first, a deadline, a list, soon or someday, when it was done, and its links
// to the items of any module. It lists them by area, list, state and a span
// of dates, tells which day it is in the configured time zone, and keeps a
// log of every change of a task's fields. A task in the trash is in no list
// but the trash's, found by no search, no link and no tool, and kept
// unchanged until it is restored or deleted for good. It searches them
// through an index of their title and notes. It notifies each user, as they
// chose, at a task's start time, in the morning of what the day holds, of the
// tasks due tomorrow and of those planned for today with no start time today.
// It owns the tables tasks, task_changes, task_links, tasks_fts, task_notify
// and task_notified.
package tasks

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/mhersson/recallsynapsis/internal/module"
	"github.com/mhersson/recallsynapsis/internal/store"
)

// migrations make the module's schema. A task's id is AUTOINCREMENT, so a
// task deleted for good never gives its id to another, and a link to it never
// leads to another task. A date is text, YYYY-MM-DD or empty for none, so
// dates compare as text, and a time is HH:MM. The index keeps no copy of the
// text, which a hit reads from the task, and its trigrams let a word find
// part of a compound. A user has a row of task_notify once they change a kind
// of notification, and task_notified is one row: the moment notified up to.
var migrations = []store.Migration{{Version: 1, SQL: `
CREATE TABLE tasks (
	id           INTEGER PRIMARY KEY AUTOINCREMENT,
	user_id      INTEGER NOT NULL REFERENCES users (id) ON DELETE CASCADE,
	area_id      INTEGER NOT NULL REFERENCES areas (id) ON DELETE CASCADE,
	title        TEXT NOT NULL,
	notes        TEXT NOT NULL,
	planned_from TEXT NOT NULL,
	planned_to   TEXT NOT NULL,
	planned_time TEXT NOT NULL,
	deadline     TEXT NOT NULL,
	list         TEXT NOT NULL,
	done_at      INTEGER,
	source       TEXT NOT NULL,
	created_at   INTEGER NOT NULL,
	changed_at   INTEGER NOT NULL,
	trashed_at   INTEGER
);

CREATE INDEX tasks_done ON tasks (user_id, done_at);

CREATE TABLE task_changes (
	id      INTEGER PRIMARY KEY,
	task_id INTEGER NOT NULL REFERENCES tasks (id) ON DELETE CASCADE,
	at      INTEGER NOT NULL,
	source  TEXT NOT NULL,
	said    TEXT NOT NULL,
	changes TEXT NOT NULL
);

CREATE INDEX task_changes_task ON task_changes (task_id);
`}, {Version: 2, SQL: `
CREATE VIRTUAL TABLE tasks_fts USING fts5 (title, notes, tokenize = 'trigram', content = '', contentless_delete = 1);

INSERT INTO tasks_fts (rowid, title, notes) SELECT id, title, notes FROM tasks;

CREATE TRIGGER tasks_fts_delete AFTER DELETE ON tasks BEGIN
	DELETE FROM tasks_fts WHERE rowid = old.id;
END;

CREATE TABLE task_links (
	task_id INTEGER NOT NULL REFERENCES tasks (id) ON DELETE CASCADE,
	ref     TEXT NOT NULL,
	PRIMARY KEY (task_id, ref)
);

CREATE INDEX task_links_ref ON task_links (ref);
`}, {Version: 3, SQL: `
CREATE TABLE task_notify (
	user_id     INTEGER PRIMARY KEY REFERENCES users (id) ON DELETE CASCADE,
	start       INTEGER NOT NULL,
	morning     INTEGER NOT NULL,
	morning_at  TEXT NOT NULL,
	deadline    INTEGER NOT NULL,
	deadline_at TEXT NOT NULL,
	untimed     INTEGER NOT NULL,
	untimed_at  TEXT NOT NULL
);

CREATE TABLE task_notified (
	id INTEGER PRIMARY KEY CHECK (id = 1),
	at INTEGER NOT NULL
);
`}}

const (
	name = "tasks"

	maxTitle     = 200
	maxBatch     = 50
	defaultLimit = 50
	maxLimit     = 500
	clockLayout  = "15:04"
)

var (
	lists    = []string{"", "soon", "someday"}
	weekdays = []string{"monday", "tuesday", "wednesday", "thursday", "friday", "saturday", "sunday"}

	errLimit   = fmt.Errorf("limit is 1 to %d: %w", maxLimit, store.ErrInvalid)
	errTrashed = fmt.Errorf("the task is in the trash, and is restored before it is changed: %w", store.ErrConflict)
)

// settings are the module's keys of [modules.tasks] beside enabled.
type settings struct {
	WeekStart       string `toml:"week_start"`
	CaptureMaxChars int    `toml:"capture_max_chars"`
}

// Check refuses a first day of the week that is no weekday's name in lower
// case, and a cap on a tool's text under 1 character.
func (s *settings) Check() error {
	switch {
	case !slices.Contains(weekdays, s.WeekStart):
		return fmt.Errorf("week_start: %q is not a weekday's name in lower case, such as \"monday\"", s.WeekStart)
	case s.CaptureMaxChars < 1:
		return fmt.Errorf("capture_max_chars: %d is not at least 1", s.CaptureMaxChars)
	}

	return nil
}

type tasks struct {
	module.Deps
	settings *settings
}

// New builds the tasks module.
func New(d module.Deps) module.Module {
	t := tasks{Deps: d, settings: &settings{WeekStart: "monday", CaptureMaxChars: 4000}}

	return module.Module{
		Name:       name,
		Settings:   t.settings,
		Migrations: migrations,
		Routes: []module.Route{
			{Pattern: "GET /api/tasks", Access: module.Read, Serve: t.serveList},
			{Pattern: "POST /api/tasks", Access: module.Write, Serve: t.serveCreate},
			{Pattern: "GET /api/tasks/{id}", Access: module.Read, Serve: t.serveGet},
			{Pattern: "PATCH /api/tasks/{id}", Access: module.Write, Serve: t.serveChange},
			{Pattern: "POST /api/tasks/{id}/trash", Access: module.Write, Serve: t.serveTrash(true)},
			{Pattern: "POST /api/tasks/{id}/restore", Access: module.Write, Serve: t.serveTrash(false)},
			{Pattern: "DELETE /api/tasks/{id}", Access: module.Owner, Serve: t.serveDelete},
			{Pattern: "DELETE /api/tasks/trash", Access: module.Owner, Serve: t.serveEmpty},
			{Pattern: "GET /api/tasks/notify", Access: module.Owner, Serve: t.serveKinds},
			{Pattern: "PATCH /api/tasks/notify", Access: module.Owner, Serve: t.serveChangeKinds},
		},
		Jobs:    []module.Job{{Name: "notify", Every: 30 * time.Second, Run: t.notify}},
		Tools:   t.tools(),
		Search:  t.search,
		Resolve: t.resolve,
		LinksTo: t.linksTo,
	}
}

// clock is which day it is, and the time, in the configured zone.
type clock struct {
	Date      string `json:"date"`
	Weekday   string `json:"weekday"`
	Time      string `json:"time"`
	Timezone  string `json:"timezone"`
	WeekStart string `json:"week_start"` // the first day of the week
}

// row is a task as a list gives it. Each date is YYYY-MM-DD and the time
// HH:MM, or "" for none, and each day is the English name of its date's
// weekday, or "" beside no date.
type row struct {
	ID             int64      `json:"id"`
	Ref            module.Ref `json:"ref"`
	Area           string     `json:"area"` // its slug
	Title          string     `json:"title"`
	PlannedFrom    string     `json:"planned_from"`
	PlannedFromDay string     `json:"planned_from_day"`
	PlannedTo      string     `json:"planned_to"`
	PlannedToDay   string     `json:"planned_to_day"`
	PlannedTime    string     `json:"planned_time"`
	Deadline       string     `json:"deadline"`
	DeadlineDay    string     `json:"deadline_day"`
	List           string     `json:"list"`
	State          string     `json:"state"`   // "done", "unsorted" when open with no list, plan or deadline, else "open"
	DoneAt         *time.Time `json:"done_at"` // null unless the task is done
	HasNotes       bool       `json:"has_notes"`
	Source         string     `json:"source"` // that of the last change
	CreatedAt      time.Time  `json:"created_at"`
	ChangedAt      time.Time  `json:"changed_at"`
	TrashedAt      *time.Time `json:"trashed_at"` // null unless the task is in the trash
}

// task is a task in full, with its links and the items that link to it,
// its log, the newest entry first, and which day it is.
type task struct {
	row
	Notes      string        `json:"notes"`
	Said       string        `json:"said"` // what its making carries
	Links      []module.Item `json:"links"`
	LinkedFrom []module.Item `json:"linked_from"`
	Log        []entry       `json:"log"`
	Now        clock         `json:"now"`
}

// entry is one change of a task as its log keeps it; the first is its making.
type entry struct {
	At      time.Time `json:"at"`
	Source  string    `json:"source"`
	Said    string    `json:"said"`
	Changes []delta   `json:"changes"`
}

// delta is one field a change changed, named as the wire names it, with what
// it was and what it became; neither for the notes and the links.
type delta struct {
	Field string  `json:"field"`
	From  *string `json:"from,omitempty"`
	To    *string `json:"to,omitempty"`
}

// draft is a new task, in the personal area unless it names another, as a
// slug or a name. A field a tool may leave out is omitempty, which its schema
// reads as optional.
type draft struct {
	Title       string       `json:"title"`
	Notes       string       `json:"notes,omitempty"`
	Area        string       `json:"area,omitempty"`
	PlannedFrom string       `json:"planned_from,omitempty"`
	PlannedTo   string       `json:"planned_to,omitempty"`
	PlannedTime string       `json:"planned_time,omitempty"`
	Deadline    string       `json:"deadline,omitempty"`
	List        string       `json:"list,omitempty"`
	Links       []module.Ref `json:"links,omitempty"`
}

// batch is the tasks one call makes, with the owner's own words, kept on the
// making of each.
type batch struct {
	Tasks []draft `json:"tasks"`
	Said  string  `json:"said,omitempty"`
}

// made is what a call that makes tasks gives.
type made struct {
	Tasks []row `json:"tasks"`
	Now   clock `json:"now"`
}

// change is what a change sets; a nil field stays as it is.
type change struct {
	Title       *string       `json:"title,omitempty"`
	Notes       *string       `json:"notes,omitempty"`
	Area        *string       `json:"area,omitempty"`
	PlannedFrom *string       `json:"planned_from,omitempty"`
	PlannedTo   *string       `json:"planned_to,omitempty"`
	PlannedTime *string       `json:"planned_time,omitempty"`
	Deadline    *string       `json:"deadline,omitempty"`
	List        *string       `json:"list,omitempty"`
	Done        *bool         `json:"done,omitempty"`
	Links       *[]module.Ref `json:"links,omitempty"` // replaces them all
}

// filter is what a list asks for; a zero field asks for nothing.
type filter struct {
	area     string // a slug or a name
	list     string // "soon", "someday" or "none"
	from, to string // the span of dates, each side open when ""
	unsorted bool
	done     bool // the done tasks, else the open ones
	trashed  bool // the trash alone, else none of it
	limit    int  // defaultLimit when 0
	offset   int
}

// page is what a list gives: its tasks, how many it was asked for in all,
// and which day it is.
type page struct {
	Tasks []row `json:"tasks"`
	Total int   `json:"total"`
	Now   clock `json:"now"`
}

// fields are what a person or a client sets of a task.
type fields struct {
	title, notes                 string
	area                         int64
	from, to, at, deadline, list string
	done                         bool
	links                        []module.Ref
}

// apply sets on f what ch gives, with area the id of the area it names. A
// from given without a to plans that one day, and a to of "" leaves the from
// alone, one day. A from of "" clears the plan whole.
func (f *fields) apply(ch change, area int64) {
	set := func(field *string, to *string) {
		if to != nil {
			*field = *to
		}
	}

	if ch.Title != nil {
		f.title = strings.TrimSpace(*ch.Title)
	}

	set(&f.notes, ch.Notes)

	if ch.Area != nil {
		f.area = area
	}

	if ch.PlannedFrom != nil {
		f.from, f.to = *ch.PlannedFrom, *ch.PlannedFrom
		if f.from == "" {
			f.at = ""
		}
	}

	set(&f.to, ch.PlannedTo)

	if f.to == "" {
		f.to = f.from
	}

	set(&f.at, ch.PlannedTime)
	set(&f.deadline, ch.Deadline)
	set(&f.list, ch.List)

	if ch.Done != nil {
		f.done = *ch.Done
	}

	if ch.Links != nil {
		f.links = *ch.Links
	}
}

// check refuses fields no task may hold.
func (f fields) check() error {
	switch {
	case f.title == "" || utf8.RuneCountInString(f.title) > maxTitle || strings.ContainsFunc(f.title, unicode.IsControl):
		return fmt.Errorf("a title is 1 to %d characters, with no control characters: %w", maxTitle, store.ErrInvalid)
	case !slices.Contains(lists, f.list):
		return fmt.Errorf(`a list is "soon", "someday" or "": %w`, store.ErrInvalid)
	}

	for _, v := range []struct{ key, value string }{{"planned_from", f.from}, {"planned_to", f.to}, {"deadline", f.deadline}} {
		if err := checkDate(v.key, v.value); err != nil {
			return err
		}
	}

	switch {
	case f.at != "" && !written(clockLayout, f.at):
		return fmt.Errorf("planned_time %q is not a time of day, HH:MM: %w", f.at, store.ErrInvalid)
	case f.from == "" && f.to != "":
		return fmt.Errorf("planned_to is set with planned_from, the plan's first day: %w", store.ErrInvalid)
	case f.from == "" && f.at != "":
		return fmt.Errorf("planned_time is set with planned_from, the day it is on: %w", store.ErrInvalid)
	case f.to < f.from:
		return fmt.Errorf("planned_to %s is before planned_from %s: %w", f.to, f.from, store.ErrInvalid)
	}

	return nil
}

// checkDate refuses a value of key that is neither "" nor a date.
func checkDate(key, value string) error {
	if value != "" && !written(time.DateOnly, value) {
		return fmt.Errorf("%s %q is not a date, YYYY-MM-DD: %w", key, value, store.ErrInvalid)
	}

	return nil
}

// written reports whether s is a value of layout, written as layout writes
// it, so 30 February and a time of 9:30 are refused.
func written(layout, s string) bool {
	v, err := time.Parse(layout, s)

	return err == nil && v.Format(layout) == s
}

// weekday is the English name of date's weekday, or "" when it is no date.
func weekday(date string) string {
	d, err := time.Parse(time.DateOnly, date)
	if err != nil {
		return ""
	}

	return d.Weekday().String()
}

// diff gives each field from and to differ in, in the order the wire lists
// them, as the log keeps it. A making is a diff from no fields.
func diff(from, to fields, slugs map[int64]string) []delta {
	var out []delta

	add := func(field, was, is string) {
		if was != is {
			out = append(out, delta{Field: field, From: &was, To: &is})
		}
	}

	add("title", from.title, to.title)
	add("area", slugs[from.area], slugs[to.area])
	add("planned_from", from.from, to.from)
	add("planned_to", from.to, to.to)
	add("planned_time", from.at, to.at)
	add("deadline", from.deadline, to.deadline)
	add("list", from.list, to.list)
	add("done", strconv.FormatBool(from.done), strconv.FormatBool(to.done))

	if from.notes != to.notes {
		out = append(out, delta{Field: "notes"})
	}

	if !slices.Equal(from.links, to.links) {
		out = append(out, delta{Field: "links"})
	}

	return out
}

// columns are a row's, in the order scanRow reads them.
const columns = `id, area_id, title, planned_from, planned_to, planned_time, deadline, list, done_at, notes <> '', source, created_at, changed_at,
	trashed_at`

const (
	// index writes the task ?1 into the search index as it now stands.
	index = `REPLACE INTO tasks_fts (rowid, title, notes) SELECT id, title, notes FROM tasks WHERE id = ?1`
	// purge rewrites the search index whole. A delete only marks a task's
	// rows in it, whose text a backup would copy until a merge drops them.
	purge = `INSERT INTO tasks_fts (tasks_fts) VALUES ('optimize')`
)

// scanRow reads a row of columns, and extra after it, naming its area by
// its slug in slugs.
func scanRow(sc interface{ Scan(dest ...any) error }, slugs map[int64]string, extra ...any) (row, error) {
	var (
		r                      row
		area, created, changed int64
		done, trashed          sql.NullInt64
	)

	err := sc.Scan(append([]any{
		&r.ID, &area, &r.Title, &r.PlannedFrom, &r.PlannedTo, &r.PlannedTime, &r.Deadline, &r.List,
		&done, &r.HasNotes, &r.Source, &created, &changed, &trashed,
	}, extra...)...)
	if err != nil {
		return row{}, err
	}

	// Each task is in an area of its user, so a task outside them is another user's.
	slug, ok := slugs[area]
	if !ok {
		return row{}, fmt.Errorf("task %d is in no area of the caller", r.ID)
	}

	r.Ref, r.Area = module.NewRef(name, r.ID), slug
	r.PlannedFromDay, r.PlannedToDay, r.DeadlineDay = weekday(r.PlannedFrom), weekday(r.PlannedTo), weekday(r.Deadline)
	r.CreatedAt, r.ChangedAt = time.UnixMilli(created).UTC(), time.UnixMilli(changed).UTC()

	if trashed.Valid {
		r.TrashedAt = new(time.UnixMilli(trashed.Int64).UTC())
	}

	if done.Valid {
		r.DoneAt = new(time.UnixMilli(done.Int64).UTC())
	}

	r.State = state(done.Valid, r.List, r.PlannedFrom, r.Deadline)

	return r, nil
}

// state is how a row, a hit or a link tells a task: "done", "unsorted" when
// it is open with no list, no plan and no deadline, else "open".
func state(done bool, list, from, deadline string) string {
	switch {
	case done:
		return "done"
	case list == "" && from == "" && deadline == "":
		return "unsorted"
	}

	return "open"
}

// today is which day it is in the configured zone.
func (t tasks) today() clock {
	at := t.Now().In(t.Zone)

	return clock{
		Date: at.Format(time.DateOnly), Weekday: at.Weekday().String(), Time: at.Format(clockLayout),
		Timezone: t.Zone.String(), WeekStart: t.settings.WeekStart,
	}
}

// places gives the slug of each area of the user by its id, and the id of
// their personal area.
func (t tasks) places(ctx context.Context, userID int64) (map[int64]string, int64, error) {
	areas, err := t.Areas(ctx, userID)
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

// get gives the caller's task with the id.
func (t tasks) get(ctx context.Context, id int64) (task, error) {
	c, _ := module.CallerFrom(ctx)

	slugs, _, err := t.places(ctx, c.UserID)
	if err != nil {
		return task{}, err
	}

	return t.read(ctx, slugs, c.UserID, id)
}

// read gives the user's task with the id in full, its links and the items
// that link to it each as the core resolves it for the caller, and which day
// it is, so the page of a task tells it without reading a list.
func (t tasks) read(ctx context.Context, slugs map[int64]string, userID, id int64) (task, error) {
	var (
		out   task
		links string
		err   error
	)

	out.row, err = scanRow(t.Store.QueryRowContext(ctx, `SELECT `+columns+`, notes, `+linkList+` FROM tasks WHERE user_id = ? AND id = ?`,
		userID, id), slugs, &out.Notes, &links)
	if errors.Is(err, sql.ErrNoRows) {
		return task{}, fmt.Errorf("task %d: %w", id, store.ErrNotFound)
	}

	if err != nil {
		return task{}, fmt.Errorf("tasks: read: %w", err)
	}

	var refs []module.Ref
	if err := json.Unmarshal([]byte(links), &refs); err != nil {
		return task{}, fmt.Errorf("tasks: read: %w", err)
	}

	from, err := t.LinksTo(ctx, out.Ref)
	if err != nil {
		return task{}, err
	}

	if out.Links, err = t.Items(ctx, refs); err != nil {
		return task{}, err
	}

	if out.LinkedFrom, err = t.Items(ctx, from); err != nil {
		return task{}, err
	}

	rows, err := t.Store.QueryContext(ctx, `SELECT at, source, said, changes FROM task_changes WHERE task_id = ? ORDER BY id DESC`, id)
	if err != nil {
		return task{}, fmt.Errorf("tasks: log: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var (
			e       entry
			at      int64
			changes string
		)

		if err := rows.Scan(&at, &e.Source, &e.Said, &changes); err != nil {
			return task{}, fmt.Errorf("tasks: log: %w", err)
		}

		if err := json.Unmarshal([]byte(changes), &e.Changes); err != nil {
			return task{}, fmt.Errorf("tasks: log: %w", err)
		}

		e.At = time.UnixMilli(at).UTC()
		out.Log = append(out.Log, e)
	}

	if err := rows.Err(); err != nil {
		return task{}, fmt.Errorf("tasks: log: %w", err)
	}

	// Every task has the entry of its making, the oldest.
	if len(out.Log) == 0 {
		return task{}, fmt.Errorf("tasks: task %d has no log", id)
	}

	out.Said, out.Now = out.Log[len(out.Log)-1].Said, t.today()

	return out, nil
}

// openOrder sorts open tasks by the first day that matters, the planned from
// or else the deadline, those with none last; on one day those with a time
// first, by time; and the oldest first among equals.
const openOrder = `coalesce(nullif(planned_from, ''), nullif(deadline, '')) NULLS LAST, nullif(planned_time, '') NULLS LAST, created_at, id`

// list gives the caller's tasks that f asks for, the open ones in openOrder,
// the done ones the most recently done first or the trash the most recently
// trashed first, and how many there are in all. A span gives the tasks whose
// plan overlaps it or whose deadline lies in it, and with no from it gives
// those of every day before its to.
func (t tasks) list(ctx context.Context, f filter) (page, error) {
	c, _ := module.CallerFrom(ctx)

	if f.limit == 0 {
		f.limit = defaultLimit
	}

	if f.limit < 0 || f.limit > maxLimit {
		return page{}, errLimit
	}

	var list any // nil asks for every list

	switch f.list {
	case "":
	case "none":
		list = ""
	case "soon", "someday":
		list = f.list
	default:
		return page{}, fmt.Errorf(`list is "soon", "someday" or "none": %w`, store.ErrInvalid)
	}

	if err := checkDate("from", f.from); err != nil {
		return page{}, err
	}

	if err := checkDate("to", f.to); err != nil {
		return page{}, err
	}

	if f.from != "" && f.to != "" && f.to < f.from {
		return page{}, fmt.Errorf("to %s is before from %s: %w", f.to, f.from, store.ErrInvalid)
	}

	var area int64

	if f.area != "" {
		a, err := t.FindArea(ctx, c.UserID, f.area)
		if err != nil {
			return page{}, err
		}

		area = a.ID
	}

	slugs, _, err := t.places(ctx, c.UserID)
	if err != nil {
		return page{}, err
	}

	done, order := "done_at IS NULL", openOrder
	if f.done {
		done, order = "done_at IS NOT NULL", "done_at DESC, id DESC"
	}

	// The trash holds open and done tasks alike: done narrows it, and leaving it out does not.
	if f.trashed {
		order = "trashed_at DESC, id DESC"

		if !f.done {
			done = "true"
		}
	}

	// A side of the span left out is open: '' is before every date.
	where := ` FROM tasks WHERE user_id = ?1 AND (trashed_at IS NOT NULL) = ?7 AND ` + done + ` AND ?2 IN (0, area_id) AND (?3 IS NULL OR list = ?3)
			AND (NOT ?4 OR done_at IS NULL AND list = '' AND planned_from = '' AND deadline = '')
			AND (?5 = '' AND ?6 = ''
				OR planned_from <> '' AND (?6 = '' OR planned_from <= ?6) AND planned_to >= ?5
				OR deadline <> '' AND (?6 = '' OR deadline <= ?6) AND deadline >= ?5)`
	args := []any{c.UserID, area, list, f.unsorted, f.from, f.to, f.trashed}

	rows, err := t.Store.QueryContext(ctx, `SELECT `+columns+`, count(*) OVER ()`+where+` ORDER BY `+order+` LIMIT ?8 OFFSET ?9`,
		append(args, f.limit, f.offset)...)
	if err != nil {
		return page{}, fmt.Errorf("tasks: list: %w", err)
	}
	defer rows.Close()

	out := page{Tasks: []row{}, Now: t.today()}

	for rows.Next() {
		r, err := scanRow(rows, slugs, &out.Total)
		if err != nil {
			return page{}, fmt.Errorf("tasks: list: %w", err)
		}

		out.Tasks = append(out.Tasks, r)
	}

	if err := rows.Err(); err != nil {
		return page{}, fmt.Errorf("tasks: list: %w", err)
	}

	// An offset past the last row leaves no row to carry the window's count.
	if f.offset > 0 && len(out.Tasks) == 0 {
		if err := t.Store.QueryRowContext(ctx, "SELECT count(*)"+where, args...).Scan(&out.Total); err != nil {
			return page{}, fmt.Errorf("tasks: count: %w", err)
		}
	}

	return out, nil
}

// create makes each task of b for the caller, all of them or, when one is
// refused, none, and logs the making of each with b's said.
func (t tasks) create(ctx context.Context, b batch) (made, error) {
	c, _ := module.CallerFrom(ctx)

	if len(b.Tasks) < 1 || len(b.Tasks) > maxBatch {
		return made{}, fmt.Errorf("a call makes 1 to %d tasks: %w", maxBatch, store.ErrInvalid)
	}

	slugs, personal, err := t.places(ctx, c.UserID)
	if err != nil {
		return made{}, err
	}

	given := make([]fields, len(b.Tasks))

	for i, d := range b.Tasks {
		f := fields{area: personal}

		if d.Area != "" {
			a, err := t.FindArea(ctx, c.UserID, d.Area)
			if err != nil {
				return made{}, fmt.Errorf("task %d of the call: %w", i+1, err)
			}

			f.area = a.ID
		}

		links, err := module.CleanLinks(d.Links)
		if err != nil {
			return made{}, fmt.Errorf("task %d of the call: %w", i+1, err)
		}

		f.apply(change{
			Title: &d.Title, Notes: &d.Notes, PlannedFrom: &d.PlannedFrom, PlannedTo: &d.PlannedTo,
			PlannedTime: &d.PlannedTime, Deadline: &d.Deadline, List: &d.List, Links: &links,
		}, 0)

		if err := f.check(); err != nil {
			return made{}, fmt.Errorf("task %d of the call: %w", i+1, err)
		}

		given[i] = f
	}

	out := made{Tasks: make([]row, len(given))}

	err = t.Store.Tx(ctx, func(tx *sql.Tx) error {
		now := t.Now().UnixMilli()

		for i, f := range given {
			r, err := scanRow(tx.QueryRowContext(ctx, `INSERT INTO tasks (user_id, area_id, title, notes, planned_from, planned_to, planned_time,
				deadline, list, source, created_at, changed_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?) RETURNING `+columns,
				c.UserID, f.area, f.title, f.notes, f.from, f.to, f.at, f.deadline, f.list, c.Source, now, now), slugs)
			if err != nil {
				return fmt.Errorf("tasks: create: %w", err)
			}

			out.Tasks[i] = r

			if _, err := tx.ExecContext(ctx, index, r.ID); err != nil {
				return fmt.Errorf("tasks: index: %w", err)
			}

			if err := keepLinks(ctx, tx, r.ID, f.links); err != nil {
				return err
			}

			if err := logChange(ctx, tx, r.ID, now, c.Source, b.Said, diff(fields{}, f, slugs)); err != nil {
				return err
			}
		}

		return nil
	})
	if err != nil {
		return made{}, err
	}

	for i, r := range out.Tasks {
		t.Changed(ctx, module.Change{Module: name, Kind: "task", ID: r.ID, Area: given[i].area})
	}

	out.Now = t.today()

	return out, nil
}

// change sets what ch gives on the caller's task with the id, and logs each
// field it changes. A change that leaves every field as it was logs nothing
// and tells no page. Marking a task done keeps when it was first done until
// it is opened again. A change of a task in the trash is ErrConflict.
func (t tasks) change(ctx context.Context, id int64, ch change) (task, error) {
	c, _ := module.CallerFrom(ctx)

	if ch == (change{}) {
		return task{}, fmt.Errorf("a change gives at least one field to change: %w", store.ErrInvalid)
	}

	slugs, _, err := t.places(ctx, c.UserID)
	if err != nil {
		return task{}, err
	}

	var area int64

	if ch.Area != nil {
		a, err := t.FindArea(ctx, c.UserID, *ch.Area)
		if err != nil {
			return task{}, err
		}

		area = a.ID
	}

	if ch.Links != nil {
		links, err := module.CleanLinks(*ch.Links)
		if err != nil {
			return task{}, err
		}

		ch.Links = &links
	}

	var (
		f       fields
		changed bool
	)

	err = t.Store.Tx(ctx, func(tx *sql.Tx) error {
		var (
			trashed bool
			links   string
		)

		err := tx.QueryRowContext(ctx, `SELECT title, notes, area_id, planned_from, planned_to, planned_time, deadline, list, done_at IS NOT NULL,
			trashed_at IS NOT NULL, `+linkList+` FROM tasks WHERE user_id = ? AND id = ?`, c.UserID, id).Scan(
			&f.title, &f.notes, &f.area, &f.from, &f.to, &f.at, &f.deadline, &f.list, &f.done, &trashed, &links)

		switch {
		case errors.Is(err, sql.ErrNoRows):
			return fmt.Errorf("task %d: %w", id, store.ErrNotFound)
		case err != nil:
			return fmt.Errorf("tasks: change: %w", err)
		case trashed:
			return errTrashed
		}

		if err := json.Unmarshal([]byte(links), &f.links); err != nil {
			return fmt.Errorf("tasks: change: %w", err)
		}

		was := f
		f.apply(ch, area)

		if err := f.check(); err != nil {
			return err
		}

		changes := diff(was, f, slugs)
		if len(changes) == 0 {
			return nil
		}

		now := t.Now().UnixMilli()

		_, err = tx.ExecContext(ctx, `UPDATE tasks SET title = ?, notes = ?, area_id = ?, planned_from = ?, planned_to = ?, planned_time = ?,
			deadline = ?, list = ?, done_at = iif(?, coalesce(done_at, ?), NULL), source = ?, changed_at = ? WHERE user_id = ? AND id = ?`,
			f.title, f.notes, f.area, f.from, f.to, f.at, f.deadline, f.list, f.done, now, c.Source, now, c.UserID, id)
		if err != nil {
			return fmt.Errorf("tasks: change: %w", err)
		}

		if was.title != f.title || was.notes != f.notes {
			if _, err := tx.ExecContext(ctx, index, id); err != nil {
				return fmt.Errorf("tasks: index: %w", err)
			}
		}

		if !slices.Equal(was.links, f.links) {
			if err := keepLinks(ctx, tx, id, f.links); err != nil {
				return err
			}
		}

		changed = true

		return logChange(ctx, tx, id, now, c.Source, "", changes)
	})
	if err != nil {
		return task{}, err
	}

	if changed {
		t.Changed(ctx, module.Change{Module: name, Kind: "task", ID: id, Area: f.area})
	}

	return t.read(ctx, slugs, c.UserID, id)
}

// trash puts the caller's task with the id in the trash, or takes it out
// when trashed is false. Neither writes an entry of the log or moves the task
// in its list, and finding the task where it is asked to be changes nothing.
func (t tasks) trash(ctx context.Context, id int64, trashed bool) (task, error) {
	c, _ := module.CallerFrom(ctx)

	slugs, _, err := t.places(ctx, c.UserID)
	if err != nil {
		return task{}, err
	}

	var (
		at   any // null takes the task out
		area int64
	)

	if trashed {
		at = t.Now().UnixMilli()
	}

	err = t.Store.QueryRowContext(ctx, `UPDATE tasks SET trashed_at = ?1 WHERE user_id = ?2 AND id = ?3 AND (trashed_at IS NULL) = ?4
		RETURNING area_id`, at, c.UserID, id, trashed).Scan(&area)

	switch {
	case err == nil:
		t.Changed(ctx, module.Change{Module: name, Kind: "task", ID: id, Area: area})
	case !errors.Is(err, sql.ErrNoRows):
		return task{}, fmt.Errorf("tasks: trash: %w", err)
	}

	return t.read(ctx, slugs, c.UserID, id)
}

// remove deletes the caller's task with the id for good, with its log, its
// links and its text in the search index. A task outside the trash is
// ErrConflict.
func (t tasks) remove(ctx context.Context, id int64) error {
	c, _ := module.CallerFrom(ctx)

	var area int64

	err := t.Store.Tx(ctx, func(tx *sql.Tx) error {
		var trashed bool

		err := tx.QueryRowContext(ctx, `SELECT area_id, trashed_at IS NOT NULL FROM tasks WHERE user_id = ? AND id = ?`,
			c.UserID, id).Scan(&area, &trashed)

		switch {
		case errors.Is(err, sql.ErrNoRows):
			return fmt.Errorf("task %d: %w", id, store.ErrNotFound)
		case err != nil:
			return fmt.Errorf("tasks: delete: %w", err)
		case !trashed:
			return fmt.Errorf("task %d is not in the trash, and only a task in the trash is deleted: %w", id, store.ErrConflict)
		}

		if _, err := tx.ExecContext(ctx, `DELETE FROM tasks WHERE user_id = ? AND id = ?`, c.UserID, id); err != nil {
			return fmt.Errorf("tasks: delete: %w", err)
		}

		if _, err := tx.ExecContext(ctx, purge); err != nil {
			return fmt.Errorf("tasks: delete: %w", err)
		}

		return nil
	})
	if err != nil {
		return err
	}

	t.Changed(ctx, module.Change{Module: name, Kind: "task", ID: id, Area: area})

	return nil
}

// empty deletes every task in the caller's trash for good, as remove does.
func (t tasks) empty(ctx context.Context) error {
	c, _ := module.CallerFrom(ctx)

	err := t.Store.Tx(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `DELETE FROM tasks WHERE user_id = ? AND trashed_at IS NOT NULL`, c.UserID); err != nil {
			return fmt.Errorf("tasks: empty the trash: %w", err)
		}

		if _, err := tx.ExecContext(ctx, purge); err != nil {
			return fmt.Errorf("tasks: empty the trash: %w", err)
		}

		return nil
	})
	if err != nil {
		return err
	}

	// One change for them all, since a stream that falls behind is ended.
	t.Changed(ctx, module.Change{Module: name, Kind: "trash"})

	return nil
}

// logChange writes the entry of a change of the task id at the time at.
func logChange(ctx context.Context, tx *sql.Tx, id, at int64, source, said string, changes []delta) error {
	b, err := json.Marshal(changes)
	if err != nil {
		return fmt.Errorf("tasks: log: %w", err)
	}

	_, err = tx.ExecContext(ctx, `INSERT INTO task_changes (task_id, at, source, said, changes) VALUES (?, ?, ?, ?, ?)`,
		id, at, source, said, string(b))
	if err != nil {
		return fmt.Errorf("tasks: log: %w", err)
	}

	return nil
}
