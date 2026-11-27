package knowledge_test

import (
	"bufio"
	"encoding/json"
	"fmt"
	"maps"
	"net/http"
	"net/http/httptest"
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
	"github.com/mhersson/recallsynapsis/internal/knowledge"
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

// start is the fixture's clock: late on Monday 23 November 2026 in UTC, when
// Tuesday has begun in the fixture's zone.
var start = time.Date(2026, 11, 23, 22, 30, 0, 0, time.UTC)

// today is the date at start in the fixture's zone.
const today = "2026-11-24"

// fixture serves a database whose user ada has her personal area, the
// projects Garden and Orchard and the archived project Attic, and whose user
// grace has her personal area and a project Garden of her own, with notes
// switched on beside knowledge. Over REST only a session changes a decision,
// and each sign-in spends an argon2 run, so a test signs in once.
type fixture struct {
	h      http.Handler
	store  *store.Store
	auth   *auth.Service
	core   *core.Service
	hub    *api.Hub
	clock  atomic.Int64 // the Unix milliseconds of the last reading of the clock
	zone   *time.Location
	users  map[string]int64
	laptop string // ada's write token
	phone  string // ada's read token
	grace  string // grace's read token
	areas  map[string]int64
	dir    string
}

func newFixture(t *testing.T) *fixture {
	t.Helper()

	f := newDatabase(t)
	f.load(t, object{"enabled": true})

	return f
}

// newDatabase is the fixture before it serves, with no migration of knowledge applied.
func newDatabase(t *testing.T) *fixture {
	t.Helper()

	st, err := store.Open(t.Context(), filepath.Join(t.TempDir(), "recallsynapsis.db"), slices.Concat(auth.Migrations, core.Migrations))
	require.NoError(t, err)

	t.Cleanup(func() { _ = st.Close() })

	zone, err := time.LoadLocation("Europe/Helsinki")
	require.NoError(t, err)

	f := &fixture{
		store: st, core: core.New(st), hub: api.NewHub(t.Context().Done()), zone: zone, users: map[string]int64{},
		areas: map[string]int64{}, dir: t.TempDir(),
	}
	f.auth = auth.New(st, time.Now)
	f.clock.Store(start.UnixMilli())

	for _, name := range []string{"ada", "grace"} {
		var id int64
		require.NoError(t, st.QueryRowContext(t.Context(), `INSERT INTO users (name, created_at) VALUES (?, 0) RETURNING id`, name).Scan(&id))
		require.NoError(t, f.core.EnsurePersonal(t.Context(), id))
		require.NoError(t, f.auth.SetPasswordHash(t.Context(), id, passwordHash()))

		f.users[name] = id
	}

	token := func(user, name, access string) string {
		_, text, err := f.auth.CreateToken(t.Context(), f.users[user], name, access)
		require.NoError(t, err)

		return "Bearer " + text
	}

	f.laptop, f.phone, f.grace = token("ada", "laptop", "write"), token("ada", "phone", "read"), token("grace", "tablet", "read")

	project := func(user, name string) module.Area {
		a, err := f.core.CreateProject(t.Context(), module.Caller{UserID: f.users[user]}, name, "")
		require.NoError(t, err)

		return a
	}

	f.areas["garden"], f.areas["orchard"], f.areas["attic"] = project("ada", "Garden").ID, project("ada", "Orchard").ID, project("ada", "Attic").ID
	project("grace", "Garden")

	archived := true
	_, err = f.core.ChangeArea(t.Context(), module.Caller{UserID: f.users["ada"]}, f.areas["attic"], core.Change{Archived: &archived})
	require.NoError(t, err)

	personal, err := f.core.Find(t.Context(), f.users["ada"], "personal")
	require.NoError(t, err)

	f.areas["personal"] = personal.ID

	return f
}

// load serves the database again, with table as [modules.knowledge], or none when nil.
func (f *fixture) load(t *testing.T, table object) {
	t.Helper()

	tables := map[string]map[string]any{"notes": {"enabled": true}}
	if table != nil {
		tables["knowledge"] = table
	}

	// Each reading of the clock is a millisecond on from the last, so no two writes share a time.
	now := func() time.Time { return time.UnixMilli(f.clock.Add(1)) }

	ms, err := f.core.LoadModules(t.Context(), []func(module.Deps) module.Module{notes.New, knowledge.New}, &config.Config{Modules: tables},
		module.Deps{Now: now, Zone: f.zone, Dir: f.dir, Changed: f.hub.Changed})
	require.NoError(t, err)

	f.h = api.New(api.Deps{Auth: f.auth, Core: f.core, Modules: ms, PublicURL: "https://recall.example.org", Hub: f.hub})
}

// signIn gives the session of the user named name, as do takes it.
func (f *fixture) signIn(t *testing.T, name string) string {
	t.Helper()

	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/api/auth/login",
		strings.NewReader(`{"name":"`+name+`","password":"`+password+`"}`))
	rec := httptest.NewRecorder()
	f.h.ServeHTTP(rec, req)
	require.Equal(t, http.StatusNoContent, rec.Code, rec.Body.String())

	cookies := rec.Result().Cookies()
	require.Len(t, cookies, 1)

	return "Cookie " + cookies[0].Name + "=" + cookies[0].Value
}

