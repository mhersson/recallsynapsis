package notes_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mhersson/recallsynapsis/internal/module"
	"github.com/mhersson/recallsynapsis/internal/notes"
	"github.com/mhersson/recallsynapsis/internal/store"
)

// hits are the hits of GET /api/search for text, within area unless it is
// empty, which must all be under notes.
func (f *fixture) hits(t *testing.T, text, area, auth string) []object {
	t.Helper()

	q := url.Values{"q": {text}}
	if area != "" {
		q.Set("area", area)
	}

	code, got := f.do(t, http.MethodGet, "/api/search?"+q.Encode(), "", auth)
	require.Equal(t, http.StatusOK, code, got)

	out := []object{}

	for _, g := range got["groups"].([]any) {
		require.Equal(t, "notes", g.(object)["module"])

		for _, h := range g.(object)["hits"].([]any) {
			out = append(out, h.(object))
		}
	}

	return out
}

// found are the titles of the hits of text in every area, in their order.
func (f *fixture) found(t *testing.T, text, auth string) []string {
	t.Helper()

	return titlesOf(f.hits(t, text, "", auth))
}

func titlesOf(hits []object) []string {
	out := []string{}
	for _, h := range hits {
		out = append(out, h["title"].(string))
	}

	return out
}

func TestSearchFindsPartOfAWordInAnyCase(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	f.create(t, `{"title":"Garasjeporten","body":"Smør hengslene før vinteren.","tags":["hus"]}`, f.laptop)
	f.create(t, `{"title":"Årsplan","body":"Port og gjerde males i mai.","area":"garden","tags":["hage"]}`, f.laptop)
	f.create(t, `{"body":"Øl til festen\nKjøp mer","kind":"idea","inbox":true}`, f.laptop)
	f.create(t, `{"title":"Bær","body":"Plukk blåbær i august.","tags":["hage"]}`, f.laptop)
	f.create(t, `{"title":"Reise","body":"Tur til İstanbul."}`, f.laptop)

	for _, tt := range []struct {
		text string
		want []string
	}{
		{"port", []string{"Garasjeporten", "Årsplan"}},
		{"GARASJEPORT", []string{"Garasjeporten"}},
		{"hengslene garasje", []string{"Garasjeporten"}},
		{"port mai", []string{"Årsplan"}},
		{"port hus", []string{"Garasjeporten"}},
		{"hage blåbær", []string{"Bær"}},
		{"port vinteren mai", []string{}},
		{"årsplan", []string{"Årsplan"}},
		{"ÅRSPLAN", []string{"Årsplan"}},
		{"BÆR", []string{"Bær"}},
		{"øl", []string{"Øl til festen"}},
		{"ØL", []string{"Øl til festen"}},
		{"år", []string{"Årsplan"}},
		{"Æ", []string{"Bær"}},
		{"ø", []string{"Garasjeporten", "Øl til festen"}},
		{"port og", []string{"Årsplan"}},
		{"øl port", []string{}},
		{"İstanbul", []string{"Reise"}},
		{"  ", []string{}},
	} {
		assert.ElementsMatch(t, tt.want, f.found(t, tt.text, f.phone), "%q", tt.text)
	}
}

func TestSearchTakesAnyTextAsText(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	f.create(t, `{"title":"Syntaks","body":"Han skrev \"OR NOT\" og (NEAR*) i title:feltet, med ^start og -minus."}`, f.laptop)
	f.create(t, `{"title":"Annet","body":"Ingenting her."}`, f.laptop)

	for _, tt := range []struct {
		text string
		want []string
	}{
		{`"OR NOT"`, []string{"Syntaks"}},
		{`OR NOT`, []string{"Syntaks"}},
		{`(NEAR*)`, []string{"Syntaks"}},
		{`title:feltet`, []string{"Syntaks"}},
		{`^start -minus`, []string{"Syntaks"}},
		{`"`, []string{"Syntaks"}},
		{`*`, []string{"Syntaks"}},
		{`NEAR(her ingenting)`, []string{}},
		{`title:Annet`, []string{}},
		{`""""`, []string{}},
		{`AND`, []string{}},
		{"OR\x00NOT", []string{"Syntaks"}},
	} {
		assert.Equal(t, tt.want, f.found(t, tt.text, f.phone), "%q", tt.text)
	}
}

