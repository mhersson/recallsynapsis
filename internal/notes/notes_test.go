package notes_test

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mhersson/recallsynapsis/internal/api"
	"github.com/mhersson/recallsynapsis/internal/auth"
	"github.com/mhersson/recallsynapsis/internal/config"
	"github.com/mhersson/recallsynapsis/internal/core"
	"github.com/mhersson/recallsynapsis/internal/module"
	"github.com/mhersson/recallsynapsis/internal/notes"
	"github.com/mhersson/recallsynapsis/internal/store"
)

type object = map[string]any

// fixture serves a database whose user ada has her personal area and the
// project Garden, and whose user grace has her personal area alone. Each
// user is reached by the Authorization header of a token, never a session,
// so no test spends an argon2 run.
type fixture struct {
	h       http.Handler
	store   *store.Store
	auth    *auth.Service
	core    *core.Service
	hub     *api.Hub
	tick    atomic.Int64
	ada     int64
	laptop  string // ada's write token
	desk    string // another write token of ada's
	phone   string // ada's read token
	grace   string // grace's write token
	areas   map[string]int64
	tempDir string
}

func newFixture(t *testing.T) *fixture {
	t.Helper()

	st, err := store.Open(t.Context(), filepath.Join(t.TempDir(), "recallsynapsis.db"), slices.Concat(auth.Migrations, core.Migrations))
	require.NoError(t, err)

	t.Cleanup(func() { _ = st.Close() })

	f := &fixture{store: st, core: core.New(st), hub: api.NewHub(t.Context().Done()), areas: map[string]int64{}, tempDir: t.TempDir()}
	f.auth = auth.New(st, time.Now)

	// Not auth's EnsureUser, which keeps one user and renames them.
	user := func(name string) int64 {
		var id int64
		require.NoError(t, st.QueryRowContext(t.Context(), `INSERT INTO users (name, created_at) VALUES (?, 0) RETURNING id`, name).Scan(&id))
		require.NoError(t, f.core.EnsurePersonal(t.Context(), id))

		return id
	}
	token := func(user int64, name, access string) string {
		_, text, err := f.auth.CreateToken(t.Context(), user, name, access)
		require.NoError(t, err)

		return "Bearer " + text
	}

	f.ada = user("ada")
	grace := user("grace")
	f.laptop, f.desk, f.phone, f.grace = token(f.ada, "laptop", "write"), token(f.ada, "desk", "write"), token(f.ada, "phone", "read"),
		token(grace, "tablet", "write")

	garden, err := f.core.CreateProject(t.Context(), f.ada, "Garden", "")
	require.NoError(t, err)

	personal, err := f.core.Find(t.Context(), f.ada, "personal")
	require.NoError(t, err)

	f.areas["garden"], f.areas["personal"] = garden.ID, personal.ID

	f.load(t, object{"enabled": true})

	return f
}

// load serves the database again, with table as [modules.notes], or none when nil.
func (f *fixture) load(t *testing.T, table object) {
	t.Helper()

	tables := map[string]map[string]any{}
	if table != nil {
		tables["notes"] = table
	}

	// Each reading of the clock is a second on from the last, so no two writes share a time.
	now := func() time.Time { return time.Unix(1_000_000+f.tick.Add(1), 0) }

	ms, err := f.core.LoadModules(t.Context(), []func(module.Deps) module.Module{notes.New}, &config.Config{Modules: tables},
		module.Deps{Now: now, Dir: f.tempDir, Changed: f.hub.Changed})
	require.NoError(t, err)

	f.h = api.New(api.Deps{Auth: f.auth, Core: f.core, Modules: ms, PublicURL: "https://recall.example.org", Hub: f.hub})
}

// do serves one request with the Authorization header auth and decodes its answer.
func (f *fixture) do(t *testing.T, method, target, body, auth string) (int, object) {
	t.Helper()

	req := httptest.NewRequestWithContext(t.Context(), method, target, strings.NewReader(body))
	req.Header.Set("Authorization", auth)

	rec := httptest.NewRecorder()
	f.h.ServeHTTP(rec, req)
	assert.Equal(t, "application/json", rec.Header().Get("Content-Type"))

	var got object
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got), rec.Body.String())

	return rec.Code, got
}