// do serves one request with auth, the Authorization header of a token or a
// session as signIn gives it, and decodes its answer, nil for a 204.
func (f *fixture) do(t *testing.T, method, target, body, auth string) (int, object) {
	t.Helper()

	req := httptest.NewRequestWithContext(t.Context(), method, target, strings.NewReader(body))
	if cookie, ok := strings.CutPrefix(auth, "Cookie "); ok {
		req.Header.Set("Cookie", cookie)
		req.Header.Set("X-Recallsynapsis-CSRF", "1")
	} else {
		req.Header.Set("Authorization", auth)
	}

	rec := httptest.NewRecorder()
	f.h.ServeHTTP(rec, req)

	if rec.Code == http.StatusNoContent {
		assert.Empty(t, rec.Body.String())

		return rec.Code, nil
	}

	assert.Equal(t, "application/json", rec.Header().Get("Content-Type"))

	var got object
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got), rec.Body.String())

	return rec.Code, got
}

// pack saves the package body with auth and returns the decisions it answers.
func (f *fixture) pack(t *testing.T, body, auth string) []object {
	t.Helper()

	code, got := f.do(t, http.MethodPost, "/api/knowledge", body, auth)
	require.Equal(t, http.StatusOK, code, got)

	return objects(got["decisions"])
}

// add adds the decision of the JSON object draft to the area with auth and returns it.
func (f *fixture) add(t *testing.T, area, draft, auth string) object {
	t.Helper()

	return f.pack(t, `{"area":"`+area+`","add":[`+draft+`]}`, auth)[0]
}

// mend mends decision from its revision with the fields of the JSON object
// body, in a package of the area, with auth, and returns it.
func (f *fixture) mend(t *testing.T, area string, decision object, body, auth string) object {
	t.Helper()

	return f.pack(t, mending(area, decision, decision["revision"], body), auth)[0]
}

// mending is the package of area that mends decision from revision with the fields of the JSON object body.
func mending(area string, decision object, revision any, body string) string {
	entry := object{}
	if err := json.Unmarshal([]byte(body), &entry); err != nil {
		panic(err)
	}

	entry["ref"], entry["revision"] = decision["ref"], revision

	b, err := json.Marshal(object{"area": area, "mend": []object{entry}})
	if err != nil {
		panic(err)
	}

	return string(b)
}

// read gives decision as it now reads with auth, as a list gives it: a
// decision in full, which linkedFrom gives, without what links to it.
func (f *fixture) read(t *testing.T, decision object, auth string) object {
	t.Helper()

	code, got := f.do(t, http.MethodGet, path(decision), "", auth)
	require.Equal(t, http.StatusOK, code, got)
	require.Contains(t, got, "linked_from")
	delete(got, "linked_from")

	return got
}

// linkedFrom gives the items that link to decision, as it reads in full with ada's phone.
func (f *fixture) linkedFrom(t *testing.T, decision object) any {
	t.Helper()

	code, got := f.do(t, http.MethodGet, path(decision), "", f.phone)
	require.Equal(t, http.StatusOK, code, got)

	return got["linked_from"]
}

// note makes a note of the body JSON with ada's laptop, links it to refs, and returns it.
func (f *fixture) note(t *testing.T, body string, refs ...any) object {
	t.Helper()

	code, made := f.do(t, http.MethodPost, "/api/notes", body, f.laptop)
	require.Equal(t, http.StatusCreated, code, made)

	links, err := json.Marshal(object{"revision": 1, "links": refs})
	require.NoError(t, err)

	code, got := f.do(t, http.MethodPatch, fmt.Sprintf("/api/notes/%v", made["id"]), string(links), f.laptop)
	require.Equal(t, http.StatusOK, code, got)

	return got
}

// links gives the links of note as it now reads with auth.
func (f *fixture) links(t *testing.T, note object, auth string) any {
	t.Helper()

	code, got := f.do(t, http.MethodGet, fmt.Sprintf("/api/notes/%v", note["id"]), "", auth)
	require.Equal(t, http.StatusOK, code, got)

	return got["links"]
}

// linkTo is a link to decision as it reads while it is there, in state.
func linkTo(decision object, state string) object {
	return object{"ref": decision["ref"], "kind": "decision", "state": state, "title": decision["decision"], "available": true}
}

// unavailable is a link to ref that leads nowhere.
func unavailable(ref any) object {
	return object{"ref": ref, "kind": "", "state": "", "title": "", "available": false}
}

// list gives the decisions of the area as its list answers them with ada's phone.
func (f *fixture) list(t *testing.T, area string) []object {
	t.Helper()

	code, got := f.do(t, http.MethodGet, "/api/knowledge?area="+area, "", f.phone)
	require.Equal(t, http.StatusOK, code, got)

	return objects(got["decisions"])
}

// line is decision as another names it.
func line(decision object) object {
	return object{"ref": decision["ref"], "decision": decision["decision"], "decided": decision["decided"]}
}

func objects(v any) []object {
	out := []object{}
	for _, o := range v.([]any) {
		out = append(out, o.(object))
	}

	return out
}

