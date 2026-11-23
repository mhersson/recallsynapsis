package push_test

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mhersson/recallsynapsis/internal/module"
	"github.com/mhersson/recallsynapsis/internal/push"
)

const contact = "https://recall.example.org"

func TestSendHeaders(t *testing.T) {
	for name, enc := range map[string]*base64.Encoding{"unpadded keys": base64.RawURLEncoding, "padded keys": base64.URLEncoding} {
		t.Run(name, func(t *testing.T) {
			b := newBrowser(t)
			srv := newService(t, http.StatusCreated, "")
			sender := newSender(t, srv)
			sub := b.subscription(srv.URL+"/push/JzLQ3raZ", enc)
			m := module.Message{Title: "Ring the carpenter", Body: "14:00 · Hytta", URL: "/tasks/7", Tag: "tasks-7"}

			require.NoError(t, sender.Send(t.Context(), sub, m, 10*time.Hour))

			p := srv.received(t)
			assert.Equal(t, http.MethodPost, p.method)
			assert.Equal(t, "/push/JzLQ3raZ", p.path)
			assert.Equal(t, "aes128gcm", p.header.Get("Content-Encoding"))
			assert.Equal(t, "36000", p.header.Get("TTL"))
			assert.Equal(t, "high", p.header.Get("Urgency"))
			assert.Equal(t, "tasks-7", p.header.Get("Topic"))

			authorization, err := sender.Signer.Authorization(sub.Endpoint, contact)
			require.NoError(t, err)
			assert.Equal(t, authorization, p.header.Get("Authorization"))

			token, _, ok := strings.Cut(strings.TrimPrefix(p.header.Get("Authorization"), "vapid t="), ", k=")
			require.True(t, ok)

			var claims struct{ Aud, Sub string }
			require.NoError(t, json.Unmarshal(unb64(t, strings.Split(token, ".")[1]), &claims))
			assert.Equal(t, srv.URL, claims.Aud)
			assert.Equal(t, contact, claims.Sub)

			assert.JSONEq(t,
				`{"title":"Ring the carpenter","body":"14:00 · Hytta","url":"/tasks/7","tag":"tasks-7"}`,
				string(b.open(t, p.body)))
		})
	}
}

func TestSendWithNoTagHasNoTopic(t *testing.T) {
	srv := newService(t, http.StatusCreated, "")
	m := module.Message{Title: "4 today", Body: "Rake", URL: "/tasks"}

	require.NoError(t, newSender(t, srv).Send(t.Context(), newBrowser(t).subscription(srv.URL, base64.RawURLEncoding), m, time.Hour))

	assert.NotContains(t, srv.received(t).header, "Topic")
}

func TestSendGoneSubscription(t *testing.T) {
	m := module.Message{Title: "Ring the carpenter", Body: "14:00", URL: "/tasks/3", Tag: "tasks-3"}
	send := func(t *testing.T, status int) (*service, error) {
		t.Helper()

		srv := newService(t, status, "quota exceeded\nsecond line")

		return srv, newSender(t, srv).Send(t.Context(), newBrowser(t).subscription(srv.URL+"/push/JzLQ3raZ", base64.RawURLEncoding), m, time.Hour)
	}

	for _, status := range []int{http.StatusOK, http.StatusCreated, http.StatusAccepted} {
		t.Run(strconv.Itoa(status), func(t *testing.T) {
			_, err := send(t, status)
			require.NoError(t, err)
		})
	}

	for _, status := range []int{http.StatusNotFound, http.StatusGone} {
		t.Run(strconv.Itoa(status), func(t *testing.T) {
			_, err := send(t, status)
			require.ErrorIs(t, err, push.ErrGone)
		})
	}

	for _, status := range []int{http.StatusRequestEntityTooLarge, http.StatusTooManyRequests, http.StatusInternalServerError} {
		t.Run(strconv.Itoa(status), func(t *testing.T) {
			srv, err := send(t, status)

			f, ok := errors.AsType[*push.Failure](err)
			require.True(t, ok, err)
			assert.Equal(t, srv.Listener.Addr().String()+" answered "+strconv.Itoa(status), f.Line)
			assert.Contains(t, err.Error(), "quota exceeded")
			assert.NotContains(t, err.Error(), "second line")
		})
	}

	notReached := func(t *testing.T, srv *httptest.Server, client *http.Client) {
		t.Helper()

		sender := &push.Sender{Client: client, Signer: newSigner(t), Contact: contact}

		err := sender.Send(t.Context(), newBrowser(t).subscription(srv.URL+"/push/JzLQ3raZ", base64.RawURLEncoding), m, time.Hour)

		f, ok := errors.AsType[*push.Failure](err)
		require.True(t, ok, err)
		assert.Equal(t, srv.Listener.Addr().String()+" was not reached", f.Line)
		assert.NotContains(t, err.Error(), "JzLQ3raZ", "the endpoint is a capability")
	}

	t.Run("unreachable", func(t *testing.T) {
		srv := newService(t, http.StatusCreated, "")
		srv.Close()

		notReached(t, srv.Server, srv.Client())
	})

	t.Run("no answer in time", func(t *testing.T) {
		// The body read, the server learns when the client leaves.
		srv := httptest.NewTLSServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
			_, _ = io.Copy(io.Discard, r.Body)
			<-r.Context().Done()
		}))
		t.Cleanup(srv.Close)

		client := srv.Client()
		client.Timeout = 50 * time.Millisecond

		notReached(t, srv, client)
	})
}