// create makes a note of the body JSON with auth and returns it.
func (f *fixture) create(t *testing.T, body, auth string) object {
	t.Helper()

	code, got := f.do(t, http.MethodPost, "/api/notes", body, auth)
	require.Equal(t, http.StatusCreated, code, got)

	return got
}

// change sends the change body to note and returns the note it answers.
func (f *fixture) change(t *testing.T, note object, body, auth string) object {
	t.Helper()

	code, got := f.do(t, http.MethodPatch, path(note), body, auth)
	require.Equal(t, http.StatusOK, code, got)

	return got
}

// titles are the titles of the notes a list answers, in its order, and its total.
func (f *fixture) titles(t *testing.T, query, auth string) ([]string, int) {
	t.Helper()

	code, got := f.do(t, http.MethodGet, "/api/notes"+query, "", auth)
	require.Equal(t, http.StatusOK, code, got)

	out := []string{}
	for _, n := range got["notes"].([]any) {
		out = append(out, n.(object)["title"].(string))
	}

	return out, int(got["total"].(float64))
}

func path(note object) string {
	return fmt.Sprintf("/api/notes/%v", note["id"])
}

// refused wants a refusal with status, in the one error shape.
func refused(t *testing.T, status, code int, got object) {
	t.Helper()

	assert.Equal(t, status, code, got)
	assert.Len(t, got, 1)
	assert.NotEmpty(t, got["error"])
}

func (f *fixture) count(t *testing.T, table string) int {
	t.Helper()

	var n int
	require.NoError(t, f.store.QueryRowContext(t.Context(), `SELECT count(*) FROM `+table).Scan(&n))

	return n
}

func TestANoteReadsBackAsItWasMade(t *testing.T) {
	t.Parallel()

	f := newFixture(t)

	made := f.create(t, `{"title":"Seeds","body":"Tomatoes and beans","area":"Garden","tags":["#Spring","bær"]}`, f.laptop)
	assert.Equal(t, "notes:"+fmt.Sprint(made["id"]), made["ref"])
	assert.Equal(t, "garden", made["area"])
	assert.Equal(t, "note", made["kind"])
	assert.Equal(t, "Seeds", made["title"])
	assert.Equal(t, "Tomatoes and beans", made["body"])
	assert.Equal(t, "Tomatoes and beans", made["excerpt"])
	assert.Equal(t, false, made["inbox"])
	assert.Equal(t, []any{"bær", "spring"}, made["tags"])
	assert.InDelta(t, 1, made["revision"], 0)
	assert.Equal(t, "laptop", made["source"])
	assert.Equal(t, made["created_at"], made["changed_at"])

	at, err := time.Parse(time.RFC3339, made["created_at"].(string))
	require.NoError(t, err)
	assert.Equal(t, time.UTC, at.Location())

	code, got := f.do(t, http.MethodGet, path(made), "", f.phone)
	require.Equal(t, http.StatusOK, code)
	assert.Equal(t, made, got)

	var (
		revision     int
		source, tags string
	)
	require.NoError(t, f.store.QueryRowContext(t.Context(), `SELECT revision, source, tags FROM note_revisions WHERE note_id = ?`,
		made["id"]).Scan(&revision, &source, &tags))
	assert.Equal(t, 1, revision)
	assert.Equal(t, "laptop", source)
	assert.JSONEq(t, `["bær","spring"]`, tags)

	plain := f.create(t, `{"body":"`+strings.Repeat("ø", 250)+`"}`, f.laptop)
	assert.Equal(t, "personal", plain["area"], "the personal area unless one is named")
	assert.Equal(t, "note", plain["kind"])
	assert.Empty(t, plain["title"])
	assert.Equal(t, []any{}, plain["tags"])
	assert.Equal(t, strings.Repeat("ø", 200), plain["excerpt"], "the first 200 characters of the body")
}

