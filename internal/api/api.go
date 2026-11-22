// Package api serves the REST API under /api/ from one route table, with the
// routes of the modules switched on beside the core's, checks the access each
// route declares in one place, and answers every refusal, and every path no
// route matches once the mux has redirected it to its clean form, in one JSON
// shape.
package api

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/mhersson/recallsynapsis/internal/auth"
	"github.com/mhersson/recallsynapsis/internal/core"
	"github.com/mhersson/recallsynapsis/internal/module"
	"github.com/mhersson/recallsynapsis/internal/store"
)

// Deps is what the handler works on.
type Deps struct {
	Auth      *auth.Service
	Core      *core.Service
	Modules   *core.Modules
	Version   string
	PublicURL string // the configured public_url; the cookie is Secure when it is https
	Timezone  string
	BackupDir string // the configured backup.dir, whose newest snapshot is the last backup
}

// route is one REST route.
type route struct {
	pattern string
	access  module.Access
	serve   func(*server, http.ResponseWriter, *http.Request) error
}

var routes = []route{
	{"GET /api/health", module.Open, (*server).health},
	{"POST /api/auth/login", module.Open, (*server).login},
	{"POST /api/auth/logout", module.Read, (*server).logout},
	{"GET /api/session", module.Read, (*server).session},
	{"GET /api/areas", module.Read, (*server).areas},
	{"POST /api/areas", module.Write, (*server).createArea},
	{"PATCH /api/areas/{id}", module.Write, (*server).changeArea},
	{"GET /api/search", module.Read, (*server).search},
	{"GET /api/tokens", module.Owner, (*server).tokens},
	{"POST /api/tokens", module.Owner, (*server).createToken},
	{"DELETE /api/tokens/{id}", module.Owner, (*server).deleteToken},
}

type server struct {
	Deps
	secure bool
}

// New returns the handler of every route.
func New(d Deps) http.Handler {
	s := &server{Deps: d, secure: strings.HasPrefix(d.PublicURL, "https://")}
	mux := http.NewServeMux()

	handle := func(pattern string, level module.Access, serve func(http.ResponseWriter, *http.Request) error) {
		mux.HandleFunc(pattern, func(w http.ResponseWriter, r *http.Request) {
			r, err := s.authorize(w, r, level)
			if err == nil {
				err = serve(w, r)
			}

			if err != nil {
				fail(w, r, err)
			}
		})
	}

	for _, rt := range routes {
		handle(rt.pattern, rt.access, func(w http.ResponseWriter, r *http.Request) error { return rt.serve(s, w, r) })
	}

	for _, m := range d.Modules.On() {
		for _, rt := range m.Routes {
			handle(rt.Pattern, rt.Access, rt.Serve)
		}
	}

	// Else the mux answers a request that matches no route in plain text.
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fail(w, r, &module.Refusal{Status: http.StatusNotFound, Text: "no route matches the method and the path"})
	})

	return mux
}

// fail answers err as JSON, with the status and the line classify gives it.
func fail(w http.ResponseWriter, r *http.Request, err error) {
	status, text := classify(r.Context(), r.Pattern, err)
	module.Reply(w, status, map[string]string{"error": text})
}

// classify gives err's status and the one line the caller sees. A broken rule,
// a conflict or an absence is answered with its whole text; any other error
// that is not the caller's is logged with the route and given without detail.
func classify(ctx context.Context, route string, err error) (int, string) {
	switch e, ok := errors.AsType[*module.Refusal](err); {
	case ok:
		return e.Status, e.Text
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

// pathID reads the id in the path; one that is not a number names no row.
func pathID(r *http.Request) (int64, error) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		return 0, &module.Refusal{Status: http.StatusNotFound, Text: "the id in the path is not a number"}
	}

	return id, nil
}

func (s *server) health(w http.ResponseWriter, _ *http.Request) error {
	module.Reply(w, http.StatusOK, map[string]string{"status": "ok", "version": s.Version})

	return nil
}
