package tasks

import (
	"fmt"
	"net/http"
	"net/url"
	"strconv"

	"github.com/mhersson/recallsynapsis/internal/module"
	"github.com/mhersson/recallsynapsis/internal/store"
)

// serveList answers the tasks that ?area=, ?list=, ?from=, ?to=, ?unsorted=, ?done= and ?limit= ask for.
func (t tasks) serveList(w http.ResponseWriter, r *http.Request) error {
	q := r.URL.Query()
	f := filter{area: q.Get("area"), list: q.Get("list"), from: q.Get("from"), to: q.Get("to")}

	var err error

	if f.unsorted, err = flag(q, "unsorted"); err != nil {
		return err
	}

	if f.done, err = flag(q, "done"); err != nil {
		return err
	}

	// list takes a limit of 0 for its default, which a request names by leaving it out.
	if v := q.Get("limit"); v != "" {
		if f.limit, err = strconv.Atoi(v); err != nil || f.limit == 0 {
			return errLimit
		}
	}

	out, err := t.list(r.Context(), f)
	if err != nil {
		return err
	}

	module.Reply(w, http.StatusOK, out)

	return nil
}

func (t tasks) serveCreate(w http.ResponseWriter, r *http.Request) error {
	var b batch
	if err := module.Decode(w, r, &b); err != nil {
		return err
	}

	out, err := t.create(r.Context(), b)
	if err != nil {
		return err
	}

	module.Reply(w, http.StatusCreated, out)

	return nil
}

func (t tasks) serveGet(w http.ResponseWriter, r *http.Request) error {
	id, err := taskID(r)
	if err != nil {
		return err
	}

	out, err := t.get(r.Context(), id)
	if err != nil {
		return err
	}

	module.Reply(w, http.StatusOK, out)

	return nil
}

func (t tasks) serveChange(w http.ResponseWriter, r *http.Request) error {
	id, err := taskID(r)
	if err != nil {
		return err
	}

	var ch change
	if err := module.Decode(w, r, &ch); err != nil {
		return err
	}

	out, err := t.change(r.Context(), id, ch)
	if err != nil {
		return err
	}

	module.Reply(w, http.StatusOK, out)

	return nil
}

// flag reads the query's key: true for "true", false for "false" or none.
func flag(q url.Values, key string) (bool, error) {
	switch q.Get(key) {
	case "", "false":
		return false, nil
	case "true":
		return true, nil
	}

	return false, &module.Refusal{Status: http.StatusBadRequest, Text: key + ` is "true" or "false"`}
}

// taskID reads the id in the path; one that is not a number names no task.
func taskID(r *http.Request) (int64, error) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		return 0, fmt.Errorf("task %q: %w", r.PathValue("id"), store.ErrNotFound)
	}

	return id, nil
}
