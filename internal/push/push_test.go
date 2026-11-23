package push_test

import (
	"bufio"
	"crypto/ecdh"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mhersson/recallsynapsis/internal/api"
	"github.com/mhersson/recallsynapsis/internal/auth"
	"github.com/mhersson/recallsynapsis/internal/config"
	"github.com/mhersson/recallsynapsis/internal/core"
	"github.com/mhersson/recallsynapsis/internal/module"
	"github.com/mhersson/recallsynapsis/internal/push"
	"github.com/mhersson/recallsynapsis/internal/store"
)

type object = map[string]any

const password = "correct horse battery"

// passwordHash is password's hash, made once since each costs an argon2 run.
var passwordHash = sync.OnceValue(func() string {
	h, err := auth.HashPassword(password)
	if err != nil {
		panic(err)
	}

	return h
})

// start is the fixture's clock: 14:00 on Monday 23 November 2026 in Oslo,
// ten hours before the day ends there.
var start = time.Date(2026, 11, 23, 13, 0, 0, 0, time.UTC)

// fixture serves a database whose users ada and grace each have their
// personal area, with push on. Push's routes ask for the session cookie,
// which signIn gives.
type fixture struct {
	h      http.Handler
	store  *store.Store
	auth   *auth.Service
	core   *core.Service
	hub    *api.Hub
	client *http.Client // trusts the test's push services
	ms     *core.Modules
	ada    int64
	grace  int64
	laptop string // ada's write token
}

func newFixture(t *testing.T) *fixture {
	t.Helper()

	st, err := store.Open(t.Context(), filepath.Join(t.TempDir(), "recallsynapsis.db"), slices.Concat(auth.Migrations, core.Migrations))
	require.NoError(t, err)

	t.Cleanup(func() { _ = st.Close() })

	// Every TLS server of httptest has the one certificate, so the client of one trusts them all.
	probe := httptest.NewTLSServer(http.NotFoundHandler())
	probe.Close()

	f := &fixture{store: st, auth: auth.New(st, time.Now), core: core.New(st), hub: api.NewHub(t.Context().Done()), client: probe.Client()}

	user := func(name string) int64 {
		var id int64
		require.NoError(t, st.QueryRowContext(t.Context(), `INSERT INTO users (name, created_at) VALUES (?, 0) RETURNING id`, name).Scan(&id))
		require.NoError(t, f.core.EnsurePersonal(t.Context(), id))
		require.NoError(t, f.auth.SetPasswordHash(t.Context(), id, passwordHash()))

		return id
	}
	token := func(user int64, name string) string {
		_, text, err := f.auth.CreateToken(t.Context(), user, name, "write")
		require.NoError(t, err)

		return "Bearer " + text
	}

	f.ada, f.grace = user("ada"), user("grace")
	f.laptop = token(f.ada, "laptop")
	f.load(t, object{"enabled": true})

	return f
}

// load serves the database again, with table as [modules.push], or none when nil.
func (f *fixture) load(t *testing.T, table object) {
	t.Helper()

	tables := map[string]map[string]any{}
	if table != nil {
		tables["push"] = table
	}

	zone, err := time.LoadLocation("Europe/Oslo")
	require.NoError(t, err)

	ms, err := f.core.LoadModules(t.Context(), []func(module.Deps) module.Module{push.WithClient(f.client)},
		&config.Config{Modules: tables}, module.Deps{Now: func() time.Time { return start }, Zone: zone, PublicURL: contact, Changed: f.hub.Changed})
	require.NoError(t, err)

	f.ms, f.h = ms, api.New(api.Deps{Auth: f.auth, Core: f.core, Modules: ms, PublicURL: contact, Hub: f.hub})
}

// signIn gives the user's session, as do takes it.
func (f *fixture) signIn(t *testing.T, name string) string {
	t.Helper()

	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/api/auth/login",
		strings.NewReader(`{"name":"`+name+`","password":"`+password+`"}`))
	rec := httptest.NewRecorder()
	f.h.ServeHTTP(rec, req)
	require.Equal(t, http.StatusNoContent, rec.Code, rec.Body.String())

	cookies := rec.Result().Cookies()
	require.Len(t, cookies, 1)

	return "Cookie " + cookies[0].Name + "=" + cookies[0].Value
}

