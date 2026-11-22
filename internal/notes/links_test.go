package notes_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// linkTo is a link to note as it reads while the note is there: of kind, in state, and called title.
func linkTo(note object, kind, state, title string) object {
	return object{"ref": note["ref"], "kind": kind, "state": state, "title": title, "available": true}
}

// unavailable is a link to ref that leads nowhere.
func unavailable(ref any) object {
	return object{"ref": ref, "kind": "", "state": "", "title": "", "available": false}
}

// links is the body of a change from revision that gives the links refs.
func links(revision int, refs ...any) string {
	b, err := json.Marshal(object{"revision": revision, "links": append([]any{}, refs...)})
	if err != nil {
		panic(err)
	}

	return string(b)
}

func TestALinkShowsWhatItNames(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	session := f.signIn(t)

	beds := f.create(t, `{"title":"Beds","area":"garden"}`, f.laptop)
	idea := f.create(t, `{"body":"  Plant more\nin May","kind":"idea","inbox":true}`, f.laptop)
	gone := f.create(t, `{"title":"Gone"}`, f.laptop)
	deleted := f.change(t, f.create(t, `{"title":"Deleted"}`, f.laptop), links(1, beds["ref"]), f.laptop)
	seeds := f.create(t, `{"title":"Seeds","inbox":true}`, f.laptop)

	assert.Equal(t, []any{}, seeds["links"])
	assert.Equal(t, []any{}, seeds["linked_from"])

	seeds = f.change(t, seeds, links(1, "tasks:1", idea["ref"], beds["ref"], gone["ref"], deleted["ref"], fmt.Sprintf("notes:0%v", beds["id"])), f.desk)
	assert.InDelta(t, 2, seeds["revision"], 0)
	assert.Equal(t, true, seeds["inbox"], "a change of links files nothing")
	assert.Equal(t, []any{
		unavailable("tasks:1"), linkTo(idea, "idea", "inbox", "Plant more"), linkTo(beds, "note", "filed", "Beds"),
		linkTo(gone, "note", "filed", "Gone"), linkTo(deleted, "note", "filed", "Deleted"),
	}, seeds["links"], "in the order given, each once, and an item no module has leads nowhere")

	f.trash(t, gone, "/trash", f.laptop)
	f.trash(t, deleted, "/trash", f.laptop)

	code, got := f.do(t, http.MethodDelete, path(deleted), "", session)
	require.Equal(t, http.StatusNoContent, code, got)
	assert.Equal(t, []any{
		unavailable("tasks:1"), linkTo(idea, "idea", "inbox", "Plant more"), linkTo(beds, "note", "filed", "Beds"),
		unavailable(gone["ref"]), unavailable(deleted["ref"]),
	}, f.read(t, seeds, f.phone)["links"], "a link to a trashed or a deleted note leads nowhere")

	var left int
	require.NoError(t, f.store.QueryRowContext(t.Context(), `SELECT count(*) FROM note_links WHERE note_id = ?`, deleted["id"]).Scan(&left))
	assert.Zero(t, left, "a note deleted for good takes its links with it")

	f.trash(t, gone, "/restore", f.laptop)
	linked := f.read(t, seeds, f.phone)["links"]
	assert.Equal(t, linkTo(gone, "note", "filed", "Gone"), linked.([]any)[3], "and leads there again once restored")

	plan := f.change(t, f.create(t, `{"title":"Plan"}`, f.laptop), links(1, beds["ref"]), f.laptop)
	assert.Equal(t, []any{linkTo(plan, "note", "filed", "Plan"), linkTo(seeds, "note", "inbox", "Seeds")},
		f.read(t, beds, f.phone)["linked_from"], "a note lists the notes that link to it, the most recently changed first")

	seeds = f.change(t, seeds, `{"revision":2,"title":"Frø"}`, f.laptop)
	assert.Equal(t, linked, seeds["links"], "a change that gives no links keeps them")
	assert.Equal(t, linked, f.change(t, seeds, `{"append":"Sow in March"}`, f.laptop)["links"])
	assert.Equal(t, []any{linkTo(seeds, "note", "inbox", "Frø"), linkTo(plan, "note", "filed", "Plan")}, f.read(t, beds, f.phone)["linked_from"])

	code, got = f.do(t, http.MethodGet, path(seeds)+"/revisions/4", "", f.phone)
	require.Equal(t, http.StatusOK, code, got)
	assert.Equal(t, []any{"tasks:1", idea["ref"], beds["ref"], gone["ref"], deleted["ref"]}, got["links"], "and its revision has them")

	f.trash(t, plan, "/trash", f.laptop)
	assert.Equal(t, []any{linkTo(seeds, "note", "inbox", "Frø")}, f.read(t, beds, f.phone)["linked_from"], "a trashed note links to nothing")

	many := make([]any, 101)
	for i := range many {
		many[i] = fmt.Sprintf("tasks:%d", i)
	}

	for _, body := range []string{links(4, "notes"), links(4, "notes:"), links(4, ":1"), links(4, "notes:x"), links(4, "notes:1:2"), links(4, ""), links(4, many...)} {
		code, got := f.do(t, http.MethodPatch, path(seeds), body, f.laptop)
		refused(t, http.StatusBadRequest, code, got)
	}

	assert.InDelta(t, 4, f.read(t, seeds, f.phone)["revision"], 0, "a link that is not module:id, or one too many, changes nothing")
	assert.Len(t, f.change(t, seeds, links(4, many[:100]...), f.laptop)["links"], 100)

	cleared := f.change(t, seeds, links(5), f.laptop)
	assert.Equal(t, []any{}, cleared["links"])
	assert.Equal(t, []any{}, f.read(t, idea, f.phone)["linked_from"])
}

