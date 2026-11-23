package tasks

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/mhersson/recallsynapsis/internal/module"
	"github.com/mhersson/recallsynapsis/internal/store"
)

// linkList is the JSON list of a task's links in the order given: a change
// that gives them writes them all anew, each with a rowid above the rest.
const linkList = `(SELECT json_group_array(ref ORDER BY task_links.rowid) FROM task_links WHERE task_id = tasks.id)`

// keepLinks replaces the task's links with links.
func keepLinks(ctx context.Context, tx *sql.Tx, id int64, links []module.Ref) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM task_links WHERE task_id = ?`, id); err != nil {
		return fmt.Errorf("tasks: links: %w", err)
	}

	for _, l := range links {
		if _, err := tx.ExecContext(ctx, `INSERT INTO task_links (task_id, ref) VALUES (?, ?)`, id, l); err != nil {
			return fmt.Errorf("tasks: links: %w", err)
		}
	}

	return nil
}

// resolve gives the caller's task with the id as a link shows it. A task in
// the trash is ErrNotFound, as an absent one and another user's are.
func (t tasks) resolve(ctx context.Context, id int64) (module.Item, error) {
	c, _ := module.CallerFrom(ctx)

	var (
		it                   = module.Item{Kind: "task"}
		list, from, deadline string
		done                 bool
	)

	err := t.Store.QueryRowContext(ctx, `SELECT title, list, planned_from, deadline, done_at IS NOT NULL FROM tasks
		WHERE user_id = ? AND id = ? AND trashed_at IS NULL`, c.UserID, id).Scan(&it.Title, &list, &from, &deadline, &done)
	if errors.Is(err, sql.ErrNoRows) {
		return module.Item{}, fmt.Errorf("task %d: %w", id, store.ErrNotFound)
	}

	if err != nil {
		return module.Item{}, fmt.Errorf("tasks: resolve: %w", err)
	}

	it.State = state(done, list, from, deadline)

	return it, nil
}

// linksTo gives the caller's tasks outside the trash that link to ref, the
// most recently changed first.
func (t tasks) linksTo(ctx context.Context, ref module.Ref) ([]module.Ref, error) {
	c, _ := module.CallerFrom(ctx)

	// CROSS JOIN keeps task_links_ref the outer loop: the planner would rather
	// walk every task of the user.
	rows, err := t.Store.QueryContext(ctx, `SELECT id FROM task_links CROSS JOIN tasks ON tasks.id = task_id
		WHERE ref = ? AND user_id = ? AND trashed_at IS NULL ORDER BY changed_at DESC, id DESC`, ref, c.UserID)
	if err != nil {
		return nil, fmt.Errorf("tasks: links to %s: %w", ref, err)
	}
	defer rows.Close()

	var out []module.Ref

	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("tasks: links to %s: %w", ref, err)
		}

		out = append(out, module.NewRef(name, id))
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("tasks: links to %s: %w", ref, err)
	}

	return out, nil
}