func path(decision object) string {
	return fmt.Sprintf("/api/knowledge/%v", decision["id"])
}

// refused wants a refusal with status, in the one error shape, and gives its line.
func refused(t *testing.T, status, code int, got object) string {
	t.Helper()

	assert.Equal(t, status, code, got)
	assert.Len(t, got, 1)

	line, _ := got["error"].(string)
	assert.NotEmpty(t, line)
	assert.NotContains(t, line, "\n")

	return line
}

func (f *fixture) count(t *testing.T, table string) int {
	t.Helper()

	var n int
	require.NoError(t, f.store.QueryRowContext(t.Context(), `SELECT count(*) FROM `+table).Scan(&n))

	return n
}

func TestDecisionsAddedInOneCallReadBackInTheirList(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	web := f.signIn(t, "ada")

	made := f.pack(t, `{"area":"Garden","add":[
		{"decision":"Beds are raised","why":"The soil is clay.","source":"The plan of the garden","decided":"2026-10-01"},
		{"decision":" Water at dawn ","why":"Less is lost to the sun.\n\nAnd the slugs sleep."}]}`, web)
	require.Len(t, made, 2)

	raised, dawn := made[0], made[1]
	assert.Equal(t, object{
		"id": raised["id"], "ref": fmt.Sprintf("knowledge:%v", raised["id"]), "area": "garden", "decision": "Beds are raised",
		"why": "The soil is clay.", "source": "The plan of the garden", "decided": "2026-10-01", "state": "stands",
		"replaces": nil, "replaced_by": nil, "revision": float64(1),
		"created_at": raised["created_at"], "changed_at": raised["created_at"], "changed_by": "web",
	}, raised)

	at, err := time.Parse(time.RFC3339, raised["created_at"].(string))
	require.NoError(t, err)
	assert.Equal(t, time.UTC, at.Location())

	assert.Equal(t, "Water at dawn", dawn["decision"])
	assert.Equal(t, "Less is lost to the sun.\n\nAnd the slugs sleep.", dawn["why"])
	assert.Equal(t, today, dawn["decided"], "today in the configured zone, while in UTC it is still the 23rd")
	assert.Empty(t, dawn["source"], "a source left out is none")
	assert.Equal(t, "stands", dawn["state"])
	assert.InDelta(t, 1, dawn["revision"], 0)
	assert.Equal(t, "web", dawn["changed_by"])

	mulch := f.add(t, "garden", `{"decision":"Mulch in autumn","why":"It feeds the soil."}`, web)

	code, got := f.do(t, http.MethodGet, "/api/knowledge?area=garden", "", f.phone)
	require.Equal(t, http.StatusOK, code, got)
	assert.Equal(t, object{"decisions": []any{mulch, dawn, raised}, "today": today}, got,
		"the newest day first, and among those of a day the newest made first")
	assert.Equal(t, raised, f.read(t, raised, f.phone))
	assert.Empty(t, f.list(t, "orchard"), "a project's list holds its own decisions alone")
}

func TestAPackageIsSavedWholeOrNotAtAll(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	web := f.signIn(t, "ada")

	raised := `{"decision":"Beds are raised","why":"The soil is clay."}`
	entries := func(n int) string {
		return `{"area":"garden","add":[` + strings.TrimSuffix(strings.Repeat(raised+",", n), ",") + `]}`
	}

	code, got := f.do(t, http.MethodPost, "/api/knowledge",
		`{"area":"garden","add":[`+strings.Repeat(raised+",", 6)+`{"decision":"Water at dawn","why":" "}]}`, web)
	assert.Contains(t, refused(t, http.StatusBadRequest, code, got), "7", "the line names the entry refused")
	assert.Zero(t, f.count(t, "decisions"), "and nothing of the package is saved")

	for _, body := range []string{
		`{"area":"garden","add":[{"decision":"` + strings.Repeat("å", 201) + `","why":"Clay."}]}`,
		`{"area":"garden","add":[{"decision":"Beds are\nraised","why":"Clay."}]}`,
		`{"area":"garden","add":[{"decision":"  ","why":"Clay."}]}`,
		`{"area":"garden","add":[{"decision":"Beds are raised","why":""}]}`,
		`{"area":"garden","add":[{"decision":"Beds are raised"}]}`,
		`{"area":"garden","add":[{"decision":"Beds are raised","why":"` + strings.Repeat("å", 2001) + `"}]}`,
		`{"area":"garden","add":[{"decision":"Beds are raised","why":"Clay.","source":"` + strings.Repeat("å", 501) + `"}]}`,
		`{"area":"garden","add":[{"decision":"Beds are raised","why":"Clay.","source":"The plan,\nthe talk"}]}`,
		`{"area":"garden","add":[{"decision":"Beds are raised","why":"Clay.","decided":"2026-02-30"}]}`,
		`{"area":"garden","add":[{"decision":"Beds are raised","why":"Clay.","decided":"1 October"}]}`,
		entries(21),
		entries(0),
		`{"area":"garden"}`,
		`{"add":[` + raised + `]}`,
		`{"area":"garden","add":[` + raised + `],"said":"keep it"}`,
	} {
		code, got := f.do(t, http.MethodPost, "/api/knowledge", body, web)
		refused(t, http.StatusBadRequest, code, got)
	}

	assert.Zero(t, f.count(t, "decisions"))
	assert.Zero(t, f.count(t, "decision_revisions"))

	code, got = f.do(t, http.MethodPost, "/api/knowledge", `{"area":"nowhere","add":[`+raised+`]}`, web)
	refused(t, http.StatusNotFound, code, got)

	full := f.add(t, "garden", fmt.Sprintf(`{"decision":"%s","why":"%s","source":"%s"}`,
		strings.Repeat("å", 200), strings.Repeat("å", 1996)+`\n\t\nå`, strings.Repeat("å", 500)), web)
	assert.Equal(t, strings.Repeat("å", 200), full["decision"], "each text at its cap is kept")
	assert.Len(t, f.pack(t, entries(20), web), 20)
}

