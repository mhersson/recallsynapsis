package tasks

import (
	"context"
	"fmt"
	"time"

	"github.com/mhersson/recallsynapsis/internal/module"
	"github.com/mhersson/recallsynapsis/internal/store"
)

const maxHits = 50

const (
	// hitColumns are a hit's, in the order search reads them.
	hitColumns = `tasks.id, area_id, tasks.title, tasks.notes, list, planned_from, deadline, done_at IS NOT NULL, changed_at`
	// within keeps a search to the user ?1's tasks outside the trash, in the area ?2 unless it is 0.
	within = `user_id = ?1 AND ?2 IN (0, area_id) AND trashed_at IS NULL`
)

// search gives the caller's tasks outside the trash, done or open, within q's
// area unless it is 0, that hold every word of q's text in their title or
// notes, in any case: the best ranked first, and at most maxHits. A word too
// short for the index is looked for in the tasks the other words found, or in
// every task, the most recently changed first, when no word is long enough.
func (t tasks) search(ctx context.Context, q module.Query) ([]module.Hit, error) {
	c, _ := module.CallerFrom(ctx)

	words := store.SearchWords(q.Text)
	if words.None() {
		return nil, nil
	}

	// No LIMIT: a short word is looked for here, so the tasks are read in
	// their order until maxHits of them pass.
	query, args := `SELECT `+hitColumns+` FROM tasks WHERE `+within+` ORDER BY changed_at DESC, id DESC`, []any{c.UserID, q.AreaID}

	if words.Match != "" {
		// CROSS JOIN keeps the index the outer loop, which sorts what it finds by rank itself.
		query = `SELECT ` + hitColumns + ` FROM tasks_fts CROSS JOIN tasks ON tasks.id = tasks_fts.rowid
			WHERE tasks_fts MATCH ?3 AND ` + within + ` ORDER BY rank`

		args = append(args, words.Match)
	}

	rows, err := t.Store.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("tasks: search: %w", err)
	}
	defer rows.Close()

	var hits []module.Hit

	for len(hits) < maxHits && rows.Next() {
		var (
			h                                  module.Hit
			id, changed                        int64
			title, notes, list, from, deadline string
			done                               bool
		)

		if err := rows.Scan(&id, &h.AreaID, &title, &notes, &list, &from, &deadline, &done, &changed); err != nil {
			return nil, fmt.Errorf("tasks: search: %w", err)
		}

		if !words.Holds(title, notes) {
			continue
		}

		h.Ref, h.Kind, h.Title, h.Snippet = module.NewRef(name, id), "task", title, words.Snippet(notes)
		h.State, h.ChangedAt = state(done, list, from, deadline), time.UnixMilli(changed).UTC()
		hits = append(hits, h)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("tasks: search: %w", err)
	}

	return hits, nil
}
