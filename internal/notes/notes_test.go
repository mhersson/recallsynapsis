package notes_test

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"maps"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"slices"
	"strings"
	"sync"
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

const password = "correct horse battery"

// passwordHash is password's hash, made once since each costs an argon2 run.
var passwordHash = sync.OnceValue(func() string {
	h, err := auth.HashPassword(password)
	if err != nil {
		panic(err)
	}

	return h
})

// fixture serves a database whose user ada has her personal area and the
// project Garden, and whose user grace has her personal area alone. Each
// user is reached by the Authorization header of a token, and only a test of
// what no token may do signs in, since each sign-in spends an argon2 run.
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

	f := newDatabase(t)
	f.load(t, object{"enabled": true})

	return f
}

// newDatabase is the fixture before it serves, with no migration of notes applied.
func newDatabase(t *testing.T) *fixture {
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

// signIn gives the session of the user named name, as do takes it.
func (f *fixture) signIn(t *testing.T, name string) string {
	t.Helper()

	var id int64
	require.NoError(t, f.store.QueryRowContext(t.Context(), `SELECT id FROM users WHERE name = ?`, name).Scan(&id))
	require.NoError(t, f.auth.SetPasswordHash(t.Context(), id, passwordHash()))

	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/api/auth/login",
		strings.NewReader(`{"name":"`+name+`","password":"`+password+`"}`))
	rec := httptest.NewRecorder()
	f.h.ServeHTTP(rec, req)
	require.Equal(t, http.StatusNoContent, rec.Code, rec.Body.String())

	cookies := rec.Result().Cookies()
	require.Len(t, cookies, 1)

	return "Cookie " + cookies[0].Name + "=" + cookies[0].Value
}

// send serves one request of body, with the Content-Type kind unless it is
// empty, and gives what it answered, as serve does.
func (f *fixture) send(t *testing.T, method, target string, body []byte, kind, auth string) *httptest.ResponseRecorder {
	t.Helper()

	req := httptest.NewRequestWithContext(t.Context(), method, target, bytes.NewReader(body))
	if kind != "" {
		req.Header.Set("Content-Type", kind)
	}

	return f.serve(req, auth)
}

// serve serves req with auth, the Authorization header of a token or a
// session as signIn gives it, and gives what it answered.
func (f *fixture) serve(req *http.Request, auth string) *httptest.ResponseRecorder {
	if cookie, ok := strings.CutPrefix(auth, "Cookie "); ok {
		req.Header.Set("Cookie", cookie)
		req.Header.Set("X-Recallsynapsis-CSRF", "1")
	} else {
		req.Header.Set("Authorization", auth)
	}

	rec := httptest.NewRecorder()
	f.h.ServeHTTP(rec, req)

	return rec
}

