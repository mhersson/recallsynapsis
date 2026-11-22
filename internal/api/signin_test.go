package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mhersson/recallsynapsis/internal/auth"
	"github.com/mhersson/recallsynapsis/internal/config"
	"github.com/mhersson/recallsynapsis/internal/core"
	"github.com/mhersson/recallsynapsis/internal/module"
	"github.com/mhersson/recallsynapsis/internal/store"
	"github.com/mhersson/recallsynapsis/internal/testkit"
)

const (
	day      = 24 * time.Hour
	password = "correct horse battery"
	csrf     = csrfHeader + ": 1"
)

// passwordHash is password's hash, made once since each costs an argon2 run.
var passwordHash = sync.OnceValue(func() string {
	h, err := auth.HashPassword(password)
	if err != nil {
		panic(err)
	}

	return h
})

type fixture struct {
	h         http.Handler
	auth      *auth.Service
	core      *core.Service
	store     *store.Store
	clock     *time.Time
	user      int64
	port      atomic.Int32
	publicURL string
}

// newFixture serves a new database whose user ada has password as her password
// and her personal area, with the fake module off, having no [modules.fake] table.
func newFixture(t *testing.T, publicURL string) *fixture {
	t.Helper()

	st, err := store.Open(t.Context(), filepath.Join(t.TempDir(), "recallsynapsis.db"), slices.Concat(auth.Migrations, core.Migrations))
	require.NoError(t, err)

	t.Cleanup(func() { _ = st.Close() })

	clock := time.Unix(1_000_000, 0)
	svc := auth.New(st, func() time.Time { return clock })

	user, err := svc.EnsureUser(t.Context(), "ada")
	require.NoError(t, err)
	require.NoError(t, svc.SetPasswordHash(t.Context(), user, passwordHash()))

	areas := core.New(st)
	require.NoError(t, areas.EnsurePersonal(t.Context(), user))

	f := &fixture{auth: svc, core: areas, store: st, clock: &clock, user: user, publicURL: publicURL}
	f.load(t, nil)

	return f
}

// load serves the fixture's database again, with fake as the fake module's table.
func (f *fixture) load(t *testing.T, fake map[string]any) {
	t.Helper()

	tables := map[string]map[string]any{}
	if fake != nil {
		tables["fake"] = fake
	}

	ms, err := f.core.LoadModules(t.Context(), []func(module.Deps) module.Module{testkit.New}, &config.Config{Modules: tables},
		module.Deps{Now: func() time.Time { return *f.clock }, Dir: t.TempDir(), Changed: func(module.Change) {}})
	require.NoError(t, err)

	f.h = New(Deps{Auth: f.auth, Core: f.core, Modules: ms, Version: "1.2.3", PublicURL: f.publicURL, Timezone: "Europe/Oslo"})
}

// send serves one request from the address 192.0.2.1, on a new port as each
// connection is; each header is "Name: value".
func (f *fixture) send(t *testing.T, method, target, body string, headers ...string) *httptest.ResponseRecorder {
	t.Helper()

	req := httptest.NewRequestWithContext(t.Context(), method, target, strings.NewReader(body))
	req.RemoteAddr = "192.0.2.1:" + strconv.Itoa(40000+int(f.port.Add(1)))

	for _, h := range headers {
		name, value, _ := strings.Cut(h, ": ")
		req.Header.Add(name, value)
	}

	rec := httptest.NewRecorder()
	f.h.ServeHTTP(rec, req)

	return rec
}

// json serves one request and decodes its answer.
func (f *fixture) json(t *testing.T, method, target, body string, headers ...string) (int, map[string]any) {
	t.Helper()

	rec := f.send(t, method, target, body, headers...)
	assert.Equal(t, "application/json", rec.Header().Get("Content-Type"))

	var got map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got), rec.Body.String())

	return rec.Code, got
}

// sessionCookie is the session cookie rec sets, with the attributes every
// one of them carries under an https public_url.
func sessionCookie(t *testing.T, rec *httptest.ResponseRecorder) *http.Cookie {
	t.Helper()

	for _, c := range rec.Result().Cookies() {
		if c.Name == cookieName {
			assert.Equal(t, "/", c.Path)
			assert.True(t, c.HttpOnly)
			assert.Equal(t, http.SameSiteStrictMode, c.SameSite)
			assert.True(t, c.Secure)

			return c
		}
	}

	t.Fatal("no session cookie in the answer")

	return nil
}

