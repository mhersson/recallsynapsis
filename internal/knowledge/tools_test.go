package knowledge_test

import (
	"encoding/json"
	"fmt"
	"maps"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mhersson/recallsynapsis/internal/core"
	"github.com/mhersson/recallsynapsis/internal/module"
)

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

// revisions are the revisions of decision as its history lists them.
func (f *fixture) revisions(t *testing.T, decision object) []object {
	t.Helper()

	code, got := f.do(t, http.MethodGet, path(decision)+"/revisions", "", f.phone)
	require.Equal(t, http.StatusOK, code, got)

	return objects(got["revisions"])
}

func TestGetKnowledgeGivesAProjectOrADecision(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	web := f.signIn(t, "ada")

	brief, revision := "Raised beds in the back garden.\n\nClay below, compost above.", int64(0)
	_, err := f.core.ChangeArea(t.Context(), module.Caller{UserID: f.users["ada"], Source: "web"}, f.areas["garden"],
		core.Change{Brief: &brief, BriefRevision: &revision})
	require.NoError(t, err)

	raised := f.add(t, "garden", `{"decision":"Beds are raised","why":"The soil is clay.","decided":"2026-10-01"}`, web)
	flat := f.add(t, "garden", fmt.Sprintf(`{"decision":"Beds are flat","why":"Sand.","decided":"2026-11-01","replaces":%q}`, raised["ref"]), web)
	dawn := f.add(t, "garden", `{"decision":"Water at dawn","why":"Less is lost to the sun."}`, web)
	f.add(t, "orchard", `{"decision":"Prune in February","why":"Before the sap rises."}`, web)
	f.note(t, `{"title":"Beds","area":"garden"}`, raised["ref"])

	cs := f.client(t, f.phone)
	named := objects(use(t, cs, "list_areas", object{"area": "garden"})["areas"])[0]
	about := object{
		"area": "garden", "name": "Garden", "brief": brief, "brief_revision": named["brief_revision"],
		"brief_changed_at": named["brief_changed_at"], "brief_changed_by": named["brief_changed_by"], "replaced_count": float64(1),
	}
	assert.Equal(t, object{"brief": brief, "brief_revision": float64(1), "brief_changed_by": "web"},
		pick(about, "brief", "brief_revision", "brief_changed_by"), "the brief whole, with its revision, when it changed and by what")
	assert.NotNil(t, about["brief_changed_at"])

	with := func(more object) object {
		out := maps.Clone(about)
		maps.Copy(out, more)

		return out
	}

	stands := []any{line(dawn), line(flat)}
	assert.Equal(t, with(object{"decisions": stands}), use(t, cs, "get_knowledge", object{"area": "Garden"}),
		"each decision that stands as its line, the newest first, how many are replaced, and no why")
	assert.Equal(t, with(object{"decisions": stands, "replaced": []any{line(raised)}}),
		use(t, cs, "get_knowledge", object{"area": "garden", "replaced": true}), "with replaced, the replaced listed too")

	rows := f.list(t, "garden")
	assert.Equal(t, with(object{"decisions": []any{rows[0], rows[1]}}), use(t, cs, "get_knowledge", object{"area": "garden", "full": true}),
		"with full, each standing decision as a list gives it")
	assert.Equal(t, with(object{"decisions": []any{rows[0], rows[1]}, "replaced": []any{rows[2]}}),
		use(t, cs, "get_knowledge", object{"area": "garden", "full": true, "replaced": true}))

	for _, d := range []object{raised, flat} {
		assert.Equal(t, f.full(t, d), use(t, cs, "get_knowledge", object{"ref": d["ref"]}),
			"with a ref, the decision in full, with what it replaced or what replaced it, and what links to it")
	}

	assert.Equal(t, object{
		"area": "personal", "name": "Personal", "brief": "", "brief_revision": float64(0), "decisions": []any{},
		"replaced_count": float64(0),
	}, use(t, cs, "get_knowledge", object{"area": "personal"}),
		"the personal area gives its brief and no decision, at revision 0 with no change, as list_areas gives it")
	assert.Equal(t, object{"areas": []any{object{"slug": "personal", "name": "Personal", "kind": "personal", "brief": "", "brief_revision": float64(0)}}},
		use(t, cs, "list_areas", object{"area": "personal"}))

	assert.Equal(t, []any{}, use(t, cs, "get_knowledge", object{"area": "orchard", "replaced": true})["replaced"],
		"replaced asked for with none replaced")

	grace := f.client(t, f.grace)
	assert.Equal(t, []any{}, use(t, grace, "get_knowledge", object{"area": "garden"})["decisions"], "grace's own Garden holds none of ada's")
	fails(t, grace, "get_knowledge", object{"ref": raised["ref"]})
	fails(t, grace, "get_knowledge", object{"area": "orchard"})

	for _, args := range []object{
		{"area": "garden", "ref": raised["ref"]},
		{},
		{"full": true},
		{"ref": "notes:1"},
		{"ref": "knowledge:9999"},
		{"area": "nowhere"},
	} {
		fails(t, cs, "get_knowledge", args)
	}
}

