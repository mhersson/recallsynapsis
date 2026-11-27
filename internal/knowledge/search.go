package knowledge

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
	hitColumns = `decisions.id, area_id, decisions.decision, decisions.why, decisions.source,
		EXISTS (SELECT 1 FROM decisions n WHERE n.replaces = decisions.id), changed_at`
	// within keeps a search to the user ?1's decisions, in the area ?2 unless it is 0.
	within = `user_id = ?1 AND ?2 IN (0, area_id)`
)

// search gives the caller's decisions, standing or replaced, within q's area
// unless it is 0, that hold every word of q's text in their line, why or
// source, in any case: the best ranked first, and at most maxHits. A word too
// short for the index is looked for in the decisions the other words found,
// or in every decision, the most recently changed first, when no word is long
// enough.
func (k knowledge) search(ctx context.Context, q module.Query) ([]module.Hit, error) {
	c, _ := module.CallerFrom(ctx)

	words := store.SearchWords(q.Text)
	if words.None() {
		return nil, nil
	}

	// No LIMIT: a short word is looked for here, so the decisions are read in
	// their order until maxHits of them pass.
	query, args := `SELECT `+hitColumns+` FROM decisions WHERE `+within+` ORDER BY changed_at DESC, id DESC`, []any{c.UserID, q.AreaID}

	if words.Match != "" {
		// CROSS JOIN keeps the index the outer loop, which sorts what it finds by rank itself.
		query = `SELECT ` + hitColumns + ` FROM decisions_fts CROSS JOIN decisions ON decisions.id = decisions_fts.rowid
			WHERE decisions_fts MATCH ?3 AND ` + within + ` ORDER BY rank`

		args = append(args, words.Match)
	}

	rows, err := k.Store.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("knowledge: search: %w", err)
	}
	defer rows.Close()

	var hits []module.Hit

	for len(hits) < maxHits && rows.Next() {
		var (
			h                 module.Hit
			id, changed       int64
			line, why, source string
			replaced          bool
		)

		if err := rows.Scan(&id, &h.AreaID, &line, &why, &source, &replaced, &changed); err != nil {
			return nil, fmt.Errorf("knowledge: search: %w", err)
		}

		if !words.Holds(line, why, source) {
			continue
		}

		h.Ref, h.Kind, h.Title, h.Snippet = module.NewRef(name, id), "decision", line, words.Snippet(why)
		h.State, h.ChangedAt = state(replaced), time.UnixMilli(changed).UTC()
		hits = append(hits, h)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("knowledge: search: %w", err)
	}

	return hits, nil
}