// signIn signs ada in and returns the Cookie header her session sends.
func (f *fixture) signIn(t *testing.T) string {
	t.Helper()

	rec := f.send(t, http.MethodPost, "/api/auth/login", `{"name":"ada","password":"`+password+`"}`)
	require.Equal(t, http.StatusNoContent, rec.Code, rec.Body.String())

	return "Cookie: " + cookieName + "=" + sessionCookie(t, rec).Value
}

// token makes a token with access and returns the Authorization header it sends.
func (f *fixture) token(t *testing.T, name, access string) string {
	t.Helper()

	_, text, err := f.auth.CreateToken(t.Context(), f.user, name, access)
	require.NoError(t, err)

	return "Authorization: Bearer " + text
}

func TestSignInSetsTheSessionCookie(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		publicURL string
		secure    bool
	}{
		{"https://recall.example.org", true},
		{"http://192.0.2.9:7119", false},
	} {
		t.Run(tt.publicURL, func(t *testing.T) {
			t.Parallel()

			f := newFixture(t, tt.publicURL)

			rec := f.send(t, http.MethodPost, "/api/auth/login", `{"name":"ada","password":"`+password+`"}`)
			require.Equal(t, http.StatusNoContent, rec.Code)

			var c *http.Cookie
			for _, set := range rec.Result().Cookies() {
				c = set
			}

			require.NotNil(t, c)
			assert.Equal(t, cookieName, c.Name)
			assert.NotEmpty(t, c.Value)
			assert.Equal(t, "/", c.Path)
			assert.True(t, c.HttpOnly)
			assert.Equal(t, http.SameSiteStrictMode, c.SameSite)
			assert.Equal(t, tt.secure, c.Secure)
			assert.Equal(t, f.clock.Add(30*day).Unix(), c.Expires.Unix())
		})
	}
}

func TestTheSession(t *testing.T) {
	t.Parallel()

	f := newFixture(t, "https://recall.example.org")
	cookie := f.signIn(t)

	code, got := f.json(t, http.MethodGet, "/api/session", "", cookie)
	assert.Equal(t, http.StatusOK, code)
	assert.Equal(t, map[string]any{
		"user":        map[string]any{"name": "ada"},
		"modules":     []any{},
		"version":     "1.2.3",
		"timezone":    "Europe/Oslo",
		"last_backup": nil,
	}, got)

	code, _ = f.json(t, http.MethodGet, "/api/session", "", f.token(t, "phone", "read"))
	assert.Equal(t, http.StatusOK, code, "a read token reads")

	for _, headers := range [][]string{
		nil,
		{"Cookie: " + cookieName + "=not-a-session"},
		{"Authorization: Bearer recall_not-a-token"},
		{"Authorization: Basic YWRhOnNlY3JldA=="},
	} {
		code, got := f.json(t, http.MethodGet, "/api/session", "", headers...)
		assert.Equal(t, http.StatusUnauthorized, code, headers)
		refusal(t, got)
	}
}

func TestTheLastBackupIsTheNewestSnapshot(t *testing.T) {
	t.Parallel()

	f := newFixture(t, "https://recall.example.org")
	cookie := f.signIn(t)

	// serving gives the last backup each request to a handler over dir answers.
	serving := func(dir string) func() any {
		h := New(Deps{Auth: f.auth, Core: f.core, Modules: &core.Modules{}, PublicURL: f.publicURL, BackupDir: dir})

		return func() any {
			f.h = h

			code, got := f.json(t, http.MethodGet, "/api/session", "", cookie)
			require.Equal(t, http.StatusOK, code)

			return got["last_backup"]
		}
	}

	dir := t.TempDir()
	assert.Nil(t, serving(filepath.Join(dir, "absent"))(), "no backup yet")

	last := serving(dir)
	assert.Nil(t, last(), "no snapshot yet")

	// Made at 03:00:05 and a half in Oslo, which is 02:00:05 in UTC.
	path := filepath.Join(dir, "recallsynapsis-20261122.db")
	made := time.Date(2026, 11, 22, 3, 0, 5, 5e8, time.FixedZone("CET", 3600))

	require.NoError(t, os.WriteFile(path, nil, 0o600))
	require.NoError(t, os.Chtimes(path, made, made))
	assert.Equal(t, "2026-11-22T02:00:05Z", last(), "read when asked")

	// A backup directory that cannot be read leaves the session as it is.
	assert.Nil(t, serving(path)())
}

