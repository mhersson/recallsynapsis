package api

import (
	"net/http"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// areaPath is the path of the area an answer gives.
func areaPath(t *testing.T, area map[string]any) string {
	t.Helper()

	id, ok := area["id"].(float64)
	require.True(t, ok, area)

	return "/api/areas/" + strconv.FormatInt(int64(id), 10)
}

// listed is the names of the areas GET target gives.
func (f *fixture) listed(t *testing.T, target string, headers ...string) []string {
	t.Helper()

	code, got := f.json(t, http.MethodGet, target, "", headers...)
	require.Equal(t, http.StatusOK, code, got)

	list, ok := got["areas"].([]any)
	require.True(t, ok, got)

	names := []string{}

	for _, a := range list {
		area, ok := a.(map[string]any)
		require.True(t, ok)

		name, ok := area["name"].(string)
		require.True(t, ok, area)

		names = append(names, name)
	}

	return names
}

func TestAreas(t *testing.T) {
	t.Parallel()

	f := newFixture(t, "https://recall.example.org")
	cookie := f.signIn(t)

	code, got := f.json(t, http.MethodGet, "/api/areas", "", cookie)
	require.Equal(t, http.StatusOK, code)

	list, ok := got["areas"].([]any)
	require.True(t, ok)
	require.Len(t, list, 1)

	personal, ok := list[0].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, map[string]any{
		"id": personal["id"], "kind": "personal", "name": "Personal", "slug": "personal",
		"brief": "", "brief_revision": float64(0), "brief_changed_at": nil, "brief_changed_by": "", "archived": false,
	}, personal)

	code, garden := f.json(t, http.MethodPost, "/api/areas", `{"name":" Garden ","brief":"The beds."}`, cookie, csrf)
	require.Equal(t, http.StatusCreated, code)
	assert.NotEmpty(t, garden["brief_changed_at"])
	assert.Equal(t, map[string]any{
		"id": garden["id"], "kind": "project", "name": "Garden", "slug": "garden",
		"brief": "The beds.", "brief_revision": float64(1), "brief_changed_at": garden["brief_changed_at"], "brief_changed_by": "web",
		"archived": false,
	}, garden)

	code, got = f.json(t, http.MethodPost, "/api/areas", `{"name":"gARDEN","brief":""}`, cookie, csrf)
	assert.Equal(t, http.StatusConflict, code, "a second project with the same name in any case")
	refusal(t, got)

	code, other := f.json(t, http.MethodPost, "/api/areas", `{"name":"garden!"}`, cookie, csrf)
	require.Equal(t, http.StatusCreated, code)
	assert.Equal(t, "garden-2", other["slug"], "two names that give the same slug get different slugs")

	code, got = f.json(t, http.MethodPatch, areaPath(t, other), `{"name":"GARDEN"}`, cookie, csrf)
	assert.Equal(t, http.StatusConflict, code, "nor may a rename take another's name in any case")
	refusal(t, got)

	for _, body := range []string{`{"name":""}`, `{"name":"a\nb"}`, `{"name":"shed","kind":"personal"}`, `{"name":"shed","archived":true}`} {
		code, got := f.json(t, http.MethodPost, "/api/areas", body, cookie, csrf)
		assert.Equal(t, http.StatusBadRequest, code, body)
		refusal(t, got)
	}

	for _, body := range []string{`{"name":"Home"}`, `{"archived":true}`, `{"slug":"home"}`} {
		code, got := f.json(t, http.MethodPatch, areaPath(t, personal), body, cookie, csrf)
		assert.Equal(t, http.StatusBadRequest, code, body)
		refusal(t, got)
	}

	code, got = f.json(t, http.MethodPatch, areaPath(t, personal), `{"brief":"Mine.","brief_revision":0}`, cookie, csrf)
	require.Equal(t, http.StatusOK, code)
	assert.Equal(t, "Mine.", got["brief"], "the personal area's brief can change")
	assert.Equal(t, "Personal", got["name"])

	code, got = f.json(t, http.MethodPatch, areaPath(t, garden), `{"archived":true}`, cookie, csrf)
	require.Equal(t, http.StatusOK, code)
	assert.Equal(t, true, got["archived"])

	assert.Equal(t, []string{"Personal", "garden!"}, f.listed(t, "/api/areas", cookie))
	assert.Equal(t, []string{"Personal", "garden!"}, f.listed(t, "/api/areas?archived=false", cookie))
	assert.Equal(t, []string{"Personal", "Garden", "garden!"}, f.listed(t, "/api/areas?archived=true", cookie))

	code, got = f.json(t, http.MethodGet, "/api/areas?archived=yes", "", cookie)
	assert.Equal(t, http.StatusBadRequest, code)
	refusal(t, got)

	code, got = f.json(t, http.MethodPatch, areaPath(t, garden), `{"archived":false}`, cookie, csrf)
	require.Equal(t, http.StatusOK, code)
	assert.Equal(t, garden, got, "a restored project is as it was")
	assert.Equal(t, []string{"Personal", "Garden", "garden!"}, f.listed(t, "/api/areas", cookie))

	for _, target := range []string{"/api/areas/999999", "/api/areas/abc"} {
		code, got := f.json(t, http.MethodPatch, target, `{"archived":true}`, cookie, csrf)
		assert.Equal(t, http.StatusNotFound, code, target)
		refusal(t, got)
	}
}

