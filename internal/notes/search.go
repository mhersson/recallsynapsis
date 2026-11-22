package notes

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/mhersson/recallsynapsis/internal/module"
)

const (
	maxHits   = 50
	shortWord = 3 // characters: the trigram index finds no word shorter
	maxLabel  = 80

	snippetLen  = 120 // characters
	snippetLead = 30  // of them before what was found
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

	var long, short, match []string

	// A NUL parts words too, since FTS5 reads a quoted text only up to one.
	for w := range strings.FieldsFuncSeq(q.Text, func(r rune) bool { return unicode.IsSpace(r) || r == 0 }) {
		if utf8.RuneCountInString(w) < shortWord {
			short = append(short, fold(w))

			continue
		}

		// Quoted, any text is a phrase to FTS5, and never its syntax. FTS5
		// folds it itself, since Go's fold differs in a few letters, such as İ.
		long, match = append(long, fold(w)), append(match, `"`+strings.ReplaceAll(w, `"`, `""`)+`"`)
	}

	if len(long)+len(short) == 0 {
		return nil, nil
	}

	// A snippet is placed by the long words, which a short one would mostly
	// find first at the start of the body.
	anchors := long
	if len(anchors) == 0 {
		anchors = short
	}

	// No LIMIT: a short word is looked for here, so the notes are read in
	// their order until maxHits of them pass.
	query, args := `SELECT `+hitColumns+` FROM notes WHERE `+within+` ORDER BY changed_at DESC, id DESC`, []any{c.UserID, q.AreaID}

	if len(match) > 0 {
		// CROSS JOIN keeps the index the outer loop, which sorts what it finds by rank itself.
		query = `SELECT ` + hitColumns + ` FROM notes_fts CROSS JOIN notes ON notes.id = notes_fts.rowid
			WHERE notes_fts MATCH ?3 AND ` + within + ` ORDER BY rank`

		args = append(args, strings.Join(match, " "))
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

		if len(short) > 0 {
			// The line breaks keep a word from running from one field into the next.
			text := fold(title + "\n" + body + "\n" + tags)

			if slices.ContainsFunc(short, func(w string) bool { return !strings.Contains(text, w) }) {
				continue
			}
		}

		h.Ref, h.Title, h.Snippet = module.NewRef(name, id), label(title, body), snippet(body, anchors)
		h.State, h.ChangedAt = state(inbox), time.UnixMilli(changed).UTC()
		hits = append(hits, h)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("notes: search: %w", err)
	}

	return hits, nil
}

// fold lower-cases s one rune for one, so a rune stands at the same place in
// s and in its fold. SQLite's own lower folds ASCII alone.
func fold(s string) string {
	return strings.Map(unicode.ToLower, s)
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

// snippet gives the body around the first place where one of words, which
// are folded, stands in it, or the start of the body when none does: one
// line of whole words, at most snippetLen characters and an ellipsis at a cut.
func snippet(body string, words []string) string {
	body = strings.TrimSpace(body)
	low, at := fold(body), -1

	for _, w := range words {
		if i := strings.Index(low, w); i >= 0 && (at < 0 || i < at) {
			at = i
		}
	}

	runes := []rune(body)
	i := utf8.RuneCountInString(low[:max(at, 0)])
	start := max(0, min(i-snippetLead, len(runes)-snippetLen))
	end := min(len(runes), start+snippetLen)
	lead, rest := string(runes[start:i]), string(runes[i:end])

	// A cut inside a word moves to a space, so no word is cut in two.
	if start > 0 && !unicode.IsSpace(runes[start-1]) {
		if j := strings.IndexFunc(lead, unicode.IsSpace); j >= 0 {
			lead = lead[j:]
		}
	}

	if end < len(runes) && !unicode.IsSpace(runes[end]) {
		if j := strings.LastIndexFunc(rest, unicode.IsSpace); j > 0 {
			rest = rest[:j]
		}
	}

	out := strings.Join(strings.Fields(lead+rest), " ")

	if start > 0 {
		out = "…" + out
	}

	if end < len(runes) {
		out += "…"
	}

	return out
}