func TestMessageCut(t *testing.T) {
	// Two title lengths, so that one cut falls inside a two-octet rune.
	for _, title := range []string{strings.Repeat("T", 100), strings.Repeat("T", 101)} {
		b := newBrowser(t)
		srv := newService(t, http.StatusCreated, "")
		m := module.Message{Title: title, Body: strings.Repeat("é", 2000), URL: "/tasks/12345", Tag: "tasks-12345"}

		require.NoError(t, newSender(t, srv).Send(t.Context(), b.subscription(srv.URL, base64.RawURLEncoding), m, time.Hour))

		plain := b.open(t, srv.received(t).body)
		assert.LessOrEqual(t, len(plain), 2048)
		assert.Greater(t, len(plain), 2040)

		var cut module.Message
		require.NoError(t, json.Unmarshal(plain, &cut))
		assert.Equal(t, m.Title, cut.Title)
		assert.Equal(t, m.URL, cut.URL)
		assert.Equal(t, m.Tag, cut.Tag)
		assert.True(t, strings.HasPrefix(m.Body, cut.Body), "cut at a rune boundary")
	}
}

func TestClientFollowsNoRedirect(t *testing.T) {
	var reached atomic.Int32

	elsewhere := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		reached.Add(1)
		w.WriteHeader(http.StatusCreated)
	}))
	t.Cleanup(elsewhere.Close)

	srv := httptest.NewServer(http.RedirectHandler(elsewhere.URL+"/push/JzLQ3raZ", http.StatusTemporaryRedirect))
	t.Cleanup(srv.Close)

	sender := &push.Sender{Client: push.NewClient(), Signer: newSigner(t), Contact: contact}
	m := module.Message{Title: "Ring the carpenter", Body: "14:00", URL: "/tasks/3", Tag: "tasks-3"}

	err := sender.Send(t.Context(), newBrowser(t).subscription(srv.URL+"/push/JzLQ3raZ", base64.RawURLEncoding), m, time.Hour)

	f, ok := errors.AsType[*push.Failure](err)
	require.True(t, ok, err)
	assert.True(t, strings.HasSuffix(f.Line, " answered "+strconv.Itoa(http.StatusTemporaryRedirect)), f.Line)
	assert.Zero(t, reached.Load(), "a push reaches only the endpoint's host")
}

// sent is a push as the push service received it.
type sent struct {
	method, path string
	header       http.Header
	body         []byte
}

// service is a push service of the test, on TLS as every push service is. It
// answers each push with its status and reply, and passes on what it received.
type service struct {
	*httptest.Server
	status atomic.Int64
	got    chan sent
}

func newService(t *testing.T, status int, reply string) *service {
	t.Helper()

	s := &service{got: make(chan sent, 8)}
	s.status.Store(int64(status))
	s.Server = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		s.got <- sent{method: r.Method, path: r.URL.Path, header: r.Header.Clone(), body: body}

		w.WriteHeader(int(s.status.Load()))
		_, _ = io.WriteString(w, reply)
	}))
	t.Cleanup(s.Close)

	return s
}

// received is the push the service got; a push is sent only once the service answered.
func (s *service) received(t *testing.T) sent {
	t.Helper()

	select {
	case p := <-s.got:
		return p
	default:
		require.FailNow(t, "the push service received nothing")

		return sent{}
	}
}

// quiet wants the service to have received nothing more.
func (s *service) quiet(t *testing.T) {
	t.Helper()

	select {
	case p := <-s.got:
		assert.Fail(t, "the push service received a push", p.path)
	default:
	}
}

func newSigner(t *testing.T) *push.Signer {
	t.Helper()

	key, err := push.NewKey()
	require.NoError(t, err)

	now := time.Date(2026, 11, 20, 18, 0, 0, 0, time.UTC)

	return push.NewSigner(key, func() time.Time { return now })
}

// newSender sends with a client that trusts the test's push services.
func newSender(t *testing.T, srv *service) *push.Sender {
	t.Helper()

	return &push.Sender{Client: srv.Client(), Signer: newSigner(t), Contact: contact}
}

func (b browser) subscription(endpoint string, enc *base64.Encoding) push.Subscription {
	return push.Subscription{
		Endpoint: endpoint,
		P256DH:   enc.EncodeToString(b.key.PublicKey().Bytes()),
		Auth:     enc.EncodeToString(b.auth),
	}
}