func TestADecisionReplacesOneThatStandsInItsProject(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	web := f.signIn(t, "ada")

	clay := f.add(t, "garden", `{"decision":"Beds are raised","why":"The soil is clay.","decided":"2026-10-01"}`, web)
	sand := f.add(t, "garden", fmt.Sprintf(`{"decision":"Beds are flat","why":"Sand was brought in.","replaces":"%v"}`, clay["ref"]), web)

	assert.Equal(t, "stands", sand["state"])
	assert.Equal(t, line(clay), sand["replaces"])
	assert.Nil(t, sand["replaced_by"])

	clay = f.read(t, clay, f.phone)
	assert.Equal(t, "replaced", clay["state"])
	assert.Equal(t, line(sand), clay["replaced_by"])
	assert.Nil(t, clay["replaces"])
	assert.InDelta(t, 1, clay["revision"], 0, "being replaced writes no revision")
	assert.Equal(t, []object{sand, clay}, f.list(t, "garden"), "the list holds the replaced too")

	apples := f.add(t, "orchard", `{"decision":"Grow apples","why":"They keep."}`, web)
	replacing := func(ref any) string {
		return fmt.Sprintf(`{"area":"garden","add":[{"decision":"Beds are sunken","why":"Wind.","replaces":"%v"}]}`, ref)
	}

	code, got := f.do(t, http.MethodPost, "/api/knowledge", replacing(clay["ref"]), web)
	assert.Contains(t, refused(t, http.StatusBadRequest, code, got), sand["ref"], "the line names what replaced it")

	for _, tt := range []struct {
		body   string
		status int
	}{
		{replacing(apples["ref"]), http.StatusBadRequest},
		{replacing("knowledge:9999"), http.StatusNotFound},
		{replacing("notes:1"), http.StatusBadRequest},
		{replacing("knowledge"), http.StatusBadRequest},
		{replacing("knowledge:0"), http.StatusBadRequest},
		{mending("garden", sand, 1, fmt.Sprintf(`{"replaces":%q}`, sand["ref"])), http.StatusBadRequest},
		{mending("garden", clay, 1, fmt.Sprintf(`{"replaces":%q}`, sand["ref"])), http.StatusBadRequest},
		{fmt.Sprintf(`{"area":"garden","mend":[{"ref":%q,"revision":1,"replaces":%q}],
			"add":[{"decision":"Beds are terraced","why":"The slope.","replaces":%[2]q}]}`, sand["ref"], clay["ref"]), http.StatusBadRequest},
	} {
		code, got := f.do(t, http.MethodPost, "/api/knowledge", tt.body, web)
		refused(t, tt.status, code, got)
	}

	water := f.add(t, "garden", `{"decision":"Water at dawn","why":"Less is lost."}`, web)

	for _, body := range []string{
		fmt.Sprintf(`{"area":"garden","add":[{"decision":"Water at dusk","why":"Wind.","replaces":%q},
			{"decision":"Water at noon","why":"Heat.","replaces":%[1]q}]}`, water["ref"]),
		fmt.Sprintf(`{"area":"garden","mend":[{"ref":%q,"revision":1,"replaces":%q}],
			"add":[{"decision":"Water at noon","why":"Heat.","replaces":%[2]q}]}`, sand["ref"], water["ref"]),
	} {
		code, got := f.do(t, http.MethodPost, "/api/knowledge", body, web)
		line := refused(t, http.StatusBadRequest, code, got)
		assert.Contains(t, line, water["ref"], "two entries of one package that replace the same decision")

		next := f.add(t, "orchard", `{"decision":"Grow pears","why":"They keep."}`, web)
		assert.NotContains(t, line, next["ref"], "and the line names no decision the package would have made")
	}

	assert.Equal(t, 6, f.count(t, "decisions"), "a decision of another project, one that is not there, itself, a circle and one replaced twice")
	assert.Equal(t, []object{water, sand, clay}, f.list(t, "garden"))

	loam := f.add(t, "garden", fmt.Sprintf(`{"decision":"Beds are of loam","why":"Compost.","replaces":"%v"}`, sand["ref"]), web)
	assert.Equal(t, line(sand), loam["replaces"], "a decision that replaces another can be replaced in turn")
	assert.Equal(t, "replaced", f.read(t, sand, f.phone)["state"])
}

