package api

import (
	"net/http"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/mhersson/recallsynapsis/internal/auth"
)

func (s *server) tokens(w http.ResponseWriter, r *http.Request) error {
	list, err := s.Auth.Tokens(r.Context(), caller(r).UserID)
	if err != nil {
		return err
	}

	reply(w, http.StatusOK, map[string][]auth.Token{"tokens": list})

	return nil
}

func (s *server) createToken(w http.ResponseWriter, r *http.Request) error {
	var in struct {
		Name   string `json:"name"`
		Access string `json:"access"`
	}

	if err := decode(w, r, &in); err != nil {
		return err
	}

	// A token's name is its caller's Source, so "web" stays the cookie's.
	name := strings.TrimSpace(in.Name)

	switch {
	case name == "" || utf8.RuneCountInString(name) > 64 || strings.ContainsFunc(name, unicode.IsControl):
		return &apiError{http.StatusBadRequest, "a token's name is 1 to 64 characters, with no control characters"}
	case strings.EqualFold(name, "web"):
		return &apiError{http.StatusBadRequest, `"web" names the web app as a source; name the token for its client`}
	case in.Access != "read" && in.Access != "write":
		return &apiError{http.StatusBadRequest, `a token's access is "read" or "write"`}
	}

	t, text, err := s.Auth.CreateToken(r.Context(), caller(r).UserID, name, in.Access)
	if err != nil {
		return err
	}

	reply(w, http.StatusCreated, struct {
		auth.Token
		Text string `json:"token"`
	}{t, text})

	return nil
}

func (s *server) deleteToken(w http.ResponseWriter, r *http.Request) error {
	id, err := pathID(r)
	if err != nil {
		return err
	}

	if err := s.Auth.DeleteToken(r.Context(), caller(r).UserID, id); err != nil {
		return err
	}

	w.WriteHeader(http.StatusNoContent)

	return nil
}