func TestAWrongNameAndAWrongPasswordGetTheSameAnswer(t *testing.T) {
	t.Parallel()

	f := newFixture(t, "https://recall.example.org")

	wrongName := f.send(t, http.MethodPost, "/api/auth/login", `{"name":"grace","password":"`+password+`"}`)
	wrongPassword := f.send(t, http.MethodPost, "/api/auth/login", `{"name":"ada","password":"wrong horse battery"}`)

	assert.Equal(t, http.StatusUnauthorized, wrongName.Code)
	assert.Equal(t, wrongName.Code, wrongPassword.Code)
	assert.Equal(t, wrongName.Body.String(), wrongPassword.Body.String())
	assert.Empty(t, wrongName.Result().Cookies())
	assert.Empty(t, wrongPassword.Result().Cookies())
}

func TestTheThirdFailureStartsTheWait(t *testing.T) {
	t.Parallel()

	f := newFixture(t, "https://recall.example.org")

	// The address is the connection's: a forwarded header the client writes changes nothing.
	for _, forwarded := range []string{"198.51.100.1", "198.51.100.2", "198.51.100.3"} {
		code, got := f.json(t, http.MethodPost, "/api/auth/login", `{"name":"ada","password":"wrong horse battery"}`, "X-Forwarded-For: "+forwarded)
		assert.Equal(t, http.StatusUnauthorized, code)
		refusal(t, got)
	}

	rec := f.send(t, http.MethodPost, "/api/auth/login", `{"name":"ada","password":"`+password+`"}`, "X-Forwarded-For: 198.51.100.4")
	assert.Equal(t, http.StatusTooManyRequests, rec.Code)
	assert.Equal(t, "1", rec.Header().Get("Retry-After"))
	assert.Empty(t, rec.Result().Cookies())

	*f.clock = f.clock.Add(time.Second)

	f.signIn(t)
}

func TestTheAccessOfEachRoute(t *testing.T) {
	t.Parallel()

	f := newFixture(t, "https://recall.example.org")
	cookie := f.signIn(t)
	readToken, writeToken := f.token(t, "phone", "read"), f.token(t, "laptop", "write")

	tests := []struct {
		name    string
		level   module.Access
		method  string
		headers []string
		status  int // 0 when admitted
		caller  module.Caller
	}{
		{"nothing on an open route", module.Open, http.MethodPost, nil, 0, module.Caller{}},
		{"nothing", module.Read, http.MethodGet, nil, http.StatusUnauthorized, module.Caller{}},
		{"a read token reading", module.Read, http.MethodGet, []string{readToken}, 0, module.Caller{UserID: f.user, Source: "phone"}},
		{"a scheme in any case", module.Read, http.MethodGet, []string{strings.Replace(readToken, "Bearer", "bEARER", 1)}, 0, module.Caller{UserID: f.user, Source: "phone"}},
		{"a read token writing", module.Write, http.MethodPost, []string{readToken}, http.StatusForbidden, module.Caller{}},
		{"a write token writing", module.Write, http.MethodPost, []string{writeToken}, 0, module.Caller{UserID: f.user, Source: "laptop", Write: true}},
		{"a write token on an owner route", module.Owner, http.MethodGet, []string{writeToken}, http.StatusForbidden, module.Caller{}},
		{"a read token on an owner route", module.Owner, http.MethodGet, []string{readToken}, http.StatusForbidden, module.Caller{}},
		{"a token before the cookie", module.Owner, http.MethodGet, []string{readToken, cookie}, http.StatusForbidden, module.Caller{}},
		{"the cookie on an owner route", module.Owner, http.MethodPost, []string{cookie, csrf}, 0, module.Caller{UserID: f.user, Source: "web", Write: true}},
		{"the cookie reading without the header", module.Read, http.MethodGet, []string{cookie}, 0, module.Caller{UserID: f.user, Source: "web", Write: true}},
		{"the cookie writing without the header", module.Write, http.MethodPost, []string{cookie}, http.StatusForbidden, module.Caller{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			req := httptest.NewRequestWithContext(t.Context(), tt.method, "/", nil)
			for _, h := range tt.headers {
				name, value, _ := strings.Cut(h, ": ")
				req.Header.Add(name, value)
			}

			rec := httptest.NewRecorder()
			s := &server{Auth: f.auth}

			r, err := s.authorize(rec, req, tt.level)
			if tt.status != 0 {
				fail(rec, req, err)
				assert.Equal(t, tt.status, rec.Code)

				var got map[string]any
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
				refusal(t, got)

				return
			}

			require.NoError(t, err)

			c, ok := module.CallerFrom(r.Context())
			assert.Equal(t, tt.level != module.Open, ok)
			assert.Equal(t, tt.caller, c)
		})
	}
}

