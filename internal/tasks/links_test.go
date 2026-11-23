package tasks_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// linkTo is a link to item as it reads while the item is there: of kind, in state, and called title.
func linkTo(item object, kind, state, title string) object {
	return object{"ref": item["ref"], "kind": kind, "state": state, "title": title, "available": true}
}

// unavailable is a link to ref that leads nowhere.
func unavailable(ref any) object {
	return object{"ref": ref, "kind": "", "state": "", "title": "", "available": false}
}

// links is the body of a change that gives the links refs, from revision
// when it is not 0, as a change of a note names it.
func links(revision int, refs ...any) string {
	body := object{"links": append([]any{}, refs...)}
	if revision > 0 {
		body["revision"] = revision
	}

	b, err := json.Marshal(body)
	if err != nil {
		panic(err)
	}

	return string(b)
}

// note makes a note of the body JSON with ada's laptop and returns it.
func (f *fixture) note(t *testing.T, body string) object {
	t.Helper()

	code, got := f.do(t, http.MethodPost, "/api/notes", body, f.laptop)
	require.Equal(t, http.StatusCreated, code, got)

	return got
}

// readNote gives note as it now reads with ada's phone.
func (f *fixture) readNote(t *testing.T, note object) object {
	t.Helper()

	code, got := f.do(t, http.MethodGet, fmt.Sprintf("/api/notes/%v", note["id"]), "", f.phone)
	require.Equal(t, http.StatusOK, code, got)

	return got
}

// linkNote gives note the links refs, from revision, and returns it.
func (f *fixture) linkNote(t *testing.T, note object, revision int, refs ...any) object {
	t.Helper()

	code, got := f.do(t, http.MethodPatch, fmt.Sprintf("/api/notes/%v", note["id"]), links(revision, refs...), f.laptop)
	require.Equal(t, http.StatusOK, code, got)

	return got
}

func TestALinkShowsWhatItNames(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	session := f.signIn(t)

	beds := f.note(t, `{"title":"Beds","area":"garden"}`)
	idea := f.note(t, `{"body":"Plant more\nin May","kind":"idea","inbox":true}`)
	plan := f.one(t, `{"title":"Plan the beds","list":"soon"}`)
	gone := f.one(t, `{"title":"Gone"}`)

	made := f.one(t, fmt.Sprintf(`{"title":"Garage","links":["%v","%v","knowledge:1","notes:0%v","%v"]}`,
		beds["ref"], plan["ref"], beds["id"], gone["ref"]))
	garage := f.read(t, made, f.phone)

	assert.Equal(t, []any{
		linkTo(beds, "note", "filed", "Beds"), linkTo(plan, "task", "open", "Plan the beds"), unavailable("knowledge:1"),
		linkTo(gone, "task", "unsorted", "Gone"),
	}, garage["links"], "in the order given, each once, and an item no module has leads nowhere")
	assert.Equal(t, []any{field("title", "", "Garage"), field("area", "", "personal"), object{"field": "links"}},
		objects(garage["log"])[0]["changes"], "the making names the links, with no from and no to")

	assert.Equal(t, []any{linkTo(garage, "task", "unsorted", "Garage")}, f.readNote(t, beds)["linked_from"],
		"a note lists the task that links to it, with its state")

	f.change(t, garage, `{"done":true}`, f.laptop)
	assert.Equal(t, []any{linkTo(garage, "task", "done", "Garage")}, f.readNote(t, beds)["linked_from"], "and the state follows the task")
	assert.Equal(t, []any{linkTo(garage, "task", "done", "Garage")}, f.read(t, plan, f.phone)["linked_from"], "as a task does")

	seeds := f.linkNote(t, f.note(t, `{"title":"Seeds"}`), 1, plan["ref"])
	assert.Equal(t, []any{linkTo(plan, "task", "open", "Plan the beds")}, seeds["links"], "a note links to a task")
	assert.Equal(t, []any{linkTo(seeds, "note", "filed", "Seeds"), linkTo(garage, "task", "done", "Garage")},
		f.read(t, plan, f.phone)["linked_from"], "and the task lists it, the notes before the tasks")

	f.trash(t, gone, "/trash", f.laptop)
	f.trash(t, plan, "/trash", f.laptop)
	assert.Equal(t, []any{linkTo(beds, "note", "filed", "Beds"), unavailable(plan["ref"]), unavailable("knowledge:1"), unavailable(gone["ref"])},
		f.read(t, garage, f.phone)["links"], "a link to a trashed task leads nowhere")
	assert.Equal(t, []any{unavailable(plan["ref"])}, f.readNote(t, seeds)["links"], "from a note too")

	f.trash(t, plan, "/restore", f.laptop)
	assert.Equal(t, []any{linkTo(plan, "task", "open", "Plan the beds")}, f.readNote(t, seeds)["links"], "and leads there again once restored")

	f.trash(t, garage, "/trash", f.laptop)
	assert.Equal(t, []any{}, f.readNote(t, beds)["linked_from"], "a trashed task links to nothing")
	assert.Equal(t, []any{linkTo(seeds, "note", "filed", "Seeds")}, f.read(t, plan, f.phone)["linked_from"])
	f.trash(t, garage, "/restore", f.laptop)

	garage = f.change(t, garage, links(0, idea["ref"], beds["ref"]), f.desk)
	assert.Equal(t, []any{linkTo(idea, "idea", "inbox", "Plant more"), linkTo(beds, "note", "filed", "Beds")}, garage["links"],
		"a change replaces the links")
	assert.Equal(t, []any{object{"field": "links"}}, objects(garage["log"])[0]["changes"])
	assert.Equal(t, []any{linkTo(seeds, "note", "filed", "Seeds")}, f.read(t, plan, f.phone)["linked_from"])

	entries := len(garage["log"].([]any))
	assert.Equal(t, garage, f.change(t, garage, links(0, idea["ref"], beds["ref"]), f.laptop), "the same links change nothing")
	assert.Len(t, f.change(t, garage, `{"title":"Tidy the garage"}`, f.laptop)["links"], 2, "a change that gives no links keeps them")
	assert.Len(t, f.read(t, garage, f.phone)["log"], entries+1)

	many := make([]any, 101)
	for i := range many {
		many[i] = fmt.Sprintf("notes:%d", i)
	}

	for _, bad := range [][]any{{"notes"}, {"notes:"}, {":1"}, {"notes:x"}, {""}, many} {
		code, got := f.do(t, http.MethodPatch, path(garage), links(0, bad...), f.laptop)
		refused(t, http.StatusBadRequest, code, got)

		b, err := json.Marshal(object{"tasks": []any{object{"title": "Rake", "links": bad}}})
		require.NoError(t, err)

		code, got = f.do(t, http.MethodPost, "/api/tasks", string(b), f.laptop)
		refused(t, http.StatusBadRequest, code, got)
	}

	assert.Len(t, f.read(t, garage, f.phone)["links"], 2, "a link that is not module:id, or one too many, changes nothing")

	before := f.count(t, "tasks")
	code, got := f.do(t, http.MethodPost, "/api/tasks", `{"tasks":[{"title":"Rake"},{"title":"Sweep","links":["notes"]}]}`, f.laptop)
	refused(t, http.StatusBadRequest, code, got)
	assert.Equal(t, before, f.count(t, "tasks"), "nor saves any task of its call")
	assert.Len(t, f.change(t, garage, links(0, many[:100]...), f.laptop)["links"], 100)

	cleared := f.change(t, garage, links(0), f.laptop)
	assert.Equal(t, []any{}, cleared["links"])
	assert.Equal(t, []any{}, f.readNote(t, idea)["linked_from"])

	f.change(t, garage, links(0, beds["ref"]), f.laptop)
	f.trash(t, garage, "/trash", f.laptop)

	code, got = f.do(t, http.MethodDelete, path(garage), "", session)
	require.Equal(t, http.StatusNoContent, code, got)

	var left int
	require.NoError(t, f.store.QueryRowContext(t.Context(), `SELECT count(*) FROM task_links WHERE task_id = ?`, garage["id"]).Scan(&left))
	assert.Zero(t, left, "a task deleted for good takes its links with it")
	assert.Equal(t, []any{}, f.readNote(t, beds)["linked_from"])

	code, got = f.do(t, http.MethodDelete, "/api/tasks/trash", "", session)
	require.Equal(t, http.StatusNoContent, code, got)
	assert.Equal(t, []any{unavailable(gone["ref"])}, f.linkNote(t, seeds, 2, gone["ref"])["links"], "a link to a task deleted for good leads nowhere")
}