func TestTheBriefsRevisions(t *testing.T) {
	t.Parallel()

	f := newFixture(t, "https://recall.example.org")
	cookie := f.signIn(t)

	code, garden := f.json(t, http.MethodPost, "/api/areas", `{"name":"Garden","brief":"One."}`, cookie, csrf)
	require.Equal(t, http.StatusCreated, code)

	path := areaPath(t, garden)

	code, got := f.json(t, http.MethodPatch, path, `{"brief":"One.\n\nTwo.","brief_revision":1}`, cookie, csrf)
	require.Equal(t, http.StatusOK, code)
	assert.InDelta(t, 2, got["brief_revision"], 0)

	for _, tt := range []struct {
		body   string
		status int
	}{
		{`{"brief":"Stale.","brief_revision":1}`, http.StatusConflict},
		{`{"brief":"Blind."}`, http.StatusBadRequest},
		{`{"brief":"` + strings.Repeat("a", 4001) + `","brief_revision":2}`, http.StatusBadRequest},
	} {
		code, got := f.json(t, http.MethodPatch, path, tt.body, cookie, csrf)
		assert.Equal(t, tt.status, code, tt.body)
		refusal(t, got)
	}

	code, got = f.json(t, http.MethodGet, path+"/revisions", "", cookie)
	require.Equal(t, http.StatusOK, code)

	list, ok := got["revisions"].([]any)
	require.True(t, ok, got)
	require.Len(t, list, 2, "a refused change writes no revision")

	newest, ok := list[0].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, map[string]any{"revision": float64(2), "at": newest["at"], "by": "web", "said": "", "restored_from": nil}, newest)

	code, got = f.json(t, http.MethodGet, path+"/revisions/1", "", cookie)
	require.Equal(t, http.StatusOK, code)
	assert.Equal(t, map[string]any{"revision": float64(1), "at": got["at"], "by": "web", "said": "", "restored_from": nil, "brief": "One."}, got)

	code, got = f.json(t, http.MethodPost, path+"/revisions/1/restore", "", cookie, csrf)
	require.Equal(t, http.StatusOK, code)
	assert.Equal(t, "One.", got["brief"])
	assert.InDelta(t, 3, got["brief_revision"], 0)

	code, got = f.json(t, http.MethodGet, path+"/revisions/3", "", cookie)
	require.Equal(t, http.StatusOK, code)
	assert.InDelta(t, 1, got["restored_from"], 0)

	for _, target := range []string{path + "/revisions/9", path + "/revisions/x", "/api/areas/999999/revisions", "/api/areas/999999/revisions/1"} {
		code, got := f.json(t, http.MethodGet, target, "", cookie)
		assert.Equal(t, http.StatusNotFound, code, target)
		refusal(t, got)
	}

	code, got = f.json(t, http.MethodPost, path+"/revisions/9/restore", "", cookie, csrf)
	assert.Equal(t, http.StatusNotFound, code)
	refusal(t, got)

	code, got = f.json(t, http.MethodPost, "/api/areas", `{"name":"Shed"}`, cookie, csrf)
	require.Equal(t, http.StatusCreated, code)
	assert.InDelta(t, 0, got["brief_revision"], 0, "a project made with no brief is at 0")

	code, got = f.json(t, http.MethodGet, areaPath(t, got)+"/revisions", "", cookie)
	require.Equal(t, http.StatusOK, code)
	assert.Equal(t, map[string]any{"revisions": []any{}}, got, "and has no history")
}

