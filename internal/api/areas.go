package api

import (
	"net/http"

	"github.com/mhersson/recallsynapsis/internal/core"
	"github.com/mhersson/recallsynapsis/internal/module"
)

func (s *server) areas(w http.ResponseWriter, r *http.Request) error {
	var archived bool

	switch r.URL.Query().Get("archived") {
	case "", "false":
	case "true":
		archived = true
	default:
		return &module.Refusal{Status: http.StatusBadRequest, Text: `archived is "true" or "false"`}
	}

	list, err := s.Core.Areas(r.Context(), caller(r).UserID, archived)
	if err != nil {
		return err
	}

	module.Reply(w, http.StatusOK, map[string][]module.Area{"areas": list})

	return nil
}

func (s *server) createArea(w http.ResponseWriter, r *http.Request) error {
	var in struct {
		Name  string `json:"name"`
		Brief string `json:"brief"`
	}

	if err := module.Decode(w, r, &in); err != nil {
		return err
	}

	a, err := s.Core.CreateProject(r.Context(), caller(r), in.Name, in.Brief)
	if err != nil {
		return err
	}

	s.areaChanged(r, a)
	module.Reply(w, http.StatusCreated, a)

	return nil
}

func (s *server) changeArea(w http.ResponseWriter, r *http.Request) error {
	id, err := pathID(r, "id")
	if err != nil {
		return err
	}

	var in core.Change
	if err := module.Decode(w, r, &in); err != nil {
		return err
	}

	a, err := s.Core.ChangeArea(r.Context(), caller(r), id, in)
	if err != nil {
		return err
	}

	s.areaChanged(r, a)
	module.Reply(w, http.StatusOK, a)

	return nil
}

func (s *server) areaHistory(w http.ResponseWriter, r *http.Request) error {
	id, err := pathID(r, "id")
	if err != nil {
		return err
	}

	out, err := s.Core.History(r.Context(), caller(r).UserID, id)
	if err != nil {
		return err
	}

	module.Reply(w, http.StatusOK, map[string][]core.Entry{"revisions": out})

	return nil
}

func (s *server) areaRevision(w http.ResponseWriter, r *http.Request) error {
	id, rev, err := revisionPath(r)
	if err != nil {
		return err
	}

	v, err := s.Core.Revision(r.Context(), caller(r).UserID, id, rev)
	if err != nil {
		return err
	}

	module.Reply(w, http.StatusOK, v)

	return nil
}

func (s *server) restoreArea(w http.ResponseWriter, r *http.Request) error {
	id, rev, err := revisionPath(r)
	if err != nil {
		return err
	}

	a, err := s.Core.Restore(r.Context(), caller(r), id, rev)
	if err != nil {
		return err
	}

	s.areaChanged(r, a)
	module.Reply(w, http.StatusOK, a)

	return nil
}

// revisionPath reads the area's id and the revision in the path.
func revisionPath(r *http.Request) (int64, int64, error) {
	id, err := pathID(r, "id")
	if err != nil {
		return 0, 0, err
	}

	rev, err := pathID(r, "revision")

	return id, rev, err
}

// areaChanged tells the caller's open pages that the area a changed: it is
// both the item and its area.
func (s *server) areaChanged(r *http.Request, a module.Area) {
	s.Hub.Changed(r.Context(), module.Change{Module: "core", Kind: "area", ID: a.ID, Area: a.ID})
}
