package api

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mhersson/recallsynapsis/internal/module"
)

// within bounds how long a test waits for a stream.
const within = 10 * time.Second

// stream is an open event stream. Each line it sends but blank and comment
// lines arrives on lines, which closes when the stream ends, and each comment
// line on comments.
type stream struct {
	lines    chan string
	comments chan struct{}
}

// getEvents sends GET /api/events to the server at url with each header "Name: value".
func getEvents(t *testing.T, url string, headers ...string) *http.Response {
	t.Helper()

	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, url+"/api/events", nil)
	require.NoError(t, err)

	for _, h := range headers {
		name, value, _ := strings.Cut(h, ": ")
		req.Header.Add(name, value)
	}

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	t.Cleanup(func() { _ = resp.Body.Close() })

	return resp
}

// refusedEvents wants the server at url to refuse the event stream to the
// headers as unauthorised, in the one error shape.
func refusedEvents(t *testing.T, url string, headers ...string) {
	t.Helper()

	resp := getEvents(t, url, headers...)
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)

	// A stream opened in error starts with its retry line, which fails the decoding at once.
	var got map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&got))
	refusal(t, got)
}

// openStream opens the event stream of the server at url with each header
// "Name: value", and wants it to open by asking for a reconnect within a second.
func openStream(t *testing.T, url string, headers ...string) *stream {
	t.Helper()

	resp := getEvents(t, url, headers...)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "text/event-stream", resp.Header.Get("Content-Type"))
	assert.Equal(t, "no-store", resp.Header.Get("Cache-Control"))
	assert.Equal(t, "no", resp.Header.Get("X-Accel-Buffering"))

	s := &stream{lines: make(chan string), comments: make(chan struct{}, 1)}

	go func() {
		defer close(s.lines)

		sc := bufio.NewScanner(resp.Body)
		for sc.Scan() {
			switch line := sc.Text(); {
			case line == "":
			case strings.HasPrefix(line, ":"):
				select {
				case s.comments <- struct{}{}:
				default:
				}
			default:
				s.lines <- line
			}
		}
	}()

	// Closing the body ends the reader, which may be waiting to send a line.
	t.Cleanup(func() {
		_ = resp.Body.Close()

		for range s.lines {
		}
	})

	select {
	case line := <-s.lines:
		require.Equal(t, "retry: 1000", line)
	case <-time.After(within):
		require.FailNow(t, "the stream sent nothing")
	}

	return s
}

// expect wants the stream's next line to be one event whose data is the JSON want.
func (s *stream) expect(t *testing.T, want string) {
	t.Helper()

	select {
	case line, ok := <-s.lines:
		require.True(t, ok, "the stream ended before %s", want)

		data, found := strings.CutPrefix(line, "data: ")
		require.True(t, found, "an event is a data line alone, not %q", line)
		assert.JSONEq(t, want, data)
	case <-time.After(within):
		require.FailNow(t, "no event arrived", want)
	}
}

// ends wants the stream to end with nothing more sent.
func (s *stream) ends(t *testing.T) {
	t.Helper()

	select {
	case line, ok := <-s.lines:
		require.False(t, ok, "the stream sent %q", line)
	case <-time.After(within):
		require.FailNow(t, "the stream did not end")
	}
}

// areaEvent is the event of a change to the area an answer gives.
func areaEvent(t *testing.T, area map[string]any) string {
	t.Helper()

	id, ok := area["id"].(float64)
	require.True(t, ok, area)

	return fmt.Sprintf(`{"module":"core","kind":"area","id":%[1]d,"area":%[1]d}`, int64(id))
}

// personal is the personal area of the caller the headers sign in, as GET /api/areas gives it.
func (f *fixture) personal(t *testing.T, headers ...string) map[string]any {
	t.Helper()

	code, got := f.json(t, http.MethodGet, "/api/areas", "", headers...)
	require.Equal(t, http.StatusOK, code, got)

	list, ok := got["areas"].([]any)
	require.True(t, ok, got)

	area, ok := list[0].(map[string]any)
	require.True(t, ok, got)

	return area
}

