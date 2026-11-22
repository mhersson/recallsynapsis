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
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mhersson/recallsynapsis/internal/core"
	"github.com/mhersson/recallsynapsis/internal/module"
	"github.com/mhersson/recallsynapsis/internal/store"
)

// do sends a request with no body to h and returns the status and the decoded JSON body.
func do(t *testing.T, h http.Handler, method, target string) (int, map[string]any) {
	t.Helper()

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), method, target, nil))

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

	code, got := do(t, New(Deps{Modules: &core.Modules{}, Version: "1.2.3"}), http.MethodGet, "/api/health")

	assert.Equal(t, http.StatusOK, code)
	assert.Equal(t, map[string]any{"status": "ok", "version": "1.2.3"}, got)
}

func TestAnyOtherRequestIsAJSONNotFound(t *testing.T) {
	t.Parallel()

	h := New(Deps{Modules: &core.Modules{}, Version: "1.2.3"})

	for _, tt := range []struct{ method, target string }{
		{http.MethodGet, "/"},
		{http.MethodGet, "/api/nothing"},
		{http.MethodGet, "/api/health/"},
		{http.MethodPost, "/api/health"},
	} {
		t.Run(tt.method+" "+tt.target, func(t *testing.T) {
			t.Parallel()

			code, got := do(t, h, tt.method, tt.target)
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
		{"the caller's refusal", &module.Refusal{Status: http.StatusTeapot, Text: "no"}, http.StatusTeapot},
		{"a wrapped broken rule", fmt.Errorf("the personal area keeps its name: %w", store.ErrInvalid), http.StatusBadRequest},
		{"a wrapped conflict", fmt.Errorf("area name %q: %w", "home", store.ErrConflict), http.StatusConflict},
		{"a wrapped absence", fmt.Errorf("area 7: %w", store.ErrNotFound), http.StatusNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fail(w, r, tt.err) })
			code, got := do(t, h, http.MethodGet, "/")

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
	code, got := do(t, h, http.MethodGet, "/")

	assert.Equal(t, http.StatusInternalServerError, code)
	assert.NotContains(t, refusal(t, got), "/secret/path")
	assert.Contains(t, logged.String(), "/secret/path")
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

		module.Reply(w, http.StatusOK, map[string]int64{"id": id})
	})

	code, got := do(t, mux, http.MethodGet, "/things/42")
	assert.Equal(t, http.StatusOK, code)
	assert.Equal(t, map[string]any{"id": float64(42)}, got)

	for _, id := range []string{"abc", "1.5", "99999999999999999999"} {
		code, got := do(t, mux, http.MethodGet, "/things/"+id)
		assert.Equal(t, http.StatusNotFound, code, id)
		refusal(t, got)
	}
}
