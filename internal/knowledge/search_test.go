package knowledge_test

import (
	"bytes"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mhersson/recallsynapsis/internal/core"
	"github.com/mhersson/recallsynapsis/internal/knowledge"
	"github.com/mhersson/recallsynapsis/internal/module"
	"github.com/mhersson/recallsynapsis/internal/store"
)

// hits are the hits of GET /api/search for text, within area unless it is
// empty, by the module whose group they are in.
func (f *fixture) hits(t *testing.T, text, area, auth string) map[string][]object {
	t.Helper()

	q := url.Values{"q": {text}}
	if area != "" {
		q.Set("area", area)
	}

	code, got := f.do(t, http.MethodGet, "/api/search?"+q.Encode(), "", auth)
	require.Equal(t, http.StatusOK, code, got)

	out := map[string][]object{}
	for _, g := range objects(got["groups"]) {
		out[g["module"].(string)] = objects(g["hits"])
	}

	return out
}

// found are the lines of ada's decisions that text finds in every area, in their order.
func (f *fixture) found(t *testing.T, text string) []string {
	t.Helper()

	return titles(f.hits(t, text, "", f.phone)["knowledge"])
}

func titles(hits []object) []string {
	out := []string{}
	for _, h := range hits {
		out = append(out, h["title"].(string))
	}

	return out
}

// indexes reports whether a block of the search index holds the bytes of text.
func (f *fixture) indexes(t *testing.T, text string) bool {
	t.Helper()

	rows, err := f.store.QueryContext(t.Context(), `SELECT block FROM decisions_fts_data`)
	require.NoError(t, err)

	defer rows.Close()

	found := false

	for rows.Next() {
		var block []byte
		require.NoError(t, rows.Scan(&block))

		found = found || bytes.Contains(block, []byte(text))
	}

	require.NoError(t, rows.Err())

	return found
}

func TestSearchFindsAWordOfALineAWhyOrASource(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	web := f.signIn(t, "ada")
	f.pack(t, `{"area":"garden","add":[
		{"decision":"Beds are raised","why":"The soil is heavy clay.","source":"Hagemøtet i oktober"},
		{"decision":"Vanningsanlegget går ved soloppgang","why":"Less is lost to the sun."}]}`, web)
	f.add(t, "orchard", `{"decision":"Trærne beskjæres i februar","why":"Before the sap rises."}`, web)

	for _, tt := range []struct {
		text string
		want []string
	}{
		{"raised", []string{"Beds are raised"}},
		{"CLAY", []string{"Beds are raised"}},
		{"oktober", []string{"Beds are raised"}},
		{"hagemøte", []string{"Beds are raised"}},
		{"ok", []string{"Beds are raised"}},
		{"vanningsan", []string{"Vanningsanlegget går ved soloppgang"}},
		{"VANNINGSAN", []string{"Vanningsanlegget går ved soloppgang"}},
		{"va", []string{"Vanningsanlegget går ved soloppgang"}},
		{"VA", []string{"Vanningsanlegget går ved soloppgang"}},
		{"sap fe", []string{"Trærne beskjæres i februar"}},
		{"clay sun", []string{}},
		{"  ", []string{}},
	} {
		assert.Equal(t, tt.want, f.found(t, tt.text), "%q", tt.text)
	}
}

func TestAHitShowsItsDecision(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	web := f.signIn(t, "ada")
	clay := f.add(t, "garden", `{"decision":"Beds are raised","why":"The soil is heavy clay.","source":"Hagemøtet"}`, web)
	flat := f.add(t, "garden", fmt.Sprintf(`{"decision":"Beds are flat","why":"The clay is gone.","replaces":%q}`, clay["ref"]), web)
	pots := f.add(t, "orchard", `{"decision":"Trees grow in pots","why":"`+strings.Repeat("ord ", 60)+`clay`+strings.Repeat(" røtter", 60)+`"}`, web)

	code, got := f.do(t, http.MethodPost, "/api/notes", `{"title":"Clay","area":"garden"}`, f.laptop)
	require.Equal(t, http.StatusCreated, code, got)

	groups := f.hits(t, "clay", "", f.phone)
	assert.Equal(t, []string{"Clay"}, titles(groups["notes"]), "beside the notes' hits")

	byRef := map[any]object{}
	for _, h := range groups["knowledge"] {
		byRef[h["ref"]] = h
	}

	hit := func(d object, state string) object {
		return object{
			"ref": d["ref"], "kind": "decision", "state": state, "title": d["decision"], "snippet": d["why"], "area": d["area"],
			"changed_at": d["changed_at"],
		}
	}

	require.Contains(t, byRef, pots["ref"])
	snippet := byRef[pots["ref"]]["snippet"].(string)
	delete(byRef, pots["ref"])

	assert.Equal(t, map[any]object{clay["ref"]: hit(clay, "replaced"), flat["ref"]: hit(flat, "stands")}, byRef,
		"a standing and a replaced decision are both found, each with its line and its why as the snippet")
	assert.LessOrEqual(t, utf8.RuneCountInString(snippet), 122, snippet)

	words := strings.Fields(strings.TrimSuffix(strings.TrimPrefix(snippet, "…"), "…"))
	assert.Positive(t, slices.Index(words, "clay"), "a long why gives a snippet around what was found")
	assert.Subset(t, []string{"ord", "clay", "røtter"}, words, "of whole words")
	assert.Equal(t, []object{hit(clay, "replaced")}, f.hits(t, "hagemøtet", "", f.phone)["knowledge"], "the snippet is of the why when the source holds the word")

	for _, tt := range []struct {
		text, area string
		want       []string
	}{
		{"clay", "garden", []string{"Beds are raised", "Beds are flat"}},
		{"clay", "Orchard", []string{"Trees grow in pots"}},
		{"cl", "orchard", []string{"Trees grow in pots"}},
		{"clay", "personal", []string{}},
	} {
		assert.ElementsMatch(t, tt.want, titles(f.hits(t, tt.text, tt.area, f.phone)["knowledge"]),
			"a search within an area finds its decisions alone: %q in %s", tt.text, tt.area)
	}

	brief, revision := "Raised beds of qzxqzx clay.", int64(0)
	_, err := f.core.ChangeArea(t.Context(), module.Caller{UserID: f.users["ada"], Source: "web"}, f.areas["garden"],
		core.Change{Brief: &brief, BriefRevision: &revision})
	require.NoError(t, err)
	assert.Empty(t, f.hits(t, "qzxqzx", "", f.phone), "the brief gives no hit")
}