func TestACookieRequestWithoutTheCSRFHeaderChangesNothing(t *testing.T) {
	t.Parallel()

	f := newFixture(t, "https://recall.example.org")
	cookie := f.signIn(t)

	code, got := f.json(t, http.MethodPost, "/api/tokens", `{"name":"phone","access":"read"}`, cookie)
	assert.Equal(t, http.StatusForbidden, code)
	refusal(t, got)

	code, got = f.json(t, http.MethodPost, "/api/auth/logout", "", cookie)
	assert.Equal(t, http.StatusForbidden, code)
	refusal(t, got)

	tok, _, err := f.auth.CreateToken(t.Context(), f.user, "phone", "read")
	require.NoError(t, err)

	code, got = f.json(t, http.MethodDelete, "/api/tokens/"+strconv.FormatInt(tok.ID, 10), "", cookie)
	assert.Equal(t, http.StatusForbidden, code)
	refusal(t, got)

	code, got = f.json(t, http.MethodGet, "/api/tokens", "", cookie)
	assert.Equal(t, http.StatusOK, code, "the session lives on")

	list, ok := got["tokens"].([]any)
	require.True(t, ok)
	assert.Len(t, list, 1, "and the one token made elsewhere is neither joined nor revoked")
}

func TestLogout(t *testing.T) {
	t.Parallel()

	f := newFixture(t, "https://recall.example.org")
	cookie := f.signIn(t)
	other := f.signIn(t)

	rec := f.send(t, http.MethodPost, "/api/auth/logout", "", cookie, csrf)
	assert.Equal(t, http.StatusNoContent, rec.Code)

	cleared := sessionCookie(t, rec)
	assert.Empty(t, cleared.Value)
	assert.Negative(t, cleared.MaxAge)

	code, _ := f.json(t, http.MethodGet, "/api/session", "", cookie)
	assert.Equal(t, http.StatusUnauthorized, code)

	code, _ = f.json(t, http.MethodGet, "/api/session", "", other)
	assert.Equal(t, http.StatusOK, code, "another session lives on")

	rec = f.send(t, http.MethodPost, "/api/auth/logout", "", f.token(t, "phone", "read"))
	assert.Equal(t, http.StatusNoContent, rec.Code, "a token has no session to end")

	// A session old enough to be renewed is answered with the cleared cookie alone.
	*f.clock = f.clock.Add(2 * day)

	rec = f.send(t, http.MethodPost, "/api/auth/logout", "", other, csrf)
	require.Equal(t, http.StatusNoContent, rec.Code)
	require.Len(t, rec.Result().Cookies(), 1)
	assert.Negative(t, sessionCookie(t, rec).MaxAge)
}

func TestTheSessionSlides(t *testing.T) {
	t.Parallel()

	f := newFixture(t, "https://recall.example.org")
	cookie := f.signIn(t)

	rec := f.send(t, http.MethodGet, "/api/session", "", cookie)
	assert.Empty(t, rec.Result().Cookies(), "a fresh session is not sent again")

	*f.clock = f.clock.Add(29 * day)

	rec = f.send(t, http.MethodGet, "/api/session", "", cookie)
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, f.clock.Add(30*day).Unix(), sessionCookie(t, rec).Expires.Unix())

	*f.clock = f.clock.Add(30 * day)

	code, _ := f.json(t, http.MethodGet, "/api/session", "", cookie)
	assert.Equal(t, http.StatusUnauthorized, code)
}

