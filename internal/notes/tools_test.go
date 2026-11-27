package notes_test

import (
	"encoding/json"
	"maps"
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mhersson/recallsynapsis/internal/module"
)

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

// pick is what note holds under keys.
func pick(note object, keys ...string) object {
	out := object{}
	for _, k := range keys {
		out[k] = note[k]
	}

	return out
}

// listed is note as a list gives it: in full, less what only a note in full gives.
func listed(note object) object {
	out := maps.Clone(note)
	for _, key := range []string{"body", "said", "links", "linked_from", "files"} {
		delete(out, key)
	}

	return out
}

func TestTheToolsOfNotesAreThereWhileItIsOn(t *testing.T) {
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

	assert.ElementsMatch(t, []string{"list_areas", "search", "update_area", "create_note", "list_notes", "get_note", "update_note", "trash_note"}, names())

	f.load(t, object{"enabled": false})
	assert.ElementsMatch(t, []string{"list_areas", "search", "update_area"}, names())
}

func TestACaptureIsFiledWhereTheOwnerSaid(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	cabin, err := f.core.CreateProject(t.Context(), module.Caller{UserID: f.ada}, "Hytte og båt", "")
	require.NoError(t, err)

	cs := f.client(t, f.laptop)

	for _, tt := range []struct {
		args       object
		area, kind string
		inbox      bool
	}{
		{object{"text": "Ring rørleggeren", "said": "Husk å ringe rørleggeren i morgen"}, "personal", "note", true},
		{object{"text": "Ring elektrikeren", "kind": "note"}, "personal", "note", true},
		{object{"text": "Bytt olje", "area": "Hytte og båt"}, cabin.Slug, "note", false},
		{object{"text": "Mal båten", "area": strings.ToUpper(cabin.Slug)}, cabin.Slug, "note", false},
		{object{"text": "Solceller på taket", "kind": "idea"}, "personal", "idea", false},
		{object{"text": "Et drivhus", "title": "Drivhus", "kind": "idea", "area": "garden", "tags": []string{"#Vår"}}, "garden", "idea", false},
	} {
		made := use(t, cs, "create_note", tt.args)
		title, _ := tt.args["title"].(string)
		assert.Equal(t, object{"area": tt.area, "kind": tt.kind, "inbox": tt.inbox, "title": title, "source": "laptop"},
			pick(made, "area", "kind", "inbox", "title", "source"), tt.args)

		full := f.read(t, made, f.phone)
		assert.Equal(t, listed(full), made, "it gives the note as a list gives it")
		assert.Equal(t, tt.args["text"], full["body"])

		said, _ := tt.args["said"].(string)
		assert.Equal(t, said, full["said"])

		code, first := f.do(t, http.MethodGet, path(made)+"/revisions/1", "", f.phone)
		require.Equal(t, http.StatusOK, code, first)
		assert.Equal(t, said, first["said"], "said is kept on the first revision")
	}

	for _, tt := range []struct {
		args  object
		want  []any
		total float64
	}{
		{object{"area": cabin.Name}, []any{"Mal båten", "Bytt olje"}, 2},
		{object{"kind": "idea"}, []any{"Et drivhus", "Solceller på taket"}, 2},
		{object{"inbox": true}, []any{"Ring elektrikeren", "Ring rørleggeren"}, 2},
		{object{"inbox": false}, []any{"Et drivhus", "Solceller på taket", "Mal båten", "Bytt olje"}, 4},
		{object{"tag": "#Vår"}, []any{"Et drivhus"}, 1},
		{object{"limit": 1}, []any{"Et drivhus"}, 6},
	} {
		got := use(t, cs, "list_notes", tt.args)

		excerpts := []any{}
		for _, n := range got["notes"].([]any) {
			excerpts = append(excerpts, n.(object)["excerpt"])
		}

		assert.Equal(t, tt.want, excerpts, tt.args)
		assert.InDelta(t, tt.total, got["total"], 0, tt.args)
	}

	code, got := f.do(t, http.MethodPost, "/api/notes", `{"body":"Lost","area":"nowhere"}`, f.laptop)
	require.Equal(t, http.StatusNotFound, code, got)

	count := f.count(t, "notes")
	assert.Equal(t, got["error"], fails(t, cs, "create_note", object{"text": "Lost", "area": "nowhere"}), "the line REST answers")
	assert.Equal(t, count, f.count(t, "notes"), "nothing is saved")
}

