package tasks

import (
	"cmp"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/mhersson/recallsynapsis/internal/module"
	"github.com/mhersson/recallsynapsis/internal/store"
)

// kinds are the notifications a user gets, each switched on or off, and the
// hour, HH:MM, of each that comes once a day.
type kinds struct {
	Start      bool   `json:"start"` // at a task's start time
	Morning    bool   `json:"morning"`
	MorningAt  string `json:"morning_at"`
	Deadline   bool   `json:"deadline"` // the tasks due tomorrow
	DeadlineAt string `json:"deadline_at"`
	Untimed    bool   `json:"untimed"` // the tasks planned for today with no start time today
	UntimedAt  string `json:"untimed_at"`
}

// fresh are the kinds of a user who has changed none.
var fresh = kinds{Start: true, Morning: true, MorningAt: "07:00", DeadlineAt: "18:00", UntimedAt: "09:00"}

// kindColumns are task_notify's columns of the kinds, in the order dest gives them.
const kindColumns = `start, morning, morning_at, deadline, deadline_at, untimed, untimed_at`

func (k *kinds) dest() []any {
	return []any{&k.Start, &k.Morning, &k.MorningAt, &k.Deadline, &k.DeadlineAt, &k.Untimed, &k.UntimedAt}
}

// readKinds gives the caller's kinds.
func (t tasks) readKinds(ctx context.Context) (kinds, error) {
	c, _ := module.CallerFrom(ctx)
	k := fresh

	err := t.Store.QueryRowContext(ctx, `SELECT `+kindColumns+` FROM task_notify WHERE user_id = ?`, c.UserID).Scan(k.dest()...)
	if errors.Is(err, sql.ErrNoRows) {
		return fresh, nil
	}

	if err != nil {
		return kinds{}, fmt.Errorf("tasks: read the notifications: %w", err)
	}

	return k, nil
}