func TestANoteMustBeOneTheModuleCanKeep(t *testing.T) {
	t.Parallel()

	f := newFixture(t)

	for _, tt := range []struct {
		body   string
		status int
	}{
		{`{"title":"  ","body":" \n "}`, http.StatusBadRequest},
		{`{"title":"` + strings.Repeat("å", 201) + `"}`, http.StatusBadRequest},
		{`{"title":"Two\nlines"}`, http.StatusBadRequest},
		{`{"title":"Seeds","kind":"task"}`, http.StatusBadRequest},
		{`{"title":"Seeds","said":"keep this"}`, http.StatusBadRequest},
		{`{"title":"Seeds"} {}`, http.StatusBadRequest},
		{`{"title":"Seeds","area":"nowhere"}`, http.StatusNotFound},
	} {
		code, got := f.do(t, http.MethodPost, "/api/notes", tt.body, f.laptop)
		refused(t, tt.status, code, got)
	}

	assert.Equal(t, 0, f.count(t, "notes"), "a refused note leaves nothing behind")
	assert.Equal(t, 0, f.count(t, "note_revisions"))

	code, got := f.do(t, http.MethodPost, "/api/notes", `{"title":"Seeds"}`, f.phone)
	refused(t, http.StatusForbidden, code, got)

	trimmed := f.create(t, `{"title":"  `+strings.Repeat("å", 200)+`  ","kind":"idea"}`, f.laptop)
	assert.Equal(t, strings.Repeat("å", 200), trimmed["title"])
	assert.Equal(t, "idea", trimmed["kind"])
}

func TestAModuleSwitchedOffKeepsItsNotes(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	made := []object{
		f.create(t, `{"title":"Seeds","area":"garden","tags":["spring"]}`, f.laptop),
		f.change(t, f.create(t, `{"body":"Call the plumber","inbox":true}`, f.desk), `{"revision":1,"title":"Plumber"}`, f.laptop),
	}

	for _, table := range []object{{"enabled": false}, nil} {
		f.load(t, table)

		for _, r := range []struct{ method, target, body string }{
			{http.MethodGet, "/api/notes", ""},
			{http.MethodPost, "/api/notes", `{"title":"Lost"}`},
			{http.MethodGet, "/api/notes/tags", ""},
			{http.MethodGet, path(made[0]), ""},
			{http.MethodPatch, path(made[0]), `{"revision":1,"title":"Lost"}`},
		} {
			code, got := f.do(t, r.method, r.target, r.body, f.laptop)
			refused(t, http.StatusNotFound, code, got)
		}

		assert.Equal(t, 2, f.count(t, "notes"))
		assert.Equal(t, 1, f.count(t, "note_tags"))
		assert.Equal(t, 3, f.count(t, "note_revisions"))
	}

	f.load(t, object{"enabled": true})

	for _, note := range made {
		code, got := f.do(t, http.MethodGet, path(note), "", f.laptop)
		require.Equal(t, http.StatusOK, code)
		assert.Equal(t, note, got)
	}
}

