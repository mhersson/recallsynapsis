package tasks_test

import (
	"bytes"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mhersson/recallsynapsis/internal/module"
	"github.com/mhersson/recallsynapsis/internal/store"
	"github.com/mhersson/recallsynapsis/internal/tasks"
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

// found are the titles of the tasks' hits of text in every area, in their order.
func (f *fixture) found(t *testing.T, text, auth string) []string {
	t.Helper()

	return titlesOf(f.hits(t, text, "", auth)["tasks"])
}

func titlesOf(hits []object) []string {
	out := []string{}
	for _, h := range hits {
		out = append(out, h["title"].(string))
	}

	return out
}

// indexes reports whether a block of the search index holds the bytes of text.
func (f *fixture) indexes(t *testing.T, text string) bool {
	t.Helper()

	rows, err := f.store.QueryContext(t.Context(), `SELECT block FROM tasks_fts_data`)
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

func TestSearchFindsPartOfAWordInAnyCase(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	f.one(t, `{"title":"Smør garasjeporten","notes":"Hengslene før vinteren"}`)
	f.one(t, `{"title":"Male gjerdet","notes":"Port og gjerde i mai","area":"garden"}`)
	f.one(t, `{"title":"Øl til festen"}`)
	f.one(t, `{"title":"Reise til İstanbul"}`)

	for _, tt := range []struct {
		text string
		want []string
	}{
		{"port", []string{"Smør garasjeporten", "Male gjerdet"}},
		{"PORT", []string{"Smør garasjeporten", "Male gjerdet"}},
		{"po", []string{"Smør garasjeporten", "Male gjerdet"}},
		{"PO", []string{"Smør garasjeporten", "Male gjerdet"}},
		{"garasjepor", []string{"Smør garasjeporten"}},
		{"GARASJEPOR", []string{"Smør garasjeporten"}},
		{"hengslene garasje", []string{"Smør garasjeporten"}},
		{"port mai", []string{"Male gjerdet"}},
		{"port vinteren mai", []string{}},
		{"øl", []string{"Øl til festen"}},
		{"ØL", []string{"Øl til festen"}},
		{"İstanbul", []string{"Reise til İstanbul"}},
		{`"port" (mai*)`, []string{}},
		{"  ", []string{}},
	} {
		assert.ElementsMatch(t, tt.want, f.found(t, tt.text, f.phone), "%q", tt.text)
	}
}

func TestSearchFollowsEveryWrite(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	session := f.signIn(t)
	garage := f.one(t, `{"title":"Rydd garasjen","notes":"Tomater i drivhuset","area":"garden"}`)
	f.one(t, `{"title":"Kjøp tomater"}`)

	assert.ElementsMatch(t, []string{"Rydd garasjen", "Kjøp tomater"}, f.found(t, "tomater", f.phone))

	f.change(t, garage, `{"title":"Vask bilen","notes":"Bønner"}`, f.laptop)

	for _, tt := range []struct {
		text string
		want []string
	}{
		{"garasjen", []string{}},
		{"bilen", []string{"Vask bilen"}},
		{"tomater", []string{"Kjøp tomater"}},
		{"bønner", []string{"Vask bilen"}},
	} {
		assert.Equal(t, tt.want, f.found(t, tt.text, f.phone), "a changed task is found by its new text alone: %q", tt.text)
	}

	for _, tt := range []struct {
		text, area string
		want       []string
	}{
		{"bilen", "garden", []string{"Vask bilen"}},
		{"bilen", "personal", []string{}},
		{"va", "Garden", []string{"Vask bilen"}},
		{"kj", "personal", []string{"Kjøp tomater"}},
		{"kj", "garden", []string{}},
	} {
		assert.Equal(t, tt.want, titlesOf(f.hits(t, tt.text, tt.area, f.phone)["tasks"]),
			"a search within an area finds its tasks alone: %q in %s", tt.text, tt.area)
	}

	f.change(t, garage, `{"done":true}`, f.laptop)

	hits := f.hits(t, "bilen", "", f.phone)["tasks"]
	require.Len(t, hits, 1, "a done task is found")
	assert.Equal(t, "done", hits[0]["state"], "with its state")

	f.trash(t, garage, "/trash", f.laptop)
	assert.Equal(t, []string{}, f.found(t, "bilen", f.phone), "a trashed task is not found")
	assert.Equal(t, []string{}, f.found(t, "va", f.phone), "by a short word neither")

	f.trash(t, garage, "/restore", f.laptop)
	assert.Equal(t, []string{"Vask bilen"}, f.found(t, "bilen", f.phone), "and is found again once restored")

	secret := f.trash(t, f.one(t, `{"title":"Kode qxjqxj","notes":"zvkzvk"}`), "/trash", f.laptop)
	indexed := f.count(t, "tasks_fts")
	require.True(t, f.indexes(t, "qxj"), "the index holds the text of a trashed task")
	require.True(t, f.indexes(t, "zvk"))

	code, got := f.do(t, http.MethodDelete, path(secret), "", session)
	require.Equal(t, http.StatusNoContent, code, got)
	assert.Equal(t, indexed-1, f.count(t, "tasks_fts"), "a task deleted for good leaves the index")
	assert.False(t, f.indexes(t, "qxj"), "with nothing of its title left in it")
	assert.False(t, f.indexes(t, "zvk"), "nor of its notes")

	f.trash(t, garage, "/trash", f.laptop)
	f.trash(t, f.one(t, `{"title":"Siste wqjwqj"}`), "/trash", f.laptop)
	require.True(t, f.indexes(t, "wqj"))

	code, got = f.do(t, http.MethodDelete, "/api/tasks/trash", "", session)
	require.Equal(t, http.StatusNoContent, code, got)
	assert.Equal(t, indexed-2, f.count(t, "tasks_fts"), "and so does every task the trash held")
	assert.False(t, f.indexes(t, "wqj"), "with nothing of their text left in it")
	assert.Equal(t, []string{"Kjøp tomater"}, f.found(t, "tomater", f.phone))
}

func TestAHitShowsItsTask(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	code, got := f.do(t, http.MethodPost, "/api/notes", `{"title":"Hengslene","body":"Olje"}`, f.laptop)
	require.Equal(t, http.StatusCreated, code, got)

	loose := f.one(t, `{"title":"Hengslene"}`)
	garage := f.one(t, `{"title":"Smør porten","notes":"`+strings.Repeat("ord ", 60)+`hengslene`+strings.Repeat(" drivhus", 60)+`","area":"garden","list":"soon"}`)
	done := f.change(t, f.one(t, `{"title":"Olje hengslene","planned_from":"2026-11-28"}`), `{"done":true}`, f.laptop)

	groups := f.hits(t, "hengslene", "", f.phone)
	assert.Equal(t, []string{"Hengslene"}, titlesOf(groups["notes"]), "beside the notes' hits")
	require.Len(t, groups["tasks"], 3)

	byRef := map[any]object{}
	for _, h := range groups["tasks"] {
		byRef[h["ref"]] = h
	}

	assert.Equal(t, object{
		"ref": loose["ref"], "kind": "task", "state": "unsorted", "title": "Hengslene", "snippet": "", "area": "personal",
		"changed_at": loose["changed_at"],
	}, byRef[loose["ref"]])

	assert.Equal(t, object{"kind": "task", "state": "done", "title": "Olje hengslene", "area": "personal", "changed_at": done["changed_at"]},
		pick(byRef[done["ref"]], "kind", "state", "title", "area", "changed_at"))

	hit := byRef[garage["ref"]]
	assert.Equal(t, object{"kind": "task", "state": "open", "title": "Smør porten", "area": "garden"}, pick(hit, "kind", "state", "title", "area"))

	snippet := hit["snippet"].(string)
	assert.LessOrEqual(t, utf8.RuneCountInString(snippet), 122, snippet)

	words := strings.Fields(strings.TrimSuffix(strings.TrimPrefix(snippet, "…"), "…"))
	assert.Positive(t, slices.Index(words, "hengslene"), "the snippet is of the notes, around what was found")
	assert.Subset(t, []string{"ord", "hengslene", "drivhus"}, words, "of whole words")
}

// pick is the keys of o alone.
func pick(o object, keys ...string) object {
	out := object{}
	for _, k := range keys {
		out[k] = o[k]
	}

	return out
}

func TestSearchFindsTheCallersTasksAlone(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	f.one(t, `{"title":"Garasjeporten","area":"garden"}`)
	f.create(t, `{"tasks":[{"title":"Porten hennes"}]}`, f.grace)

	for _, text := range []string{"port", "po"} {
		assert.Equal(t, []string{"Garasjeporten"}, f.found(t, text, f.phone), text)
		assert.Equal(t, []string{"Porten hennes"}, f.found(t, text, f.grace), text)
	}

	code, got := f.do(t, http.MethodGet, "/api/search?q=port&area=garden", "", f.grace)
	refused(t, http.StatusNotFound, code, got)
}

func TestTasksMadeBeforeTheIndexAreFound(t *testing.T) {
	t.Parallel()

	f := newDatabase(t)
	first := tasks.New(module.Deps{}).Migrations[0]
	first.Owner = "tasks"
	require.NoError(t, f.store.Migrate(t.Context(), []store.Migration{first}))

	for _, task := range []struct {
		id           int
		title, notes string
		trashed      any
	}{
		{1, "Smør garasjeporten", "Hengslene", nil},
		{2, "Porten", "", 9},
	} {
		_, err := f.store.ExecContext(t.Context(), `INSERT INTO tasks (id, user_id, area_id, title, notes, planned_from, planned_to, planned_time,
			deadline, list, source, created_at, changed_at, trashed_at) VALUES (?, ?, ?, ?, ?, '', '', '', '', '', 'laptop', 1, 1, ?)`,
			task.id, f.ada, f.areas["garden"], task.title, task.notes, task.trashed)
		require.NoError(t, err)

		_, err = f.store.ExecContext(t.Context(), `INSERT INTO task_changes (task_id, at, source, said, changes) VALUES (?, 1, 'laptop', '', '[]')`, task.id)
		require.NoError(t, err)
	}

	f.load(t, object{"enabled": true})

	for _, text := range []string{"port", "hengslene", "po"} {
		assert.Equal(t, []string{"Smør garasjeporten"}, f.found(t, text, f.phone), text)
	}

	f.change(t, object{"id": 1}, `{"title":"Låsen"}`, f.laptop)
	assert.Equal(t, []string{}, f.found(t, "port", f.phone), "its row in the index is replaced, not doubled")
	assert.Equal(t, []string{"Låsen"}, f.found(t, "hengslene", f.phone))

	f.trash(t, object{"id": 2}, "/restore", f.laptop)
	assert.Equal(t, []string{"Porten"}, f.found(t, "port", f.phone), "a task in the trash at the upgrade is found once restored")
}