func TestAMendChangesWhatItGivesFromTheRevisionItNames(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	web := f.signIn(t, "ada")

	clay := f.add(t, "garden", `{"decision":"Beds are raised","why":"The soil is clay.","source":"The plan","decided":"2026-10-01"}`, web)
	sand := f.add(t, "garden", `{"decision":"Beds are flat","why":"Sand was brought in."}`, web)

	mended := f.mend(t, "garden", clay, `{"why":" The soil is heavy clay. "}`, web)
	assert.InDelta(t, 2, mended["revision"], 0)
	assert.Equal(t, "The soil is heavy clay.", mended["why"])
	assert.Equal(t, clay["created_at"], mended["created_at"])
	assert.NotEqual(t, clay["changed_at"], mended["changed_at"])

	for key, kept := range clay {
		if !slices.Contains([]string{"why", "revision", "changed_at"}, key) {
			assert.Equal(t, kept, mended[key], "a mend changes only the fields it gives, %s", key)
		}
	}

	revisions := f.count(t, "decision_revisions")

	code, got := f.do(t, http.MethodPost, "/api/knowledge", mending("garden", clay, 1, `{"why":"Clay."}`), web)
	assert.Contains(t, refused(t, http.StatusConflict, code, got), "2", "the line names the revision it is at")

	code, got = f.do(t, http.MethodPost, "/api/knowledge",
		fmt.Sprintf(`{"area":"garden","add":[{"decision":"Water at dawn","why":"Less is lost."}],"mend":[{"ref":%q,"revision":1,"why":"Loam."},
			{"ref":%q,"revision":1,"why":"Clay."}]}`, sand["ref"], clay["ref"]), web)
	refused(t, http.StatusConflict, code, got)
	assert.Equal(t, 2, f.count(t, "decisions"), "and the whole package with it")
	assert.Equal(t, revisions, f.count(t, "decision_revisions"))
	assert.Equal(t, sand, f.read(t, sand, f.phone))

	replacing := f.mend(t, "garden", sand, fmt.Sprintf(`{"replaces":%q}`, clay["ref"]), web)
	assert.Equal(t, line(mended), replacing["replaces"], "a mend changes what a decision replaces")
	assert.Equal(t, "replaced", f.read(t, clay, f.phone)["state"])

	cleared := f.mend(t, "garden", replacing, `{"replaces":""}`, web)
	assert.Nil(t, cleared["replaces"], `and "" clears it`)
	assert.InDelta(t, 3, cleared["revision"], 0)
	assert.Equal(t, "stands", f.read(t, clay, f.phone)["state"])

	both := f.pack(t, fmt.Sprintf(`{"area":"garden","mend":[{"ref":%q,"revision":2,"decision":"Beds are raised high","decided":"2026-09-30"},
		{"ref":%q,"revision":3,"source":"The talk"}]}`, clay["ref"], sand["ref"]), web)
	assert.Equal(t, []any{"Beds are raised high", "2026-09-30", "The talk"}, []any{both[0]["decision"], both[0]["decided"], both[1]["source"]})

	apples := f.add(t, "orchard", `{"decision":"Grow apples","why":"They keep."}`, web)
	revisions = f.count(t, "decision_revisions")

	for _, tt := range []struct {
		body   string
		status int
	}{
		{mending("garden", both[0], 3, `{}`), http.StatusBadRequest},
		{fmt.Sprintf(`{"area":"garden","mend":[{"ref":%q,"why":"Clay."}]}`, clay["ref"]), http.StatusBadRequest},
		{mending("garden", apples, 1, `{"why":"They keep all winter."}`), http.StatusBadRequest},
		{mending("garden", object{"ref": "knowledge:9999"}, 1, `{"why":"Clay."}`), http.StatusNotFound},
		{mending("garden", object{"ref": "notes:1"}, 1, `{"why":"Clay."}`), http.StatusBadRequest},
		{mending("garden", both[0], 3, `{"decided":""}`), http.StatusBadRequest},
		{mending("garden", both[0], 3, `{"why":" "}`), http.StatusBadRequest},
		{mending("garden", both[0], 3, `{"said":"keep it"}`), http.StatusBadRequest},
	} {
		code, got := f.do(t, http.MethodPost, "/api/knowledge", tt.body, web)
		refused(t, tt.status, code, got)
	}

	assert.Equal(t, revisions, f.count(t, "decision_revisions"), "a mend with no field, with no revision, or of another project")

	same := f.pack(t, fmt.Sprintf(`{"area":"garden","mend":[{"ref":%q,"revision":3,"decision":"Beds are raised high","why":"The soil is heavy clay."},
		{"ref":%q,"revision":4,"source":" The talk "}]}`, clay["ref"], sand["ref"]), web)
	assert.Equal(t, both, same, "a mend that leaves a decision as it is answers it as it is")
	assert.Equal(t, revisions, f.count(t, "decision_revisions"), "and writes no revision")

	code, got = f.do(t, http.MethodPost, "/api/knowledge", mending("garden", both[0], 2, `{"decision":"Beds are raised high"}`), web)
	refused(t, http.StatusConflict, code, got)

	code, got = f.do(t, http.MethodPost, "/api/knowledge", fmt.Sprintf(`{"area":"garden","mend":[{"ref":%q,"revision":3,"why":"Clay."},
		{"ref":%[1]q,"revision":3,"source":"The plan"}]}`, clay["ref"]), web)
	refused(t, http.StatusBadRequest, code, got)
	assert.Equal(t, both[0], f.read(t, clay, f.phone), "one package mends a decision once")
}

