package api

import (
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// add makes an item of the fake module and returns its ref.
func (f *fixture) add(t *testing.T, title, area string, headers ...string) string {
	t.Helper()

	code, got := f.json(t, http.MethodPost, "/api/fake/items", `{"title":"`+title+`","area":"`+area+`"}`, headers...)
	require.Equal(t, http.StatusCreated, code, got)

	ref, ok := got["ref"].(string)
	require.True(t, ok, got)

	return ref
}

// search answers GET /api/search with q and area, an empty area left out.
func (f *fixture) search(t *testing.T, q, area string, headers ...string) []any {
	t.Helper()

	query := url.Values{"q": {q}}
	if area != "" {
		query.Set("area", area)
	}

	code, got := f.json(t, http.MethodGet, "/api/search?"+query.Encode(), "", headers...)
	require.Equal(t, http.StatusOK, code, got)

	groups, ok := got["groups"].([]any)
	require.True(t, ok, got)

	return groups
}

func (f *fixture) modules(t *testing.T, headers ...string) []any {
	t.Helper()

	code, got := f.json(t, http.MethodGet, "/api/session", "", headers...)
	require.Equal(t, http.StatusOK, code, got)

	return got["modules"].([]any)
}

func TestAModuleSwitchedOn(t *testing.T) {
	t.Parallel()

	f := newFixture(t, "https://recall.example.org")
	f.load(t, map[string]any{"enabled": true})
	cookie := f.signIn(t)

	code, garden := f.json(t, http.MethodPost, "/api/areas", `{"name":"Garden"}`, cookie, csrf)
	require.Equal(t, http.StatusCreated, code)

	assert.Equal(t, []any{"fake"}, f.modules(t, cookie))

	beds := f.add(t, "Raised beds", "garden", cookie, csrf)
	seeds := f.add(t, "Seeds for the beds", "Personal", cookie, csrf)
	f.add(t, "Tools", "personal", cookie, csrf)

	hit := func(ref, title, area string) map[string]any {
		return map[string]any{
			"ref": ref, "kind": "item", "title": title, "snippet": title, "area": area, "state": "kept",
			"changed_at": f.clock.UTC().Format(time.RFC3339),
		}
	}

	assert.Equal(t, []any{map[string]any{"module": "fake", "hits": []any{
		hit(beds, "Raised beds", "garden"), hit(seeds, "Seeds for the beds", "personal"),
	}}}, f.search(t, "BEDS", "", cookie), "its hits are grouped under its name, each with its area's slug")

	assert.Equal(t, []any{map[string]any{"module": "fake", "hits": []any{hit(beds, "Raised beds", "garden")}}},
		f.search(t, "beds", garden["slug"].(string), cookie), "area limits the search")
	assert.Len(t, f.search(t, "beds", "GARDEN", cookie), 1, "area may be a name too")
	assert.Equal(t, []any{}, f.search(t, "nothing", "", cookie), "no group without hits")

	read := f.token(t, "phone", "read")
	assert.Len(t, f.search(t, "beds", "", read), 1, "a read token searches")

	code, got := f.json(t, http.MethodPost, "/api/fake/items", `{"title":"x","area":"personal"}`, read)
	assert.Equal(t, http.StatusForbidden, code, "its route has the access check of a core route")
	refusal(t, got)

	code, got = f.json(t, http.MethodPost, "/api/fake/items", `{"title":"x","area":"personal","admin":true}`, cookie, csrf)
	assert.Equal(t, http.StatusBadRequest, code, "its route decodes the body strictly")
	refusal(t, got)

	code, got = f.json(t, http.MethodPost, "/api/fake/items", `{"title":"x","area":"personal"}`, cookie)
	assert.Equal(t, http.StatusForbidden, code, "a cookie request needs the CSRF header on its route too")
	refusal(t, got)

	for target, status := range map[string]int{
		"/api/search":                     http.StatusBadRequest,
		"/api/search?q=":                  http.StatusBadRequest,
		"/api/search?q=beds&area=nowhere": http.StatusNotFound,
	} {
		code, got := f.json(t, http.MethodGet, target, "", cookie)
		assert.Equal(t, status, code, target)
		refusal(t, got)
	}

	code, got = f.json(t, http.MethodGet, "/api/search?q=beds", "")
	assert.Equal(t, http.StatusUnauthorized, code)
	refusal(t, got)
}

func TestAModuleSwitchedOffIsAbsentAndKeepsItsData(t *testing.T) {
	t.Parallel()

	f := newFixture(t, "https://recall.example.org")
	f.load(t, map[string]any{"enabled": true})
	cookie := f.signIn(t)
	f.add(t, "Raised beds", "personal", cookie, csrf)

	for _, fake := range []map[string]any{{"enabled": false}, nil} {
		f.load(t, fake)

		assert.Equal(t, []any{}, f.modules(t, cookie))
		assert.Equal(t, []any{}, f.search(t, "beds", "", cookie), "search has no group for it")

		code, got := f.json(t, http.MethodPost, "/api/fake/items", `{"title":"x","area":"personal"}`, cookie, csrf)
		assert.Equal(t, http.StatusNotFound, code, "its route is gone")
		refusal(t, got)

		var rows int
		require.NoError(t, f.store.QueryRowContext(t.Context(), `SELECT count(*) FROM fake_items`).Scan(&rows))
		assert.Equal(t, 1, rows, "its table and rows are still there")
	}

	f.load(t, map[string]any{"enabled": true})
	assert.Len(t, f.search(t, "beds", "", cookie), 1, "switched on again, it finds what it had")
}

func TestASecondUsersHitsNeverReachTheFirst(t *testing.T) {
	t.Parallel()

	f := newFixture(t, "https://recall.example.org")
	f.load(t, map[string]any{"enabled": true})
	ada := f.signIn(t)

	var id int64
	require.NoError(t, f.store.QueryRowContext(t.Context(), `INSERT INTO users (name, created_at) VALUES ('grace', 0) RETURNING id`).Scan(&id))
	require.NoError(t, f.core.EnsurePersonal(t.Context(), id))

	_, text, err := f.auth.CreateToken(t.Context(), id, "laptop", "write")
	require.NoError(t, err)

	grace := "Authorization: Bearer " + text
	mine := f.add(t, "Beds of ada", "personal", ada, csrf)
	theirs := f.add(t, "Beds of grace", "personal", grace)

	refs := func(groups []any) []any {
		out := []any{}

		for _, g := range groups {
			for _, h := range g.(map[string]any)["hits"].([]any) {
				out = append(out, h.(map[string]any)["ref"])
			}
		}

		return out
	}

	assert.Equal(t, []any{mine}, refs(f.search(t, "beds", "", ada)))
	assert.Equal(t, []any{theirs}, refs(f.search(t, "beds", "", grace)))
	assert.Equal(t, []any{mine}, refs(f.search(t, "beds", "personal", ada)))
}