// do serves one request with auth, the Authorization header of a token, a
// session as signIn gives it, or none, and decodes its answer, nil for a 204.
func (f *fixture) do(t *testing.T, method, target, body, auth string) (int, object) {
	t.Helper()

	req := httptest.NewRequestWithContext(t.Context(), method, target, strings.NewReader(body))
	if cookie, ok := strings.CutPrefix(auth, "Cookie "); ok {
		req.Header.Set("Cookie", cookie)
		req.Header.Set("X-Recallsynapsis-CSRF", "1")
	} else if auth != "" {
		req.Header.Set("Authorization", auth)
	}

	rec := httptest.NewRecorder()
	f.h.ServeHTTP(rec, req)

	if rec.Code == http.StatusNoContent {
		assert.Empty(t, rec.Body.String())

		return rec.Code, nil
	}

	assert.Equal(t, "application/json", rec.Header().Get("Content-Type"))

	var got object
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got), rec.Body.String())

	return rec.Code, got
}

// register registers the browser of the subscription body with session and gives its id.
func (f *fixture) register(t *testing.T, body, session string) float64 {
	t.Helper()

	code, got := f.do(t, http.MethodPut, "/api/push", body, session)
	require.Equal(t, http.StatusOK, code, got)
	require.Len(t, got, 1)

	return got["id"].(float64)
}

// state is what GET /api/push answers session, with query.
func (f *fixture) state(t *testing.T, query, session string) object {
	t.Helper()

	code, got := f.do(t, http.MethodGet, "/api/push"+query, "", session)
	require.Equal(t, http.StatusOK, code, got)

	return got
}

// browserOf is the browser id as GET /api/push gives it to session, nil when it is none of theirs.
func (f *fixture) browserOf(t *testing.T, id float64, session string) any {
	t.Helper()

	return f.state(t, fmt.Sprintf("?browser=%v", id), session)["browser"]
}

// notify sends the user the message m, given as JSON, as a sender does: from
// a context with no caller.
func (f *fixture) notify(t *testing.T, user int64, m string) {
	t.Helper()

	var msg module.Message
	require.NoError(t, json.Unmarshal([]byte(m), &msg))
	f.ms.Notify(t.Context(), user, msg)
}

// refused wants a refusal with status, in the one error shape.
func refused(t *testing.T, status, code int, got object) {
	t.Helper()

	assert.Equal(t, status, code, got)
	assert.Len(t, got, 1)
	assert.NotEmpty(t, got["error"])
}

func (f *fixture) count(t *testing.T, table string) int {
	t.Helper()

	var n int
	require.NoError(t, f.store.QueryRowContext(t.Context(), `SELECT count(*) FROM `+table).Scan(&n))

	return n
}

// json is the subscription to endpoint as the page sends it.
func (b browser) json(endpoint string) string {
	return fmt.Sprintf(`{"endpoint":%q,"keys":{"p256dh":%q,"auth":%q}}`,
		endpoint, base64.RawURLEncoding.EncodeToString(b.key.PublicKey().Bytes()), base64.RawURLEncoding.EncodeToString(b.auth))
}

// routes are push's routes, with a body each takes.
var routes = []struct{ method, target, body string }{
	{http.MethodGet, "/api/push", ""},
	{http.MethodPut, "/api/push", `{"endpoint":"https://push.example.net/p/JzLQ3raZ","keys":{"p256dh":"","auth":""}}`},
	{http.MethodDelete, "/api/push/1", ""},
	{http.MethodDelete, "/api/push", ""},
}

func TestABrowserIsRegisteredWithTheSessionAlone(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	session := f.signIn(t, "ada")
	srv := newService(t, http.StatusCreated, "")

	got := f.state(t, "", session)
	key, _ := got["key"].(string)
	_, err := ecdh.P256().NewPublicKey(unb64(t, key))
	require.NoError(t, err, "the key is a P-256 point, as applicationServerKey takes it")
	assert.Equal(t, object{"key": key, "on": 0.0, "browser": nil}, got)

	phone := newBrowser(t).json(srv.URL + "/push/phone")
	id := f.register(t, phone, session)
	assert.InDelta(t, id, f.register(t, phone, session), 0, "registered twice it is one browser")
	assert.NotEqual(t, id, f.register(t, newBrowser(t).json(srv.URL+"/push/desk"), session))

	assert.Equal(t, object{"key": key, "on": 2.0, "browser": object{"id": id, "failed_at": nil, "failure": ""}},
		f.state(t, fmt.Sprintf("?browser=%v", id), session))

	f.load(t, object{"enabled": true})
	assert.Equal(t, key, f.state(t, "", session)["key"], "the same key after a restart")

	for auth, status := range map[string]int{f.laptop: http.StatusForbidden, "": http.StatusUnauthorized} {
		for _, r := range routes {
			code, got := f.do(t, r.method, r.target, r.body, auth)
			refused(t, status, code, got)
		}
	}

	assert.InDelta(t, 2, f.state(t, "", session)["on"], 0, "no refused call changed anything")

	code, answer := f.do(t, http.MethodGet, "/api/push?browser=phone", "", session)
	refused(t, http.StatusBadRequest, code, answer)
}