func TestSearchFollowsEveryWrite(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	seeds := f.create(t, `{"title":"Frø","body":"Tomater i drivhuset","area":"garden","tags":["vår"]}`, f.laptop)
	f.create(t, `{"title":"Bed","body":"Tomater på kjøkkenet"}`, f.laptop)

	assert.ElementsMatch(t, []string{"Frø", "Bed"}, f.found(t, "tomater", f.phone))

	f.change(t, seeds, `{"revision":1,"body":"Bønner i drivhuset","tags":["sommer"]}`, f.laptop)

	for _, tt := range []struct {
		text string
		want []string
	}{
		{"tomater", []string{"Bed"}},
		{"bønner", []string{"Frø"}},
		{"vår", []string{}},
		{"sommer", []string{"Frø"}},
	} {
		assert.Equal(t, tt.want, f.found(t, tt.text, f.phone), "a changed note is found by its new text alone: %q", tt.text)
	}

	f.change(t, seeds, `{"append":"Og agurk"}`, f.laptop)
	assert.Equal(t, []string{"Frø"}, f.found(t, "agurk", f.phone))

	for _, tt := range []struct {
		text, area string
		want       []string
	}{
		{"i", "garden", []string{"Frø"}},
		{"i", "Garden", []string{"Frø"}},
		{"er", "garden", []string{"Frø"}},
		{"er", "personal", []string{"Bed"}},
		{"bed", "personal", []string{"Bed"}},
		{"drivhuset", "personal", []string{}},
		{"kjøkkenet", "garden", []string{}},
	} {
		assert.Equal(t, tt.want, titlesOf(f.hits(t, tt.text, tt.area, f.phone)), "a search within an area finds its notes alone: %q in %s", tt.text, tt.area)
	}

	f.trash(t, seeds, "/trash", f.laptop)
	assert.Equal(t, []string{}, f.found(t, "bønner", f.phone), "a trashed note is not found")
	assert.Equal(t, []string{}, f.found(t, "fr", f.phone), "by a short word neither")

	f.trash(t, seeds, "/restore", f.laptop)
	assert.Equal(t, []string{"Frø"}, f.found(t, "bønner", f.phone), "and is found again once restored")

	code, got := f.do(t, http.MethodPost, path(seeds)+"/revisions/1/restore", "", f.laptop)
	require.Equal(t, http.StatusOK, code, got)
	assert.ElementsMatch(t, []string{"Frø", "Bed"}, f.found(t, "tomater", f.phone), "a restored revision is found by its text")
	assert.Equal(t, []string{"Frø"}, f.found(t, "vår", f.phone))
	assert.Equal(t, []string{}, f.found(t, "agurk", f.phone))

	session := f.signIn(t)
	secret := f.trash(t, f.create(t, `{"title":"Kode","body":"qxjqxj"}`, f.laptop), "/trash", f.laptop)
	indexed := f.count(t, "notes_fts")
	require.True(t, f.indexes(t, "qxj"), "the index holds the text of a trashed note")

	code, got = f.do(t, http.MethodDelete, path(secret), "", session)
	require.Equal(t, http.StatusNoContent, code, got)
	assert.Equal(t, indexed-1, f.count(t, "notes_fts"), "a note deleted for good leaves the index")
	assert.False(t, f.indexes(t, "qxj"), "with nothing of its text left in it")

	f.trash(t, seeds, "/trash", f.laptop)
	f.trash(t, f.create(t, `{"title":"Last","body":"zvkzvk"}`, f.laptop), "/trash", f.laptop)
	require.True(t, f.indexes(t, "zvk"))

	code, got = f.do(t, http.MethodDelete, "/api/notes/trash", "", session)
	require.Equal(t, http.StatusNoContent, code, got)
	assert.Equal(t, indexed-2, f.count(t, "notes_fts"), "and so does every note the trash held")
	assert.False(t, f.indexes(t, "zvk"), "with nothing of their text left in it")
}

