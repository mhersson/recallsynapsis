package api

import (
	"context"
	"net/http"

	"github.com/mhersson/recallsynapsis/internal/core"
	"github.com/mhersson/recallsynapsis/internal/module"
)

// search answers the hits of q, within the area whose slug or name area is
// when it is given.
func (s *server) search(w http.ResponseWriter, r *http.Request) error {
	q := r.URL.Query()
	if q.Get("q") == "" {
		return &module.Refusal{Status: http.StatusBadRequest, Text: "q is required: the text to search for"}
	}

	groups, err := s.groups(r.Context(), q.Get("q"), q.Get("area"))
	if err != nil {
		return err
	}

	module.Reply(w, http.StatusOK, map[string][]core.Group{"groups": groups})

	return nil
}

// groups gives the caller's hits of text, within the area whose slug or name
// area is when it is given.
func (s *server) groups(ctx context.Context, text, area string) ([]core.Group, error) {
	q := module.Query{Text: text}

	if area != "" {
		c, _ := module.CallerFrom(ctx)

		a, err := s.Core.Find(ctx, c.UserID, area)
		if err != nil {
			return nil, err
		}

		q.AreaID = a.ID
	}

	return s.Modules.Search(ctx, q)
}
