package tasks_test

import (
	"encoding/json"
	"maps"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// today is the fixture's today in its zone, while it is still the day before in UTC.
const today = "2026-11-24"

// client connects the SDK's client to /mcp, as a client sending auth does.
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

// use calls the tool through cs, which must answer, and decodes its answer.
func use(t *testing.T, cs *mcp.ClientSession, tool string, args object) object {
	t.Helper()

	res, err := cs.CallTool(t.Context(), &mcp.CallToolParams{Name: tool, Arguments: args})
	require.NoError(t, err)
	require.False(t, res.IsError, res.Content)

	b, err := json.Marshal(res.StructuredContent)
	require.NoError(t, err)

	var got object
	require.NoError(t, json.Unmarshal(b, &got))

	return got
}

// fails calls the tool through cs, which must answer an error result of one
// line, and gives the line.
func fails(t *testing.T, cs *mcp.ClientSession, tool string, args object) string {
	t.Helper()

	res, err := cs.CallTool(t.Context(), &mcp.CallToolParams{Name: tool, Arguments: args})
	require.NoError(t, err)
	require.True(t, res.IsError, res.StructuredContent)
	require.Len(t, res.Content, 1)

	text, ok := res.Content[0].(*mcp.TextContent)
	require.True(t, ok, "%T", res.Content[0])
	assert.NotEmpty(t, text.Text)
	assert.NotContains(t, text.Text, "\n")

	return text.Text
}

// listed is task as a list gives it: in full, less what only a task in full gives.
func listed(task object) object {
	out := maps.Clone(task)
	for _, key := range []string{"notes", "said", "links", "linked_from", "log", "now"} {
		delete(out, key)
	}

	return out
}

// makes calls create_tasks through cs and gives the tasks it made.
func makes(t *testing.T, cs *mcp.ClientSession, args object) []object {
	t.Helper()

	return objects(use(t, cs, "create_tasks", args)["tasks"])
}

func TestTheToolsOfTasksAreThereWhileItIsOn(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	names := func() []string {
		res, err := f.client(t, f.phone).ListTools(t.Context(), nil)
		require.NoError(t, err)

		out := []string{}
		for _, tl := range res.Tools {
			out = append(out, tl.Name)
		}

		return out
	}
	others := []string{"list_areas", "search", "create_note", "list_notes", "get_note", "update_note", "trash_note"}

	assert.ElementsMatch(t, slices.Concat(others, []string{"create_tasks", "list_tasks", "get_task", "update_task", "trash_task"}), names())

	for _, table := range []object{{"enabled": false}, nil} {
		f.load(t, table)
		assert.ElementsMatch(t, others, names())
	}
}

func TestTasksSaidAtOnceAreMadeInOneCall(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	said := "next weekend I will tidy the garage, wash the car and buy materials"

	got := use(t, f.client(t, f.laptop), "create_tasks", object{"today": today, "said": said, "tasks": []object{
		{"title": "Tidy the garage", "planned_from": "2026-11-28", "planned_to": "2026-11-29"},
		{"title": "Wash the car", "planned_from": "2026-11-28", "planned_to": "2026-11-29", "area": "Garden"},
		{"title": "Buy materials", "planned_from": "2026-11-28", "planned_to": "2026-11-29", "list": "soon", "notes": "Screws"},
	}})

	code, list := f.do(t, http.MethodGet, "/api/tasks?from=2026-11-28&to=2026-11-29", "", f.phone)
	require.Equal(t, http.StatusOK, code, list)
	assert.Equal(t, object{"tasks": list["tasks"], "now": list["now"]}, got, "it gives the tasks as a list gives them, and now")

	made := objects(got["tasks"])
	require.Len(t, made, 3)

	for i, area := range []string{"personal", "garden", "personal"} {
		assert.Equal(t, object{
			"area": area, "planned_from": "2026-11-28", "planned_from_day": "Saturday", "planned_to": "2026-11-29", "planned_to_day": "Sunday",
			"deadline": "", "source": "laptop",
		}, pick(made[i], "area", "planned_from", "planned_from_day", "planned_to", "planned_to_day", "deadline", "source"))

		full := f.read(t, made[i], f.phone)
		assert.Equal(t, said, full["said"])

		log := objects(full["log"])
		require.Len(t, log, 1)
		assert.Equal(t, object{"source": "laptop", "said": said}, pick(log[0], "source", "said"), "said is kept on the making of each")
	}
}

func TestACallThatSetsADateGivesToday(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	cs := f.client(t, f.laptop)
	made := makes(t, cs, object{"tasks": []object{{"title": "Garage"}, {"title": "Car", "list": "soon"}}})
	assert.Equal(t, []any{"unsorted", "open"}, []any{made[0]["state"], made[1]["state"]}, "a call that sets no date needs no today")

	ref := made[0]["ref"]
	before := f.read(t, made[0], f.phone)

	for _, day := range []any{nil, "2026-11-23", "2026-11-25", "24.11.2026"} {
		for _, call := range []struct {
			tool string
			args object
		}{
			{"create_tasks", object{"tasks": []object{{"title": "Rake"}, {"title": "Mow", "planned_from": "2026-11-28"}}}},
			{"create_tasks", object{"tasks": []object{{"title": "Mow", "deadline": "2026-11-30"}}}},
			{"update_task", object{"ref": ref, "planned_from": "2026-11-28"}},
			{"update_task", object{"ref": ref, "planned_to": "2026-11-29"}},
			{"update_task", object{"ref": ref, "deadline": "2026-11-30", "title": "Tidy the garage"}},
		} {
			args := maps.Clone(call.args)
			if day != nil {
				args["today"] = day
			}

			line := fails(t, cs, call.tool, args)
			for _, want := range []string{today, "Tuesday", "Europe/Helsinki", "monday"} {
				assert.Contains(t, line, want, "%s with today %v", call.tool, day)
			}
		}
	}

	assert.Equal(t, 2, f.count(t, "tasks"), "nothing is saved")
	assert.Equal(t, before, f.read(t, made[0], f.phone), "nothing changes")

	planned := use(t, cs, "update_task", object{"ref": ref, "today": today, "planned_from": "2026-11-28", "deadline": "2026-11-30"})
	assert.Equal(t, object{"planned_from": "2026-11-28", "planned_to": "2026-11-28", "deadline": "2026-11-30"},
		pick(planned, "planned_from", "planned_to", "deadline"))

	cleared := use(t, cs, "update_task", object{"ref": ref, "planned_from": "", "deadline": ""})
	assert.Equal(t, object{"planned_from": "", "planned_to": "", "deadline": "", "state": "unsorted"},
		pick(cleared, "planned_from", "planned_to", "deadline", "state"), "clearing a date needs no today")

	f.load(t, object{"enabled": true, "week_start": "sunday"})
	assert.Contains(t, fails(t, f.client(t, f.laptop), "create_tasks", object{"tasks": []object{{"title": "Mow", "planned_from": "2026-11-28"}}}),
		"sunday", "the line gives the first day of the week as configured")
}

func TestATextOverTheCapIsRefused(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	f.load(t, object{"enabled": true, "capture_max_chars": int64(12)})
	cs := f.client(t, f.laptop)
	exact, over := strings.Repeat("æøå", 4), strings.Repeat("æøå", 4)+"Æ"

	for _, args := range []object{
		{"tasks": []object{{"title": "Rake", "notes": exact}, {"title": "Mow", "notes": over}}},
		{"tasks": []object{{"title": "Rake"}}, "said": over},
	} {
		assert.Contains(t, fails(t, cs, "create_tasks", args), "12", "the line names the cap")
	}

	fails(t, cs, "create_tasks", object{"tasks": slices.Repeat([]object{{"title": "Rake"}}, 51)})
	fails(t, cs, "create_tasks", object{"tasks": []object{{"title": strings.Repeat("ø", 201)}}})
	assert.Zero(t, f.count(t, "tasks"), "nothing is saved")

	made := makes(t, cs, object{"tasks": []object{{"title": "Tidy the garage before winter", "notes": exact}}, "said": exact})[0]
	full := f.read(t, made, f.phone)
	assert.Equal(t, []any{exact, exact}, []any{full["notes"], full["said"]}, "texts of exactly the cap, in letters of two bytes each, and a title has its own")

	assert.Contains(t, fails(t, cs, "update_task", object{"ref": made["ref"], "notes": over}), "12")
	assert.Equal(t, full, f.read(t, made, f.phone), "nothing changes")

	rest := f.create(t, `{"tasks":[{"title":"Rake","notes":"`+over+`"}],"said":"`+over+`"}`, f.laptop)[0]
	assert.Equal(t, over+over, f.change(t, rest, `{"notes":"`+over+over+`"}`, f.laptop)["notes"], "REST is not capped")

	f.load(t, object{"enabled": true})
	assert.Contains(t, fails(t, f.client(t, f.laptop), "create_tasks", object{"tasks": []object{{"title": "Rake", "notes": strings.Repeat("ø", 4001)}}}),
		"4000", "the cap is 4000 unless set")

	for _, chars := range []any{int64(0), int64(-1), "many"} {
		for _, table := range []object{{"enabled": true, "capture_max_chars": chars}, {"capture_max_chars": chars}} {
			_, err := f.loadModules(t, table)
			require.Error(t, err, "on or off")
			assert.Contains(t, err.Error(), "modules.tasks.capture_max_chars")
		}
	}
}

func TestListTasksGivesWhatRESTGives(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	made := f.create(t, `{"tasks":[
		{"title":"Tidy the garage","planned_from":"2026-11-28","planned_to":"2026-11-29"},
		{"title":"Wash the car","planned_from":"2026-11-28","planned_to":"2026-11-29","area":"garden"},
		{"title":"Buy materials","deadline":"2026-11-29","area":"garden","list":"soon"},
		{"title":"Paint the shed","planned_from":"2026-12-05"},
		{"title":"Rake"}]}`, f.laptop)
	f.change(t, made[3], `{"done":true}`, f.laptop)

	cs := f.client(t, f.phone)

	for _, tt := range []struct {
		args  object
		query string
		want  []string
	}{
		{nil, "", []string{"Tidy the garage", "Wash the car", "Buy materials", "Rake"}},
		{object{"from": "2026-11-28", "to": "2026-11-29"}, "?from=2026-11-28&to=2026-11-29", []string{"Tidy the garage", "Wash the car", "Buy materials"}},
		{object{"from": "2026-11-28", "to": "2026-11-29", "area": "Garden"}, "?from=2026-11-28&to=2026-11-29&area=Garden", []string{"Wash the car", "Buy materials"}},
		{object{"to": "2026-11-28"}, "?to=2026-11-28", []string{"Tidy the garage", "Wash the car"}},
		{object{"list": "soon"}, "?list=soon", []string{"Buy materials"}},
		{object{"list": "none", "area": "personal"}, "?list=none&area=personal", []string{"Tidy the garage", "Rake"}},
		{object{"unsorted": true}, "?unsorted=true", []string{"Rake"}},
		{object{"done": true}, "?done=true", []string{"Paint the shed"}},
		{object{"limit": 1}, "?limit=1", []string{"Tidy the garage"}},
	} {
		got := use(t, cs, "list_tasks", tt.args)

		code, want := f.do(t, http.MethodGet, "/api/tasks"+tt.query, "", f.phone)
		require.Equal(t, http.StatusOK, code, want)
		assert.Equal(t, want, got, tt.args)

		titles := []string{}
		for _, r := range objects(got["tasks"]) {
			titles = append(titles, r["title"].(string))
		}

		assert.Equal(t, tt.want, titles, tt.args)
	}

	for _, tt := range []struct {
		args  object
		query string
	}{
		{object{"list": "next"}, "?list=next"},
		{object{"from": "2026-11-31"}, "?from=2026-11-31"},
		{object{"from": "2026-11-29", "to": "2026-11-28"}, "?from=2026-11-29&to=2026-11-28"},
		{object{"area": "nowhere"}, "?area=nowhere"},
		{object{"limit": 501}, "?limit=501"},
	} {
		code, want := f.do(t, http.MethodGet, "/api/tasks"+tt.query, "", f.phone)
		require.NotEqual(t, http.StatusOK, code, want)
		assert.Equal(t, want["error"], fails(t, cs, "list_tasks", tt.args), "the line REST answers")
	}
}

func TestUpdateTaskChangesATaskAsRESTDoes(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	made := makes(t, f.client(t, f.laptop), object{"tasks": []object{{"title": "Garage"}}})[0]
	cs := f.client(t, f.desk)
	ref := made["ref"]

	planned := use(t, cs, "update_task", object{
		"ref": ref, "today": today, "planned_from": "2026-11-28", "planned_to": "2026-11-29", "planned_time": "09:00", "deadline": "2026-12-01",
	})
	assert.Equal(t, f.read(t, made, f.phone), planned, "it gives the task in full")
	assert.Equal(t, object{"planned_to_day": "Sunday", "deadline_day": "Tuesday", "state": "open", "source": "desk"},
		pick(planned, "planned_to_day", "deadline_day", "state", "source"))

	again := use(t, cs, "update_task", object{"ref": ref, "today": today, "planned_from": "2026-12-05"})
	assert.Equal(t, object{"planned_from": "2026-12-05", "planned_to": "2026-12-05", "planned_time": "09:00", "deadline": "2026-12-01"},
		pick(again, "planned_from", "planned_to", "planned_time", "deadline"), "planned again for one day")

	done := use(t, cs, "update_task", object{"ref": ref, "done": true})
	assert.Equal(t, "done", done["state"])
	assert.NotNil(t, done["done_at"])

	opened := use(t, cs, "update_task", object{"ref": ref, "done": false})
	assert.Equal(t, pick(again, "planned_from", "planned_to", "planned_time", "deadline", "state", "done_at"),
		pick(opened, "planned_from", "planned_to", "planned_time", "deadline", "state", "done_at"), "opened again, it is where it was")

	cleared := use(t, cs, "update_task", object{"ref": ref, "planned_from": "", "deadline": ""})
	assert.Equal(t, object{"planned_from": "", "planned_to": "", "planned_time": "", "deadline": "", "state": "unsorted"},
		pick(cleared, "planned_from", "planned_to", "planned_time", "deadline", "state"))

	assert.Len(t, f.changes(t, made), 6, "each change is logged")

	code, got := f.do(t, http.MethodPatch, path(made), `{"planned_time":"09:00"}`, f.laptop)
	require.Equal(t, http.StatusBadRequest, code, got)
	assert.Equal(t, got["error"], fails(t, cs, "update_task", object{"ref": ref, "planned_time": "09:00"}), "the line REST answers")

	fails(t, cs, "update_task", object{"ref": ref})

	code, got = f.do(t, http.MethodGet, "/api/tasks/9999", "", f.phone)
	require.Equal(t, http.StatusNotFound, code, got)
	assert.Equal(t, got["error"], fails(t, cs, "get_task", object{"ref": "tasks:9999"}))

	for _, bad := range []string{"notes:1", "tasks:x", "tasks:", "1", ""} {
		fails(t, cs, "get_task", object{"ref": bad})
	}

	assert.Equal(t, f.read(t, made, f.phone), use(t, cs, "get_task", object{"ref": ref}), "get_task gives the task in full")
}

func TestAReadTokenChangesNothingThroughATool(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	made := f.one(t, `{"title":"Garage","area":"garden"}`)
	want := f.read(t, made, f.phone)
	cs := f.client(t, f.phone)

	for _, call := range []struct {
		tool string
		args object
	}{
		{"create_tasks", object{"tasks": []object{{"title": "Lost"}}}},
		{"update_task", object{"ref": made["ref"], "title": "Lost"}},
		{"update_task", object{"ref": made["ref"], "done": true}},
		{"trash_task", object{"ref": made["ref"]}},
	} {
		fails(t, cs, call.tool, call.args)
	}

	assert.Equal(t, 1, f.count(t, "tasks"))
	assert.Equal(t, want, f.read(t, made, f.phone), "nothing changes")

	assert.Equal(t, want, use(t, cs, "get_task", object{"ref": made["ref"]}), "it may still read")
	assert.Equal(t, []any{listed(want)}, use(t, cs, "list_tasks", nil)["tasks"])
}

func TestNoToolFindsATaskInTheTrash(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	cs := f.client(t, f.laptop)
	made := makes(t, cs, object{"tasks": []object{{"title": "Old plan", "area": "garden", "list": "soon"}, {"title": "New plan", "area": "garden", "list": "soon"}}})

	trashed := use(t, cs, "trash_task", object{"ref": made[0]["ref"]})
	require.NotNil(t, trashed["trashed_at"])

	code, got := f.do(t, http.MethodGet, "/api/tasks?trashed=true", "", f.phone)
	require.Equal(t, http.StatusOK, code, got)
	assert.Equal(t, []any{trashed}, got["tasks"], "it gives the task as the trash lists it")

	for _, args := range []object{nil, {"area": "garden"}, {"list": "soon"}} {
		got := use(t, cs, "list_tasks", args)
		assert.Equal(t, []any{listed(f.read(t, made[1], f.phone))}, got["tasks"], args)
		assert.InDelta(t, 1, got["total"], 0)
	}

	code, got = f.do(t, http.MethodGet, path(made[0]), "", f.grace)
	require.Equal(t, http.StatusNotFound, code, got)

	for _, call := range []struct {
		tool string
		args object
	}{
		{"get_task", object{"ref": made[0]["ref"]}},
		{"update_task", object{"ref": made[0]["ref"], "title": "Lost"}},
		{"update_task", object{"ref": made[0]["ref"], "done": true}},
		{"trash_task", object{"ref": made[0]["ref"]}},
	} {
		assert.Equal(t, got["error"], fails(t, cs, call.tool, call.args), "the line of a task that is not there, %s", call.tool)
	}

	assert.Equal(t, trashed, listed(f.read(t, made[0], f.phone)), "nothing changes, and REST still reads it")
}

func TestAToolCrossesNoUser(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	mine := makes(t, f.client(t, f.laptop), object{"tasks": []object{{"title": "My plan", "area": "garden"}}})[0]
	want := f.read(t, mine, f.phone)
	theirs := f.client(t, f.grace)

	assert.Empty(t, use(t, theirs, "list_tasks", nil)["tasks"])

	for _, call := range []struct {
		tool string
		args object
	}{
		{"list_tasks", object{"area": "garden"}},
		{"get_task", object{"ref": mine["ref"]}},
		{"update_task", object{"ref": mine["ref"], "title": "Mine now"}},
		{"trash_task", object{"ref": mine["ref"]}},
		{"create_tasks", object{"tasks": []object{{"title": "Mine now", "area": "garden"}}}},
	} {
		fails(t, theirs, call.tool, call.args)
	}

	assert.Equal(t, 1, f.count(t, "tasks"))
	assert.Equal(t, want, f.read(t, mine, f.phone), "her tries changed nothing")

	ours := makes(t, theirs, object{"tasks": []object{{"title": "Her plan"}}})[0]
	assert.Equal(t, "tablet", ours["source"])
	fails(t, f.client(t, f.laptop), "get_task", object{"ref": ours["ref"]})
}
