package module

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
)

// Access is what a route or a tool asks of the request's authority.
type Access int

const (
	Open  Access = iota // nothing
	Read                // the session cookie or any token
	Write               // the session cookie or a write token
	Owner               // the session cookie alone, so no token makes a token or deletes anything for good
)

// Refusal is a refusal that is the caller's: a status and one line saying why.
type Refusal struct {
	Status int
	Text   string
}

func (e *Refusal) Error() string { return e.Text }

// MaxBody is the most of a JSON body the server reads, 1 MiB.
const MaxBody = 1 << 20

// Reply answers v as JSON with status.
func Reply(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// Decode reads the body, at most 1 MiB of it, into v, refusing a field v does not have.
func Decode(w http.ResponseWriter, r *http.Request, v any) error {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, MaxBody))
	dec.DisallowUnknownFields()

	err := dec.Decode(v)
	if err == nil {
		// Decode stops at the end of the first value, so what follows it is read here.
		if err = dec.Decode(new(json.RawMessage)); errors.Is(err, io.EOF) {
			return nil
		}

		if err == nil {
			err = errors.New("more than one value")
		}
	}

	if _, ok := errors.AsType[*http.MaxBytesError](err); ok {
		return &Refusal{http.StatusRequestEntityTooLarge, "the body is larger than 1 MiB"}
	}

	return &Refusal{http.StatusBadRequest, "the body is not the JSON this route takes: " + err.Error()}
}