func TestUpdateKnowledgeSavesAPackageAsRESTDoes(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	web := f.signIn(t, "ada")
	raised := f.add(t, "garden", `{"decision":"Beds are raised","why":"The soil is clay."}`, web)
	dawn := f.add(t, "garden", `{"decision":"Water at dawn","why":"Less is lost."}`, web)
	said := "Yes, keep those three."

	cs := f.client(t, f.laptop)
	got := objects(use(t, cs, "update_knowledge", object{
		"area": "Garden", "said": said,
		"add": []object{
			{"decision": "Beds are flat", "why": "Sand was brought in.", "replaces": raised["ref"]},
			{"decision": "Mulch in autumn", "why": "It feeds the soil.", "source": "The talk with the neighbour"},
		},
		"mend": []object{{"ref": dawn["ref"], "revision": 1, "why": "Less is lost to the sun."}},
	})["decisions"])
	require.Len(t, got, 3)

	for i, d := range got {
		assert.Equal(t, f.read(t, d, f.phone), d, "it gives what it mended, then what it added, as a list gives them")
		assert.Equal(t, "laptop", d["changed_by"], "the token's name as what changed it")
		assert.Equal(t, object{"by": "laptop", "said": said}, pick(f.revisions(t, d)[0], "by", "said"), "and said on its revision: %d", i)
	}

	assert.Equal(t, []any{dawn["ref"], "Less is lost to the sun.", float64(2)}, []any{got[0]["ref"], got[0]["why"], got[0]["revision"]})
	assert.Equal(t, line(raised), got[1]["replaces"])
	assert.Equal(t, "replaced", f.read(t, raised, f.phone)["state"])
	assert.Equal(t, object{"by": "web", "said": ""}, pick(f.revisions(t, dawn)[1], "by", "said"), "an earlier revision keeps its own")

	before, revisions := f.list(t, "garden"), f.count(t, "decision_revisions")
	mulch := object{"decision": "Mulch", "why": "Soil."}

	for _, tt := range []struct {
		args object
		want string // a part of the line, or "" when no number in it tells it from another
	}{
		{object{"area": "garden", "add": []object{mulch, mulch, {"decision": "Lost", "why": " "}}}, "3"},
		{object{"area": "garden", "add": []object{mulch}, "mend": []object{{"ref": dawn["ref"], "revision": 1, "why": "Sun."}}}, ""},
		{object{"area": "garden", "add": slices.Repeat([]object{mulch}, 21)}, "20"},
		{object{"area": "garden", "add": []object{mulch}, "said": strings.Repeat("å", 4001)}, "4000"},
		{object{"area": "personal", "add": []object{mulch}}, ""},
	} {
		assert.Contains(t, fails(t, cs, "update_knowledge", tt.args), tt.want)
	}

	assert.Equal(t, before, f.list(t, "garden"), "a refused call saves nothing")
	assert.Equal(t, revisions, f.count(t, "decision_revisions"))

	exact := strings.Repeat("å", 4000)
	made := objects(use(t, cs, "update_knowledge", object{"area": "garden", "add": []object{mulch}, "said": exact})["decisions"])[0]
	assert.Equal(t, exact, f.revisions(t, made)[0]["said"], "a said of exactly the cap, in letters of two bytes each")
	assert.Empty(t, f.revisions(t, objects(use(t, cs, "update_knowledge", object{"area": "garden", "add": []object{mulch}})["decisions"])[0])[0]["said"],
		"and said may be left out")
}

func TestAReadTokenRecordsNothing(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	web := f.signIn(t, "ada")
	raised := f.add(t, "garden", `{"decision":"Beds are raised","why":"The soil is clay."}`, web)

	cs := f.client(t, f.phone)
	fails(t, cs, "update_knowledge", object{"area": "garden", "add": []object{{"decision": "Mulch", "why": "Soil."}}})
	fails(t, cs, "update_knowledge", object{"area": "garden", "mend": []object{{"ref": raised["ref"], "revision": 1, "why": "Loam."}}})

	assert.Equal(t, []object{raised}, f.list(t, "garden"), "nothing changes")
	assert.Equal(t, 1, f.count(t, "decision_revisions"))
	assert.Equal(t, line(raised), objects(use(t, cs, "get_knowledge", object{"area": "garden"})["decisions"])[0], "and it reads")
}

// pick is the keys of o alone.
func pick(o object, keys ...string) object {
	out := object{}
	for _, k := range keys {
		out[k] = o[k]
	}

	return out
}