func TestALinkToATaskOfAModuleSwitchedOffLeadsNowhere(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	beds := f.note(t, `{"title":"Beds"}`)
	garage := f.one(t, fmt.Sprintf(`{"title":"Garage","links":["%v"]}`, beds["ref"]))
	seeds := f.linkNote(t, f.note(t, `{"title":"Seeds"}`), 1, garage["ref"])

	for _, table := range []object{{"enabled": false}, nil} {
		f.load(t, table)

		assert.Equal(t, []any{unavailable(garage["ref"])}, f.readNote(t, seeds)["links"], "a note's link to a task leads nowhere")
		assert.Equal(t, []any{}, f.readNote(t, beds)["linked_from"], "and the task links to nothing")
	}

	f.load(t, object{"enabled": true})
	assert.Equal(t, []any{linkTo(garage, "task", "unsorted", "Garage")}, f.readNote(t, seeds)["links"])
	assert.Equal(t, []any{linkTo(garage, "task", "unsorted", "Garage")}, f.readNote(t, beds)["linked_from"])
}

func TestALinkCrossesNoUser(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	mine := f.one(t, `{"title":"Garage"}`)
	theirs := f.create(t, fmt.Sprintf(`{"tasks":[{"title":"Orchard","links":["%v"]}]}`, mine["ref"]), f.grace)[0]
	mine = f.change(t, mine, links(0, theirs["ref"]), f.laptop)

	assert.Equal(t, []any{unavailable(theirs["ref"])}, mine["links"], "another user's task is never resolved")
	assert.Equal(t, []any{unavailable(mine["ref"])}, f.read(t, theirs, f.grace)["links"])
	assert.Equal(t, []any{}, mine["linked_from"], "nor listed as linking to one")
	assert.Equal(t, []any{}, f.read(t, theirs, f.grace)["linked_from"])

	seeds := f.linkNote(t, f.note(t, `{"title":"Seeds"}`), 1, theirs["ref"])
	assert.Equal(t, []any{unavailable(theirs["ref"])}, seeds["links"], "nor from a note")
}
