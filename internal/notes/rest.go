package notes

import (
	"errors"
	"fmt"
	"mime"
	"net/http"
	"os"
	"slices"
	"strconv"
	"time"

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

// serveLimits answers the cap on a file, in bytes.
func (n notes) serveLimits(w http.ResponseWriter, _ *http.Request) error {
	module.Reply(w, http.StatusOK, map[string]int64{"file_max_bytes": n.settings.FileMaxMiB << 20})

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

func (n notes) serveHistory(w http.ResponseWriter, r *http.Request) error {
	id, err := noteID(r)
	if err != nil {
		return err
	}

	out, err := n.history(r.Context(), id)
	if err != nil {
		return err
	}

	module.Reply(w, http.StatusOK, map[string][]entry{"revisions": out})

	return nil
}

func (n notes) serveRevision(w http.ResponseWriter, r *http.Request) error {
	id, rev, err := ids(r, "revision")
	if err != nil {
		return err
	}

	out, err := n.revision(r.Context(), id, rev)
	if err != nil {
		return err
	}

	module.Reply(w, http.StatusOK, out)

	return nil
}

func (n notes) serveRestoreRevision(w http.ResponseWriter, r *http.Request) error {
	id, rev, err := ids(r, "revision")
	if err != nil {
		return err
	}

	out, err := n.restoreRevision(r.Context(), id, rev)
	if err != nil {
		return err
	}

	module.Reply(w, http.StatusOK, out)

	return nil
}

// serveAttach keeps the body as a file of the note, named in ?name= and of
// the type its Content-Type declares, and answers the file.
func (n notes) serveAttach(w http.ResponseWriter, r *http.Request) error {
	id, err := noteID(r)
	if err != nil {
		return err
	}

	out, err := n.attach(r.Context(), id, r.URL.Query().Get("name"), r.Header.Get("Content-Type"),
		http.MaxBytesReader(w, r.Body, n.settings.FileMaxMiB<<20))
	if _, ok := errors.AsType[*http.MaxBytesError](err); ok {
		return &module.Refusal{Status: http.StatusRequestEntityTooLarge, Text: fmt.Sprintf("a file is at most %d MiB", n.settings.FileMaxMiB)}
	}

	// A connection that drops mid-body ends the request's context, as when a
	// phone cancels an upload: the client's doing, so no failure to log.
	if err != nil && r.Context().Err() != nil {
		return &module.Refusal{Status: http.StatusBadRequest, Text: "the upload ended before the file did"}
	}

	if err != nil {
		return err
	}

	module.Reply(w, http.StatusCreated, out)

	return nil
}

// serveFile answers a file's contents so that none runs as a page, a script
// or a style: never sniffed, sandboxed, inline as its own type only when it is
// one of the inline images, and else a download of application/octet-stream.
// Its contents never change, so the browser keeps it.
func (n notes) serveFile(w http.ResponseWriter, r *http.Request) error {
	id, fileID, err := ids(r, "file")
	if err != nil {
		return err
	}

	f, path, err := n.attachment(r.Context(), id, fileID)
	if err != nil {
		return err
	}

	contents, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("notes: file: %w", err)
	}
	defer contents.Close()

	disposition, kind := "attachment", "application/octet-stream"
	if slices.Contains(inline, f.Type) {
		disposition, kind = "inline", f.Type
	}

	h := w.Header()
	h.Set("Content-Type", kind)
	h.Set("Content-Disposition", mime.FormatMediaType(disposition, map[string]string{"filename": f.Name}))
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("Content-Security-Policy", "sandbox")
	// The cookie reaches here from any page of the same site, as a sibling
	// subdomain; this keeps such a page from loading a file as a script or an image.
	h.Set("Cross-Origin-Resource-Policy", "same-origin")
	h.Set("Cache-Control", "private, max-age=31536000, immutable")
	http.ServeContent(w, r, "", time.Time{}, contents)

	return nil
}

func (n notes) serveDetach(w http.ResponseWriter, r *http.Request) error {
	id, fileID, err := ids(r, "file")
	if err != nil {
		return err
	}

	if err := n.detach(r.Context(), id, fileID); err != nil {
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

// ids reads the note's id and the one under key in the path, the revision
// or the file it names; one that is not a number names none.
func ids(r *http.Request, key string) (int64, int64, error) {
	id, err := noteID(r)
	if err != nil {
		return 0, 0, err
	}

	n, err := strconv.ParseInt(r.PathValue(key), 10, 64)
	if err != nil {
		return 0, 0, fmt.Errorf("%s %q: %w", key, r.PathValue(key), store.ErrNotFound)
	}

	return id, n, nil
}
