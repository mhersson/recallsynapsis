// Package api serves the REST API under /api/ from one route table, checks
// the access each route declares in one place, and answers every refusal, and
// every path no route matches once the mux has redirected it to its clean
// form, in one JSON shape.
package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/mhersson/recallsynapsis/internal/auth"
	"github.com/mhersson/recallsynapsis/internal/core"
	"github.com/mhersson/recallsynapsis/internal/store"
)

const maxBody = 1 << 20

// Deps is what the handler works on.
type Deps struct {
	Auth      *auth.Service
	Core      *core.Service
	Version   string
	PublicURL string // the configured public_url; the cookie is Secure when it is https
	Timezone  string
}

// access is what a route asks of the request's authority.
type access int

const (
	open  access = iota // nothing
	read                // the session cookie or any token
	write               // the session cookie or a write token
	owner               // the session cookie alone, so no token makes or revokes a token
)

// route is one REST route.
type route struct {
	pattern string
	access  access
	serve   func(*server, http.ResponseWriter, *http.Request) error
}

var routes = []route{
	{"GET /api/health", open, (*server).health},
	{"POST /api/auth/login", open, (*server).login},
	{"POST /api/auth/logout", read, (*server).logout},
	{"GET /api/session", read, (*server).session},
	{"GET /api/areas", read, (*server).areas},
	{"POST /api/areas", write, (*server).createArea},
	{"PATCH /api/areas/{id}", write, (*server).changeArea},
	{"GET /api/tokens", owner, (*server).tokens},
	{"POST /api/tokens", owner, (*server).createToken},
	{"DELETE /api/tokens/{id}", owner, (*server).deleteToken},
}

type server struct {
	Deps
	secure bool
}

// New returns the handler of every route.
func New(d Deps) http.Handler {
	s := &server{Deps: d, secure: strings.HasPrefix(d.PublicURL, "https://")}
	mux := http.NewServeMux()

	for _, rt := range routes {
		mux.HandleFunc(rt.pattern, func(w http.ResponseWriter, r *http.Request) {
			r, err := s.authorize(w, r, rt.access)
			if err == nil {
				err = rt.serve(s, w, r)
			}

			if err != nil {
				fail(w, r, err)
			}
		})
	}

	// Else the mux answers a request that matches no route in plain text.
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fail(w, r, &apiError{http.StatusNotFound, "no route matches the method and the path"})
	})

	return mux
}

// apiError is a refusal that is the caller's: a status and one line saying why.
type apiError struct {
	status int
	text   string
}

func (e *apiError) Error() string { return e.text }

// fail answers err as JSON, with the status and the line classify gives it.
func fail(w http.ResponseWriter, r *http.Request, err error) {
	status, text := classify(r.Context(), r.Pattern, err)
	reply(w, status, map[string]string{"error": text})
}

// classify gives err's status and the one line the caller sees. A broken rule,
// a conflict or an absence is answered with its whole text; any other error
// that is not the caller's is logged with the route and given without detail.
func classify(ctx context.Context, route string, err error) (int, string) {
	switch e, ok := errors.AsType[*apiError](err); {
	case ok:
		return e.status, e.text
	case errors.Is(err, store.ErrInvalid):
		return http.StatusBadRequest, err.Error()
	case errors.Is(err, store.ErrConflict):
		return http.StatusConflict, err.Error()
	case errors.Is(err, store.ErrNotFound):
		return http.StatusNotFound, err.Error()
	}

	slog.ErrorContext(ctx, "request failed", "route", route, "err", err)

	return http.StatusInternalServerError, "internal error"
}

func reply(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// decode reads the body, at most maxBody of it, into v, refusing a field v does not have.
func decode(w http.ResponseWriter, r *http.Request, v any) error {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBody))
	dec.DisallowUnknownFields()

	err := dec.Decode(v)
	if err == nil {
		// Decode stops at the end of the first value, so what follows it is read here.
		if err = dec.Decode(new(json.RawMessage)); errors.Is(err, io.EOF) {
			return nil
		}

		if err == nil {
			err = errors.New("more than one value")
		}
	}

	if _, ok := errors.AsType[*http.MaxBytesError](err); ok {
		return &apiError{http.StatusRequestEntityTooLarge, "the body is larger than 1 MiB"}
	}

	return &apiError{http.StatusBadRequest, "the body is not the JSON this route takes: " + err.Error()}
}

// pathID reads the id in the path; one that is not a number names no row.
func pathID(r *http.Request) (int64, error) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		return 0, &apiError{http.StatusNotFound, "the id in the path is not a number"}
	}

	return id, nil
}

func (s *server) health(w http.ResponseWriter, _ *http.Request) error {
	reply(w, http.StatusOK, map[string]string{"status": "ok", "version": s.Version})

	return nil
}