func TestATextOverTheCapIsRefused(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	f.load(t, object{"enabled": true, "capture_max_chars": int64(12)})
	cs := f.client(t, f.laptop)
	exact, over := strings.Repeat("æøå", 4), strings.Repeat("æøå", 4)+"Æ"

	for _, args := range []object{{"text": over}, {"text": "Kort", "said": over}} {
		assert.Contains(t, fails(t, cs, "create_note", args), "12", "the line names the cap")
	}

	assert.Zero(t, f.count(t, "notes"), "nothing is saved")

	made := use(t, cs, "create_note", object{"text": exact, "said": exact})
	assert.Equal(t, exact, f.read(t, made, f.phone)["body"], "a text of exactly the cap, in letters of two bytes each")

	for _, args := range []object{{"ref": made["ref"], "revision": 1, "body": over}, {"ref": made["ref"], "append": over}} {
		assert.Contains(t, fails(t, cs, "update_note", args), "12")
	}

	assert.InDelta(t, 1, f.read(t, made, f.phone)["revision"], 0, "nothing changes")

	appended := use(t, cs, "update_note", object{"ref": made["ref"], "append": exact})
	assert.Equal(t, exact+"\n\n"+exact, appended["body"], "the cap holds for each text on its own")
	assert.Equal(t, exact, use(t, cs, "update_note", object{"ref": made["ref"], "revision": 2, "body": exact})["body"])

	rest := f.create(t, `{"body":"`+over+`"}`, f.laptop)
	assert.Equal(t, over+"\n\n"+over, f.change(t, rest, `{"append":"`+over+`"}`, f.laptop)["body"], "REST is not capped")

	f.load(t, object{"enabled": true})
	assert.Contains(t, fails(t, f.client(t, f.laptop), "create_note", object{"text": strings.Repeat("ø", 4001)}), "4000",
		"the cap is 4000 unless set")
}

func TestAReadTokenChangesNothingThroughATool(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	made := f.create(t, `{"title":"Seeds","area":"garden"}`, f.laptop)
	cs := f.client(t, f.phone)

	for _, call := range []struct {
		tool string
		args object
	}{
		{"create_note", object{"text": "Lost"}},
		{"update_note", object{"ref": made["ref"], "revision": 1, "title": "Lost"}},
		{"update_note", object{"ref": made["ref"], "append": "Lost"}},
		{"trash_note", object{"ref": made["ref"]}},
	} {
		fails(t, cs, call.tool, call.args)
	}

	assert.Equal(t, 1, f.count(t, "notes"))
	assert.Equal(t, made, f.read(t, made, f.phone), "nothing changes")

	assert.Equal(t, made, use(t, cs, "get_note", object{"ref": made["ref"]}), "it may still read")
	assert.Equal(t, object{"notes": []any{listed(made)}, "total": float64(1)}, use(t, cs, "list_notes", nil))
}

func TestUpdateNoteChangesANoteAsRESTDoes(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	made := use(t, f.client(t, f.laptop), "create_note", object{"text": "Tomater"})
	cs := f.client(t, f.desk)

	appended := use(t, cs, "update_note", object{"ref": made["ref"], "append": "Bønner"})
	assert.Equal(t, "Tomater\n\nBønner", appended["body"], "appending names no revision")
	assert.InDelta(t, 2, appended["revision"], 0)
	assert.Equal(t, "desk", appended["source"])
	assert.Equal(t, true, appended["inbox"], "and files nothing")
	assert.Equal(t, f.read(t, made, f.phone), appended, "it gives the note in full")

	code, got := f.do(t, http.MethodPatch, path(made), `{"revision":1,"body":"Erter"}`, f.laptop)
	require.Equal(t, http.StatusConflict, code, got)
	assert.Equal(t, got["error"], fails(t, cs, "update_note", object{"ref": made["ref"], "revision": 1, "body": "Erter"}),
		"a new body from an old revision is refused with the line REST answers")

	changed := use(t, cs, "update_note", object{
		"ref": made["ref"], "revision": 2, "title": "Hagen", "kind": "idea", "area": "Garden", "tags": []string{"Vår"}, "links": []string{"notes:999"},
	})
	assert.Equal(t, object{"title": "Hagen", "kind": "idea", "area": "garden", "inbox": false, "tags": []any{"vår"}, "revision": float64(3)},
		pick(changed, "title", "kind", "area", "inbox", "tags", "revision"))
	assert.Equal(t, []any{unavailable("notes:999")}, changed["links"])

	back := use(t, cs, "update_note", object{"ref": made["ref"], "revision": 3, "inbox": true})
	assert.Equal(t, object{"area": "personal", "inbox": true}, pick(back, "area", "inbox"))

	fails(t, cs, "update_note", object{"ref": made["ref"], "revision": 4})
	fails(t, cs, "update_note", object{"ref": made["ref"], "revision": 4, "body": "Erter", "append": "Erter"})

	for _, ref := range []string{"notes:9999", "tasks:1", "notes:x", "notes:", "1", ""} {
		fails(t, cs, "get_note", object{"ref": ref})
	}
}