// indexes reports whether a block of the search index holds the bytes of text.
func (f *fixture) indexes(t *testing.T, text string) bool {
	t.Helper()

	rows, err := f.store.QueryContext(t.Context(), `SELECT block FROM notes_fts_data`)
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

func TestAHitShowsItsNote(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	capture := f.create(t, `{"body":"  \nØl til festen på lørdag\nKjøp mer","kind":"idea","inbox":true}`, f.laptop)
	long := f.create(t, `{"body":"`+strings.Repeat("ø", 100)+` fest"}`, f.laptop)
	filed := f.create(t, `{"title":"Festen","body":"`+strings.Repeat("ord ", 60)+`festen`+strings.Repeat(" drivhus", 60)+`","area":"garden"}`, f.laptop)

	hits := f.hits(t, "fest", "", f.phone)
	require.Len(t, hits, 3)

	byRef := map[any]object{}
	for _, h := range hits {
		byRef[h["ref"]] = h
	}

	assert.Equal(t, object{
		"ref": capture["ref"], "kind": "idea", "state": "inbox", "title": "Øl til festen på lørdag",
		"snippet": "Øl til festen på lørdag Kjøp mer", "area": "personal", "changed_at": capture["changed_at"],
	}, byRef[capture["ref"]], "a capture with no title goes by the first line of its body")

	assert.Equal(t, strings.Repeat("ø", 80), byRef[long["ref"]]["title"], "cut at 80 characters")

	hit := byRef[filed["ref"]]
	assert.Equal(t, object{"kind": "note", "state": "filed", "title": "Festen", "area": "garden"},
		object{"kind": hit["kind"], "state": hit["state"], "title": hit["title"], "area": hit["area"]})

	snippet := hit["snippet"].(string)
	assert.LessOrEqual(t, utf8.RuneCountInString(snippet), 122, snippet)

	words := strings.Fields(strings.TrimSuffix(strings.TrimPrefix(snippet, "…"), "…"))
	assert.Positive(t, slices.Index(words, "festen"), "the snippet is around what was found")
	assert.Subset(t, []string{"ord", "festen", "drivhus"}, words, "of whole words")
	assert.True(t, strings.HasPrefix(snippet, "…") && strings.HasSuffix(snippet, "…"), snippet)

	cs := f.client(t, f.phone)

	res, err := cs.CallTool(t.Context(), &mcp.CallToolParams{Name: "search", Arguments: object{"query": "fest", "area": "garden"}})
	require.NoError(t, err)
	require.False(t, res.IsError, res.Content)

	b, err := json.Marshal(res.StructuredContent)
	require.NoError(t, err)

	var answer object
	require.NoError(t, json.Unmarshal(b, &answer))
	assert.Equal(t, object{"groups": []any{object{"module": "notes", "hits": []any{hit}}}}, answer, "the tool gives the hits REST gives")
}

func TestSearchRanksAndGivesAtMostFifty(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	langt := f.create(t, `{"title":"Langt","body":"`+strings.Repeat("ord ", 300)+`port"}`, f.laptop)
	port := f.create(t, `{"title":"Port","body":"port, port og port"}`, f.laptop)

	assert.Equal(t, []string{"Port", "Langt"}, f.found(t, "port", f.phone), "the note that is most about the word first")
	assert.Equal(t, []string{"Port", "Langt"}, f.found(t, "po", f.phone), "without the index, the most recently changed first")

	f.change(t, port, `{"append":"Og mer"}`, f.laptop)
	f.change(t, langt, `{"append":"Og mer"}`, f.laptop)
	assert.Equal(t, []string{"Langt", "Port"}, f.found(t, "po", f.phone), "by when it changed, not when it was made")

	for i := range 50 {
		f.create(t, fmt.Sprintf(`{"title":"Porten %d"}`, i), f.laptop)
	}

	for _, text := range []string{"port", "po", "port o"} {
		assert.Len(t, f.found(t, text, f.phone), 50, text)
	}

	assert.Equal(t, []string{"Langt"}, f.found(t, "port ng", f.phone), "a short word is looked for in every note the index finds, not the first 50")
}

func TestSearchFindsTheCallersNotesAlone(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	f.create(t, `{"title":"Garasjeporten","area":"garden"}`, f.laptop)
	f.create(t, `{"title":"Porten hennes"}`, f.grace)

	for _, text := range []string{"port", "po"} {
		assert.Equal(t, []string{"Garasjeporten"}, f.found(t, text, f.phone), text)
		assert.Equal(t, []string{"Porten hennes"}, f.found(t, text, f.grace), text)
	}

	code, got := f.do(t, http.MethodGet, "/api/search?q=port&area=garden", "", f.grace)
	refused(t, http.StatusNotFound, code, got)
}

func TestNotesMadeBeforeTheIndexAreFound(t *testing.T) {
	t.Parallel()

	f := newDatabase(t)
	first := notes.New(module.Deps{}).Migrations[0]
	first.Owner = "notes"
	require.NoError(t, f.store.Migrate(t.Context(), []store.Migration{first}))

	for _, n := range []struct {
		id          int
		title, body string
		trashed     any
	}{
		{1, "Garasjeporten", "Smør hengslene", nil},
		{2, "Porten", "", 9},
	} {
		_, err := f.store.ExecContext(t.Context(), `INSERT INTO notes (id, user_id, area_id, kind, title, body, inbox, revision, source, created_at, changed_at, trashed_at)
			VALUES (?, ?, ?, 'note', ?, ?, 0, 1, 'laptop', 1, 1, ?)`, n.id, f.ada, f.areas["garden"], n.title, n.body, n.trashed)
		require.NoError(t, err)
	}

	_, err := f.store.ExecContext(t.Context(), `INSERT INTO note_tags (note_id, tag) VALUES (1, 'hus')`)
	require.NoError(t, err)

	f.load(t, object{"enabled": true})

	for _, text := range []string{"port", "hengslene", "hus", "po"} {
		assert.Equal(t, []string{"Garasjeporten"}, f.found(t, text, f.phone), text)
	}

	f.change(t, object{"id": 1}, `{"revision":1,"title":"Låsen"}`, f.laptop)
	assert.Equal(t, []string{}, f.found(t, "port", f.phone), "its row in the index is replaced, not doubled")
	assert.Equal(t, []string{"Låsen"}, f.found(t, "låsen", f.phone))

	f.trash(t, object{"id": 2}, "/restore", f.laptop)
	assert.Equal(t, []string{"Porten"}, f.found(t, "port", f.phone), "a note in the trash at the upgrade is found once restored")
}

// client connects the SDK's own client to /mcp with the Authorization header auth.
func (f *fixture) client(t *testing.T, auth string) *mcp.ClientSession {
	t.Helper()

	srv := httptest.NewServer(f.h)
	t.Cleanup(srv.Close)

	cs, err := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "1"}, nil).Connect(t.Context(), &mcp.StreamableClientTransport{
		Endpoint: srv.URL + "/mcp", HTTPClient: &http.Client{Transport: bearer(auth)}, MaxRetries: -1,
	}, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = cs.Close() })

	return cs
}

// bearer sends every request with itself as its Authorization header.
type bearer string

func (b bearer) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())
	r.Header.Set("Authorization", string(b))

	return http.DefaultTransport.RoundTrip(r)
}