// do serves one request as send does and decodes its answer, nil for a 204.
func (f *fixture) do(t *testing.T, method, target, body, auth string) (int, object) {
	t.Helper()

	rec := f.send(t, method, target, []byte(body), "", auth)
	if rec.Code == http.StatusNoContent {
		assert.Empty(t, rec.Body.String())

		return rec.Code, nil
	}

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

// read gives note as it now reads.
func (f *fixture) read(t *testing.T, note object, auth string) object {
	t.Helper()

	code, got := f.do(t, http.MethodGet, path(note), "", auth)
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
			{http.MethodPost, path(made[0]) + "/trash", ""},
			{http.MethodPost, path(made[0]) + "/restore", ""},
			{http.MethodDelete, path(made[0]), ""},
			{http.MethodDelete, "/api/notes/trash", ""},
			{http.MethodGet, path(made[0]) + "/revisions", ""},
			{http.MethodGet, path(made[0]) + "/revisions/1", ""},
			{http.MethodPost, path(made[0]) + "/revisions/1/restore", ""},
			{http.MethodGet, "/api/notes/limits", ""},
			{http.MethodPost, path(made[0]) + "/files?name=seeds.txt", "Tomatoes"},
			{http.MethodGet, path(made[0]) + "/files/1", ""},
			{http.MethodDelete, path(made[0]) + "/files/1", ""},
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

// trash sends note to the trash, or out of it with "/restore", and returns the note it answers.
func (f *fixture) trash(t *testing.T, note object, to, auth string) object {
	t.Helper()

	code, got := f.do(t, http.MethodPost, path(note)+to, "", auth)
	require.Equal(t, http.StatusOK, code, got)

	return got
}

func TestATrashedNoteIsInNoListButTheTrash(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	seeds := f.create(t, `{"title":"Seeds","area":"garden","tags":["spring"]}`, f.laptop)
	seeds = f.change(t, seeds, `{"revision":1,"body":"Tomatoes"}`, f.desk)
	plumber := f.create(t, `{"title":"Plumber","inbox":true}`, f.laptop)
	f.create(t, `{"title":"Beds","area":"garden"}`, f.laptop)

	assert.Nil(t, seeds["trashed_at"], "null unless the note is in the trash")

	f.trash(t, plumber, "/trash", f.laptop)

	trashed := f.trash(t, seeds, "/trash", f.laptop)
	require.NotNil(t, trashed["trashed_at"])

	at, err := time.Parse(time.RFC3339, trashed["trashed_at"].(string))
	require.NoError(t, err)
	assert.Equal(t, time.UTC, at.Location())

	want := maps.Clone(seeds)
	want["trashed_at"] = trashed["trashed_at"]
	assert.Equal(t, want, trashed, "trashing changes nothing else: no revision, source or time of change")
	assert.Equal(t, trashed, f.trash(t, seeds, "/trash", f.desk), "trashing a trashed note changes nothing")

	for _, tt := range []struct {
		query string
		want  []string
	}{
		{"", []string{"Beds"}},
		{"?area=garden", []string{"Beds"}},
		{"?tag=spring", []string{}},
		{"?inbox=true", []string{}},
		{"?trashed=false", []string{"Beds"}},
		{"?trashed=true", []string{"Seeds", "Plumber"}},
		{"?trashed=true&area=garden", []string{"Seeds"}},
	} {
		listed, total := f.titles(t, tt.query, f.phone)
		assert.Equal(t, tt.want, listed, tt.query)
		assert.Equal(t, len(tt.want), total, tt.query)
	}

	listed, total := f.titles(t, "?trashed=true&limit=1", f.phone)
	assert.Equal(t, []string{"Seeds"}, listed, "the most recently trashed first")
	assert.Equal(t, 2, total, "the Trash's count")

	code, got := f.do(t, http.MethodGet, "/api/notes/tags", "", f.phone)
	require.Equal(t, http.StatusOK, code)
	assert.Equal(t, object{"tags": []any{}}, got, "a trashed note's tags are counted nowhere")

	assert.Equal(t, trashed, f.read(t, seeds, f.phone), "a trashed note is still read")

	for _, body := range []string{`{"revision":2,"title":"Lost"}`, `{"append":"Lost"}`} {
		code, got = f.do(t, http.MethodPatch, path(seeds), body, f.laptop)
		refused(t, http.StatusConflict, code, got)
	}

	code, got = f.do(t, http.MethodGet, "/api/notes?trashed=yes", "", f.phone)
	refused(t, http.StatusBadRequest, code, got)

	restored := f.trash(t, seeds, "/restore", f.laptop)
	assert.Equal(t, seeds, restored, "restored, it is as it was, with the source of its last change")
	assert.Equal(t, restored, f.trash(t, seeds, "/restore", f.desk), "restoring a note outside the trash changes nothing")

	listed, _ = f.titles(t, "", f.phone)
	assert.Equal(t, []string{"Beds", "Seeds"}, listed, "back where it was in its list")

	var revisions int
	require.NoError(t, f.store.QueryRowContext(t.Context(), `SELECT count(*) FROM note_revisions WHERE note_id = ?`, seeds["id"]).Scan(&revisions))
	assert.Equal(t, 2, revisions, "with its history, and no revision of its own")

	for _, to := range []string{"/trash", "/restore"} {
		code, got = f.do(t, http.MethodPost, path(seeds)+to, "", f.phone)
		refused(t, http.StatusForbidden, code, got)

		code, got = f.do(t, http.MethodPost, path(seeds)+to, "", f.grace)
		refused(t, http.StatusNotFound, code, got)

		code, got = f.do(t, http.MethodPost, "/api/notes/9999"+to, "", f.laptop)
		refused(t, http.StatusNotFound, code, got)
	}

	assert.Equal(t, seeds, f.read(t, seeds, f.phone), "the refused tries changed nothing")
}

func TestOnlyASessionDeletesFromTheTrash(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	session := f.signIn(t, "ada")

	seeds := f.change(t, f.create(t, `{"title":"Seeds","tags":["spring"]}`, f.laptop), `{"revision":1,"body":"Tomatoes"}`, f.laptop)
	beds := f.create(t, `{"title":"Beds","area":"garden","tags":["spring"]}`, f.laptop)
	plums := f.create(t, `{"title":"Plums"}`, f.laptop)
	kept := f.create(t, `{"title":"Kept","tags":["spring"]}`, f.laptop)
	theirs := f.create(t, `{"title":"Theirs","tags":["spring"]}`, f.grace)

	code, got := f.do(t, http.MethodDelete, path(seeds), "", session)
	refused(t, http.StatusConflict, code, got)

	for _, note := range []object{seeds, beds, plums} {
		f.trash(t, note, "/trash", f.laptop)
	}

	f.trash(t, theirs, "/trash", f.grace)

	for _, token := range []string{f.laptop, f.phone} {
		for _, target := range []string{path(seeds), "/api/notes/trash"} {
			code, got := f.do(t, http.MethodDelete, target, "", token)
			refused(t, http.StatusForbidden, code, got)
		}
	}

	code, got = f.do(t, http.MethodPost, path(theirs)+"/restore", "", f.laptop)
	refused(t, http.StatusNotFound, code, got)

	listed, _ := f.titles(t, "?trashed=true", f.phone)
	assert.Equal(t, []string{"Plums", "Beds", "Seeds"}, listed, "no token deleted anything")

	code, got = f.do(t, http.MethodDelete, path(seeds), "", session)
	require.Equal(t, http.StatusNoContent, code, got)

	code, got = f.do(t, http.MethodGet, path(seeds), "", f.phone)
	refused(t, http.StatusNotFound, code, got)

	for _, table := range []string{"note_tags", "note_revisions"} {
		var n int
		require.NoError(t, f.store.QueryRowContext(t.Context(), `SELECT count(*) FROM `+table+` WHERE note_id = ?`, seeds["id"]).Scan(&n))
		assert.Zero(t, n, "its %s go with it", table)
	}

	for _, target := range []string{path(seeds), "/api/notes/9999", "/api/notes/seeds", path(theirs)} {
		code, got = f.do(t, http.MethodDelete, target, "", session)
		refused(t, http.StatusNotFound, code, got)
	}

	code, got = f.do(t, http.MethodDelete, "/api/notes/trash", "", session)
	require.Equal(t, http.StatusNoContent, code, got)

	listed, total := f.titles(t, "?trashed=true", f.phone)
	assert.Equal(t, []string{}, listed)
	assert.Zero(t, total)

	listed, _ = f.titles(t, "", f.phone)
	assert.Equal(t, []string{"Kept"}, listed, "emptying deletes no note outside the trash")
	assert.Equal(t, []any{"spring"}, f.read(t, kept, f.phone)["tags"])

	listed, _ = f.titles(t, "?trashed=true", f.grace)
	assert.Equal(t, []string{"Theirs"}, listed, "nor another user's trash")
	assert.Equal(t, []any{"spring"}, f.read(t, theirs, f.grace)["tags"])

	assert.Equal(t, 2, f.count(t, "notes"))
	assert.Equal(t, 2, f.count(t, "note_tags"))
	assert.Equal(t, 2, f.count(t, "note_revisions"))

	code, _ = f.do(t, http.MethodDelete, "/api/notes/trash", "", session)
	assert.Equal(t, http.StatusNoContent, code, "an empty trash empties")

	last := f.trash(t, f.create(t, `{"title":"Last"}`, f.laptop), "/trash", f.laptop)

	code, got = f.do(t, http.MethodDelete, path(last), "", session)
	require.Equal(t, http.StatusNoContent, code, got)
	assert.NotEqual(t, last["id"], f.create(t, `{"title":"Next"}`, f.laptop)["id"], "the id of a note deleted for good is never given again")
}

// history gives each revision of note as its history lists it, the newest first, as "revision source restored_from".
func (f *fixture) history(t *testing.T, note object, auth string) []string {
	t.Helper()

	code, got := f.do(t, http.MethodGet, path(note)+"/revisions", "", auth)
	require.Equal(t, http.StatusOK, code, got)

	out := []string{}

	for _, r := range got["revisions"].([]any) {
		r := r.(object)
		assert.Empty(t, r["said"])
		out = append(out, fmt.Sprintf("%v %v %v", r["revision"], r["source"], r["restored_from"]))
	}

	return out
}

func TestEveryChangeIsKeptInTheHistory(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	made := f.create(t, `{"title":"Seeds","body":"Tomatoes","area":"garden","tags":["spring","beds"]}`, f.laptop)
	f.change(t, made, `{"revision":1,"title":"Seedlings","tags":["summer"]}`, f.desk)
	f.change(t, made, `{"append":"Beans"}`, f.laptop)
	changed := f.change(t, made, `{"revision":3,"kind":"idea","inbox":true}`, f.desk)

	assert.Equal(t, []string{"4 desk <nil>", "3 laptop <nil>", "2 desk <nil>", "1 laptop <nil>"}, f.history(t, made, f.phone))

	code, got := f.do(t, http.MethodGet, path(made)+"/revisions/1", "", f.phone)
	require.Equal(t, http.StatusOK, code, got)
	assert.Equal(t, object{
		"revision": float64(1), "at": made["created_at"], "source": "laptop", "said": "", "restored_from": nil,
		"title": "Seeds", "body": "Tomatoes", "kind": "note", "area": "garden", "inbox": false, "tags": []any{"beds", "spring"},
		"links": []any{},
	}, got)

	code, got = f.do(t, http.MethodGet, path(made)+"/revisions/4", "", f.phone)
	require.Equal(t, http.StatusOK, code, got)
	assert.Equal(t, changed["changed_at"], got["at"])
	assert.Equal(t, object{"title": "Seedlings", "body": "Tomatoes\n\nBeans", "kind": "idea", "area": "personal", "inbox": true},
		object{"title": got["title"], "body": got["body"], "kind": got["kind"], "area": got["area"], "inbox": got["inbox"]})

	code, restored := f.do(t, http.MethodPost, path(made)+"/revisions/1/restore", "", f.laptop)
	require.Equal(t, http.StatusOK, code, restored)
	assert.InDelta(t, 5, restored["revision"], 0)
	assert.Equal(t, "laptop", restored["source"])
	assert.Equal(t, "Seeds", restored["title"])
	assert.Equal(t, "Tomatoes", restored["body"])
	assert.Equal(t, []any{"beds", "spring"}, restored["tags"])
	assert.Equal(t, "idea", restored["kind"], "the kind stays as it is")
	assert.Equal(t, "personal", restored["area"], "and the area")
	assert.Equal(t, true, restored["inbox"], "and the inbox flag")
	assert.NotEqual(t, changed["changed_at"], restored["changed_at"])
	assert.Equal(t, restored, f.read(t, made, f.phone))

	history := f.history(t, made, f.phone)
	assert.Equal(t, []string{"5 laptop 1", "4 desk <nil>", "3 laptop <nil>", "2 desk <nil>", "1 laptop <nil>"}, history,
		"a restore is a revision of its own, which names the one it brought back")

	code, got = f.do(t, http.MethodGet, path(made)+"/revisions", "", f.phone)
	require.Equal(t, http.StatusOK, code, got)
	assert.Equal(t, []any{
		object{"revision": float64(5), "at": restored["changed_at"], "source": "laptop", "said": "", "restored_from": float64(1)},
		object{"revision": float64(4), "at": changed["changed_at"], "source": "desk", "said": "", "restored_from": nil},
	}, got["revisions"].([]any)[:2], "a revision as the history lists it")

	code, got = f.do(t, http.MethodGet, path(made)+"/revisions/5", "", f.phone)
	require.Equal(t, http.StatusOK, code, got)
	assert.InDelta(t, 1, got["restored_from"], 0)
	assert.Equal(t, []any{"beds", "spring"}, got["tags"])

	code, got = f.do(t, http.MethodGet, "/api/notes/tags", "", f.phone)
	require.Equal(t, http.StatusOK, code)
	assert.Equal(t, object{"tags": []any{object{"tag": "beds", "count": float64(1)}, object{"tag": "spring", "count": float64(1)}}}, got)

	for _, target := range []string{"/revisions/9", "/revisions/0", "/revisions/one", "/revisions/9/restore", "/revisions/one/restore"} {
		method := http.MethodGet
		if strings.HasSuffix(target, "/restore") {
			method = http.MethodPost
		}

		code, got = f.do(t, method, path(made)+target, "", f.laptop)
		refused(t, http.StatusNotFound, code, got)
	}

	for _, target := range []string{"/revisions", "/revisions/1"} {
		code, got = f.do(t, http.MethodGet, "/api/notes/9999"+target, "", f.laptop)
		refused(t, http.StatusNotFound, code, got)

		code, got = f.do(t, http.MethodGet, path(made)+target, "", f.grace)
		refused(t, http.StatusNotFound, code, got)
	}

	code, got = f.do(t, http.MethodPost, path(made)+"/revisions/1/restore", "", f.grace)
	refused(t, http.StatusNotFound, code, got)

	code, got = f.do(t, http.MethodPost, path(made)+"/revisions/1/restore", "", f.phone)
	refused(t, http.StatusForbidden, code, got)

	trashed := f.trash(t, made, "/trash", f.laptop)
	assert.Equal(t, history, f.history(t, made, f.phone), "a trashed note's history is still read")

	code, got = f.do(t, http.MethodGet, path(made)+"/revisions/2", "", f.phone)
	require.Equal(t, http.StatusOK, code, got)
	assert.Equal(t, "Seedlings", got["title"])

	code, got = f.do(t, http.MethodPost, path(made)+"/revisions/2/restore", "", f.laptop)
	refused(t, http.StatusConflict, code, got)
	assert.Equal(t, trashed, f.read(t, made, f.phone), "a trashed note cannot be restored to a revision")

	assert.Equal(t, restored, f.trash(t, made, "/restore", f.laptop))
	assert.Equal(t, history, f.history(t, made, f.phone), "restored from the trash, it keeps its history")
}

func TestANoteInFullGivesWhatItsFirstRevisionSaid(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	made := f.create(t, `{"body":"Sow the beans"}`, f.laptop)
	assert.Contains(t, made, "said")
	assert.Empty(t, made["said"])

	_, err := f.store.ExecContext(t.Context(), `UPDATE note_revisions SET said = 'Husk å så bønnene' WHERE note_id = ?`, made["id"])
	require.NoError(t, err)

	assert.Empty(t, f.create(t, `{"body":"Water the peas"}`, f.laptop)["said"], "each note gives its own")

	changed := f.change(t, made, `{"revision":1,"title":"Beans"}`, f.desk)
	assert.Equal(t, "Husk å så bønnene", changed["said"], "a later revision keeps the first one's")
	assert.Equal(t, changed, f.read(t, made, f.phone))

	code, got := f.do(t, http.MethodGet, path(made)+"/revisions", "", f.phone)
	require.Equal(t, http.StatusOK, code, got)
	revisions := got["revisions"].([]any)
	assert.Empty(t, revisions[0].(object)["said"])
	assert.Equal(t, "Husk å så bønnene", revisions[1].(object)["said"], "the history gives it on the first revision alone")

	code, got = f.do(t, http.MethodGet, "/api/notes", "", f.phone)
	require.Equal(t, http.StatusOK, code, got)
	assert.NotContains(t, got["notes"].([]any)[0], "said", "a list gives no said")
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

	code, got = f.do(t, http.MethodPost, path(made)+"/revisions/1/restore", "", f.laptop)
	require.Equal(t, http.StatusOK, code, got)
	expect(got, "personal")

	expect(f.trash(t, made, "/trash", f.laptop), "personal")
	f.trash(t, made, "/trash", f.laptop)

	beds := f.create(t, `{"title":"Beds","area":"garden"}`, f.laptop)
	expect(beds, "garden") // and not the trashing that changed nothing
	expect(f.trash(t, made, "/restore", f.laptop), "personal")
	expect(f.trash(t, made, "/trash", f.laptop), "personal")

	session := f.signIn(t, "ada")

	code, got = f.do(t, http.MethodDelete, path(made), "", session)
	require.Equal(t, http.StatusNoContent, code, got)
	expect(made, "personal")

	attached := f.attach(t, beds, "beds.txt", "text/plain", []byte("Two by four"), session)
	expect(beds, "garden")

	code, got = f.do(t, http.MethodDelete, filePath(beds, attached), "", session)
	require.Equal(t, http.StatusNoContent, code, got)
	expect(beds, "garden")

	expect(f.trash(t, beds, "/trash", f.laptop), "garden")

	code, got = f.do(t, http.MethodDelete, "/api/notes/trash", "", session)
	require.Equal(t, http.StatusNoContent, code, got)
	assert.JSONEq(t, `{"module":"notes","kind":"trash","id":0,"area":0}`, strings.TrimPrefix(next(), "data: "), "emptying tells once")
}
