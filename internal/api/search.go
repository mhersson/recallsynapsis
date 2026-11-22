package api

import (
	"net/http"

	"github.com/mhersson/recallsynapsis/internal/core"
	"github.com/mhersson/recallsynapsis/internal/module"
)

// search answers the hits of q, within the area whose slug or name area is
// when it is given.
func (s *server) search(w http.ResponseWriter, r *http.Request) error {
	q := module.Query{Text: r.URL.Query().Get("q")}
	if q.Text == "" {
		return &module.Refusal{Status: http.StatusBadRequest, Text: "q is required: the text to search for"}
	}

	if area := r.URL.Query().Get("area"); area != "" {
		a, err := s.Core.Find(r.Context(), caller(r).UserID, area)
		if err != nil {
			return err
		}

		q.AreaID = a.ID
	}

	groups, err := s.Modules.Search(r.Context(), q)
	if err != nil {
		return err
	}

	module.Reply(w, http.StatusOK, map[string][]core.Group{"groups": groups})

	return nil
}