func TestNoToolFindsANoteInTheTrash(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	cs := f.client(t, f.laptop)
	made := use(t, cs, "create_note", object{"text": "Gammel plan", "area": "garden"})
	kept := use(t, cs, "create_note", object{"text": "Ny plan", "area": "garden"})

	trashed := use(t, cs, "trash_note", object{"ref": made["ref"]})
	require.NotNil(t, trashed["trashed_at"])

	code, got := f.do(t, http.MethodGet, "/api/notes?trashed=true", "", f.phone)
	require.Equal(t, http.StatusOK, code, got)
	assert.Equal(t, []any{trashed}, got["notes"], "it gives the note as the trash lists it")

	for _, args := range []object{nil, {"area": "garden"}, {"inbox": false}} {
		assert.Equal(t, object{"notes": []any{kept}, "total": float64(1)}, use(t, cs, "list_notes", args))
	}

	code, got = f.do(t, http.MethodGet, path(made), "", f.grace)
	require.Equal(t, http.StatusNotFound, code, got)

	for _, call := range []struct {
		tool string
		args object
	}{
		{"get_note", object{"ref": made["ref"]}},
		{"update_note", object{"ref": made["ref"], "revision": 1, "title": "Lost"}},
		{"update_note", object{"ref": made["ref"], "append": "Lost"}},
		{"trash_note", object{"ref": made["ref"]}},
	} {
		assert.Equal(t, got["error"], fails(t, cs, call.tool, call.args), "the line of a note that is not there, %s", call.tool)
	}

	assert.Equal(t, trashed, listed(f.read(t, made, f.phone)), "nothing changes")
}

func TestGetNoteGivesTheNoteAsRESTDoes(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	made := f.create(t, `{"title":"Frø","body":"Tomater","area":"garden","tags":["vår"]}`, f.laptop)
	plan := f.create(t, `{"title":"Plan"}`, f.laptop)
	f.change(t, plan, links(1, made["ref"]), f.laptop)

	contents := []byte("Two by four")
	f.attach(t, made, "beds.txt", "text/plain", contents, f.signIn(t, "ada"))

	got := use(t, f.client(t, f.phone), "get_note", object{"ref": made["ref"]})
	assert.Equal(t, f.read(t, made, f.phone), got)
	assert.Equal(t, []any{linkTo(plan, "note", "filed", "Plan")}, got["linked_from"])

	files := got["files"].([]any)
	require.Len(t, files, 1)
	assert.ElementsMatch(t, []string{"id", "name", "type", "size"}, slices.Collect(maps.Keys(files[0].(object))))

	b, err := json.Marshal(got)
	require.NoError(t, err)
	assert.NotContains(t, string(b), string(contents), "nothing of a file's contents")
	assert.NotContains(t, string(b), sum(contents), "nor its hash")
	assert.NotContains(t, string(b), f.tempDir, "nor its path")
}

func TestAToolCrossesNoUser(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	mine := use(t, f.client(t, f.laptop), "create_note", object{"text": "Min plan", "area": "garden"})
	theirs := f.client(t, f.grace)

	assert.Equal(t, object{"notes": []any{}, "total": float64(0)}, use(t, theirs, "list_notes", nil))

	for _, call := range []struct {
		tool string
		args object
	}{
		{"list_notes", object{"area": "garden"}},
		{"get_note", object{"ref": mine["ref"]}},
		{"update_note", object{"ref": mine["ref"], "revision": 1, "title": "Mine now"}},
		{"update_note", object{"ref": mine["ref"], "append": "Mine now"}},
		{"trash_note", object{"ref": mine["ref"]}},
		{"create_note", object{"text": "Mine now", "area": "garden"}},
	} {
		fails(t, theirs, call.tool, call.args)
	}

	assert.Equal(t, 1, f.count(t, "notes"))
	assert.Equal(t, mine, listed(f.read(t, mine, f.phone)), "her tries changed nothing")

	ours := use(t, theirs, "create_note", object{"text": "Hennes plan"})
	assert.Equal(t, "tablet", ours["source"])
	fails(t, f.client(t, f.laptop), "get_note", object{"ref": ours["ref"]})
}