// grace adds the user grace with her personal area, and returns the
// Authorization header of her write token.
func (f *fixture) grace(t *testing.T) string {
	t.Helper()

	var id int64
	require.NoError(t, f.store.QueryRowContext(t.Context(), `INSERT INTO users (name, created_at) VALUES ('grace', 0) RETURNING id`).Scan(&id))
	require.NoError(t, f.core.EnsurePersonal(t.Context(), id))

	_, text, err := f.auth.CreateToken(t.Context(), id, "laptop", "write")
	require.NoError(t, err)

	return "Authorization: Bearer " + text
}

func TestAStreamNeedsASessionOrAToken(t *testing.T) {
	t.Parallel()

	f := newFixture(t, "https://recall.example.org")
	srv := httptest.NewServer(f.h)
	t.Cleanup(srv.Close)

	ended := f.signIn(t)
	rec := f.send(t, http.MethodPost, "/api/auth/logout", "", ended, csrf)
	require.Equal(t, http.StatusNoContent, rec.Code)

	refusedEvents(t, srv.URL)
	refusedEvents(t, srv.URL, "Authorization: Bearer recall_"+strings.Repeat("x", 43))
	refusedEvents(t, srv.URL, ended)
}

func TestAChangeReachesTheOpenStreamsOfItsUserAlone(t *testing.T) {
	t.Parallel()

	f := newFixture(t, "https://recall.example.org")
	srv := httptest.NewServer(f.h)
	t.Cleanup(srv.Close)

	cookie, grace := f.signIn(t), f.grace(t)

	// A GET needs no CSRF header, so a browser's EventSource opens it with the cookie alone.
	web, phone := openStream(t, srv.URL, cookie), openStream(t, srv.URL, f.token(t, "phone", "read"))
	theirs := openStream(t, srv.URL, grace)

	code, garden := f.json(t, http.MethodPost, "/api/areas", `{"name":"Garden"}`, cookie, csrf)
	require.Equal(t, http.StatusCreated, code)

	code, _ = f.json(t, http.MethodPatch, areaPath(t, garden), `{"archived":true}`, cookie, csrf)
	require.Equal(t, http.StatusOK, code)

	personal := f.personal(t, cookie)
	code, _ = f.json(t, http.MethodPatch, areaPath(t, personal), `{"description":"Mine."}`, cookie, csrf)
	require.Equal(t, http.StatusOK, code)

	for _, s := range []*stream{web, phone} {
		s.expect(t, areaEvent(t, garden))
		s.expect(t, areaEvent(t, garden))
		s.expect(t, areaEvent(t, personal))
	}

	code, orchard := f.json(t, http.MethodPost, "/api/areas", `{"name":"Orchard"}`, grace)
	require.Equal(t, http.StatusCreated, code)
	theirs.expect(t, areaEvent(t, orchard))

	code, shed := f.json(t, http.MethodPost, "/api/areas", `{"name":"Shed"}`, cookie, csrf)
	require.Equal(t, http.StatusCreated, code)
	web.expect(t, areaEvent(t, shed))
}

func TestARefusedOrFailedChangeTellsOfNothing(t *testing.T) {
	t.Parallel()

	f := newFixture(t, "https://recall.example.org")
	srv := httptest.NewServer(f.h)
	t.Cleanup(srv.Close)

	cookie, grace := f.signIn(t), f.grace(t)
	s, theirs := openStream(t, srv.URL, cookie), openStream(t, srv.URL, grace)

	code, garden := f.json(t, http.MethodPost, "/api/areas", `{"name":"Garden"}`, cookie, csrf)
	require.Equal(t, http.StatusCreated, code)
	s.expect(t, areaEvent(t, garden))

	personal, read := areaPath(t, f.personal(t, cookie)), f.token(t, "phone", "read")

	for _, tt := range []struct {
		method, target, body string
		headers              []string
		status               int
	}{
		{http.MethodPost, "/api/areas", `{"name":"Shed"}`, []string{read}, http.StatusForbidden},
		{http.MethodPost, "/api/areas", `{"name":"Shed"}`, []string{cookie}, http.StatusForbidden},
		{http.MethodPost, "/api/areas", `{"name":"GARDEN"}`, []string{cookie, csrf}, http.StatusConflict},
		{http.MethodPost, "/api/areas", `{"name":""}`, []string{cookie, csrf}, http.StatusBadRequest},
		{http.MethodPatch, personal, `{"name":"Home"}`, []string{cookie, csrf}, http.StatusBadRequest},
		{http.MethodPatch, "/api/areas/999999", `{"archived":true}`, []string{cookie, csrf}, http.StatusNotFound},
		{http.MethodPatch, areaPath(t, garden), `{"archived":true}`, []string{grace}, http.StatusNotFound},
	} {
		code, got := f.json(t, tt.method, tt.target, tt.body, tt.headers...)
		assert.Equal(t, tt.status, code, tt)
		refusal(t, got)
	}

	code, _ = f.json(t, http.MethodPatch, areaPath(t, garden), `{"description":"The beds."}`, cookie, csrf)
	require.Equal(t, http.StatusOK, code)
	s.expect(t, areaEvent(t, garden))

	code, orchard := f.json(t, http.MethodPost, "/api/areas", `{"name":"Orchard"}`, grace)
	require.Equal(t, http.StatusCreated, code)
	theirs.expect(t, areaEvent(t, orchard))
}