func TestRestoringARevisionBringsItsLinksBack(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	beds := f.create(t, `{"title":"Beds"}`, f.laptop)
	plan := f.create(t, `{"title":"Plan"}`, f.laptop)
	seeds := f.create(t, `{"title":"Seeds"}`, f.laptop)

	f.change(t, seeds, links(1, "tasks:7", beds["ref"]), f.laptop)
	f.change(t, seeds, links(2, plan["ref"]), f.laptop)

	code, got := f.do(t, http.MethodGet, path(seeds)+"/revisions/2", "", f.phone)
	require.Equal(t, http.StatusOK, code, got)
	assert.Equal(t, []any{"tasks:7", beds["ref"]}, got["links"], "a revision keeps its links as references")

	code, got = f.do(t, http.MethodPost, path(seeds)+"/revisions/2/restore", "", f.laptop)
	require.Equal(t, http.StatusOK, code, got)
	assert.Equal(t, []any{unavailable("tasks:7"), linkTo(beds, "note", "filed", "Beds")}, got["links"])
	assert.Equal(t, []any{}, f.read(t, plan, f.phone)["linked_from"])
	assert.Equal(t, []any{linkTo(seeds, "note", "filed", "Seeds")}, f.read(t, beds, f.phone)["linked_from"])

	code, got = f.do(t, http.MethodGet, path(seeds)+"/revisions/4", "", f.phone)
	require.Equal(t, http.StatusOK, code, got)
	assert.Equal(t, []any{"tasks:7", beds["ref"]}, got["links"], "the restore's own revision has them")

	code, got = f.do(t, http.MethodPost, path(seeds)+"/revisions/1/restore", "", f.laptop)
	require.Equal(t, http.StatusOK, code, got)
	assert.Equal(t, []any{}, got["links"], "a revision with no links brings back none")
}

func TestALinkCrossesNoUser(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	mine := f.create(t, `{"title":"Beds"}`, f.laptop)
	theirs := f.change(t, f.create(t, `{"title":"Orchard"}`, f.grace), links(1, mine["ref"]), f.grace)
	mine = f.change(t, mine, links(1, theirs["ref"]), f.laptop)

	assert.Equal(t, []any{unavailable(theirs["ref"])}, mine["links"], "another user's note is never resolved")
	assert.Equal(t, []any{unavailable(mine["ref"])}, theirs["links"])
	assert.Equal(t, []any{}, mine["linked_from"], "nor listed as linking to one")
	assert.Equal(t, []any{}, f.read(t, theirs, f.grace)["linked_from"])
}