func TestTheRevisionsOfADecisionReadAsItWasThen(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	web := f.signIn(t, "ada")

	sand := f.add(t, "garden", `{"decision":"Beds are flat","why":"Sand was brought in."}`, web)
	clay := f.add(t, "garden", `{"decision":"Beds are raised","why":"The soil is clay.","decided":"2026-10-01"}`, web)
	clay = f.mend(t, "garden", clay, `{"decision":"Beds are raised high","source":"The talk"}`, web)
	clay = f.mend(t, "garden", clay, fmt.Sprintf(`{"replaces":%q}`, sand["ref"]), web)

	code, got := f.do(t, http.MethodGet, path(clay)+"/revisions", "", f.phone)
	require.Equal(t, http.StatusOK, code, got)

	entries := objects(got["revisions"])
	require.Len(t, entries, 3)

	for i, e := range entries {
		assert.InDelta(t, 3-i, e["revision"], 0, "the newest first")
		assert.Equal(t, object{"revision": e["revision"], "at": e["at"], "by": "web", "said": ""}, e)
	}

	assert.Equal(t, clay["changed_at"], entries[0]["at"])
	assert.Equal(t, clay["created_at"], entries[2]["at"])

	for _, tt := range []struct {
		revision int
		want     object
	}{
		{1, object{"decision": "Beds are raised", "why": "The soil is clay.", "source": "", "decided": "2026-10-01", "replaces": ""}},
		{2, object{"decision": "Beds are raised high", "why": "The soil is clay.", "source": "The talk", "decided": "2026-10-01", "replaces": ""}},
		{3, object{
			"decision": "Beds are raised high", "why": "The soil is clay.", "source": "The talk", "decided": "2026-10-01",
			"replaces": sand["ref"],
		}},
	} {
		code, got := f.do(t, http.MethodGet, fmt.Sprintf("%s/revisions/%d", path(clay), tt.revision), "", f.phone)
		require.Equal(t, http.StatusOK, code, got)

		maps.Copy(tt.want, entries[3-tt.revision])

		assert.Equal(t, tt.want, got, "revision %d", tt.revision)
	}

	for _, target := range []string{
		path(clay) + "/revisions/4", path(clay) + "/revisions/x", "/api/knowledge/9999/revisions",
		"/api/knowledge/9999/revisions/1", "/api/knowledge/9999", "/api/knowledge/x",
	} {
		code, got := f.do(t, http.MethodGet, target, "", f.phone)
		refused(t, http.StatusNotFound, code, got)
	}
}

func TestDeletingADecisionLeavesTheOthersStandingOrReplacingNothing(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	web := f.signIn(t, "ada")

	clay := f.add(t, "garden", `{"decision":"Beds are raised","why":"The soil is clay.","decided":"2026-10-01"}`, web)
	sand := f.add(t, "garden", fmt.Sprintf(`{"decision":"Beds are flat","why":"Sand.","replaces":%q}`, clay["ref"]), web)
	loam := f.add(t, "garden", fmt.Sprintf(`{"decision":"Beds are of loam","why":"Compost.","replaces":%q}`, sand["ref"]), web)
	f.mend(t, "garden", sand, `{"why":"Sand was brought in."}`, web)

	code, got := f.do(t, http.MethodDelete, path(sand), "", web)
	require.Equal(t, http.StatusNoContent, code, got)

	clay = f.read(t, clay, f.phone)
	assert.Equal(t, "stands", clay["state"], "the decision it replaced stands again")
	assert.Nil(t, clay["replaced_by"])

	kept := f.read(t, loam, f.phone)
	assert.Nil(t, kept["replaces"], "and the one that replaced it replaces nothing")

	loam["replaces"] = nil
	assert.Equal(t, loam, kept, "with no revision written for that")
	assert.Equal(t, []object{kept, clay}, f.list(t, "garden"))
	assert.Equal(t, 2, f.count(t, "decision_revisions"), "its revisions go with it")

	for _, target := range []string{path(sand), path(sand) + "/revisions", path(sand) + "/revisions/1"} {
		code, got := f.do(t, http.MethodGet, target, "", f.phone)
		refused(t, http.StatusNotFound, code, got)
	}

	code, got = f.do(t, http.MethodDelete, path(sand), "", web)
	refused(t, http.StatusNotFound, code, got)

	code, got = f.do(t, http.MethodDelete, "/api/knowledge/x", "", web)
	refused(t, http.StatusNotFound, code, got)

	last := f.add(t, "garden", `{"decision":"Beds are of clay","why":"Back to clay."}`, web)
	code, got = f.do(t, http.MethodDelete, path(last), "", web)
	require.Equal(t, http.StatusNoContent, code, got)
	assert.NotEqual(t, last["id"], f.add(t, "garden", `{"decision":"Beds are of clay","why":"Clay."}`, web)["id"],
		"the newest decision's id, deleted, is never given again")
}

