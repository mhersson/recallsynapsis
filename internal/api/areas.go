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

	module.Reply(w, http.StatusOK, map[string][]core.Area{"areas": list})

	return nil
}

func (s *server) createArea(w http.ResponseWriter, r *http.Request) error {
	var in struct {
		Name        string `json:"name"`
		Description string `json:"description"`
	}

	if err := module.Decode(w, r, &in); err != nil {
		return err
	}

	a, err := s.Core.CreateProject(r.Context(), caller(r).UserID, in.Name, in.Description)
	if err != nil {
		return err
	}

	module.Reply(w, http.StatusCreated, a)

	return nil
}

func (s *server) changeArea(w http.ResponseWriter, r *http.Request) error {
	id, err := pathID(r)
	if err != nil {
		return err
	}

	var in core.Change
	if err := module.Decode(w, r, &in); err != nil {
		return err
	}

	a, err := s.Core.ChangeArea(r.Context(), caller(r).UserID, id, in)
	if err != nil {
		return err
	}

	module.Reply(w, http.StatusOK, a)

	return nil
}