func TestTheAccessOfTheAreaRoutes(t *testing.T) {
	t.Parallel()

	f := newFixture(t, "https://recall.example.org")
	cookie := f.signIn(t)
	readToken, writeToken := f.token(t, "phone", "read"), f.token(t, "laptop", "write")

	assert.Equal(t, []string{"Personal"}, f.listed(t, "/api/areas", readToken), "a read token reads")

	code, garden := f.json(t, http.MethodPost, "/api/areas", `{"name":"garden","brief":"Beds."}`, writeToken)
	require.Equal(t, http.StatusCreated, code, "a write token makes a project")
	assert.Equal(t, "laptop", garden["brief_changed_by"])

	path := areaPath(t, garden)

	for _, headers := range [][]string{{readToken}, {writeToken}} {
		for _, target := range []string{path + "/revisions", path + "/revisions/1"} {
			code, got := f.json(t, http.MethodGet, target, "", headers...)
			assert.Equal(t, http.StatusOK, code, "a token reads the history: %s %v", target, got)
		}
	}

	for _, headers := range [][]string{{readToken}, {writeToken}, {cookie}} {
		if headers[0] == readToken {
			code, got := f.json(t, http.MethodPost, "/api/areas", `{"name":"shed"}`, headers...)
			assert.Equal(t, http.StatusForbidden, code)
			refusal(t, got)
		}

		for _, body := range []string{`{"brief":"Theirs.","brief_revision":1}`, `{"name":"orchard"}`, `{"archived":true}`} {
			code, got := f.json(t, http.MethodPatch, path, body, headers...)
			assert.Equal(t, http.StatusForbidden, code, body)
			refusal(t, got)
		}

		code, got := f.json(t, http.MethodPost, path+"/revisions/1/restore", "", headers...)
		assert.Equal(t, http.StatusForbidden, code)
		refusal(t, got)
	}

	code, got := f.json(t, http.MethodPost, "/api/areas", `{"name":"shed"}`, cookie)
	assert.Equal(t, http.StatusForbidden, code, "the cookie needs the CSRF header to make one")
	refusal(t, got)

	assert.Equal(t, []string{"Personal", "garden"}, f.listed(t, "/api/areas", cookie), "nothing was made, renamed or archived")

	code, got = f.json(t, http.MethodGet, path+"/revisions", "", cookie)
	require.Equal(t, http.StatusOK, code)
	assert.Len(t, got["revisions"], 1, "and no brief was changed or restored")
}

func TestAUsersAreasAreInvisibleToAnother(t *testing.T) {
	t.Parallel()

	f := newFixture(t, "https://recall.example.org")
	ada := f.signIn(t)

	code, garden := f.json(t, http.MethodPost, "/api/areas", `{"name":"garden"}`, ada, csrf)
	require.Equal(t, http.StatusCreated, code)

	var id int64
	require.NoError(t, f.store.QueryRowContext(t.Context(), `INSERT INTO users (name, created_at) VALUES ('grace', 0) RETURNING id`).Scan(&id))
	require.NoError(t, f.core.EnsurePersonal(t.Context(), id))

	_, text, err := f.auth.CreateToken(t.Context(), id, "laptop", "write")
	require.NoError(t, err)

	grace := "Authorization: Bearer " + text

	assert.Equal(t, []string{"Personal"}, f.listed(t, "/api/areas?archived=true", grace))

	code, _ = f.json(t, http.MethodPatch, areaPath(t, garden), `{"brief":"Mine.","brief_revision":0}`, ada, csrf)
	require.Equal(t, http.StatusOK, code)

	for _, target := range []string{areaPath(t, garden) + "/revisions", areaPath(t, garden) + "/revisions/1"} {
		code, got := f.json(t, http.MethodGet, target, "", grace)
		assert.Equal(t, http.StatusNotFound, code, target)
		refusal(t, got)
	}

	code, got := f.json(t, http.MethodPost, "/api/areas", `{"name":"Garden"}`, grace)
	require.Equal(t, http.StatusCreated, code)
	assert.Equal(t, "garden", got["slug"], "names and slugs are unique per user")

	assert.Equal(t, []string{"Personal", "garden"}, f.listed(t, "/api/areas?archived=true", ada))
}
