package notes

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/mhersson/recallsynapsis/internal/module"
	"github.com/mhersson/recallsynapsis/internal/store"
)

const (
	maxHits  = 50
	maxLabel = 80
)

const (
	// hitColumns are a hit's, in the order search reads them.
	hitColumns = `notes.id, area_id, kind, notes.title, notes.body, inbox, changed_at,
		coalesce((SELECT group_concat(tag, ' ') FROM note_tags WHERE note_id = notes.id), '')`
	// within keeps a search to the user ?1's notes outside the trash, in the area ?2 unless it is 0.
	within = `user_id = ?1 AND ?2 IN (0, area_id) AND trashed_at IS NULL`
)

// search gives the caller's notes outside the trash, within q's area unless
// it is 0, that hold every word of q's text in their title, body or tags, in
// any case: the best ranked first, and at most maxHits. A word too short for
// the index is looked for in the notes the other words found, or in every
// note, the most recently changed first, when no word is long enough.
func (n notes) search(ctx context.Context, q module.Query) ([]module.Hit, error) {
	c, _ := module.CallerFrom(ctx)

	words := store.SearchWords(q.Text)
	if words.None() {
		return nil, nil
	}

	// No LIMIT: a short word is looked for here, so the notes are read in
	// their order until maxHits of them pass.
	query, args := `SELECT `+hitColumns+` FROM notes WHERE `+within+` ORDER BY changed_at DESC, id DESC`, []any{c.UserID, q.AreaID}

	if words.Match != "" {
		// CROSS JOIN keeps the index the outer loop, which sorts what it finds by rank itself.
		query = `SELECT ` + hitColumns + ` FROM notes_fts CROSS JOIN notes ON notes.id = notes_fts.rowid
			WHERE notes_fts MATCH ?3 AND ` + within + ` ORDER BY rank`

		args = append(args, words.Match)
	}

	rows, err := n.Store.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("notes: search: %w", err)
	}
	defer rows.Close()

	var hits []module.Hit

	for len(hits) < maxHits && rows.Next() {
		var (
			h                 module.Hit
			id, changed       int64
			title, body, tags string
			inbox             bool
		)

		if err := rows.Scan(&id, &h.AreaID, &h.Kind, &title, &body, &inbox, &changed, &tags); err != nil {
			return nil, fmt.Errorf("notes: search: %w", err)
		}

		if !words.Holds(title, body, tags) {
			continue
		}

		h.Ref, h.Title, h.Snippet = module.NewRef(name, id), label(title, body), words.Snippet(body)
		h.State, h.ChangedAt = state(inbox), time.UnixMilli(changed).UTC()
		hits = append(hits, h)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("notes: search: %w", err)
	}

	return hits, nil
}

// label is what a hit or a link calls a note: its title, or the first line
// of its body, cut at maxLabel characters, when it has none.
func label(title, body string) string {
	if title != "" {
		return title
	}

	line, _, _ := strings.Cut(strings.TrimSpace(body), "\n")
	line = strings.TrimSpace(line)

	if r := []rune(line); len(r) > maxLabel {
		return string(r[:maxLabel])
	}

	return line
}

// state is how a hit or a link tells a capture that waits to be filed.
func state(inbox bool) string {
	if inbox {
		return "inbox"
	}

	return "filed"
}
