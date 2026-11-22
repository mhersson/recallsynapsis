package notes

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"mime"
	"os"
	"path/filepath"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/mhersson/recallsynapsis/internal/module"
	"github.com/mhersson/recallsynapsis/internal/store"
)

// maxName is the most bytes of a file's name, as the usual file systems keep.
const maxName = 255

// inline are the types a file is shown as in the page; any other is a download.
var inline = []string{"image/png", "image/jpeg", "image/gif", "image/webp"}

// file is an attachment as a note gives it.
type file struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
	Type string `json:"type"`
	Size int64  `json:"size"`
}

// fileList is the JSON list of a note's files, in the order they were attached.
const fileList = `(SELECT json_group_array(json_object('id', id, 'name', name, 'type', type, 'size', size) ORDER BY id)
	FROM note_files WHERE note_id = notes.id)`

// attach keeps what body holds as a file of the caller's note with the id,
// named by the base name of given and of the type kind declares. A note in
// the trash is ErrConflict.
func (n notes) attach(ctx context.Context, id int64, given, kind string, body io.Reader) (file, error) {
	c, _ := module.CallerFrom(ctx)

	f := file{Type: mediaType(kind)}

	var err error
	if f.Name, err = fileName(given); err != nil {
		return file{}, err
	}

	// Refused before the body is read.
	if _, err := changeable(ctx, n.Store, c.UserID, id); err != nil {
		return file{}, err
	}

	tmp, sum, size, err := n.receive(body)
	if err != nil {
		return file{}, err
	}
	defer func() { _ = os.Remove(tmp) }() // gone already once it is in files/

	f.Size = size

	n.files.Lock()
	defer n.files.Unlock()

	if err := n.place(tmp, sum); err != nil {
		return file{}, err
	}

	var area int64

	err = n.Store.Tx(ctx, func(tx *sql.Tx) error {
		var err error
		if area, err = changeable(ctx, tx, c.UserID, id); err != nil {
			return err
		}

		err = tx.QueryRowContext(ctx, `INSERT INTO note_files (note_id, name, type, size, sha256) VALUES (?, ?, ?, ?, ?) RETURNING id`,
			id, f.Name, f.Type, f.Size, sum).Scan(&f.ID)
		if err != nil {
			return fmt.Errorf("notes: attach: %w", err)
		}

		return nil
	})
	if err != nil {
		n.unlink(ctx, sum)

		return file{}, err
	}

	n.Changed(ctx, module.Change{Module: name, Kind: "note", ID: id, Area: area})

	return f, nil
}

// receive writes body under tmp/, hashing it as it goes, and gives the path
// it wrote, the SHA-256 of what it holds in hex, and its size.
func (n notes) receive(body io.Reader) (string, string, int64, error) {
	dir := filepath.Join(n.Dir, "tmp")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", "", 0, fmt.Errorf("notes: upload: %w", err)
	}

	f, err := os.CreateTemp(dir, "upload-*")
	if err != nil {
		return "", "", 0, fmt.Errorf("notes: upload: %w", err)
	}

	h := sha256.New()

	size, err := io.Copy(io.MultiWriter(f, h), body)
	if err == nil {
		err = f.Sync() // else a crash could leave files/ a name with less behind it
	}

	if cerr := f.Close(); err == nil {
		err = cerr
	}

	if err != nil {
		_ = os.Remove(f.Name())

		return "", "", 0, fmt.Errorf("notes: upload: %w", err)
	}

	return f.Name(), hex.EncodeToString(h.Sum(nil)), size, nil
}

// place moves the upload at tmp into files/ as sum, unless files/ holds
// those contents already.
func (n notes) place(tmp, sum string) error {
	dir := filepath.Join(n.Dir, "files")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("notes: upload: %w", err)
	}

	to := filepath.Join(dir, sum)

	switch _, err := os.Lstat(to); {
	case err == nil:
		return nil
	case !errors.Is(err, fs.ErrNotExist):
		return fmt.Errorf("notes: upload: %w", err)
	}

	if err := os.Rename(tmp, to); err != nil {
		return fmt.Errorf("notes: upload: %w", err)
	}

	return nil
}

// attachment gives the caller's file fileID of their note with the id, and
// the path of its contents. A note in the trash still gives its files.
func (n notes) attachment(ctx context.Context, id, fileID int64) (file, string, error) {
	c, _ := module.CallerFrom(ctx)

	var (
		f   = file{ID: fileID}
		sum string
	)

	err := n.Store.QueryRowContext(ctx, `SELECT name, type, size, sha256 FROM note_files JOIN notes ON notes.id = note_id
		WHERE user_id = ? AND note_id = ? AND note_files.id = ?`, c.UserID, id, fileID).Scan(&f.Name, &f.Type, &f.Size, &sum)
	if errors.Is(err, sql.ErrNoRows) {
		return file{}, "", fmt.Errorf("file %d of note %d: %w", fileID, id, store.ErrNotFound)
	}

	if err != nil {
		return file{}, "", fmt.Errorf("notes: file: %w", err)
	}

	return f, filepath.Join(n.Dir, "files", sum), nil
}

