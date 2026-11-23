package store

import (
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"
)

const (
	shortWord = 3 // characters: the trigram index finds no word shorter

	snippetLen  = 120 // characters
	snippetLead = 30  // of them before what was found
)

// Words are the words of a search's text, as a module looks for them in its
// FTS5 index with the trigram tokenizer and in the text it reads back.
type Words struct {
	// Match finds the words long enough for the index, or is "" when none
	// is. Quoted, any text is a phrase to FTS5, and never its syntax. FTS5
	// folds it itself, since Go's fold differs in a few letters, such as İ.
	Match       string
	long, short []string // folded
}

// SearchWords parts text into its words. A NUL parts words too, since FTS5
// reads a quoted text only up to one.
func SearchWords(text string) Words {
	var (
		w     Words
		match []string
	)

	for word := range strings.FieldsFuncSeq(text, func(r rune) bool { return unicode.IsSpace(r) || r == 0 }) {
		if utf8.RuneCountInString(word) < shortWord {
			w.short = append(w.short, fold(word))

			continue
		}

		w.long, match = append(w.long, fold(word)), append(match, `"`+strings.ReplaceAll(word, `"`, `""`)+`"`)
	}

	w.Match = strings.Join(match, " ")

	return w
}

// None reports whether the text held no word.
func (w Words) None() bool {
	return len(w.long)+len(w.short) == 0
}

// Holds reports whether fields hold every word too short for the index, in
// any case.
func (w Words) Holds(fields ...string) bool {
	if len(w.short) == 0 {
		return true
	}

	// The line breaks keep a word from running from one field into the next.
	text := fold(strings.Join(fields, "\n"))

	return !slices.ContainsFunc(w.short, func(s string) bool { return !strings.Contains(text, s) })
}

// Snippet gives text around the first of the words in it, or its start when
// none is: one line of whole words, at most 120 characters, and an ellipsis
// at a cut. It is placed by the long words, which a short one would mostly
// find first at the start of the text.
func (w Words) Snippet(text string) string {
	anchors := w.long
	if len(anchors) == 0 {
		anchors = w.short
	}

	return snippet(text, anchors)
}

// fold lower-cases s one rune for one, so a rune stands at the same place in
// s and in its fold. SQLite's own lower folds ASCII alone.
func fold(s string) string {
	return strings.Map(unicode.ToLower, s)
}

// snippet gives the text around the first place where one of words, which
// are folded, stands in it, or the start of the text when none does: one
// line of whole words, at most snippetLen characters and an ellipsis at a cut.
func snippet(text string, words []string) string {
	text = strings.TrimSpace(text)
	low, at := fold(text), -1

	for _, w := range words {
		if i := strings.Index(low, w); i >= 0 && (at < 0 || i < at) {
			at = i
		}
	}

	runes := []rune(text)
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
