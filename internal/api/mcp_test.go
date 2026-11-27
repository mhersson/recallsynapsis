package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	sdkauth "github.com/modelcontextprotocol/go-sdk/auth"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mhersson/recallsynapsis/internal/config"
	"github.com/mhersson/recallsynapsis/internal/module"
)

// sendHeader sends every request with the header "Name: value" it holds.
type sendHeader string

func (h sendHeader) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())
	name, value, _ := strings.Cut(string(h), ": ")
	r.Header.Set(name, value)

	return http.DefaultTransport.RoundTrip(r)
}

// connect connects the SDK's own client to /mcp of the handler the fixture
// serves now, with header on every request.
func (f *fixture) connect(t *testing.T, header string) (*mcp.ClientSession, error) {
	t.Helper()

	srv := httptest.NewServer(f.h)
	t.Cleanup(srv.Close)

	cs, err := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "1"}, nil).Connect(t.Context(), &mcp.StreamableClientTransport{
		Endpoint: srv.URL + "/mcp", HTTPClient: &http.Client{Transport: sendHeader(header)}, MaxRetries: -1,
	}, nil)
	if err == nil {
		t.Cleanup(func() { _ = cs.Close() })
	}

	return cs, err
}

func (f *fixture) client(t *testing.T, header string) *mcp.ClientSession {
	t.Helper()

	cs, err := f.connect(t, header)
	require.NoError(t, err)

	return cs
}

// call calls the tool, which must answer, and decodes its answer.
func call(t *testing.T, cs *mcp.ClientSession, name string, args any) map[string]any {
	t.Helper()

	res, err := cs.CallTool(t.Context(), &mcp.CallToolParams{Name: name, Arguments: args})
	require.NoError(t, err)
	require.False(t, res.IsError, res.Content)

	b, err := json.Marshal(res.StructuredContent)
	require.NoError(t, err)

	var got map[string]any
	require.NoError(t, json.Unmarshal(b, &got))

	return got
}

// refused calls the tool, checks that it answers an error result of one
// line, and returns the line.
func refused(t *testing.T, cs *mcp.ClientSession, name string, args any) string {
	t.Helper()

	res, err := cs.CallTool(t.Context(), &mcp.CallToolParams{Name: name, Arguments: args})
	require.NoError(t, err)
	require.True(t, res.IsError)
	require.Len(t, res.Content, 1)

	text, ok := res.Content[0].(*mcp.TextContent)
	require.True(t, ok, "%T", res.Content[0])
	assert.NotEmpty(t, text.Text)
	assert.NotContains(t, text.Text, "\n")

	return text.Text
}

func tools(t *testing.T, cs *mcp.ClientSession) []string {
	t.Helper()

	res, err := cs.ListTools(t.Context(), nil)
	require.NoError(t, err)

	names := []string{}
	for _, tl := range res.Tools {
		names = append(names, tl.Name)
	}

	return names
}

func TestMCPListsTheCoreToolsAndThoseOfTheModulesOn(t *testing.T) {
	t.Parallel()

	f := newFixture(t, "https://recall.example.org")
	f.load(t, map[string]any{"enabled": true})
	read := f.token(t, "phone", "read")
	cs := f.client(t, read)

	init := cs.InitializeResult()
	assert.Equal(t, "recallsynapsis", init.ServerInfo.Name)
	assert.Equal(t, "1.2.3", init.ServerInfo.Version)

	advertised, err := json.Marshal(init.Capabilities)
	require.NoError(t, err)
	assert.JSONEq(t, `{"tools":{}}`, string(advertised), "tools only, and neither listChanged, which opens a stream, nor logging")

	assert.ElementsMatch(t, []string{"list_areas", "search", "update_area", "add_item"}, tools(t, cs))

	res, err := cs.ListTools(t.Context(), nil)
	require.NoError(t, err)

	marked := map[string]mcp.Meta{}

	for _, tl := range res.Tools {
		if tl.Meta != nil {
			marked[tl.Name] = tl.Meta
		}
	}

	assert.Equal(t, map[string]mcp.Meta{"update_area": {"anthropic/requiresUserInteraction": true}}, marked,
		"Claude Code's marker, as the JSON boolean true, on update_area alone")

	f.load(t, map[string]any{"enabled": false})
	assert.ElementsMatch(t, []string{"list_areas", "search", "update_area"}, tools(t, f.client(t, read)), "a module switched off has no tool")
}