func TestAStreamSendsACommentLineWhileNothingChanges(t *testing.T) {
	t.Parallel()

	f := newFixture(t, "https://recall.example.org")
	f.hub.keepAlive = 100 * time.Millisecond
	srv := httptest.NewServer(f.h)
	t.Cleanup(srv.Close)

	s := openStream(t, srv.URL, f.signIn(t))

	for range 2 {
		select {
		case <-s.comments:
		case <-time.After(within):
			require.FailNow(t, "no comment line arrived")
		}
	}
}

func TestAStreamEndsAtItsLifetimeAndOpensAgainOnlyWithItsAuthority(t *testing.T) {
	t.Parallel()

	f := newFixture(t, "https://recall.example.org")
	f.hub.lifetime = 200 * time.Millisecond
	srv := httptest.NewServer(f.h)
	t.Cleanup(srv.Close)

	_, kept, err := f.auth.CreateToken(t.Context(), f.user, "phone", "read")
	require.NoError(t, err)

	gone, text, err := f.auth.CreateToken(t.Context(), f.user, "laptop", "read")
	require.NoError(t, err)

	keeps, revoked := openStream(t, srv.URL, "Authorization: Bearer "+kept), openStream(t, srv.URL, "Authorization: Bearer "+text)
	require.NoError(t, f.auth.DeleteToken(t.Context(), f.user, gone.ID))

	revoked.ends(t)
	keeps.ends(t)

	refusedEvents(t, srv.URL, "Authorization: Bearer "+text)
	openStream(t, srv.URL, "Authorization: Bearer "+kept)
}

// unread is the writer of a stream whose client reads nothing: from the
// first flush, which would send the headers, it waits until it is released,
// or until the write deadline, when it fails as a connection does.
type unread struct {
	*httptest.ResponseRecorder
	flushed, read    chan struct{}
	opened, released sync.Once
	deadline         time.Time
	cancel           context.CancelFunc // ends the request's context, as a failed write does
}

// release lets the client read.
func (u *unread) release() { u.released.Do(func() { close(u.read) }) }

func (u *unread) SetWriteDeadline(d time.Time) error {
	u.deadline = d

	return nil
}

func (u *unread) FlushError() error {
	u.opened.Do(func() { close(u.flushed) })

	var expired <-chan time.Time
	if !u.deadline.IsZero() {
		expired = time.After(time.Until(u.deadline))
	}

	select {
	case <-u.read:
		return nil
	case <-expired:
		u.cancel()

		return os.ErrDeadlineExceeded
	}
}

// stall opens the event stream with the headers to a client that reads
// nothing, and returns its writer and a channel closed when the stream ends.
func (f *fixture) stall(t *testing.T, headers ...string) (*unread, <-chan struct{}) {
	t.Helper()

	ctx, cancel := context.WithCancel(t.Context())
	t.Cleanup(cancel)

	req := httptest.NewRequestWithContext(ctx, http.MethodGet, "/api/events", nil)

	for _, h := range headers {
		name, value, _ := strings.Cut(h, ": ")
		req.Header.Add(name, value)
	}

	w := &unread{ResponseRecorder: httptest.NewRecorder(), flushed: make(chan struct{}), read: make(chan struct{}), cancel: cancel}
	streamed := make(chan struct{})

	go func() {
		defer close(streamed)

		f.h.ServeHTTP(w, req)
	}()

	t.Cleanup(func() {
		w.release()
		<-streamed
	})

	select {
	case <-w.flushed:
	case <-time.After(within):
		require.FailNow(t, "the stream never opened")
	}

	return w, streamed
}