// keepKinds keeps k as the caller's kinds, and refuses an hour that is not HH:MM.
func (t tasks) keepKinds(ctx context.Context, k kinds) error {
	c, _ := module.CallerFrom(ctx)

	for _, h := range []struct{ key, value string }{{"morning_at", k.MorningAt}, {"deadline_at", k.DeadlineAt}, {"untimed_at", k.UntimedAt}} {
		if !written(clockLayout, h.value) {
			return fmt.Errorf("%s %q is not a time of day, HH:MM: %w", h.key, h.value, store.ErrInvalid)
		}
	}

	if _, err := t.Store.ExecContext(ctx, `REPLACE INTO task_notify (user_id, `+kindColumns+`) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		c.UserID, k.Start, k.Morning, k.MorningAt, k.Deadline, k.DeadlineAt, k.Untimed, k.UntimedAt); err != nil {
		return fmt.Errorf("tasks: keep the notifications: %w", err)
	}

	t.Changed(ctx, module.Change{Module: name, Kind: "notify"})

	return nil
}

// planned is an open task outside the trash that a notification may name,
// with its area's name, or "" in the personal area, and its time today as at,
// or "": a time is on the first day of a plan, so a later day of it has none.
type planned struct {
	id, user                            int64
	title, from, to, at, deadline, area string
}

// notify hands Notify, for each user, what came due after the moment kept and
// not after now, on today's date in the configured zone, and then keeps now.
// It runs whether a module that delivers is on or not, so switching one on
// sends nothing that came due before. The very first run sends nothing.
func (t tasks) notify(ctx context.Context) error {
	now := t.Now()

	var kept int64

	err := t.Store.QueryRowContext(ctx, `SELECT at FROM task_notified`).Scan(&kept)
	if errors.Is(err, sql.ErrNoRows) {
		return t.notified(ctx, now)
	}

	if err != nil {
		return fmt.Errorf("tasks: notify: %w", err)
	}

	day := now.In(t.Zone)
	y, m, d := day.Date()
	since := time.UnixMilli(kept)
	today, tomorrow := day.Format(time.DateOnly), time.Date(y, m, d+1, 0, 0, 0, 0, time.UTC).Format(time.DateOnly)

	// time.Date gives a time the clock repeats or skips as one instant, so a day
	// the clock changes sends each once, and a time of the day before is not due.
	due := func(hhmm string) bool {
		c, err := time.Parse(clockLayout, hhmm)
		at := time.Date(y, m, d, c.Hour(), c.Minute(), 0, 0, t.Zone)

		return err == nil && at.After(since) && !at.After(now)
	}

	chosen, err := t.everyKinds(ctx)
	if err != nil {
		return err
	}

	byUser, err := t.readPlanned(ctx, today, tomorrow)
	if err != nil {
		return err
	}

	for user, ts := range byUser {
		k, ok := chosen[user]
		if !ok {
			k = fresh
		}

		for _, msg := range messages(k, ts, today, tomorrow, due) {
			t.Notify(ctx, user, msg)
		}
	}

	// The stop cuts Notify short, so the next start sends this span again, and
	// the tag makes a notification sent twice replace the first.
	if err := ctx.Err(); err != nil {
		return err
	}

	return t.notified(ctx, now)
}

// notified keeps at as the moment notified up to.
func (t tasks) notified(ctx context.Context, at time.Time) error {
	if _, err := t.Store.ExecContext(ctx, `REPLACE INTO task_notified (id, at) VALUES (1, ?)`, at.UnixMilli()); err != nil {
		return fmt.Errorf("tasks: keep the moment notified: %w", err)
	}

	return nil
}

// everyKinds gives the kinds of each user who has changed one.
func (t tasks) everyKinds(ctx context.Context) (map[int64]kinds, error) {
	rows, err := t.Store.QueryContext(ctx, `SELECT user_id, `+kindColumns+` FROM task_notify`)
	if err != nil {
		return nil, fmt.Errorf("tasks: read the notifications: %w", err)
	}
	defer rows.Close()

	out := map[int64]kinds{}

	for rows.Next() {
		var (
			user int64
			k    kinds
		)

		if err := rows.Scan(append([]any{&user}, k.dest()...)...); err != nil {
			return nil, fmt.Errorf("tasks: read the notifications: %w", err)
		}

		out[user] = k
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("tasks: read the notifications: %w", err)
	}

	return out, nil
}

// readPlanned gives each user's open tasks outside the trash, by id, that a
// notification may name today: those planned from today or earlier, and
// those due tomorrow or earlier. It joins the core's areas, rather than ask
// Deps.Areas for each user, so a run costs one query however many users.
func (t tasks) readPlanned(ctx context.Context, today, tomorrow string) (map[int64][]planned, error) {
	rows, err := t.Store.QueryContext(ctx, `SELECT tasks.id, tasks.user_id, title, planned_from, planned_to,
		iif(planned_from = ?1, planned_time, ''), deadline,
		iif(areas.kind = 'personal', '', areas.name) FROM tasks JOIN areas ON areas.id = area_id
		WHERE done_at IS NULL AND trashed_at IS NULL AND (planned_from <> '' AND planned_from <= ?1 OR deadline <> '' AND deadline <= ?2)
		ORDER BY tasks.id`, today, tomorrow)
	if err != nil {
		return nil, fmt.Errorf("tasks: notify: %w", err)
	}
	defer rows.Close()

	out := map[int64][]planned{}

	for rows.Next() {
		var p planned
		if err := rows.Scan(&p.id, &p.user, &p.title, &p.from, &p.to, &p.at, &p.deadline, &p.area); err != nil {
			return nil, fmt.Errorf("tasks: notify: %w", err)
		}

		out[p.user] = append(out[p.user], p)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("tasks: notify: %w", err)
	}

	return out, nil
}

// messages are what the kinds k send of a user's tasks ts, by id, as due
// tells which times of day came.
func messages(k kinds, ts []planned, today, tomorrow string, due func(hhmm string) bool) []module.Message {
	var out []module.Message

	covers := func(p planned) bool { return p.from != "" && p.from <= today && today <= p.to }

	if k.Start {
		for _, p := range ts {
			if p.at == "" || !due(p.at) {
				continue
			}

			text := p.at
			if p.area != "" {
				text += " · " + p.area
			}

			out = append(out, module.Message{Title: p.title, Body: text, URL: taskPath(p.id), Tag: "tasks-" + strconv.FormatInt(p.id, 10)})
		}
	}

	if k.Morning && due(k.MorningAt) {
		out = append(out, morning(ts, today, covers)...)
	}

	if k.Deadline && due(k.DeadlineAt) {
		out = append(out, summary(ts, func(p planned) bool { return p.deadline == tomorrow }, "due tomorrow", "/tasks?view=week", "tasks-deadline")...)
	}

	if k.Untimed && due(k.UntimedAt) {
		out = append(out, summary(ts, func(p planned) bool { return covers(p) && p.at == "" }, "planned for today", "/tasks?view=today", "tasks-untimed")...)
	}

	return out
}

// morning is the message of what the day holds, grouped as the Today page
// groups, each task in the first group it belongs to, or none when no group
// holds one.
func morning(ts []planned, today string, covers func(planned) bool) []module.Message {
	var left, late, now []planned

	for _, p := range ts {
		switch {
		case p.from != "" && p.to < today:
			left = append(left, p)
		case p.deadline != "" && p.deadline < today:
			late = append(late, p)
		case covers(p) || p.deadline == today:
			now = append(now, p)
		}
	}

	// Those with a time first, by time; the sort keeps the rest by id.
	slices.SortStableFunc(now, func(a, b planned) int { return strings.Compare(cmp.Or(a.at, "~"), cmp.Or(b.at, "~")) })

	var counts, lines []string

	for _, g := range []struct {
		ts []planned
		of string
	}{{now, "today"}, {left, "left from earlier"}, {late, "overdue"}} {
		if len(g.ts) > 0 {
			counts = append(counts, fmt.Sprintf("%d %s", len(g.ts), g.of))
		}
	}

	if len(counts) == 0 {
		return nil
	}

	for _, p := range now {
		if p.at != "" {
			lines = append(lines, p.at+" "+p.title)
		} else {
			lines = append(lines, p.title)
		}
	}

	lines = append(append(lines, titles(left)...), titles(late)...)

	return []module.Message{{Title: strings.Join(counts, ", "), Body: strings.Join(lines, "\n"), URL: "/tasks?view=today", Tag: "tasks-morning"}}
}

// summary is one message that counts and names the tasks of ts that holds
// takes, or none when it takes none. A tap opens the task when it is alone,
// else path.
func summary(ts []planned, holds func(planned) bool, what, path, tag string) []module.Message {
	var named []planned

	for _, p := range ts {
		if holds(p) {
			named = append(named, p)
		}
	}

	switch len(named) {
	case 0:
		return nil
	case 1:
		path = taskPath(named[0].id)
	}

	return []module.Message{{Title: fmt.Sprintf("%d %s", len(named), what), Body: strings.Join(titles(named), "\n"), URL: path, Tag: tag}}
}

func titles(ts []planned) []string {
	out := make([]string, len(ts))
	for i, p := range ts {
		out[i] = p.title
	}

	return out
}

// taskPath is the path of the task's page in the web app.
func taskPath(id int64) string {
	return "/tasks/" + strconv.FormatInt(id, 10)
}