func TestASubscriptionAPushServiceCannotTakeIsRefused(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	session := f.signIn(t, "ada")
	b := newBrowser(t)
	p256dh, auth := base64.RawURLEncoding.EncodeToString(b.key.PublicKey().Bytes()), base64.RawURLEncoding.EncodeToString(b.auth)
	subscription := func(endpoint, p256dh, auth string) string {
		return fmt.Sprintf(`{"endpoint":%q,"keys":{"p256dh":%q,"auth":%q}}`, endpoint, p256dh, auth)
	}

	for name, body := range map[string]string{
		"not https":              subscription("http://push.example.net/p/JzLQ3raZ", p256dh, auth),
		"no host":                subscription("https:///p/JzLQ3raZ", p256dh, auth),
		"not a URL":              subscription("https://push example/p/JzLQ3raZ", p256dh, auth),
		"a key that is no point": subscription("https://push.example.net/p/JzLQ3raZ", base64.RawURLEncoding.EncodeToString(make([]byte, 65)), auth),
		"a key that is no text":  subscription("https://push.example.net/p/JzLQ3raZ", "not base64!", auth),
		"a secret not 16 bytes":  subscription("https://push.example.net/p/JzLQ3raZ", p256dh, base64.RawURLEncoding.EncodeToString(b.auth[:15])),
		"a field it does not take": `{"endpoint":"https://push.example.net/p/JzLQ3raZ","expirationTime":null,"keys":{"p256dh":"` +
			p256dh + `","auth":"` + auth + `"}}`,
	} {
		code, got := f.do(t, http.MethodPut, "/api/push", body, session)
		refused(t, http.StatusBadRequest, code, got)
		assert.NotContains(t, got["error"], "JzLQ3raZ", name+": the endpoint is a capability")
	}

	assert.Zero(t, f.count(t, "push_subscriptions"))
}

func TestAMessageReachesEveryBrowserOfItsUserAlone(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	ada, grace := f.signIn(t, "ada"), f.signIn(t, "grace")
	apple, mozilla := newService(t, http.StatusCreated, ""), newService(t, http.StatusCreated, "")
	phone, desk, hers := newBrowser(t), newBrowser(t), newBrowser(t)

	f.register(t, phone.json(apple.URL+"/push/phone"), ada)
	f.register(t, desk.json(mozilla.URL+"/push/desk"), ada)
	f.register(t, hers.json(apple.URL+"/push/hers"), grace)

	key, _ := f.state(t, "", ada)["key"].(string)
	reminder := `{"title":"Ring the carpenter","body":"14:00 · Hytta","url":"/tasks/12","tag":"tasks-12"}`

	f.notify(t, f.ada, reminder)

	for _, to := range []struct {
		service *service
		browser browser
		path    string
	}{{apple, phone, "/push/phone"}, {mozilla, desk, "/push/desk"}} {
		p := to.service.received(t)
		assert.Equal(t, to.path, p.path)
		assert.JSONEq(t, reminder, string(to.browser.open(t, p.body)))
		assert.Equal(t, "aes128gcm", p.header.Get("Content-Encoding"))
		assert.Equal(t, "36000", p.header.Get("TTL"), "it waits until the day ends in the zone")
		assert.Equal(t, "high", p.header.Get("Urgency"))
		assert.Equal(t, "tasks-12", p.header.Get("Topic"))
		signed(t, p.header.Get("Authorization"), key, to.service.URL)
	}

	apple.quiet(t)
	mozilla.quiet(t)

	planned := `{"title":"2 planned for today","body":"Rake\nSweep","url":"/tasks","tag":""}`
	f.notify(t, f.grace, planned)

	p := apple.received(t)
	assert.Equal(t, "/push/hers", p.path)
	assert.JSONEq(t, planned, string(hers.open(t, p.body)))
	assert.NotContains(t, p.header, "Topic", "no tag, no topic")

	apple.quiet(t)
	mozilla.quiet(t)
}

