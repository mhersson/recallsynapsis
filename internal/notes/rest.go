package notes

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/mhersson/recallsynapsis/internal/module"
	"github.com/mhersson/recallsynapsis/internal/store"
)

// serveList answers the notes that ?area=, ?kind=, ?inbox=, ?tag=, ?trashed= and ?limit= ask for.
func (n notes) serveList(w http.ResponseWriter, r *http.Request) error {
	q := r.URL.Query()
	f := filter{area: q.Get("area"), kind: q.Get("kind"), tag: q.Get("tag")}

	switch v := q.Get("inbox"); v {
	case "":
	case "true", "false":
		inbox := v == "true"
		f.inbox = &inbox
	default:
		return &module.Refusal{Status: http.StatusBadRequest, Text: `inbox is "true" or "false"`}
	}

	switch q.Get("trashed") {
	case "", "false":
	case "true":
		f.trashed = true
	default:
		return &module.Refusal{Status: http.StatusBadRequest, Text: `trashed is "true" or "false"`}
	}

	// list takes a limit of 0 for its default, which a request names by leaving it out.
	if v := q.Get("limit"); v != "" {
		var err error
		if f.limit, err = strconv.Atoi(v); err != nil || f.limit == 0 {
			return errLimit
		}
	}

	out, err := n.list(r.Context(), f)
	if err != nil {
		return err
	}

	module.Reply(w, http.StatusOK, out)

	return nil
}

func (n notes) serveTags(w http.ResponseWriter, r *http.Request) error {
	out, err := n.tags(r.Context())
	if err != nil {
		return err
	}

	module.Reply(w, http.StatusOK, map[string][]tagCount{"tags": out})

	return nil
}

func (n notes) serveCreate(w http.ResponseWriter, r *http.Request) error {
	var d draft
	if err := module.Decode(w, r, &d); err != nil {
		return err
	}

	out, err := n.create(r.Context(), d)
	if err != nil {
		return err
	}

	module.Reply(w, http.StatusCreated, out)

	return nil
}

func (n notes) serveGet(w http.ResponseWriter, r *http.Request) error {
	id, err := noteID(r)
	if err != nil {
		return err
	}

	out, err := n.get(r.Context(), id)
	if err != nil {
		return err
	}

	module.Reply(w, http.StatusOK, out)

	return nil
}

func (n notes) serveChange(w http.ResponseWriter, r *http.Request) error {
	id, err := noteID(r)
	if err != nil {
		return err
	}

	var ch change
	if err := module.Decode(w, r, &ch); err != nil {
		return err
	}

	out, err := n.change(r.Context(), id, ch)
	if err != nil {
		return err
	}

	module.Reply(w, http.StatusOK, out)

	return nil
}

// serveTrash answers the note once it is in the trash, or out of it when
// trashed is false.
func (n notes) serveTrash(trashed bool) func(http.ResponseWriter, *http.Request) error {
	return func(w http.ResponseWriter, r *http.Request) error {
		id, err := noteID(r)
		if err != nil {
			return err
		}

		out, err := n.trash(r.Context(), id, trashed)
		if err != nil {
			return err
		}

		module.Reply(w, http.StatusOK, out)

		return nil
	}
}

func (n notes) serveDelete(w http.ResponseWriter, r *http.Request) error {
	id, err := noteID(r)
	if err != nil {
		return err
	}

	if err := n.remove(r.Context(), id); err != nil {
		return err
	}

	w.WriteHeader(http.StatusNoContent)

	return nil
}

func (n notes) serveEmpty(w http.ResponseWriter, r *http.Request) error {
	if err := n.empty(r.Context()); err != nil {
		return err
	}

	w.WriteHeader(http.StatusNoContent)

	return nil
}

// noteID reads the id in the path; one that is not a number names no note.
func noteID(r *http.Request) (int64, error) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		return 0, fmt.Errorf("note %q: %w", r.PathValue("id"), store.ErrNotFound)
	}

	return id, nil
}