func TestANoteThatLinksToADecisionShowsIt(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	web := f.signIn(t, "ada")

	clay := f.add(t, "garden", `{"decision":"Beds are raised","why":"The soil is clay."}`, web)
	sand := f.add(t, "garden", fmt.Sprintf(`{"decision":"Beds are flat","why":"Sand.","replaces":%q}`, clay["ref"]), web)
	beds := f.note(t, `{"title":"Beds","area":"garden"}`, clay["ref"], sand["ref"], "knowledge:9999")

	assert.Equal(t, []any{linkTo(clay, "replaced"), linkTo(sand, "stands"), unavailable("knowledge:9999")}, beds["links"],
		"a link shows a decision's line and its state")

	item := object{"ref": beds["ref"], "kind": "note", "state": "filed", "title": "Beds", "available": true}
	assert.Equal(t, []any{item}, f.linkedFrom(t, clay), "and the decision in full lists the note")
	assert.Equal(t, []any{item}, f.linkedFrom(t, sand))
	assert.Equal(t, []any{}, f.linkedFrom(t, f.add(t, "garden", `{"decision":"Water at dawn","why":"Sun."}`, web)))

	code, got := f.do(t, http.MethodDelete, path(sand), "", web)
	require.Equal(t, http.StatusNoContent, code, got)
	assert.Equal(t, []any{linkTo(clay, "stands"), unavailable(sand["ref"]), unavailable("knowledge:9999")}, f.links(t, beds, f.phone),
		"a link to a deleted decision leads nowhere")
}

func TestDecisionsAreAProjectsAnArchivedOneToo(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	web := f.signIn(t, "ada")
	raised := `{"decision":"Beds are raised","why":"The soil is clay."}`

	code, got := f.do(t, http.MethodPost, "/api/knowledge", `{"area":"personal","add":[`+raised+`]}`, web)
	refused(t, http.StatusBadRequest, code, got)
	assert.Zero(t, f.count(t, "decisions"), "the personal area has no decisions")
	assert.Empty(t, f.list(t, "personal"))

	stored := f.add(t, "Attic", raised, web)
	assert.Equal(t, "attic", stored["area"])
	assert.Equal(t, []object{stored}, f.list(t, "attic"), "an archived project's decisions are read and added to")
	assert.Equal(t, "Beds are raised high", f.mend(t, "attic", stored, `{"decision":"Beds are raised high"}`, web)["decision"])

	for _, target := range []string{"/api/knowledge", "/api/knowledge?area="} {
		code, got := f.do(t, http.MethodGet, target, "", f.phone)
		refused(t, http.StatusBadRequest, code, got)
	}

	code, got = f.do(t, http.MethodGet, "/api/knowledge?area=nowhere", "", f.phone)
	refused(t, http.StatusNotFound, code, got)
}

func TestATokenReadsEveryDecisionAndChangesNone(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	web := f.signIn(t, "ada")
	clay := f.add(t, "garden", `{"decision":"Beds are raised","why":"The soil is clay."}`, web)

	for _, token := range []string{f.laptop, f.phone} {
		for _, body := range []string{
			`{"area":"garden","add":[{"decision":"Water at dawn","why":"Less is lost."}]}`,
			mending("garden", clay, 1, `{"why":"Loam."}`),
		} {
			code, got := f.do(t, http.MethodPost, "/api/knowledge", body, token)
			refused(t, http.StatusForbidden, code, got)
		}

		code, got := f.do(t, http.MethodDelete, path(clay), "", token)
		refused(t, http.StatusForbidden, code, got)
	}

	assert.Equal(t, 1, f.count(t, "decisions"))
	assert.Equal(t, clay, f.read(t, clay, f.laptop))

	for _, target := range []string{"/api/knowledge?area=garden", path(clay), path(clay) + "/revisions", path(clay) + "/revisions/1"} {
		code, got := f.do(t, http.MethodGet, target, "", f.phone)
		assert.Equal(t, http.StatusOK, code, got)
	}
}

func TestAModuleSwitchedOffKeepsItsDecisions(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	web := f.signIn(t, "ada")

	clay := f.add(t, "garden", `{"decision":"Beds are raised","why":"The soil is clay."}`, web)
	sand := f.add(t, "garden", fmt.Sprintf(`{"decision":"Beds are flat","why":"Sand.","replaces":%q}`, clay["ref"]), web)
	want := []object{f.read(t, sand, f.phone), f.read(t, clay, f.phone)}
	beds := f.note(t, `{"title":"Beds","area":"garden"}`, clay["ref"])

	for _, table := range []object{{"enabled": false}, nil} {
		f.load(t, table)

		assert.Equal(t, []any{unavailable(clay["ref"])}, f.links(t, beds, f.phone), "a link to a decision leads nowhere")
		assert.NotContains(t, f.hits(t, "clay", "", f.phone), "knowledge", "and search finds none")

		for _, r := range []struct{ method, target, body string }{
			{http.MethodGet, "/api/knowledge?area=garden", ""},
			{http.MethodPost, "/api/knowledge", `{"area":"garden","add":[{"decision":"Lost","why":"Off."}]}`},
			{http.MethodGet, path(clay), ""},
			{http.MethodDelete, path(clay), ""},
			{http.MethodGet, path(clay) + "/revisions", ""},
			{http.MethodGet, path(clay) + "/revisions/1", ""},
		} {
			code, got := f.do(t, r.method, r.target, r.body, web)
			refused(t, http.StatusNotFound, code, got)
		}

		assert.Equal(t, 2, f.count(t, "decisions"))
		assert.Equal(t, 2, f.count(t, "decision_revisions"))
	}

	f.load(t, object{"enabled": true})
	assert.Equal(t, want, f.list(t, "garden"), "switched on again, every decision reads as before")
	assert.Equal(t, []any{linkTo(clay, "replaced")}, f.links(t, beds, f.phone))
	assert.Equal(t, []string{"Beds are raised"}, f.found(t, "clay"))
}