func TestAChangeIsMadeFromTheNotesRevision(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	made := f.create(t, `{"title":"Seeds","body":"Tomatoes"}`, f.laptop)

	changed := f.change(t, made, `{"revision":1,"title":"Seedlings"}`, f.desk)
	assert.InDelta(t, 2, changed["revision"], 0)
	assert.Equal(t, "Seedlings", changed["title"])
	assert.Equal(t, "Tomatoes", changed["body"])
	assert.Equal(t, "desk", changed["source"])
	assert.Equal(t, made["created_at"], changed["created_at"])
	assert.NotEqual(t, made["changed_at"], changed["changed_at"])

	for _, tt := range []struct {
		body   string
		status int
	}{
		{`{"revision":1,"body":"Beans"}`, http.StatusConflict},
		{`{"revision":3,"body":"Beans"}`, http.StatusConflict},
		{`{"revision":2,"body":"Beans","append":"Peas"}`, http.StatusBadRequest},
	} {
		code, got := f.do(t, http.MethodPatch, path(made), tt.body, f.laptop)
		refused(t, tt.status, code, got)
	}

	code, got := f.do(t, http.MethodGet, path(made), "", f.laptop)
	require.Equal(t, http.StatusOK, code)
	assert.Equal(t, changed, got, "a refused change changes nothing")

	appended := f.change(t, made, `{"append":"Beans"}`, f.laptop)
	assert.Equal(t, "Tomatoes\n\nBeans", appended["body"])
	assert.InDelta(t, 3, appended["revision"], 0)

	appended = f.change(t, made, `{"revision":1,"append":"Peas"}`, f.desk)
	assert.Equal(t, "Tomatoes\n\nBeans\n\nPeas", appended["body"], "one that only appends lands whatever the revision")
	assert.InDelta(t, 4, appended["revision"], 0)

	empty := f.create(t, `{"title":"Empty"}`, f.laptop)
	assert.Equal(t, "Beans", f.change(t, empty, `{"append":"Beans"}`, f.laptop)["body"])

	rows, err := f.store.QueryContext(t.Context(), `SELECT revision, source, title, body FROM note_revisions WHERE note_id = ? ORDER BY revision`, made["id"])
	require.NoError(t, err)

	defer rows.Close()

	var history []string

	for rows.Next() {
		var (
			revision            int
			source, title, body string
		)

		require.NoError(t, rows.Scan(&revision, &source, &title, &body))
		history = append(history, fmt.Sprintf("%d %s %s %q", revision, source, title, body))
	}

	require.NoError(t, rows.Err())
	assert.Equal(t, []string{
		`1 laptop Seeds "Tomatoes"`,
		`2 desk Seedlings "Tomatoes"`,
		`3 laptop Seedlings "Tomatoes\n\nBeans"`,
		`4 desk Seedlings "Tomatoes\n\nBeans\n\nPeas"`,
	}, history, "each change writes a revision with its source")
}

func TestAChangeTheModuleCannotMakeIsRefused(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	made := f.create(t, `{"title":"Seeds"}`, f.laptop)

	for _, tt := range []struct {
		body   string
		status int
	}{
		{`{"title":"Seedlings"}`, http.StatusBadRequest},
		{`{"revision":1}`, http.StatusBadRequest},
		{`{"append":""}`, http.StatusBadRequest},
		{`{"revision":1,"title":""}`, http.StatusBadRequest},
		{`{"revision":1,"kind":"task"}`, http.StatusBadRequest},
		{`{"revision":1,"tags":["two words"]}`, http.StatusBadRequest},
		{`{"revision":1,"area":"nowhere"}`, http.StatusNotFound},
	} {
		code, got := f.do(t, http.MethodPatch, path(made), tt.body, f.laptop)
		refused(t, tt.status, code, got)
	}

	code, got := f.do(t, http.MethodGet, path(made), "", f.laptop)
	require.Equal(t, http.StatusOK, code)
	assert.Equal(t, made, got)
	assert.Equal(t, 1, f.count(t, "note_revisions"))

	for _, target := range []string{"/api/notes/9999", "/api/notes/seeds"} {
		code, got := f.do(t, http.MethodPatch, target, `{"revision":1,"title":"Lost"}`, f.laptop)
		refused(t, http.StatusNotFound, code, got)

		code, got = f.do(t, http.MethodGet, target, "", f.laptop)
		refused(t, http.StatusNotFound, code, got)
	}

	code, got = f.do(t, http.MethodPatch, path(made), `{"revision":1,"title":"Lost"}`, f.phone)
	refused(t, http.StatusForbidden, code, got)
}

