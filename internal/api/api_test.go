package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mhersson/recallsynapsis/internal/store"
)

// do sends a request to h and returns the status and the decoded JSON body.
func do(t *testing.T, h http.Handler, method, target, body string) (int, map[string]any) {
	t.Helper()

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), method, target, strings.NewReader(body)))

	assert.Equal(t, "application/json", rec.Header().Get("Content-Type"))

	var got map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got), rec.Body.String())

	return rec.Code, got
}

// refusal asserts that got is the one error shape: a single, non-empty line.
func refusal(t *testing.T, got map[string]any) string {
	t.Helper()

	require.Len(t, got, 1)

	line, ok := got["error"].(string)
	require.True(t, ok)
	assert.NotEmpty(t, line)
	assert.NotContains(t, line, "\n")

	return line
}

func TestHealthNeedsNoAuthority(t *testing.T) {
	t.Parallel()

	code, got := do(t, New(Deps{Version: "1.2.3"}), http.MethodGet, "/api/health", "")

	assert.Equal(t, http.StatusOK, code)
	assert.Equal(t, map[string]any{"status": "ok", "version": "1.2.3"}, got)
}

func TestAnyOtherRequestIsAJSONNotFound(t *testing.T) {
	t.Parallel()

	h := New(Deps{Version: "1.2.3"})

	for _, tt := range []struct{ method, target string }{
		{http.MethodGet, "/"},
		{http.MethodGet, "/api/nothing"},
		{http.MethodGet, "/api/health/"},
		{http.MethodPost, "/api/health"},
	} {
		t.Run(tt.method+" "+tt.target, func(t *testing.T) {
			t.Parallel()

			code, got := do(t, h, tt.method, tt.target, "")
			assert.Equal(t, http.StatusNotFound, code)
			refusal(t, got)
		})
	}
}

func TestClassify(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		err    error
		status int
	}{
		{"the caller's refusal", &apiError{http.StatusTeapot, "no"}, http.StatusTeapot},
		{"a wrapped conflict", fmt.Errorf("area name %q: %w", "home", store.ErrConflict), http.StatusConflict},
		{"a wrapped absence", fmt.Errorf("area 7: %w", store.ErrNotFound), http.StatusNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fail(w, r, tt.err) })
			code, got := do(t, h, http.MethodGet, "/", "")

			assert.Equal(t, tt.status, code)
			refusal(t, got)
		})
	}
}

// Not parallel: it reads what reaches the default logger.
func TestAnInternalErrorIsLoggedAndNotAnswered(t *testing.T) {
	def, out, flags := slog.Default(), log.Writer(), log.Flags()

	t.Cleanup(func() {
		slog.SetDefault(def)
		log.SetOutput(out)
		log.SetFlags(flags)
	})

	var logged bytes.Buffer

	slog.SetDefault(slog.New(slog.NewTextHandler(&logged, nil)))

	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fail(w, r, errors.New("disk on fire at /secret/path"))
	})
	code, got := do(t, h, http.MethodGet, "/", "")

	assert.Equal(t, http.StatusInternalServerError, code)
	assert.NotContains(t, refusal(t, got), "/secret/path")
	assert.Contains(t, logged.String(), "/secret/path")
}

func TestDecode(t *testing.T) {
	t.Parallel()

	var got struct {
		Name string `json:"name"`
	}

	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := decode(w, r, &got); err != nil {
			fail(w, r, err)

			return
		}

		reply(w, http.StatusOK, got)
	})

	code, body := do(t, h, http.MethodPost, "/", `{"name":"ada"}`)
	assert.Equal(t, http.StatusOK, code)
	assert.Equal(t, map[string]any{"name": "ada"}, body)

	tests := []struct {
		name, body string
		status     int
	}{
		{"an unknown field", `{"name":"ada","admin":true}`, http.StatusBadRequest},
		{"two values", `{"name":"ada"}{"name":"bob"}`, http.StatusBadRequest},
		{"no value", ``, http.StatusBadRequest},
		{"a wrong type", `{"name":7}`, http.StatusBadRequest},
		{"more than the cap", `{"name":"` + strings.Repeat("a", maxBody) + `"}`, http.StatusRequestEntityTooLarge},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			code, body := do(t, h, http.MethodPost, "/", tt.body)
			assert.Equal(t, tt.status, code)
			refusal(t, body)
		})
	}
}

func TestPathID(t *testing.T) {
	t.Parallel()

	mux := http.NewServeMux()
	mux.HandleFunc("GET /things/{id}", func(w http.ResponseWriter, r *http.Request) {
		id, err := pathID(r)
		if err != nil {
			fail(w, r, err)

			return
		}

		reply(w, http.StatusOK, map[string]int64{"id": id})
	})

	code, got := do(t, mux, http.MethodGet, "/things/42", "")
	assert.Equal(t, http.StatusOK, code)
	assert.Equal(t, map[string]any{"id": float64(42)}, got)

	for _, id := range []string{"abc", "1.5", "99999999999999999999"} {
		code, got := do(t, mux, http.MethodGet, "/things/"+id, "")
		assert.Equal(t, http.StatusNotFound, code, id)
		refusal(t, got)
	}
}