func TestADecisionOfOneUserIsFoundByNoRouteOfAnother(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	ada, grace := f.signIn(t, "ada"), f.signIn(t, "grace")

	clay := f.add(t, "garden", `{"decision":"Beds are raised","why":"The soil is clay."}`, ada)

	code, got := f.do(t, http.MethodGet, "/api/knowledge?area=garden", "", f.grace)
	require.Equal(t, http.StatusOK, code, got)
	assert.Equal(t, []any{}, got["decisions"], "grace's own Garden holds none of ada's")

	for _, target := range []string{path(clay), path(clay) + "/revisions", path(clay) + "/revisions/1", "/api/knowledge?area=orchard"} {
		code, got := f.do(t, http.MethodGet, target, "", f.grace)
		refused(t, http.StatusNotFound, code, got)
	}

	for _, body := range []string{
		mending("garden", clay, 1, `{"why":"Loam."}`),
		fmt.Sprintf(`{"area":"garden","add":[{"decision":"Beds are flat","why":"Sand.","replaces":%q}]}`, clay["ref"]),
	} {
		code, got := f.do(t, http.MethodPost, "/api/knowledge", body, grace)
		refused(t, http.StatusNotFound, code, got)
	}

	code, got = f.do(t, http.MethodDelete, path(clay), "", grace)
	refused(t, http.StatusNotFound, code, got)

	code, note := f.do(t, http.MethodPost, "/api/notes", `{"title":"Beds"}`, grace)
	require.Equal(t, http.StatusCreated, code, note)
	code, note = f.do(t, http.MethodPatch, fmt.Sprintf("/api/notes/%v", note["id"]), fmt.Sprintf(`{"revision":1,"links":[%q]}`, clay["ref"]), grace)
	require.Equal(t, http.StatusOK, code, note)
	assert.Equal(t, []any{unavailable(clay["ref"])}, note["links"], "another user's decision is never resolved")
	assert.Equal(t, []any{}, f.linkedFrom(t, clay), "nor lists another user's note")

	for _, text := range []string{"clay", "cl"} {
		assert.NotContains(t, f.hits(t, text, "", f.grace), "knowledge", "nor found by another user's search: %s", text)
	}

	assert.Equal(t, 1, f.count(t, "decisions"))
	assert.Equal(t, clay, f.read(t, clay, f.phone))
}

func TestEachWriteReachesAnOpenStream(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	web := f.signIn(t, "ada")

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

	expect := func(decision object, area string) {
		t.Helper()
		assert.JSONEq(t, fmt.Sprintf(`{"module":"knowledge","kind":"decision","id":%v,"area":%d}`, decision["id"], f.areas[area]),
			strings.TrimPrefix(next(), "data: "))
	}

	made := f.pack(t, `{"area":"garden","add":[{"decision":"Beds are raised","why":"Clay."},{"decision":"Water at dawn","why":"Sun."}]}`, web)
	expect(made[0], "garden")
	expect(made[1], "garden")

	code, got := f.do(t, http.MethodPost, "/api/knowledge", `{"area":"garden","add":[{"decision":"Mulch","why":"Soil."},{"decision":"Lost"}]}`, web)
	refused(t, http.StatusBadRequest, code, got)

	f.mend(t, "garden", made[0], `{"why":"Clay."}`, web)
	sun := f.mend(t, "garden", made[1], `{"why":"The sun."}`, web)
	expect(sun, "garden") // and not the refused package, nor the mend that changed nothing
	expect(f.add(t, "attic", `{"decision":"Keep the boxes","why":"Room."}`, web), "attic")
	expect(f.mend(t, "garden", sun, fmt.Sprintf(`{"replaces":%q}`, made[0]["ref"]), web), "garden")

	code, got = f.do(t, http.MethodDelete, path(made[0]), "", web)
	require.Equal(t, http.StatusNoContent, code, got)
	expect(made[0], "garden") // and not the decision that replaced it, which now replaces nothing

	code, got = f.do(t, http.MethodDelete, path(made[0]), "", web)
	refused(t, http.StatusNotFound, code, got)
	expect(f.add(t, "garden", `{"decision":"Mulch","why":"Soil."}`, web), "garden") // and not the delete of none
}