func TestFiling(t *testing.T) {
	t.Parallel()

	f := newFixture(t)

	capture := f.create(t, `{"body":"Call the plumber","area":"garden","inbox":true}`, f.laptop)
	assert.Equal(t, true, capture["inbox"])
	assert.Equal(t, "personal", capture["area"], "a note in the inbox is in the personal area")

	for _, change := range []string{`{"revision":1,"title":"Plumber"}`, `{"revision":2,"body":"Call the plumber today"}`, `{"append":"Or tomorrow"}`} {
		capture = f.change(t, capture, change, f.laptop)
		assert.Equal(t, true, capture["inbox"], "a change of the title or the body files nothing: %s", change)
		assert.Equal(t, "personal", capture["area"], change)
	}

	for _, tt := range []struct{ change, area, kind string }{
		{`"area":"garden"`, "garden", "note"},
		{`"area":"personal"`, "personal", "note"},
		{`"kind":"idea"`, "personal", "idea"},
		{`"tags":["home"]`, "personal", "note"},
		{`"inbox":false`, "personal", "note"},
	} {
		capture := f.create(t, `{"body":"Call the plumber","inbox":true}`, f.laptop)
		filed := f.change(t, capture, `{"revision":1,`+tt.change+`}`, f.laptop)
		assert.Equal(t, false, filed["inbox"], tt.change)
		assert.Equal(t, tt.area, filed["area"], tt.change)
		assert.Equal(t, tt.kind, filed["kind"], tt.change)
	}

	filed := f.create(t, `{"body":"Prune the apple tree","area":"garden"}`, f.laptop)

	back := f.change(t, filed, `{"revision":1,"inbox":true}`, f.laptop)
	assert.Equal(t, true, back["inbox"])
	assert.Equal(t, "personal", back["area"], "a note put in the inbox moves to the personal area")

	again := f.change(t, back, `{"revision":2,"area":"garden","kind":"idea","inbox":true}`, f.laptop)
	assert.Equal(t, true, again["inbox"], "unless the same change puts it there")
	assert.Equal(t, "personal", again["area"])
	assert.Equal(t, "idea", again["kind"])
}

func TestTheListFilters(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	f.create(t, `{"title":"A","inbox":true}`, f.laptop)
	b := f.create(t, `{"title":"B","area":"garden","kind":"idea","tags":["seeds"]}`, f.laptop)
	f.create(t, `{"title":"C","area":"garden","tags":["seeds","spring"]}`, f.laptop)
	f.create(t, `{"title":"D","kind":"idea","tags":["spring"]}`, f.laptop)

	for _, tt := range []struct {
		query string
		want  []string
	}{
		{"", []string{"D", "C", "B", "A"}},
		{"?area=garden", []string{"C", "B"}},
		{"?area=GARDEN", []string{"C", "B"}},
		{"?area=Personal", []string{"D", "A"}},
		{"?kind=idea", []string{"D", "B"}},
		{"?kind=note", []string{"C", "A"}},
		{"?inbox=true", []string{"A"}},
		{"?inbox=false", []string{"D", "C", "B"}},
		{"?tag=seeds", []string{"C", "B"}},
		{"?tag=%23Spring", []string{"D", "C"}},
		{"?tag=nothing", []string{}},
		{"?area=garden&kind=note&tag=spring", []string{"C"}},
		{"?inbox=false&kind=idea&tag=spring", []string{"D"}},
		{"?area=personal&inbox=true&kind=idea", []string{}},
	} {
		got, total := f.titles(t, tt.query, f.phone)
		assert.Equal(t, tt.want, got, tt.query)
		assert.Equal(t, len(tt.want), total, tt.query)
	}

	got, total := f.titles(t, "?limit=2", f.phone)
	assert.Equal(t, []string{"D", "C"}, got)
	assert.Equal(t, 4, total, "the total counts every note asked for, whatever the limit")

	got, total = f.titles(t, "?area=garden&limit=1", f.phone)
	assert.Equal(t, []string{"C"}, got)
	assert.Equal(t, 2, total)

	got, total = f.titles(t, "?limit=4", f.phone)
	assert.Len(t, got, 4)
	assert.Equal(t, 4, total)

	f.change(t, b, `{"revision":1,"body":"Sow in March"}`, f.laptop)

	got, _ = f.titles(t, "?limit=1", f.phone)
	assert.Equal(t, []string{"B"}, got, "the most recently changed first")

	for _, tt := range []struct {
		query  string
		status int
	}{
		{"?kind=task", http.StatusBadRequest},
		{"?inbox=yes", http.StatusBadRequest},
		{"?limit=0", http.StatusBadRequest},
		{"?limit=-1", http.StatusBadRequest},
		{"?limit=501", http.StatusBadRequest},
		{"?limit=many", http.StatusBadRequest},
		{"?tag=two%20words", http.StatusBadRequest},
		{"?area=nowhere", http.StatusNotFound},
	} {
		code, got := f.do(t, http.MethodGet, "/api/notes"+tt.query, "", f.phone)
		refused(t, tt.status, code, got)
	}
}