// detach removes the file fileID from the caller's note with the id, and its
// contents once no attachment names them. A note in the trash is ErrConflict.
func (n notes) detach(ctx context.Context, id, fileID int64) error {
	c, _ := module.CallerFrom(ctx)

	var area int64

	err := n.drop(ctx, `SELECT json_group_array(sha256) FROM note_files WHERE note_id = ? AND id = ?`, []any{id, fileID}, func(tx *sql.Tx) error {
		var err error
		if area, err = changeable(ctx, tx, c.UserID, id); err != nil {
			return err
		}

		err = tx.QueryRowContext(ctx, `DELETE FROM note_files WHERE note_id = ? AND id = ? RETURNING id`, id, fileID).Scan(new(int64))
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("file %d of note %d: %w", fileID, id, store.ErrNotFound)
		}

		if err != nil {
			return fmt.Errorf("notes: detach: %w", err)
		}

		return nil
	})
	if err != nil {
		return err
	}

	n.Changed(ctx, module.Change{Module: name, Kind: "note", ID: id, Area: area})

	return nil
}

// drop runs fn in a transaction and, once it has committed, removes the
// contents no attachment names any longer of those that sums gives: a query
// of their JSON list, binding args, run in the transaction before fn. Holding
// files keeps an upload from naming contents in between.
func (n notes) drop(ctx context.Context, sums string, args []any, fn func(*sql.Tx) error) error {
	n.files.Lock()
	defer n.files.Unlock()

	var list []string

	err := n.Store.Tx(ctx, func(tx *sql.Tx) error {
		var text string
		if err := tx.QueryRowContext(ctx, sums, args...).Scan(&text); err != nil {
			return fmt.Errorf("notes: files: %w", err)
		}

		if err := json.Unmarshal([]byte(text), &list); err != nil {
			return fmt.Errorf("notes: files: %w", err)
		}

		return fn(tx)
	})
	if err != nil {
		return err
	}

	n.unlink(ctx, list...)

	return nil
}

// unlink removes the contents each of sums names, unless an attachment still
// names them. A failure leaves only contents nothing reads, so it is logged
// rather than failing a change that is stored.
func (n notes) unlink(ctx context.Context, sums ...string) {
	ctx = context.WithoutCancel(ctx)

	for _, sum := range sums {
		var named bool

		err := n.Store.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM note_files WHERE sha256 = ?)`, sum).Scan(&named)
		if err == nil && !named {
			err = os.Remove(filepath.Join(n.Dir, "files", sum))
		}

		if err != nil && !errors.Is(err, fs.ErrNotExist) {
			slog.ErrorContext(ctx, "notes: remove a file's contents", "sha256", sum, "err", err)
		}
	}
}

// querier is the store or a transaction of it.
type querier interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// changeable gives the area of the user's note with the id, and refuses one
// that is not there or is in the trash.
func changeable(ctx context.Context, q querier, userID, id int64) (int64, error) {
	var (
		area    int64
		trashed bool
	)

	err := q.QueryRowContext(ctx, `SELECT area_id, trashed_at IS NOT NULL FROM notes WHERE user_id = ? AND id = ?`, userID, id).Scan(&area, &trashed)

	switch {
	case errors.Is(err, sql.ErrNoRows):
		return 0, fmt.Errorf("note %d: %w", id, store.ErrNotFound)
	case err != nil:
		return 0, fmt.Errorf("notes: note %d: %w", id, err)
	case trashed:
		return 0, errTrashed
	}

	return area, nil
}

// fileName gives the base name of given with no control character, and
// refuses one with nothing left, one that names a directory, and one longer
// than maxName.
func fileName(given string) (string, error) {
	out := strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || r == utf8.RuneError {
			return -1
		}

		return r
	}, given[strings.LastIndexAny(given, `/\`)+1:])

	if out == "" || out == "." || out == ".." || len(out) > maxName {
		return "", fmt.Errorf("?name= is the file's name, 1 to %d bytes: %w", maxName, store.ErrInvalid)
	}

	return out, nil
}

// mediaType gives the media type kind declares, without its parameters, or
// application/octet-stream when it declares none.
func mediaType(kind string) string {
	t, _, err := mime.ParseMediaType(kind)
	if err != nil || !strings.Contains(t, "/") {
		return "application/octet-stream"
	}

	return t
}
