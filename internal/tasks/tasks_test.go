package tasks_test

import (
	"bufio"
	"context"
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
	"github.com/mhersson/recallsynapsis/internal/module"
	"github.com/mhersson/recallsynapsis/internal/notes"
	"github.com/mhersson/recallsynapsis/internal/store"
	"github.com/mhersson/recallsynapsis/internal/tasks"
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

// fixture serves a database whose user ada has her personal area and the
// project Garden, and whose user grace has her personal area alone, with
// notes switched on beside tasks, so a task is found and linked beside a
// note, and bell, a module that keeps what it is asked to deliver. Each user
// is reached by the Authorization header of a token, and only a test of what
// no token may do signs in, since each sign-in spends an argon2 run.
type fixture struct {
	h      http.Handler
	store  *store.Store
	auth   *auth.Service
	core   *core.Service
	hub    *api.Hub
	clock  atomic.Int64 // the Unix milliseconds of the last reading of the clock
	zone   *time.Location
	notify func(context.Context) error // the job of tasks, nil while it is off
	muted  bool                        // bell is off
	rang   []rang                      // what bell was asked to deliver
	cut    context.CancelFunc          // when set, bell calls it as it delivers
	ada    int64
	laptop string // ada's write token
	desk   string // another write token of ada's
	phone  string // ada's read token
	grace  string // grace's write token
	areas  map[string]int64
	dir    string // the data directory
}

func newFixture(t *testing.T) *fixture {
	t.Helper()

	f := newDatabase(t)
	f.load(t, object{"enabled": true})

	return f
}

// newDatabase is the fixture before it serves, with no migration of tasks applied.
func newDatabase(t *testing.T) *fixture {
	t.Helper()

	st, err := store.Open(t.Context(), filepath.Join(t.TempDir(), "recallsynapsis.db"), slices.Concat(auth.Migrations, core.Migrations))
	require.NoError(t, err)

	t.Cleanup(func() { _ = st.Close() })

	zone, err := time.LoadLocation("Europe/Helsinki")
	require.NoError(t, err)

	f := &fixture{store: st, core: core.New(st), hub: api.NewHub(t.Context().Done()), zone: zone, areas: map[string]int64{}, dir: t.TempDir()}
	f.auth = auth.New(st, time.Now)
	f.clock.Store(start.UnixMilli())

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

// load serves the database again, with table as [modules.tasks], or none when nil.
func (f *fixture) load(t *testing.T, table object) {
	t.Helper()

	ms, err := f.loadModules(t, table)
	require.NoError(t, err)

	f.h = api.New(api.Deps{Auth: f.auth, Core: f.core, Modules: ms, PublicURL: "https://recall.example.org", Hub: f.hub})
	f.notify = nil

	for _, m := range ms.On() {
		if m.Name == "tasks" {
			f.notify = m.Jobs[0].Run
		}
	}
}

func (f *fixture) loadModules(t *testing.T, table object) (*core.Modules, error) {
	t.Helper()

	tables := map[string]map[string]any{"notes": {"enabled": true}, "bell": {"enabled": !f.muted}}
	if table != nil {
		tables["tasks"] = table
	}

	// Each reading of the clock is a millisecond on from the last, so no two writes share a time.
	now := func() time.Time { return time.UnixMilli(f.clock.Add(1)) }

	bell := func(module.Deps) module.Module {
		return module.Module{Name: "bell", Deliver: func(_ context.Context, userID int64, m module.Message) {
			f.rang = append(f.rang, rang{userID, m})

			if f.cut != nil {
				f.cut()
			}
		}}
	}

	return f.core.LoadModules(t.Context(), []func(module.Deps) module.Module{notes.New, tasks.New, bell}, &config.Config{Modules: tables},
		module.Deps{Now: now, Zone: f.zone, Dir: f.dir, Changed: f.hub.Changed})
}

// signIn gives ada's session, as do takes it.
func (f *fixture) signIn(t *testing.T) string {
	t.Helper()

	require.NoError(t, f.auth.SetPasswordHash(t.Context(), f.ada, passwordHash()))

	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/api/auth/login",
		strings.NewReader(`{"name":"ada","password":"`+password+`"}`))
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

// create makes the tasks of the body JSON with auth and returns them.
func (f *fixture) create(t *testing.T, body, auth string) []object {
	t.Helper()

	code, got := f.do(t, http.MethodPost, "/api/tasks", body, auth)
	require.Equal(t, http.StatusCreated, code, got)

	return objects(got["tasks"])
}

// one makes the one task of the JSON object draft with ada's laptop and returns it.
func (f *fixture) one(t *testing.T, draft string) object {
	t.Helper()

	return f.create(t, `{"tasks":[`+draft+`]}`, f.laptop)[0]
}

// change sends the change body to task and returns the task it answers.
func (f *fixture) change(t *testing.T, task object, body, auth string) object {
	t.Helper()

	code, got := f.do(t, http.MethodPatch, path(task), body, auth)
	require.Equal(t, http.StatusOK, code, got)

	return got
}

// read gives task in full as it now reads.
func (f *fixture) read(t *testing.T, task object, auth string) object {
	t.Helper()

	code, got := f.do(t, http.MethodGet, path(task), "", auth)
	require.Equal(t, http.StatusOK, code, got)

	return got
}

// titles are the titles of the tasks a list answers, in its order, and its total.
func (f *fixture) titles(t *testing.T, query, auth string) ([]string, int) {
	t.Helper()

	code, got := f.do(t, http.MethodGet, "/api/tasks"+query, "", auth)
	require.Equal(t, http.StatusOK, code, got)

	out := []string{}
	for _, r := range objects(got["tasks"]) {
		out = append(out, r["title"].(string))
	}

	return out, int(got["total"].(float64))
}

// changes are the changes of each entry of task's log, the newest first.
func (f *fixture) changes(t *testing.T, task object) []any {
	t.Helper()

	out := []any{}
	for _, e := range objects(f.read(t, task, f.laptop)["log"]) {
		out = append(out, e["changes"])
	}

	return out
}

func objects(v any) []object {
	out := []object{}
	for _, o := range v.([]any) {
		out = append(out, o.(object))
	}

	return out
}

func path(task object) string {
	return fmt.Sprintf("/api/tasks/%v", task["id"])
}

// field is one change of the log, with what it was and what it became.
func field(name, from, to string) object {
	return object{"field": name, "from": from, "to": to}
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

func TestTasksMadeInOneCallReadBackWithTheirPeriod(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	said := "next weekend I will tidy the garage, wash the car and buy materials"

	code, got := f.do(t, http.MethodPost, "/api/tasks", `{"said":"`+said+`","tasks":[
		{"title":"Tidy the garage","planned_from":"2026-11-28","planned_to":"2026-11-29"},
		{"title":"Wash the car","planned_from":"2026-11-28","planned_to":"2026-11-29","area":"Garden"},
		{"title":"Buy materials","planned_from":"2026-11-28","planned_to":"2026-11-29","list":"soon","notes":"Screws"}]}`, f.laptop)
	require.Equal(t, http.StatusCreated, code, got)
	assert.Equal(t, "2026-11-24", got["now"].(object)["date"])

	made := objects(got["tasks"])
	require.Len(t, made, 3)

	for i, want := range []struct{ title, area, list string }{
		{"Tidy the garage", "personal", ""}, {"Wash the car", "garden", ""}, {"Buy materials", "personal", "soon"},
	} {
		r := made[i]
		assert.Equal(t, "tasks:"+fmt.Sprint(r["id"]), r["ref"])
		assert.Equal(t, want.title, r["title"])
		assert.Equal(t, want.area, r["area"])
		assert.Equal(t, want.list, r["list"])
		assert.Equal(t, "2026-11-28", r["planned_from"])
		assert.Equal(t, "Saturday", r["planned_from_day"])
		assert.Equal(t, "2026-11-29", r["planned_to"])
		assert.Equal(t, "Sunday", r["planned_to_day"])
		assert.Empty(t, r["planned_time"])
		assert.Empty(t, r["deadline"], "no deadline is invented")
		assert.Empty(t, r["deadline_day"])
		assert.Equal(t, "open", r["state"])
		assert.Nil(t, r["done_at"])
		assert.Equal(t, i == 2, r["has_notes"])
		assert.Equal(t, "laptop", r["source"])
		assert.Equal(t, r["created_at"], r["changed_at"])

		at, err := time.Parse(time.RFC3339, r["created_at"].(string))
		require.NoError(t, err)
		assert.Equal(t, time.UTC, at.Location())
	}

	for _, r := range made {
		assert.Equal(t, said, f.read(t, r, f.phone)["said"], "said is kept on the making of each")
	}

	full := f.read(t, made[2], f.phone)
	for k, v := range made[2] {
		assert.Equal(t, v, full[k], k)
	}

	assert.Equal(t, "Screws", full["notes"])
	assert.Equal(t, []any{object{"at": made[2]["created_at"], "source": "laptop", "said": said, "changes": []any{
		field("title", "", "Buy materials"), field("area", "", "personal"), field("planned_from", "", "2026-11-28"),
		field("planned_to", "", "2026-11-29"), field("list", "", "soon"),
		object{"field": "notes"},
	}}}, full["log"], "the making lists each field the task was given")

	for _, tt := range []struct {
		body   string
		status int
	}{
		{`{"tasks":[{"title":"Rake"},{"title":"` + strings.Repeat("å", 201) + `"},{"title":"Sweep"}]}`, http.StatusBadRequest},
		{`{"tasks":[{"title":"Rake"},{"title":"Sweep"},{"title":"Mow","area":"nowhere"}]}`, http.StatusNotFound},
	} {
		code, got := f.do(t, http.MethodPost, "/api/tasks", tt.body, f.laptop)
		refused(t, tt.status, code, got)
	}

	assert.Equal(t, 3, f.count(t, "tasks"), "a call with one task refused saves none")
	assert.Equal(t, 3, f.count(t, "task_changes"))
}

func TestATaskMustBeOneTheModuleCanKeep(t *testing.T) {
	t.Parallel()

	f := newFixture(t)

	many := strings.TrimSuffix(strings.Repeat(`{"title":"Rake"},`, 51), ",")

	for _, tt := range []struct {
		body   string
		status int
	}{
		{`{"tasks":[{"title":"Rake","planned_from":"2026-11-29","planned_to":"2026-11-28"}]}`, http.StatusBadRequest},
		{`{"tasks":[{"title":"Rake","planned_time":"09:00"}]}`, http.StatusBadRequest},
		{`{"tasks":[{"title":"Rake","planned_to":"2026-11-28"}]}`, http.StatusBadRequest},
		{`{"tasks":[{"title":"Rake","planned_from":"2026-02-30"}]}`, http.StatusBadRequest},
		{`{"tasks":[{"title":"Rake","planned_from":"2026-2-3"}]}`, http.StatusBadRequest},
		{`{"tasks":[{"title":"Rake","deadline":"tomorrow"}]}`, http.StatusBadRequest},
		{`{"tasks":[{"title":"Rake","planned_from":"2026-11-28","planned_time":"9:30"}]}`, http.StatusBadRequest},
		{`{"tasks":[{"title":"Rake","planned_from":"2026-11-28","planned_time":"24:00"}]}`, http.StatusBadRequest},
		{`{"tasks":[{"title":"` + strings.Repeat("å", 201) + `"}]}`, http.StatusBadRequest},
		{`{"tasks":[{"title":"  "}]}`, http.StatusBadRequest},
		{`{"tasks":[{"title":"Two\nlines"}]}`, http.StatusBadRequest},
		{`{"tasks":[{"title":"Rake","list":"later"}]}`, http.StatusBadRequest},
		{`{"tasks":[{"title":"Rake","done":true}]}`, http.StatusBadRequest},
		{`{"tasks":[{"title":"Rake"}]} {}`, http.StatusBadRequest},
		{`{"tasks":[]}`, http.StatusBadRequest},
		{`{"said":"Rake"}`, http.StatusBadRequest},
		{`{"tasks":[` + many + `]}`, http.StatusBadRequest},
		{`{"tasks":[{"title":"Rake","area":"nowhere"}]}`, http.StatusNotFound},
	} {
		code, got := f.do(t, http.MethodPost, "/api/tasks", tt.body, f.laptop)
		refused(t, tt.status, code, got)
	}

	code, got := f.do(t, http.MethodPost, "/api/tasks", `{"tasks":[{"title":"Rake"}]}`, f.phone)
	refused(t, http.StatusForbidden, code, got)

	assert.Equal(t, 0, f.count(t, "tasks"), "a refused task leaves nothing behind")
	assert.Equal(t, 0, f.count(t, "task_changes"))

	day := f.one(t, `{"title":"Sweep","planned_from":"2026-11-25","planned_time":"09:30"}`)
	assert.Equal(t, "2026-11-25", day["planned_to"], "a from alone is one day")
	assert.Equal(t, "Wednesday", day["planned_to_day"])
	assert.Equal(t, "09:30", day["planned_time"])

	due := f.one(t, `{"title":"Pay","deadline":"2026-11-30"}`)
	assert.Equal(t, "Monday", due["deadline_day"])
	assert.Empty(t, due["planned_from"])
	assert.Empty(t, due["planned_from_day"])

	past := f.one(t, `{"title":"Late","planned_from":"2020-01-01"}`)
	assert.Equal(t, "2020-01-01", past["planned_from"], "a date in the past is kept as given")

	trimmed := f.one(t, `{"title":"  `+strings.Repeat("å", 200)+`  "}`)
	assert.Equal(t, strings.Repeat("å", 200), trimmed["title"])

	assert.Len(t, f.create(t, `{"tasks":[`+strings.TrimSuffix(many, `,{"title":"Rake"}`)+`]}`, f.laptop), 50)
}

func TestASpanGivesTheTasksOfItsDays(t *testing.T) {
	t.Parallel()

	f := newFixture(t)

	for _, draft := range []string{
		`{"title":"Last weekend","planned_from":"2026-11-21","planned_to":"2026-11-22"}`,
		`{"title":"Plan the trip","list":"soon"}`,
		`{"title":"Garage","planned_from":"2026-11-28","planned_to":"2026-11-29"}`,
		`{"title":"Car","planned_from":"2026-11-28","planned_to":"2026-11-29","list":"soon","area":"garden"}`,
		`{"title":"Shop","planned_from":"2026-11-24"}`,
		`{"title":"Dentist","planned_from":"2026-11-24","planned_time":"14:00"}`,
		`{"title":"Call","planned_from":"2026-11-24","planned_time":"09:00"}`,
		`{"title":"Invoice","deadline":"2026-11-25"}`,
		`{"title":"Report","planned_from":"2026-12-07","deadline":"2026-11-27"}`,
		`{"title":"Sail","list":"someday"}`,
		`{"title":"Loose end"}`,
	} {
		f.one(t, draft)
	}

	for _, tt := range []struct {
		query string
		want  []string
	}{
		{"", []string{"Last weekend", "Call", "Dentist", "Shop", "Invoice", "Garage", "Car", "Report", "Plan the trip", "Sail", "Loose end"}},
		{"?from=2026-11-29&to=2026-11-29", []string{"Garage", "Car"}},
		{"?from=2026-11-28&to=2026-11-29", []string{"Garage", "Car"}},
		{"?from=2026-11-24&to=2026-11-24", []string{"Call", "Dentist", "Shop"}},
		{"?to=2026-11-24", []string{"Last weekend", "Call", "Dentist", "Shop"}},
		{"?from=2026-11-25&to=2026-11-27", []string{"Invoice", "Report"}},
		{"?from=2026-12-01", []string{"Report"}},
		{"?from=2026-12-08", []string{}},
		{"?list=soon", []string{"Car", "Plan the trip"}},
		{"?list=soon&from=2026-11-28&to=2026-11-29", []string{"Car"}},
		{"?list=someday", []string{"Sail"}},
		{"?list=none", []string{"Last weekend", "Call", "Dentist", "Shop", "Invoice", "Garage", "Report", "Loose end"}},
		{"?area=Garden", []string{"Car"}},
		{"?area=personal&from=2026-11-28&to=2026-11-29", []string{"Garage"}},
	} {
		got, total := f.titles(t, tt.query, f.phone)
		assert.Equal(t, tt.want, got, tt.query)
		assert.Equal(t, len(tt.want), total, tt.query)
	}

	for _, tt := range []struct {
		query  string
		status int
	}{
		{"?from=2026-11-29&to=2026-11-28", http.StatusBadRequest},
		{"?from=2026-02-30", http.StatusBadRequest},
		{"?to=today", http.StatusBadRequest},
		{"?list=later", http.StatusBadRequest},
		{"?unsorted=yes", http.StatusBadRequest},
		{"?done=1", http.StatusBadRequest},
		{"?area=nowhere", http.StatusNotFound},
	} {
		code, got := f.do(t, http.MethodGet, "/api/tasks"+tt.query, "", f.phone)
		refused(t, tt.status, code, got)
	}
}

func TestATaskIsUnsortedUntilItHasAListAPlanOrADeadline(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	loose := f.one(t, `{"title":"Loose end"}`)
	f.one(t, `{"title":"Garage","planned_from":"2026-11-28"}`)

	assert.Equal(t, "unsorted", loose["state"])

	got, _ := f.titles(t, "?unsorted=true", f.phone)
	assert.Equal(t, []string{"Loose end"}, got)

	got, _ = f.titles(t, "?unsorted=false", f.phone)
	assert.Equal(t, []string{"Garage", "Loose end"}, got, "false asks for nothing")

	for _, tt := range []struct{ body, state string }{
		{`{"list":"soon"}`, "open"},
		{`{"list":""}`, "unsorted"},
		{`{"deadline":"2026-12-01"}`, "open"},
		{`{"deadline":""}`, "unsorted"},
		{`{"planned_from":"2026-12-01"}`, "open"},
		{`{"planned_from":""}`, "unsorted"},
		{`{"done":true}`, "done"},
	} {
		changed := f.change(t, loose, tt.body, f.laptop)
		assert.Equal(t, tt.state, changed["state"], tt.body)

		got, _ := f.titles(t, "?unsorted=true", f.phone)
		assert.Equal(t, tt.state == "unsorted", slices.Contains(got, "Loose end"), tt.body)
	}

	got, _ = f.titles(t, "?unsorted=true&done=true", f.phone)
	assert.Equal(t, []string{}, got, "a done task is never unsorted")
}

func TestATaskMarkedDoneKeepsItsPlace(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	garage := f.one(t, `{"title":"Garage","planned_from":"2026-11-28","planned_to":"2026-11-29","planned_time":"10:00","list":"soon","deadline":"2026-11-30"}`)
	car := f.one(t, `{"title":"Car","planned_from":"2026-11-28","planned_to":"2026-11-29"}`)

	done := f.change(t, garage, `{"done":true}`, f.laptop)
	assert.Equal(t, "done", done["state"])

	at, err := time.Parse(time.RFC3339, done["done_at"].(string))
	require.NoError(t, err)
	assert.Equal(t, done["changed_at"], done["done_at"])
	assert.Equal(t, time.UTC, at.Location())

	for _, k := range []string{"planned_from", "planned_to", "planned_time", "deadline", "list", "area"} {
		assert.Equal(t, garage[k], done[k], "done keeps %s", k)
	}

	for _, query := range []string{"", "?list=soon", "?from=2026-11-28&to=2026-11-29", "?to=2026-11-30", "?done=false"} {
		got, _ := f.titles(t, query, f.phone)
		assert.NotContains(t, got, "Garage", query)
	}

	got, _ := f.titles(t, "?done=true", f.phone)
	assert.Equal(t, []string{"Garage"}, got)

	again := f.change(t, garage, `{"done":true}`, f.desk)
	assert.Equal(t, done, again, "marking a done task done changes nothing")

	renamed := f.change(t, garage, `{"title":"Tidy the garage"}`, f.desk)
	assert.Equal(t, "done", renamed["state"])
	assert.Equal(t, done["done_at"], renamed["done_at"], "a change of a done task keeps when it was done")

	f.change(t, car, `{"done":true}`, f.laptop)

	got, _ = f.titles(t, "?done=true", f.phone)
	assert.Equal(t, []string{"Car", "Tidy the garage"}, got, "the most recently done first")

	got, _ = f.titles(t, "?done=true&list=soon", f.phone)
	assert.Equal(t, []string{"Tidy the garage"}, got)

	opened := f.change(t, garage, `{"done":false}`, f.laptop)
	assert.Equal(t, "open", opened["state"])
	assert.Nil(t, opened["done_at"])

	for _, k := range []string{"planned_from", "planned_to", "planned_time", "deadline", "list", "area"} {
		assert.Equal(t, garage[k], opened[k], "opened again, %s is as it was", k)
	}

	got, _ = f.titles(t, "?list=soon", f.phone)
	assert.Equal(t, []string{"Tidy the garage"}, got)

	assert.Equal(t, []any{
		[]any{field("done", "true", "false")}, []any{field("title", "Garage", "Tidy the garage")}, []any{field("done", "false", "true")},
	}, f.changes(t, garage)[:3])
}

func TestEachChangeIsLogged(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	garage := f.one(t, `{"title":"Garage","planned_from":"2026-11-28","planned_to":"2026-11-29","planned_time":"10:00"}`)

	steps := []struct {
		body string
		want []any
	}{
		{`{"title":" Tidy the garage "}`, []any{field("title", "Garage", "Tidy the garage")}},
		{`{"notes":"Shelves first"}`, []any{object{"field": "notes"}}},
		{`{"area":"GARDEN"}`, []any{field("area", "personal", "garden")}},
		{`{"planned_from":"2026-12-05"}`, []any{field("planned_from", "2026-11-28", "2026-12-05"), field("planned_to", "2026-11-29", "2026-12-05")}},
		{`{"planned_to":"2026-12-06"}`, []any{field("planned_to", "2026-12-05", "2026-12-06")}},
		{`{"planned_to":""}`, []any{field("planned_to", "2026-12-06", "2026-12-05")}},
		{`{"deadline":"2026-12-10","list":"someday"}`, []any{field("deadline", "", "2026-12-10"), field("list", "", "someday")}},
		{`{"planned_from":""}`, []any{field("planned_from", "2026-12-05", ""), field("planned_to", "2026-12-05", ""), field("planned_time", "10:00", "")}},
	}

	for _, s := range steps {
		changed := f.change(t, garage, s.body, f.desk)
		assert.Equal(t, "desk", changed["source"])

		entry := objects(changed["log"])[0]
		assert.Equal(t, object{"at": changed["changed_at"], "source": "desk", "said": "", "changes": s.want}, entry, s.body)
	}

	got := f.read(t, garage, f.laptop)
	assert.Equal(t, "Tidy the garage", got["title"])
	assert.Equal(t, "Shelves first", got["notes"])
	assert.Equal(t, true, got["has_notes"])
	assert.Equal(t, "garden", got["area"])
	assert.Equal(t, "2026-12-10", got["deadline"])
	assert.Empty(t, got["planned_from"])
	assert.Empty(t, got["planned_to"])
	assert.Empty(t, got["planned_time"], "clearing the plan clears its time")

	log := f.changes(t, garage)
	require.Len(t, log, len(steps)+1, "one entry for each change, and the making")

	for i, s := range steps {
		assert.Equal(t, s.want, log[len(steps)-1-i], "the newest first")
	}

	same := f.change(t, garage, `{"title":"Tidy the garage","area":"garden","list":"someday","done":false,"planned_from":""}`, f.laptop)
	assert.Equal(t, got, same, "a change that changes nothing")
	assert.Len(t, f.changes(t, garage), len(steps)+1, "writes no entry")

	for _, tt := range []struct {
		target, body string
		auth         string
		status       int
	}{
		{path(garage), `{}`, f.laptop, http.StatusBadRequest},
		{path(garage), `{"planned_time":"09:00"}`, f.laptop, http.StatusBadRequest},
		{path(garage), `{"planned_to":"2026-12-01"}`, f.laptop, http.StatusBadRequest},
		{path(garage), `{"planned_from":"2026-12-02","planned_to":"2026-12-01"}`, f.laptop, http.StatusBadRequest},
		{path(garage), `{"title":""}`, f.laptop, http.StatusBadRequest},
		{path(garage), `{"deadline":"2026-13-01"}`, f.laptop, http.StatusBadRequest},
		{path(garage), `{"list":"none"}`, f.laptop, http.StatusBadRequest},
		{path(garage), `{"area":"nowhere"}`, f.laptop, http.StatusNotFound},
		{path(garage), `{"title":"Lost"}`, f.phone, http.StatusForbidden},
		{"/api/tasks/9999", `{"title":"Lost"}`, f.laptop, http.StatusNotFound},
		{"/api/tasks/x", `{"title":"Lost"}`, f.laptop, http.StatusNotFound},
	} {
		code, answer := f.do(t, http.MethodPatch, tt.target, tt.body, tt.auth)
		refused(t, tt.status, code, answer)
	}

	assert.Equal(t, got, f.read(t, garage, f.laptop), "a refused change changes nothing")
	assert.Len(t, f.changes(t, garage), len(steps)+1)
}

func TestNowIsTodayInTheConfiguredZone(t *testing.T) {
	t.Parallel()

	f := newFixture(t)

	code, got := f.do(t, http.MethodGet, "/api/tasks", "", f.phone)
	require.Equal(t, http.StatusOK, code, got)
	assert.Equal(t, object{"date": "2026-11-24", "weekday": "Tuesday", "time": "00:30", "timezone": "Europe/Helsinki", "week_start": "monday"},
		got["now"], "Tuesday in Helsinki while it is still Monday in UTC")

	now := got["now"]
	garage := f.one(t, `{"title":"Garage"}`)

	for _, full := range []object{
		f.read(t, garage, f.phone), f.change(t, garage, `{"title":"Tidy the garage"}`, f.laptop),
		f.trash(t, garage, "/trash", f.laptop), f.trash(t, garage, "/restore", f.laptop),
	} {
		assert.Equal(t, now, full["now"], "a task in full, read, changed, trashed or restored, tells it too")
	}

	f.load(t, object{"enabled": true, "week_start": "sunday"})

	_, got = f.do(t, http.MethodGet, "/api/tasks", "", f.phone)
	assert.Equal(t, "sunday", got["now"].(object)["week_start"])
	assert.Equal(t, "sunday", f.read(t, garage, f.phone)["now"].(object)["week_start"])

	for _, start := range []any{"someday", "Monday", "", int64(1)} {
		for _, table := range []object{{"enabled": true, "week_start": start}, {"week_start": start}} {
			_, err := f.loadModules(t, table)
			require.Error(t, err, "on or off")
			assert.Contains(t, err.Error(), "modules.tasks.week_start")
		}
	}
}

func TestTheListGivesAtMostFiftyUnlessAsked(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	f.create(t, `{"tasks":[`+strings.TrimSuffix(strings.Repeat(`{"title":"Rake"},`, 50), ",")+`]}`, f.laptop)
	f.one(t, `{"title":"Sweep"}`)

	got, total := f.titles(t, "", f.phone)
	assert.Len(t, got, 50)
	assert.Equal(t, 51, total)

	got, total = f.titles(t, "?limit=500", f.phone)
	assert.Len(t, got, 51)
	assert.Equal(t, 51, total)

	got, total = f.titles(t, "?limit=2&unsorted=true", f.phone)
	assert.Equal(t, []string{"Rake", "Rake"}, got)
	assert.Equal(t, 51, total)

	got, total = f.titles(t, "?limit=2&offset=50", f.phone)
	assert.Equal(t, []string{"Sweep"}, got)
	assert.Equal(t, 51, total)

	got, total = f.titles(t, "?limit=2&offset=51", f.phone)
	assert.Empty(t, got)
	assert.Equal(t, 51, total)

	for _, offset := range []string{"-1", "many"} {
		code, answer := f.do(t, http.MethodGet, "/api/tasks?offset="+offset, "", f.phone)
		refused(t, http.StatusBadRequest, code, answer)
	}

	for _, limit := range []string{"0", "-1", "501", "many"} {
		code, answer := f.do(t, http.MethodGet, "/api/tasks?limit="+limit, "", f.phone)
		refused(t, http.StatusBadRequest, code, answer)
	}
}

func TestATaskOfOneUserIsFoundByNoRouteOfAnother(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	mine := f.one(t, `{"title":"Garage","area":"garden","planned_from":"2026-11-28"}`)
	mine = f.change(t, mine, `{"done":true}`, f.laptop)

	for _, query := range []string{"", "?done=true", "?from=2026-11-28&to=2026-11-28", "?unsorted=true"} {
		got, total := f.titles(t, query, f.grace)
		assert.Equal(t, []string{}, got, query)
		assert.Zero(t, total, "a total counts the caller's tasks alone")
	}

	for _, r := range []struct{ method, target, body string }{
		{http.MethodGet, "/api/tasks?area=garden", ""},
		{http.MethodGet, "/api/tasks?trashed=true&area=garden", ""},
		{http.MethodGet, path(mine), ""},
		{http.MethodPatch, path(mine), `{"title":"Mine now"}`},
		{http.MethodPatch, path(mine), `{"done":false}`},
		{http.MethodPost, path(mine) + "/trash", ""},
		{http.MethodPost, "/api/tasks", `{"tasks":[{"title":"Lost","area":"garden"}]}`},
	} {
		code, answer := f.do(t, r.method, r.target, r.body, f.grace)
		refused(t, http.StatusNotFound, code, answer)
	}

	theirs := f.create(t, `{"tasks":[{"title":"Orchard"}]}`, f.grace)[0]
	assert.Equal(t, "personal", theirs["area"])
	assert.Equal(t, "tablet", theirs["source"])

	code, answer := f.do(t, http.MethodPatch, path(theirs), `{"area":"garden"}`, f.grace)
	refused(t, http.StatusNotFound, code, answer)

	code, answer = f.do(t, http.MethodGet, path(theirs), "", f.laptop)
	refused(t, http.StatusNotFound, code, answer)

	code, answer = f.do(t, http.MethodPatch, path(theirs), `{"title":"Mine now"}`, f.laptop)
	refused(t, http.StatusNotFound, code, answer)

	got, _ := f.titles(t, "", f.grace)
	assert.Equal(t, []string{"Orchard"}, got)

	assert.Equal(t, mine, f.read(t, mine, f.laptop), "her tries changed nothing")
}

func TestAModuleSwitchedOffKeepsItsTasks(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	made := f.create(t, `{"tasks":[{"title":"Garage","planned_from":"2026-11-28"},{"title":"Car","area":"garden"}]}`, f.laptop)
	f.change(t, made[0], `{"done":true}`, f.desk)

	want := []object{f.read(t, made[0], f.laptop), f.read(t, made[1], f.laptop)}

	assert.Equal(t, []string{"Garage"}, f.found(t, "garage", f.phone))

	for _, table := range []object{{"enabled": false}, nil} {
		f.load(t, table)

		assert.NotContains(t, f.hits(t, "garage", "", f.phone), "tasks", "a module switched off finds nothing")

		for _, r := range []struct{ method, target, body string }{
			{http.MethodGet, "/api/tasks", ""},
			{http.MethodPost, "/api/tasks", `{"tasks":[{"title":"Lost"}]}`},
			{http.MethodGet, path(made[0]), ""},
			{http.MethodPatch, path(made[0]), `{"title":"Lost"}`},
			{http.MethodPost, path(made[0]) + "/trash", ""},
			{http.MethodPost, path(made[0]) + "/restore", ""},
			{http.MethodDelete, path(made[0]), ""},
			{http.MethodDelete, "/api/tasks/trash", ""},
		} {
			code, got := f.do(t, r.method, r.target, r.body, f.laptop)
			refused(t, http.StatusNotFound, code, got)
		}

		assert.Equal(t, 2, f.count(t, "tasks"))
		assert.Equal(t, 3, f.count(t, "task_changes"))
	}

	f.load(t, object{"enabled": true})

	for i, task := range made {
		assert.Equal(t, want[i], f.read(t, task, f.laptop))
	}

	assert.Equal(t, []string{"Garage"}, f.found(t, "garage", f.phone))
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

	expect := func(task object, area string) {
		t.Helper()
		assert.JSONEq(t, fmt.Sprintf(`{"module":"tasks","kind":"task","id":%v,"area":%d}`, task["id"], f.areas[area]),
			strings.TrimPrefix(next(), "data: "))
	}

	made := f.create(t, `{"tasks":[{"title":"Garage"},{"title":"Car","area":"garden"}]}`, f.laptop)
	expect(made[0], "personal")
	expect(made[1], "garden")
	expect(makes(t, f.client(t, f.laptop), object{"tasks": []object{{"title": "Rake", "area": "garden"}}})[0], "garden")

	code, got := f.do(t, http.MethodPatch, path(made[0]), `{"planned_time":"09:00"}`, f.laptop)
	refused(t, http.StatusBadRequest, code, got)

	code, got = f.do(t, http.MethodPost, "/api/tasks", `{"tasks":[{"title":"Rake"},{"title":""}]}`, f.laptop)
	refused(t, http.StatusBadRequest, code, got)

	expect(f.change(t, made[0], `{"area":"garden"}`, f.laptop), "garden")
	f.change(t, made[0], `{"area":"garden"}`, f.laptop)
	expect(f.change(t, made[1], `{"done":true}`, f.desk), "garden") // and not the change that changed nothing

	expect(f.trash(t, made[0], "/trash", f.laptop), "garden")
	f.trash(t, made[0], "/trash", f.laptop)
	expect(f.trash(t, made[0], "/restore", f.laptop), "garden") // and not the trashing that changed nothing
	f.trash(t, made[0], "/restore", f.laptop)
	expect(f.trash(t, made[0], "/trash", f.laptop), "garden") // nor the restoring

	session := f.signIn(t)
	code, got = f.do(t, http.MethodDelete, path(made[0]), "", session)
	require.Equal(t, http.StatusNoContent, code, got)
	expect(made[0], "garden")

	expect(f.trash(t, made[1], "/trash", f.laptop), "garden")

	code, got = f.do(t, http.MethodDelete, "/api/tasks/trash", "", session)
	require.Equal(t, http.StatusNoContent, code, got)
	assert.JSONEq(t, `{"module":"tasks","kind":"trash","id":0,"area":0}`, strings.TrimPrefix(next(), "data: "), "emptying tells once")

	code, got = f.do(t, http.MethodPatch, "/api/tasks/notify", `{"morning_at":"7:00"}`, session)
	refused(t, http.StatusBadRequest, code, got)
	expect(f.create(t, `{"tasks":[{"title":"Sweep"}]}`, f.laptop)[0], "personal") // and not the refused change of the kinds

	code, got = f.do(t, http.MethodPatch, "/api/tasks/notify", `{"morning":false}`, session)
	require.Equal(t, http.StatusOK, code, got)
	assert.JSONEq(t, `{"module":"tasks","kind":"notify","id":0,"area":0}`, strings.TrimPrefix(next(), "data: "), "a change of the kinds")
}

// trash sends task to the trash, or out of it with "/restore", and returns the task it answers.
func (f *fixture) trash(t *testing.T, task object, to, auth string) object {
	t.Helper()

	code, got := f.do(t, http.MethodPost, path(task)+to, "", auth)
	require.Equal(t, http.StatusOK, code, got)

	return got
}

func TestATrashedTaskIsInNoListButTheTrash(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	garage := f.change(t, f.one(t, `{"title":"Garage","planned_from":"2026-11-28","list":"soon","area":"garden"}`), `{"notes":"Shelves"}`, f.desk)
	car := f.change(t, f.one(t, `{"title":"Car"}`), `{"done":true}`, f.laptop)
	f.one(t, `{"title":"Shop","planned_from":"2026-11-28"}`)
	loose := f.one(t, `{"title":"Loose end"}`)

	assert.Nil(t, garage["trashed_at"], "null unless the task is in the trash")

	f.trash(t, loose, "/trash", f.laptop)
	f.trash(t, car, "/trash", f.laptop)

	trashed := f.trash(t, garage, "/trash", f.laptop)
	require.NotNil(t, trashed["trashed_at"])

	at, err := time.Parse(time.RFC3339, trashed["trashed_at"].(string))
	require.NoError(t, err)
	assert.Equal(t, time.UTC, at.Location())

	want := maps.Clone(garage)
	want["trashed_at"] = trashed["trashed_at"]
	assert.Equal(t, want, trashed, "trashing changes nothing else: no entry of the log, source or time of change")
	assert.Equal(t, trashed, f.trash(t, garage, "/trash", f.desk), "trashing a trashed task changes nothing")

	for _, tt := range []struct {
		query string
		want  []string
	}{
		{"", []string{"Shop"}},
		{"?list=soon", []string{}},
		{"?area=garden", []string{}},
		{"?unsorted=true", []string{}},
		{"?from=2026-11-28&to=2026-11-28", []string{"Shop"}},
		{"?done=true", []string{}},
		{"?trashed=false", []string{"Shop"}},
		{"?trashed=true", []string{"Garage", "Car", "Loose end"}},
		{"?trashed=true&done=true", []string{"Car"}},
		{"?trashed=true&area=garden", []string{"Garage"}},
		{"?trashed=true&list=soon", []string{"Garage"}},
		{"?trashed=true&from=2026-11-28&to=2026-11-28", []string{"Garage"}},
		{"?trashed=true&unsorted=true", []string{"Loose end"}},
	} {
		listed, total := f.titles(t, tt.query, f.phone)
		assert.Equal(t, tt.want, listed, tt.query)
		assert.Equal(t, len(tt.want), total, tt.query)
	}

	listed, total := f.titles(t, "?trashed=true&limit=1", f.phone)
	assert.Equal(t, []string{"Garage"}, listed, "the most recently trashed first")
	assert.Equal(t, 3, total, "the trash's count, done and open alike")

	assert.Equal(t, trashed, f.read(t, garage, f.phone), "a trashed task is still read")

	for _, body := range []string{`{"title":"Lost"}`, `{"done":true}`, `{"title":"Garage"}`} {
		code, got := f.do(t, http.MethodPatch, path(garage), body, f.laptop)
		refused(t, http.StatusConflict, code, got)
	}

	code, got := f.do(t, http.MethodGet, "/api/tasks?trashed=yes", "", f.phone)
	refused(t, http.StatusBadRequest, code, got)

	restored := f.trash(t, garage, "/restore", f.laptop)
	assert.Equal(t, garage, restored, "restored, it is as it was, with its log")
	assert.Equal(t, restored, f.trash(t, garage, "/restore", f.desk), "restoring a task outside the trash changes nothing")
	assert.Len(t, restored["log"], 2, "trashing and restoring write no entry")

	listed, _ = f.titles(t, "?list=soon", f.phone)
	assert.Equal(t, []string{"Garage"}, listed, "back in its list")

	for _, to := range []string{"/trash", "/restore"} {
		code, got = f.do(t, http.MethodPost, path(garage)+to, "", f.phone)
		refused(t, http.StatusForbidden, code, got)

		code, got = f.do(t, http.MethodPost, path(garage)+to, "", f.grace)
		refused(t, http.StatusNotFound, code, got)

		code, got = f.do(t, http.MethodPost, "/api/tasks/9999"+to, "", f.laptop)
		refused(t, http.StatusNotFound, code, got)
	}

	assert.Equal(t, garage, f.read(t, garage, f.phone), "the refused tries changed nothing")
}

func TestOnlyASessionDeletesFromTheTrash(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	session := f.signIn(t)

	garage := f.change(t, f.one(t, `{"title":"Garage"}`), `{"notes":"Shelves"}`, f.laptop)
	car := f.one(t, `{"title":"Car","area":"garden"}`)
	shop := f.change(t, f.one(t, `{"title":"Shop"}`), `{"done":true}`, f.laptop)
	f.one(t, `{"title":"Kept"}`)
	theirs := f.create(t, `{"tasks":[{"title":"Theirs"}]}`, f.grace)[0]

	code, got := f.do(t, http.MethodDelete, path(garage), "", session)
	refused(t, http.StatusConflict, code, got)

	for _, task := range []object{garage, car, shop} {
		f.trash(t, task, "/trash", f.laptop)
	}

	f.trash(t, theirs, "/trash", f.grace)

	for _, token := range []string{f.laptop, f.phone} {
		for _, target := range []string{path(garage), "/api/tasks/trash"} {
			code, got := f.do(t, http.MethodDelete, target, "", token)
			refused(t, http.StatusForbidden, code, got)
		}
	}

	listed, _ := f.titles(t, "?trashed=true", f.phone)
	assert.Equal(t, []string{"Shop", "Car", "Garage"}, listed, "no token deleted anything")

	code, got = f.do(t, http.MethodDelete, path(garage), "", session)
	require.Equal(t, http.StatusNoContent, code, got)

	code, got = f.do(t, http.MethodGet, path(garage), "", f.phone)
	refused(t, http.StatusNotFound, code, got)

	var entries int
	require.NoError(t, f.store.QueryRowContext(t.Context(), `SELECT count(*) FROM task_changes WHERE task_id = ?`, garage["id"]).Scan(&entries))
	assert.Zero(t, entries, "its log goes with it")

	for _, target := range []string{path(garage), "/api/tasks/9999", "/api/tasks/garage", path(theirs)} {
		code, got = f.do(t, http.MethodDelete, target, "", session)
		refused(t, http.StatusNotFound, code, got)
	}

	code, got = f.do(t, http.MethodDelete, "/api/tasks/trash", "", session)
	require.Equal(t, http.StatusNoContent, code, got)

	listed, total := f.titles(t, "?trashed=true", f.phone)
	assert.Equal(t, []string{}, listed)
	assert.Zero(t, total)

	listed, _ = f.titles(t, "", f.phone)
	assert.Equal(t, []string{"Kept"}, listed, "emptying deletes no task outside the trash")

	listed, _ = f.titles(t, "?trashed=true", f.grace)
	assert.Equal(t, []string{"Theirs"}, listed, "nor another user's trash")

	assert.Equal(t, 2, f.count(t, "tasks"))
	assert.Equal(t, 2, f.count(t, "task_changes"))

	code, _ = f.do(t, http.MethodDelete, "/api/tasks/trash", "", session)
	assert.Equal(t, http.StatusNoContent, code, "an empty trash empties")

	last := f.trash(t, f.one(t, `{"title":"Last"}`), "/trash", f.laptop)

	code, got = f.do(t, http.MethodDelete, path(last), "", session)
	require.Equal(t, http.StatusNoContent, code, got)
	assert.NotEqual(t, last["id"], f.one(t, `{"title":"Next"}`)["id"], "the id of a task deleted for good is never given again")
}