// signed wants header to be a token for the push service at origin, signed
// with the key GET /api/push gives and naming public_url as the contact.
func signed(t *testing.T, header, key, origin string) {
	t.Helper()

	rest, ok := strings.CutPrefix(header, "vapid t=")
	require.True(t, ok, header)

	token, k, ok := strings.Cut(rest, ", k=")
	require.True(t, ok, header)
	assert.Equal(t, key, k)

	public, err := ecdsa.ParseUncompressedPublicKey(elliptic.P256(), unb64(t, key))
	require.NoError(t, err)

	parts := strings.Split(token, ".")
	require.Len(t, parts, 3)

	sig := unb64(t, parts[2])
	require.Len(t, sig, 64)

	digest := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	assert.True(t, ecdsa.Verify(public, digest[:], new(big.Int).SetBytes(sig[:32]), new(big.Int).SetBytes(sig[32:])))
	assert.JSONEq(t, fmt.Sprintf(`{"aud":%q,"exp":%d,"sub":%q}`, origin, start.Add(12*time.Hour).Unix(), contact), string(unb64(t, parts[1])))
}

func TestABrowserGoneIsForgottenAndAFailureKeptUntilOneGoesThrough(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	session := f.signIn(t, "ada")
	gone, broken, down, fine := newService(t, http.StatusGone, ""), newService(t, http.StatusInternalServerError, "try later"),
		newService(t, http.StatusCreated, ""), newService(t, http.StatusCreated, "")

	down.Close()

	g := f.register(t, newBrowser(t).json(gone.URL+"/push/JzLQ3raZ"), session)
	b := f.register(t, newBrowser(t).json(broken.URL+"/push/JzLQ3raZ"), session)
	d := f.register(t, newBrowser(t).json(down.URL+"/push/JzLQ3raZ"), session)
	ok := f.register(t, newBrowser(t).json(fine.URL+"/push/JzLQ3raZ"), session)

	reminder := `{"title":"Ring the carpenter","body":"14:00","url":"/tasks/12","tag":"tasks-12"}`
	f.notify(t, f.ada, reminder)

	for _, s := range []*service{gone, broken, fine} {
		s.received(t)
	}

	failed := func(id float64, line string) object {
		return object{"id": id, "failed_at": "2026-11-23T13:00:00Z", "failure": line}
	}

	assert.InDelta(t, 3, f.state(t, "", session)["on"], 0, "the browser gone is forgotten")
	assert.Nil(t, f.browserOf(t, g, session))
	assert.Equal(t, failed(b, broken.Listener.Addr().String()+" answered 500"), f.browserOf(t, b, session))
	assert.Equal(t, failed(d, down.Listener.Addr().String()+" was not reached"), f.browserOf(t, d, session))
	assert.Equal(t, object{"id": ok, "failed_at": nil, "failure": ""}, f.browserOf(t, ok, session))

	broken.status.Store(http.StatusCreated)
	f.notify(t, f.ada, reminder)

	for _, s := range []*service{broken, fine} {
		s.received(t)
	}

	gone.quiet(t)
	assert.Equal(t, object{"id": b, "failed_at": nil, "failure": ""}, f.browserOf(t, b, session), "a message that went through clears it")
	assert.Equal(t, failed(d, down.Listener.Addr().String()+" was not reached"), f.browserOf(t, d, session))
}

func TestABrowserIsRemovedForItsUserAlone(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	ada, grace := f.signIn(t, "ada"), f.signIn(t, "grace")
	srv := newService(t, http.StatusCreated, "")

	one := f.register(t, newBrowser(t).json(srv.URL+"/push/one"), ada)
	f.register(t, newBrowser(t).json(srv.URL+"/push/two"), ada)
	hers := f.register(t, newBrowser(t).json(srv.URL+"/push/hers"), grace)

	assert.Nil(t, f.browserOf(t, hers, ada), "a browser's id is given to its user alone")

	for _, target := range []string{fmt.Sprintf("/api/push/%v", hers), "/api/push/hers"} {
		code, got := f.do(t, http.MethodDelete, target, "", ada)
		refused(t, http.StatusNotFound, code, got)
	}

	code, got := f.do(t, http.MethodDelete, fmt.Sprintf("/api/push/%v", one), "", ada)
	require.Equal(t, http.StatusNoContent, code, got)
	assert.InDelta(t, 1, f.state(t, "", ada)["on"], 0)

	code, got = f.do(t, http.MethodDelete, fmt.Sprintf("/api/push/%v", one), "", ada)
	refused(t, http.StatusNotFound, code, got)

	code, got = f.do(t, http.MethodDelete, "/api/push", "", ada)
	require.Equal(t, http.StatusNoContent, code, got)
	assert.InDelta(t, 0, f.state(t, "", ada)["on"], 0)
	assert.InDelta(t, 1, f.state(t, "", grace)["on"], 0, "hers is still on")

	f.notify(t, f.ada, `{"title":"Rake","body":"","url":"/tasks/1","tag":"tasks-1"}`)
	srv.quiet(t)

	f.notify(t, f.grace, `{"title":"Sweep","body":"","url":"/tasks/2","tag":"tasks-2"}`)
	assert.Equal(t, "/push/hers", srv.received(t).path)
}

