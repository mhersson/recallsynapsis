// Package testkit is fake, a feature module as small as the tests of what
// serves modules need: a table, a route that adds to it, a search of it, and
// a job that fails on every tick so a test sees each run in the log.
package testkit

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/mhersson/recallsynapsis/internal/module"
	"github.com/mhersson/recallsynapsis/internal/store"
)

type fake struct {
	module.Deps
}

// New builds the fake module, named fake.
func New(d module.Deps) module.Module {
	f := fake{d}

	return module.Module{
		Name: "fake",
		Migrations: []store.Migration{{Version: 1, SQL: `
CREATE TABLE fake_items (
	id         INTEGER PRIMARY KEY,
	user_id    INTEGER NOT NULL REFERENCES users (id) ON DELETE CASCADE,
	area_id    INTEGER NOT NULL REFERENCES areas (id) ON DELETE CASCADE,
	title      TEXT NOT NULL,
	changed_at INTEGER NOT NULL
)`}},
		Routes: []module.Route{{Pattern: "POST /api/fake/items", Access: module.Write, Serve: f.add}},
		Jobs: []module.Job{{Name: "tick", Every: 10 * time.Millisecond, Run: func(context.Context) error {
			return errors.New("the fake's job ran")
		}}},
		Search: f.search,
	}
}

// add stores an item {"title","area"} of the caller, its area a slug or a
// name, and answers {"ref"}.
func (f fake) add(w http.ResponseWriter, r *http.Request) error {
	var in struct {
		Title string `json:"title"`
		Area  string `json:"area"`
	}

	if err := module.Decode(w, r, &in); err != nil {
		return err
	}

	c, _ := module.CallerFrom(r.Context())

	a, err := f.FindArea(r.Context(), c.UserID, in.Area)
	if err != nil {
		return err
	}

	var id int64

	err = f.Store.QueryRowContext(r.Context(), `INSERT INTO fake_items (user_id, area_id, title, changed_at)
		VALUES (?, ?, ?, ?) RETURNING id`, c.UserID, a.ID, in.Title, f.Now().UnixMilli()).Scan(&id)
	if err != nil {
		return fmt.Errorf("fake: add: %w", err)
	}

	module.Reply(w, http.StatusCreated, map[string]module.Ref{"ref": module.NewRef("fake", id)})

	return nil
}

// search finds the caller's items whose title holds the text, in any case.
func (f fake) search(ctx context.Context, q module.Query) ([]module.Hit, error) {
	c, _ := module.CallerFrom(ctx)

	rows, err := f.Store.QueryContext(ctx, `SELECT id, area_id, title, changed_at FROM fake_items
		WHERE user_id = ?1 AND ?2 IN (0, area_id) AND instr(lower(title), lower(?3)) > 0 ORDER BY id`, c.UserID, q.AreaID, q.Text)
	if err != nil {
		return nil, fmt.Errorf("fake: search: %w", err)
	}
	defer rows.Close()

	var hits []module.Hit

	for rows.Next() {
		h := module.Hit{Kind: "item", State: "kept"}

		var id, changed int64
		if err := rows.Scan(&id, &h.AreaID, &h.Title, &changed); err != nil {
			return nil, fmt.Errorf("fake: search: %w", err)
		}

		h.Ref, h.Snippet, h.ChangedAt = module.NewRef("fake", id), h.Title, time.UnixMilli(changed).UTC()
		hits = append(hits, h)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("fake: search: %w", err)
	}

	return hits, nil
}