func TestTheListGivesAtMostFiftyUnlessAsked(t *testing.T) {
	t.Parallel()

	f := newFixture(t)

	for i := range 51 {
		f.create(t, fmt.Sprintf(`{"title":"%d"}`, i), f.laptop)
	}

	got, total := f.titles(t, "", f.phone)
	assert.Len(t, got, 50)
	assert.Equal(t, 51, total)

	got, total = f.titles(t, "?limit=500", f.phone)
	assert.Len(t, got, 51)
	assert.Equal(t, 51, total)
}

func TestTags(t *testing.T) {
	t.Parallel()

	f := newFixture(t)

	made := f.create(t, `{"title":"Berries","tags":["#Garden","GARDEN","BÆR","##Plums"]}`, f.laptop)
	assert.Equal(t, []any{"bær", "garden", "plums"}, made["tags"], "lower-cased, the leading # dropped, each once")

	f.create(t, `{"title":"Beds","tags":["garden"]}`, f.laptop)
	f.create(t, `{"title":"Theirs","tags":["garden","orchard"]}`, f.grace)

	code, got := f.do(t, http.MethodGet, "/api/notes/tags", "", f.phone)
	require.Equal(t, http.StatusOK, code)
	assert.Equal(t, object{"tags": []any{
		object{"tag": "bær", "count": float64(1)},
		object{"tag": "garden", "count": float64(2)},
		object{"tag": "plums", "count": float64(1)},
	}}, got)

	for _, tag := range got["tags"].([]any) {
		listed, _ := f.titles(t, "?tag="+url.QueryEscape(tag.(object)["tag"].(string)), f.phone)
		assert.Len(t, listed, int(tag.(object)["count"].(float64)), "a tag as the list of tags gives it finds its notes")
	}

	for _, tag := range []string{"two words", "tab\there", "#", "##", "", strings.Repeat("a", 41)} {
		body, err := json.Marshal(object{"title": "Lost", "tags": []string{tag}})
		require.NoError(t, err)

		code, got := f.do(t, http.MethodPost, "/api/notes", string(body), f.laptop)
		refused(t, http.StatusBadRequest, code, got)
	}

	long := f.create(t, `{"title":"Long","tags":["#`+strings.Repeat("æ", 40)+`"]}`, f.laptop)
	assert.Equal(t, []any{strings.Repeat("æ", 40)}, long["tags"])

	retagged := f.change(t, made, `{"revision":1,"tags":["Pears"]}`, f.laptop)
	assert.Equal(t, []any{"pears"}, retagged["tags"], "a change of tags replaces them")

	cleared := f.change(t, retagged, `{"revision":2,"tags":[]}`, f.laptop)
	assert.Equal(t, []any{}, cleared["tags"])

	code, got = f.do(t, http.MethodGet, "/api/notes/tags", "", f.grace)
	require.Equal(t, http.StatusOK, code)
	assert.Equal(t, object{"tags": []any{
		object{"tag": "garden", "count": float64(1)},
		object{"tag": "orchard", "count": float64(1)},
	}}, got, "each user counts their own")
}

