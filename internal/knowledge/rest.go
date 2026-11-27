package knowledge

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/mhersson/recallsynapsis/internal/module"
	"github.com/mhersson/recallsynapsis/internal/store"
)

// serveList answers every decision of the project ?area= names, and which day it is.
func (k knowledge) serveList(w http.ResponseWriter, r *http.Request) error {
	out, err := k.list(r.Context(), r.URL.Query().Get("area"))
	if err != nil {
		return err
	}

	module.Reply(w, http.StatusOK, out)

	return nil
}

// serveChange saves the package the body holds and answers what it mended, then what it added.
func (k knowledge) serveChange(w http.ResponseWriter, r *http.Request) error {
	var ch change
	if err := module.Decode(w, r, &ch); err != nil {
		return err
	}

	out, err := k.apply(r.Context(), ch)
	if err != nil {
		return err
	}

	module.Reply(w, http.StatusOK, map[string][]row{"decisions": out})

	return nil
}

func (k knowledge) serveGet(w http.ResponseWriter, r *http.Request) error {
	id, err := decisionID(r)
	if err != nil {
		return err
	}

	out, err := k.get(r.Context(), id)
	if err != nil {
		return err
	}

	module.Reply(w, http.StatusOK, out)

	return nil
}

func (k knowledge) serveDelete(w http.ResponseWriter, r *http.Request) error {
	id, err := decisionID(r)
	if err != nil {
		return err
	}

	if err := k.remove(r.Context(), id); err != nil {
		return err
	}

	w.WriteHeader(http.StatusNoContent)

	return nil
}

func (k knowledge) serveHistory(w http.ResponseWriter, r *http.Request) error {
	id, err := decisionID(r)
	if err != nil {
		return err
	}

	out, err := k.history(r.Context(), id)
	if err != nil {
		return err
	}

	module.Reply(w, http.StatusOK, map[string][]entry{"revisions": out})

	return nil
}

func (k knowledge) serveRevision(w http.ResponseWriter, r *http.Request) error {
	id, err := decisionID(r)
	if err != nil {
		return err
	}

	rev, err := strconv.ParseInt(r.PathValue("revision"), 10, 64)
	if err != nil {
		return fmt.Errorf("revision %q: %w", r.PathValue("revision"), store.ErrNotFound)
	}

	out, err := k.revision(r.Context(), id, rev)
	if err != nil {
		return err
	}

	module.Reply(w, http.StatusOK, out)

	return nil
}

// decisionID reads the id in the path; one that is not a number names no decision.
func decisionID(r *http.Request) (int64, error) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		return 0, fmt.Errorf("decision %q: %w", r.PathValue("id"), store.ErrNotFound)
	}

	return id, nil
}