func TestPushSwitchedOffKeepsItsBrowsers(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	session := f.signIn(t, "ada")
	srv := newService(t, http.StatusCreated, "")

	f.register(t, newBrowser(t).json(srv.URL+"/push/one"), session)
	f.register(t, newBrowser(t).json(srv.URL+"/push/two"), session)

	want := f.state(t, "", session)
	reminder := `{"title":"Rake","body":"14:00","url":"/tasks/1","tag":"tasks-1"}`

	for _, table := range []object{{"enabled": false}, nil} {
		f.load(t, table)

		for _, r := range routes {
			code, got := f.do(t, r.method, r.target, r.body, session)
			refused(t, http.StatusNotFound, code, got)
		}

		f.notify(t, f.ada, reminder)
		srv.quiet(t)

		assert.Equal(t, 2, f.count(t, "push_subscriptions"))
		assert.Equal(t, 1, f.count(t, "push_key"))
	}

	f.load(t, object{"enabled": true})
	assert.Equal(t, want, f.state(t, "", session), "the browsers and the key are as they were")

	f.notify(t, f.ada, reminder)
	srv.received(t)
	srv.received(t)
}

func TestEachChangeOfABrowserReachesAnOpenStream(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	session := f.signIn(t, "ada")
	h := httptest.NewServer(f.h)
	t.Cleanup(h.Close)

	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, h.URL+"/api/events", nil)
	require.NoError(t, err)
	req.Header.Set("Authorization", f.laptop)

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	t.Cleanup(func() { _ = resp.Body.Close() })
	require.Equal(t, http.StatusOK, resp.StatusCode)

	// Each line but blank and comment lines, until the stream ends.
	lines := make(chan string, 16)

	go func() {
		defer close(lines)

		for sc := bufio.NewScanner(resp.Body); sc.Scan(); {
			if sc.Text() != "" && !strings.HasPrefix(sc.Text(), ":") {
				lines <- sc.Text()
			}
		}
	}()

	next := func() string {
		t.Helper()

		select {
		case line, ok := <-lines:
			require.True(t, ok, "the stream ended")

			return line
		case <-time.After(10 * time.Second):
			require.FailNow(t, "the stream sent nothing")
		}

		return ""
	}
	told := func() {
		t.Helper()
		assert.JSONEq(t, `{"module":"push","kind":"browser","id":0,"area":0}`, strings.TrimPrefix(next(), "data: "))
	}

	require.Equal(t, "retry: 1000", next(), "the stream is open")

	rake := `{"title":"Rake","body":"14:00","url":"/tasks/1","tag":"tasks-1"}`
	gone, failing := newService(t, http.StatusGone, ""), newService(t, http.StatusInternalServerError, "")

	f.register(t, newBrowser(t).json(gone.URL+"/push/one"), session)
	told()

	f.notify(t, f.ada, rake)
	gone.received(t)

	told() // it is gone

	id := f.register(t, newBrowser(t).json(failing.URL+"/push/two"), session)

	told()

	f.notify(t, f.ada, rake)
	failing.received(t)

	told() // its failure

	failing.status.Store(http.StatusCreated)
	f.notify(t, f.ada, rake)
	failing.received(t)

	told() // its failure cleared

	code, got := f.do(t, http.MethodDelete, fmt.Sprintf("/api/push/%v", id), "", session)
	require.Equal(t, http.StatusNoContent, code, got)
	told()

	code, got = f.do(t, http.MethodDelete, "/api/push", "", session)
	require.Equal(t, http.StatusNoContent, code, got)
	told()
}