func TestTokens(t *testing.T) {
	t.Parallel()

	f := newFixture(t, "https://recall.example.org")
	cookie := f.signIn(t)

	code, made := f.json(t, http.MethodPost, "/api/tokens", `{"name":" phone ","access":"read"}`, cookie, csrf)
	require.Equal(t, http.StatusCreated, code)

	text, ok := made["token"].(string)
	require.True(t, ok)
	assert.True(t, strings.HasPrefix(text, "recall_"))
	assert.Len(t, text, len("recall_")+43)

	created := f.clock.UTC().Format(time.RFC3339)
	assert.Equal(t, map[string]any{
		"id": made["id"], "name": "phone", "access": "read", "created_at": created, "last_used_at": nil, "token": text,
	}, made)

	*f.clock = f.clock.Add(time.Minute)

	code, _ = f.json(t, http.MethodGet, "/api/session", "", "Authorization: Bearer "+text)
	assert.Equal(t, http.StatusOK, code)

	code, list := f.json(t, http.MethodGet, "/api/tokens", "", cookie)
	assert.Equal(t, http.StatusOK, code)
	assert.Equal(t, map[string]any{"tokens": []any{map[string]any{
		"id": made["id"], "name": "phone", "access": "read", "created_at": created, "last_used_at": f.clock.UTC().Format(time.RFC3339),
	}}}, list, "the token's text is shown only when it is made")

	for _, body := range []string{
		`{"name":"","access":"read"}`,
		`{"name":"   ","access":"read"}`,
		`{"name":"` + strings.Repeat("a", 65) + `","access":"read"}`,
		`{"name":"a\nb","access":"read"}`,
		`{"name":"Web","access":"write"}`,
		`{"name":"laptop","access":"owner"}`,
		`{"name":"laptop"}`,
	} {
		code, got := f.json(t, http.MethodPost, "/api/tokens", body, cookie, csrf)
		assert.Equal(t, http.StatusBadRequest, code, body)
		refusal(t, got)
	}

	code, got := f.json(t, http.MethodPost, "/api/tokens", `{"name":"phone","access":"write"}`, cookie, csrf)
	assert.Equal(t, http.StatusConflict, code)
	refusal(t, got)

	n, ok := made["id"].(float64)
	require.True(t, ok)

	id := strconv.FormatInt(int64(n), 10)

	for _, target := range []string{"/api/tokens/999999", "/api/tokens/abc"} {
		code, got := f.json(t, http.MethodDelete, target, "", cookie, csrf)
		assert.Equal(t, http.StatusNotFound, code, target)
		refusal(t, got)
	}

	rec := f.send(t, http.MethodDelete, "/api/tokens/"+id, "", cookie, csrf)
	assert.Equal(t, http.StatusNoContent, rec.Code)

	code, _ = f.json(t, http.MethodGet, "/api/session", "", "Authorization: Bearer "+text)
	assert.Equal(t, http.StatusUnauthorized, code, "a revoked token reads nothing")
}

func TestNoTokenReachesTheTokenRoutes(t *testing.T) {
	t.Parallel()

	f := newFixture(t, "https://recall.example.org")
	writeToken := f.token(t, "laptop", "write")

	for _, tt := range []struct{ method, target, body string }{
		{http.MethodGet, "/api/tokens", ""},
		{http.MethodPost, "/api/tokens", `{"name":"more","access":"write"}`},
		{http.MethodDelete, "/api/tokens/1", ""},
	} {
		code, got := f.json(t, tt.method, tt.target, tt.body, writeToken)
		assert.Equal(t, http.StatusForbidden, code, tt.method)
		refusal(t, got)
	}

	list, err := f.auth.Tokens(t.Context(), f.user)
	require.NoError(t, err)
	assert.Len(t, list, 1, "nothing was made or revoked")
}