func TestSearchFollowsEveryWrite(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	web := f.signIn(t, "ada")
	clay := f.add(t, "garden", `{"decision":"Beds are raised","why":"The soil is clay.","source":"Hagemøtet"}`, web)

	f.mend(t, "garden", clay, `{"decision":"Beds are flat","why":"Sand was brought in.","source":"Naboen"}`, web)

	for _, tt := range []struct {
		text string
		want []string
	}{
		{"raised", []string{}},
		{"clay", []string{}},
		{"hagemøtet", []string{}},
		{"flat", []string{"Beds are flat"}},
		{"sand", []string{"Beds are flat"}},
		{"naboen", []string{"Beds are flat"}},
	} {
		assert.Equal(t, tt.want, f.found(t, tt.text), "a mended decision is found by its new words alone: %q", tt.text)
	}

	// Each trigram looked for starts with a letter no other term of the index
	// does, so FTS5's prefix compression leaves it whole in a block.
	secret := f.add(t, "garden", `{"decision":"Kode qxjqxj","why":"zvkzvk","source":"ymfymf"}`, web)
	indexed := f.count(t, "decisions_fts")

	for _, text := range []string{"qxj", "zvk", "ymf"} {
		require.True(t, f.indexes(t, text), "the index holds the text of a decision: %s", text)
	}

	code, got := f.do(t, http.MethodDelete, path(secret), "", web)
	require.Equal(t, http.StatusNoContent, code, got)
	assert.Equal(t, indexed-1, f.count(t, "decisions_fts"), "a deleted decision leaves the index")

	for _, text := range []string{"qxj", "zvk", "ymf"} {
		assert.False(t, f.indexes(t, text), "with nothing of its text left in it: %s", text)
	}

	assert.Equal(t, []string{}, f.found(t, "qxjqxj"))
	assert.Equal(t, []string{"Beds are flat"}, f.found(t, "flat"))
}

func TestDecisionsMadeBeforeTheIndexAreFound(t *testing.T) {
	t.Parallel()

	f := newDatabase(t)
	first := knowledge.New(module.Deps{}).Migrations[0]
	first.Owner = "knowledge"
	require.NoError(t, f.store.Migrate(t.Context(), []store.Migration{first}))

	_, err := f.store.ExecContext(t.Context(), `INSERT INTO decisions (id, user_id, area_id, decision, why, source, decided, replaces,
		revision, created_at, changed_at, changed_by) VALUES
		(1, ?1, ?2, 'Beds are raised', 'The soil is clay.', 'Hagemøtet', '2026-10-01', NULL, 1, 1, 1, 'web'),
		(2, ?1, ?2, 'Beds are flat', 'Sand was brought in.', '', '2026-11-01', 1, 1, 2, 2, 'web')`, f.users["ada"], f.areas["garden"])
	require.NoError(t, err)

	_, err = f.store.ExecContext(t.Context(), `INSERT INTO decision_revisions
		SELECT id, revision, changed_at, changed_by, '', decision, why, source, decided, replaces FROM decisions`)
	require.NoError(t, err)

	f.load(t, object{"enabled": true})

	for _, tt := range []struct {
		text string
		want []string
	}{
		{"raised", []string{"Beds are raised"}},
		{"clay", []string{"Beds are raised"}},
		{"hagemøtet", []string{"Beds are raised"}},
		{"sand", []string{"Beds are flat"}},
		{"fl", []string{"Beds are flat"}},
	} {
		assert.Equal(t, tt.want, f.found(t, tt.text), "%q", tt.text)
	}

	hits := f.hits(t, "clay", "", f.phone)["knowledge"]
	require.Len(t, hits, 1)
	assert.Equal(t, "replaced", hits[0]["state"])

	web := f.signIn(t, "ada")
	f.mend(t, "garden", object{"ref": "knowledge:1", "revision": 1}, `{"why":"Loam."}`, web)
	assert.Equal(t, []string{}, f.found(t, "clay"), "its row in the index is replaced, not doubled")
	assert.Equal(t, []string{"Beds are raised"}, f.found(t, "loam"))
	assert.Equal(t, 2, f.count(t, "decisions_fts"))
}