func TestANoteOfOneUserIsFoundByNoRouteOfAnother(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	mine := f.create(t, `{"title":"Seeds","area":"garden","tags":["spring"]}`, f.laptop)

	got, total := f.titles(t, "", f.grace)
	assert.Equal(t, []string{}, got)
	assert.Zero(t, total, "a total counts the caller's notes alone")

	got, _ = f.titles(t, "?tag=spring", f.grace)
	assert.Equal(t, []string{}, got)

	code, answer := f.do(t, http.MethodGet, "/api/notes?area=garden", "", f.grace)
	refused(t, http.StatusNotFound, code, answer)

	code, answer = f.do(t, http.MethodGet, "/api/notes/tags", "", f.grace)
	require.Equal(t, http.StatusOK, code)
	assert.Equal(t, object{"tags": []any{}}, answer)

	code, answer = f.do(t, http.MethodGet, path(mine), "", f.grace)
	refused(t, http.StatusNotFound, code, answer)

	for _, body := range []string{`{"revision":1,"title":"Mine now"}`, `{"append":"Mine now"}`} {
		code, answer = f.do(t, http.MethodPatch, path(mine), body, f.grace)
		refused(t, http.StatusNotFound, code, answer)
	}

	code, answer = f.do(t, http.MethodPost, "/api/notes", `{"title":"Lost","area":"garden"}`, f.grace)
	refused(t, http.StatusNotFound, code, answer)

	theirs := f.create(t, `{"title":"Orchard"}`, f.grace)
	assert.Equal(t, "personal", theirs["area"])
	assert.Equal(t, "tablet", theirs["source"])

	code, answer = f.do(t, http.MethodPatch, path(theirs), `{"revision":1,"area":"garden"}`, f.grace)
	refused(t, http.StatusNotFound, code, answer)

	code, answer = f.do(t, http.MethodGet, path(theirs), "", f.grace)
	require.Equal(t, http.StatusOK, code)
	assert.Equal(t, theirs, answer, "a note stays out of another user's area")

	code, answer = f.do(t, http.MethodGet, path(mine), "", f.laptop)
	require.Equal(t, http.StatusOK, code)
	assert.Equal(t, mine, answer, "her tries changed nothing")

	got, total = f.titles(t, "", f.laptop)
	assert.Equal(t, []string{"Seeds"}, got)
	assert.Equal(t, 1, total)

	code, answer = f.do(t, http.MethodGet, path(theirs), "", f.laptop)
	refused(t, http.StatusNotFound, code, answer)
}

func TestEachWriteReachesAnOpenStream(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	srv := httptest.NewServer(f.h)
	t.Cleanup(srv.Close)

	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, srv.URL+"/api/events", nil)
	require.NoError(t, err)
	req.Header.Set("Authorization", f.phone)

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	t.Cleanup(func() { _ = resp.Body.Close() })
	require.Equal(t, http.StatusOK, resp.StatusCode)

	// Each line but blank and comment lines, until the stream ends.
	lines := make(chan string)

	go func() {
		defer close(lines)

		sc := bufio.NewScanner(resp.Body)
		for sc.Scan() {
			if sc.Text() == "" || strings.HasPrefix(sc.Text(), ":") {
				continue
			}

			select {
			case lines <- sc.Text():
			case <-t.Context().Done():
				return
			}
		}
	}()

	next := func() string {
		t.Helper()

		select {
		case line, ok := <-lines:
			require.True(t, ok, "the stream ended")

			return line
		case <-time.After(10 * time.Second):
			require.FailNow(t, "the stream sent nothing")
		}

		return ""
	}

	require.Equal(t, "retry: 1000", next(), "the stream is open")

	expect := func(note object, area string) {
		t.Helper()
		assert.JSONEq(t, fmt.Sprintf(`{"module":"notes","kind":"note","id":%v,"area":%d}`, note["id"], f.areas[area]),
			strings.TrimPrefix(next(), "data: "))
	}

	made := f.create(t, `{"title":"Seeds","area":"garden"}`, f.laptop)
	expect(made, "garden")

	code, got := f.do(t, http.MethodPatch, path(made), `{"revision":9,"title":"Lost"}`, f.laptop)
	refused(t, http.StatusConflict, code, got)

	expect(f.change(t, made, `{"revision":1,"inbox":true}`, f.laptop), "personal")
	expect(f.change(t, made, `{"append":"Sow in March"}`, f.desk), "personal")
}
