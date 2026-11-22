package api

import (
	"net/http"
	"strconv"
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
		"id": personal["id"], "kind": "personal", "name": "Personal", "slug": "personal", "description": "", "archived": false,
	}, personal)

	code, garden := f.json(t, http.MethodPost, "/api/areas", `{"name":" Garden ","description":"The beds."}`, cookie, csrf)
	require.Equal(t, http.StatusCreated, code)
	assert.Equal(t, map[string]any{
		"id": garden["id"], "kind": "project", "name": "Garden", "slug": "garden", "description": "The beds.", "archived": false,
	}, garden)

	code, got = f.json(t, http.MethodPost, "/api/areas", `{"name":"gARDEN","description":""}`, cookie, csrf)
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

	code, got = f.json(t, http.MethodPatch, areaPath(t, personal), `{"description":"Mine."}`, cookie, csrf)
	require.Equal(t, http.StatusOK, code)
	assert.Equal(t, "Mine.", got["description"], "the personal area's description can change")
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

func TestTheAccessOfTheAreaRoutes(t *testing.T) {
	t.Parallel()

	f := newFixture(t, "https://recall.example.org")
	cookie := f.signIn(t)
	readToken, writeToken := f.token(t, "phone", "read"), f.token(t, "laptop", "write")

	assert.Equal(t, []string{"Personal"}, f.listed(t, "/api/areas", readToken), "a read token reads")

	code, garden := f.json(t, http.MethodPost, "/api/areas", `{"name":"garden"}`, writeToken)
	require.Equal(t, http.StatusCreated, code, "a write token writes")

	for _, headers := range [][]string{{readToken}, {cookie}} {
		code, got := f.json(t, http.MethodPost, "/api/areas", `{"name":"shed"}`, headers...)
		assert.Equal(t, http.StatusForbidden, code)
		refusal(t, got)

		code, got = f.json(t, http.MethodPatch, areaPath(t, garden), `{"archived":true}`, headers...)
		assert.Equal(t, http.StatusForbidden, code)
		refusal(t, got)
	}

	assert.Equal(t, []string{"Personal", "garden"}, f.listed(t, "/api/areas", cookie), "nothing was made or archived")
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

	code, got := f.json(t, http.MethodPatch, areaPath(t, garden), `{"name":"mine"}`, grace)
	assert.Equal(t, http.StatusNotFound, code)
	refusal(t, got)

	code, got = f.json(t, http.MethodPost, "/api/areas", `{"name":"Garden"}`, grace)
	require.Equal(t, http.StatusCreated, code)
	assert.Equal(t, "garden", got["slug"], "names and slugs are unique per user")

	assert.Equal(t, []string{"Personal", "garden"}, f.listed(t, "/api/areas?archived=true", ada))
}