func TestMCPAdmitsAnAPITokenAlone(t *testing.T) {
	t.Parallel()

	f := newFixture(t, "https://recall.example.org")
	cookie := f.signIn(t)
	initialize := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"test","version":"1"}}}`
	post := func(headers ...string) int {
		return f.send(t, http.MethodPost, "/mcp", initialize,
			append(headers, "Content-Type: application/json", "Accept: application/json, text/event-stream")...).Code
	}

	write := f.token(t, "laptop", "write")
	assert.Equal(t, http.StatusOK, post(write))

	assert.Equal(t, http.StatusUnauthorized, post(), "no authority")
	assert.Equal(t, http.StatusUnauthorized, post(cookie), "a session cookie alone")
	assert.Equal(t, http.StatusUnauthorized, post(cookie, csrf), "a session cookie with the CSRF header")
	assert.Equal(t, http.StatusUnauthorized, post("Authorization: Bearer recall_unknown"), "an unknown token")

	cs := f.client(t, write)

	tokens, err := f.auth.Tokens(t.Context(), f.user)
	require.NoError(t, err)
	require.Len(t, tokens, 1)
	require.NoError(t, f.auth.DeleteToken(t.Context(), f.user, tokens[0].ID))

	assert.Equal(t, http.StatusUnauthorized, post(write), "a revoked token")

	_, err = cs.ListTools(t.Context(), nil)
	require.Error(t, err, "a client of a revoked token is refused at its next call")

	_, err = f.connect(t, write)
	require.Error(t, err)
}

func TestMCPCapsTheBody(t *testing.T) {
	t.Parallel()

	f := newFixture(t, "https://recall.example.org")
	body := `{"jsonrpc":"2.0","id":1,"method":"ping","params":{"pad":"` + strings.Repeat("x", 1<<20) + `"}}`

	rec := f.send(t, http.MethodPost, "/mcp", body, f.token(t, "laptop", "read"),
		"Content-Type: application/json", "Accept: application/json, text/event-stream")
	assert.Equal(t, http.StatusRequestEntityTooLarge, rec.Code)
}

func TestMCPToolsActForTheTokensUser(t *testing.T) {
	t.Parallel()

	f := newFixture(t, "https://recall.example.org")
	f.load(t, map[string]any{"enabled": true})
	cookie := f.signIn(t)

	code, _ := f.json(t, http.MethodPost, "/api/areas", `{"name":"Garden","brief":"Beds and seeds.\n\nAnd the greenhouse."}`, cookie, csrf)
	require.Equal(t, http.StatusCreated, code)

	code, shed := f.json(t, http.MethodPost, "/api/areas", `{"name":"Shed"}`, cookie, csrf)
	require.Equal(t, http.StatusCreated, code)

	code, _ = f.json(t, http.MethodPatch, areaPath(t, shed), `{"archived":true}`, cookie, csrf)
	require.Equal(t, http.StatusOK, code)

	cs := f.client(t, f.token(t, "laptop", "write"))

	assert.Equal(t, map[string]any{"areas": []any{
		map[string]any{"slug": "personal", "name": "Personal", "kind": "personal", "brief": ""},
		map[string]any{"slug": "garden", "name": "Garden", "kind": "project", "brief": "Beds and seeds."},
	}}, call(t, cs, "list_areas", nil), "the areas not archived, each with its brief's first paragraph, with no id and no archived mark")

	beds := call(t, cs, "add_item", map[string]any{"title": "Raised beds", "area": "GARDEN"})
	assert.Equal(t, []any{map[string]any{"module": "fake", "hits": []any{map[string]any{
		"ref": beds["ref"], "kind": "item", "title": "Raised beds", "snippet": "Raised beds", "area": "garden", "state": "kept",
		"changed_at": f.clock.UTC().Format(time.RFC3339),
	}}}}, f.search(t, "beds", "", cookie), "a write tool acts for the token's user")
	call(t, cs, "add_item", map[string]any{"title": "Seeds for the beds", "area": "personal"})

	for _, area := range []string{"", "garden", "Garden"} {
		assert.Equal(t, f.search(t, "beds", area, cookie), call(t, cs, "search", map[string]any{"query": "beds", "area": area})["groups"],
			"the groups GET /api/search gives, area %q", area)
	}

	assert.Equal(t, []any{}, call(t, cs, "search", map[string]any{"query": "nothing"})["groups"])

	refused(t, cs, "search", map[string]any{"query": ""})

	code, got := f.json(t, http.MethodGet, "/api/search?q=beds&area=nowhere", "", cookie)
	require.Equal(t, http.StatusNotFound, code)
	assert.Equal(t, got["error"], refused(t, cs, "search", map[string]any{"query": "beds", "area": "nowhere"}), "the line REST answers")
	refused(t, cs, "add_item", map[string]any{"title": "Lost", "area": "nowhere"})
}

func TestListAreasGivesAnAreaItIsGivenWhole(t *testing.T) {
	t.Parallel()

	f := newFixture(t, "https://recall.example.org")
	cookie := f.signIn(t)
	brief := "Raised beds,\nand the paths.\n \t\nThe greenhouse.\n\nThe seeds."

	code, garden := f.json(t, http.MethodPost, "/api/areas", `{"name":"Garden","brief":`+strconv.Quote(brief)+`}`, cookie, csrf)
	require.Equal(t, http.StatusCreated, code)

	code, shed := f.json(t, http.MethodPost, "/api/areas", `{"name":"Tool Shed","brief":"Tools."}`, cookie, csrf)
	require.Equal(t, http.StatusCreated, code)

	code, _ = f.json(t, http.MethodPost, "/api/areas", `{"name":"Orchard","brief":"Apples and pears."}`, cookie, csrf)
	require.Equal(t, http.StatusCreated, code)

	code, shed = f.json(t, http.MethodPatch, areaPath(t, shed), `{"archived":true}`, cookie, csrf)
	require.Equal(t, http.StatusOK, code)

	cs := f.client(t, f.token(t, "phone", "read"))

	assert.Equal(t, []any{
		map[string]any{"slug": "personal", "name": "Personal", "kind": "personal", "brief": ""},
		map[string]any{"slug": "garden", "name": "Garden", "kind": "project", "brief": "Raised beds,\nand the paths."},
		map[string]any{"slug": "orchard", "name": "Orchard", "kind": "project", "brief": "Apples and pears."},
	}, call(t, cs, "list_areas", nil)["areas"], "the text before the first blank line, a line of spaces and tabs too, and a brief of one paragraph whole")

	whole := func(a map[string]any) []any {
		return []any{map[string]any{
			"slug": a["slug"], "name": a["name"], "kind": a["kind"], "brief": a["brief"],
			"brief_revision": a["brief_revision"], "brief_changed_at": a["brief_changed_at"], "brief_changed_by": a["brief_changed_by"],
		}}
	}

	assert.Equal(t, whole(garden), call(t, cs, "list_areas", map[string]any{"area": "GARDEN"})["areas"], "by its slug in any case")
	assert.Equal(t, whole(shed), call(t, cs, "list_areas", map[string]any{"area": "tool shed"})["areas"], "an archived project, by its name")
	assert.Equal(t, []any{map[string]any{"slug": "personal", "name": "Personal", "kind": "personal", "brief": "", "brief_revision": float64(0)}},
		call(t, cs, "list_areas", map[string]any{"area": "personal"})["areas"], "the revision of a brief never written")

	code, got := f.json(t, http.MethodGet, "/api/search?q=beds&area=nowhere", "", cookie)
	require.Equal(t, http.StatusNotFound, code)
	assert.Equal(t, got["error"], refused(t, cs, "list_areas", map[string]any{"area": "nowhere"}), "the line REST answers")
}

func TestUpdateAreaReplacesTheBriefAsRESTDoes(t *testing.T) {
	t.Parallel()

	f := newFixture(t, "https://recall.example.org")
	srv := httptest.NewServer(f.h)
	t.Cleanup(srv.Close)

	cookie := f.signIn(t)
	laptop := f.client(t, f.token(t, "laptop", "write"))

	code, garden := f.json(t, http.MethodPost, "/api/areas", `{"name":"Garden","brief":"The beds."}`, cookie, csrf)
	require.Equal(t, http.StatusCreated, code)

	code, shed := f.json(t, http.MethodPost, "/api/areas", `{"name":"Tool Shed"}`, cookie, csrf)
	require.Equal(t, http.StatusCreated, code)

	code, _ = f.json(t, http.MethodPatch, areaPath(t, shed), `{"archived":true}`, cookie, csrf)
	require.Equal(t, http.StatusOK, code)

	refused(t, laptop, "update_area", map[string]any{"area": "personal", "brief": "Blind."})

	for _, area := range []string{"personal", "TOOL SHED"} {
		got := call(t, laptop, "update_area", map[string]any{"area": area, "brief": "Mine.", "brief_revision": 0})
		assert.InDelta(t, 1, got["brief_revision"], 0, "the personal area, which a call with no revision left at 0, "+
			"and an archived project, by slug or by name, %s", area)
	}

	s := openStream(t, srv.URL, cookie)
	brief := "The beds.\n\nAnd the greenhouse."

	got := call(t, laptop, "update_area", map[string]any{"area": "Garden", "brief": brief, "brief_revision": 1, "said": "Yes, keep that."})
	s.expect(t, areaEvent(t, garden))
	assert.Equal(t, []any{brief, float64(2), "laptop"}, []any{got["brief"], got["brief_revision"], got["brief_changed_by"]},
		"the brief whole, one revision on, changed by the token's name")
	assert.Equal(t, []any{got}, call(t, laptop, "list_areas", map[string]any{"area": "garden"})["areas"],
		"the area as list_areas gives one it is given")

	path := areaPath(t, garden)
	code, rev := f.json(t, http.MethodGet, path+"/revisions/2", "", cookie)
	require.Equal(t, http.StatusOK, code)
	assert.Equal(t, []any{"laptop", "Yes, keep that.", brief}, []any{rev["by"], rev["said"], rev["brief"]}, "said on the revision")

	stale := refused(t, laptop, "update_area", map[string]any{"area": "garden", "brief": "Stale.", "brief_revision": 1})
	assert.Contains(t, stale, "2", "the line names the revision the brief is at")

	code, rest := f.json(t, http.MethodPatch, path, `{"brief":"Stale.","brief_revision":1}`, cookie, csrf)
	require.Equal(t, http.StatusConflict, code)
	assert.Equal(t, rest["error"], stale, "the line REST answers")

	refused(t, laptop, "update_area", map[string]any{"area": "nowhere", "brief": "Lost.", "brief_revision": 0})

	for _, args := range []map[string]any{
		{"area": "garden", "brief": strings.Repeat("a", 4001), "brief_revision": 2},
		{"area": "garden", "brief": "Wordy.", "brief_revision": 2, "said": strings.Repeat("é", 4001)},
	} {
		assert.Contains(t, refused(t, laptop, "update_area", args), "4000", "the line names the cap")
	}

	code, history := f.json(t, http.MethodGet, path+"/revisions", "", cookie)
	require.Equal(t, http.StatusOK, code)
	assert.Len(t, history["revisions"], 2, "a refused call saves nothing")

	desk := f.client(t, f.token(t, "desk", "write"))
	assert.Equal(t, got, call(t, desk, "update_area", map[string]any{"area": "garden", "brief": brief + "\n", "brief_revision": 2, "said": "Again."}),
		"a save that leaves the brief as it is answers the area as it is")

	code, history = f.json(t, http.MethodGet, path+"/revisions", "", cookie)
	require.Equal(t, http.StatusOK, code)
	assert.Len(t, history["revisions"], 2, "and writes no revision")
}

func TestMCPAReadTokenCannotCallAWriteTool(t *testing.T) {
	t.Parallel()

	f := newFixture(t, "https://recall.example.org")
	f.load(t, map[string]any{"enabled": true})
	cs := f.client(t, f.token(t, "phone", "read"))

	refused(t, cs, "add_item", map[string]any{"title": "Raised beds", "area": "personal"})
	refused(t, cs, "update_area", map[string]any{"area": "personal", "brief": "Mine.", "brief_revision": 0})

	var rows int
	require.NoError(t, f.store.QueryRowContext(t.Context(), `SELECT count(*) FROM fake_items`).Scan(&rows))
	assert.Zero(t, rows, "nothing changes")
	assert.Equal(t, []any{map[string]any{"slug": "personal", "name": "Personal", "kind": "personal", "brief": "", "brief_revision": float64(0)}},
		call(t, cs, "list_areas", map[string]any{"area": "personal"})["areas"], "nor the brief")

	assert.Equal(t, []any{}, call(t, cs, "search", map[string]any{"query": "beds"})["groups"], "it may still read")
}

func TestMCPASecondUsersAreasAndHitsNeverReachTheFirst(t *testing.T) {
	t.Parallel()

	f := newFixture(t, "https://recall.example.org")
	f.load(t, map[string]any{"enabled": true})

	var grace int64
	require.NoError(t, f.store.QueryRowContext(t.Context(), `INSERT INTO users (name, created_at) VALUES ('grace', 0) RETURNING id`).Scan(&grace))
	require.NoError(t, f.core.EnsurePersonal(t.Context(), grace))

	_, err := f.core.CreateProject(t.Context(), module.Caller{UserID: grace}, "Orchard", "")
	require.NoError(t, err)

	_, text, err := f.auth.CreateToken(t.Context(), grace, "laptop", "write")
	require.NoError(t, err)

	theirs := f.client(t, "Authorization: Bearer "+text)
	call(t, theirs, "add_item", map[string]any{"title": "Beds of grace", "area": "orchard"})

	mine := f.client(t, f.token(t, "laptop", "write"))
	ref := call(t, mine, "add_item", map[string]any{"title": "Beds of ada", "area": "personal"})["ref"]

	assert.Equal(t, []any{map[string]any{"slug": "personal", "name": "Personal", "kind": "personal", "brief": ""}},
		call(t, mine, "list_areas", nil)["areas"])

	groups := call(t, mine, "search", map[string]any{"query": "beds"})["groups"].([]any)
	require.Len(t, groups, 1)
	hits := groups[0].(map[string]any)["hits"].([]any)
	require.Len(t, hits, 1)
	assert.Equal(t, ref, hits[0].(map[string]any)["ref"])

	assert.Equal(t, refused(t, mine, "list_areas", map[string]any{"area": "nowhere"}),
		strings.Replace(refused(t, mine, "list_areas", map[string]any{"area": "orchard"}), "orchard", "nowhere", 1),
		"their area is not found, as one that does not exist")
	refused(t, mine, "search", map[string]any{"query": "beds", "area": "orchard"})
	refused(t, mine, "add_item", map[string]any{"title": "Into theirs", "area": "orchard"})
	refused(t, mine, "update_area", map[string]any{"area": "orchard", "brief": "Into theirs.", "brief_revision": 0})
	assert.Equal(t, []any{map[string]any{"slug": "orchard", "name": "Orchard", "kind": "project", "brief": "", "brief_revision": float64(0)}},
		call(t, theirs, "list_areas", map[string]any{"area": "orchard"})["areas"], "their brief is as it was")
}

func TestMCPRefusesTwoToolsOfOneName(t *testing.T) {
	t.Parallel()

	f := newFixture(t, "https://recall.example.org")
	shadow := func(module.Deps) module.Module {
		return module.Module{Name: "shadow", Tools: []module.Tool{module.NewTool("search", "Search.", module.Read,
			func(context.Context, struct{}) (struct{}, error) { return struct{}{}, nil })}}
	}

	ms, err := f.core.LoadModules(t.Context(), []func(module.Deps) module.Module{shadow},
		&config.Config{Modules: map[string]map[string]any{"shadow": {"enabled": true}}}, module.Deps{})
	require.NoError(t, err)

	assert.Panics(t, func() { New(Deps{Auth: f.auth, Core: f.core, Modules: ms}) }, "a module's tool would hide the core's")
}

func TestGuard(t *testing.T) {
	t.Parallel()

	reader := module.Caller{UserID: 7, Source: "phone"}
	writer := module.Caller{UserID: 7, Source: "laptop", Write: true}
	errDisk := errors.New("disk on fire at /secret/path")

	for _, tt := range []struct {
		name   string
		access module.Access
		caller module.Caller
		fail   error
		ran    bool
		ok     bool
	}{
		{"a read token on a read tool", module.Read, reader, nil, true, true},
		{"a write token on a read tool", module.Read, writer, nil, true, true},
		{"a write token on a write tool", module.Write, writer, nil, true, true},
		{"a read token on a write tool", module.Write, reader, nil, false, false},
		{"a write token on an owner tool", module.Owner, writer, nil, false, false},
		{"a tool that fails", module.Read, reader, errDisk, true, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			req := &mcp.CallToolRequest{
				Params: &mcp.CallToolParamsRaw{Name: "tool"},
				Extra:  &mcp.RequestExtra{TokenInfo: &sdkauth.TokenInfo{Extra: map[string]any{callerKey: tt.caller}}},
			}

			var ran bool

			err := (&server{}).guard(t.Context(), req, tt.access, func(ctx context.Context) error {
				c, _ := module.CallerFrom(ctx)
				ran = c == tt.caller

				return tt.fail
			})

			assert.Equal(t, tt.ran, ran, "the tool runs for the token's caller, and only when its access admits the token")

			if tt.ok {
				require.NoError(t, err)

				return
			}

			require.Error(t, err)
			assert.NotContains(t, err.Error(), "\n")
			assert.NotContains(t, err.Error(), "/secret/path", "an internal error is not answered")
		})
	}
}