func TestAStreamNobodyReadsHoldsNoWriterUpAndEnds(t *testing.T) {
	t.Parallel()

	f := newFixture(t, "https://recall.example.org")
	cookie := f.signIn(t)
	personal := areaPath(t, f.personal(t, cookie))
	f.hub.writeWait = time.Hour
	w, streamed := f.stall(t, cookie)

	// The stream's backlog is filled directly and a real PATCH overflows it, so no
	// bound times the store's writes; a writer that waited would hang the PATCH.
	ctx := module.WithCaller(t.Context(), module.Caller{UserID: f.user})
	for range backlog {
		f.hub.Changed(ctx, module.Change{Module: "core", Kind: "area", ID: 1, Area: 1})
	}

	code, _ := f.json(t, http.MethodPatch, personal, `{"description":"Mine."}`, cookie, csrf)
	require.Equal(t, http.StatusOK, code)

	// Its page would miss that change, so the stream ends once it has sent what
	// it held, and the page reads afresh when it opens it again.
	w.release()

	select {
	case <-streamed:
	case <-time.After(within):
		require.FailNow(t, "the stream that fell behind did not end")
	}

	assert.Equal(t, backlog, strings.Count(w.Body.String(), "data: "))
}

func TestAStreamWhoseClientStopsReadingEndsByItself(t *testing.T) {
	t.Parallel()

	f := newFixture(t, "https://recall.example.org")
	f.hub.writeWait = 50 * time.Millisecond
	_, streamed := f.stall(t, f.signIn(t))

	select {
	case <-streamed:
	case <-time.After(within):
		require.FailNow(t, "the stream waited on its client past its write deadline")
	}
}

// itemEvent is the event of the fake's item ref, added to the area an answer gives.
func itemEvent(t *testing.T, ref any, area map[string]any) string {
	t.Helper()

	s, ok := ref.(string)
	require.True(t, ok, ref)

	_, id, ok := module.Ref(s).Split()
	require.True(t, ok, s)

	return fmt.Sprintf(`{"module":"fake","kind":"item","id":%d,"area":%v}`, id, area["id"])
}

func TestAChangeMadeThroughAModuleRouteOrToolReachesTheOpenStreamsOfItsUserAlone(t *testing.T) {
	t.Parallel()

	f := newFixture(t, "https://recall.example.org")
	f.load(t, map[string]any{"enabled": true})
	srv := httptest.NewServer(f.h)
	t.Cleanup(srv.Close)

	cookie, grace := f.signIn(t), f.grace(t)
	mine, theirs := openStream(t, srv.URL, cookie), openStream(t, srv.URL, grace)
	personal := f.personal(t, cookie)

	mine.expect(t, itemEvent(t, f.add(t, "Raised beds", "personal", cookie, csrf), personal))

	cs := f.client(t, f.token(t, "laptop", "write"))
	mine.expect(t, itemEvent(t, call(t, cs, "add_item", map[string]any{"title": "Seeds", "area": "personal"})["ref"], personal))

	refused(t, f.client(t, f.token(t, "phone", "read")), "add_item", map[string]any{"title": "Lost", "area": "personal"})
	refused(t, cs, "add_item", map[string]any{"title": "Lost", "area": "nowhere"})

	code, got := f.json(t, http.MethodPost, "/api/fake/items", `{"title":"Lost","area":"nowhere"}`, cookie, csrf)
	assert.Equal(t, http.StatusNotFound, code)
	refusal(t, got)

	theirs.expect(t, itemEvent(t, call(t, f.client(t, grace), "add_item", map[string]any{"title": "Orchard", "area": "personal"})["ref"],
		f.personal(t, grace)))

	mine.expect(t, itemEvent(t, f.add(t, "Compost", "personal", cookie, csrf), personal))
}
