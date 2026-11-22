package api

import (
	"errors"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/mhersson/recallsynapsis/internal/auth"
	"github.com/mhersson/recallsynapsis/internal/module"
)

const (
	cookieName = "recallsynapsis_session"
	csrfHeader = "X-Recallsynapsis-CSRF"
)

var errNoAuthority = &module.Refusal{Status: http.StatusUnauthorized, Text: "sign in, or send an API token as Authorization: Bearer TOKEN"}

// authorize admits r to a route that asks for level, and returns r carrying
// its caller: a bearer token when the request sends one, else the session cookie.
func (s *server) authorize(w http.ResponseWriter, r *http.Request, level module.Access) (*http.Request, error) {
	if level == module.Open {
		return r, nil
	}

	c, err := s.identify(w, r, level)
	if err != nil {
		return r, err
	}

	return r.WithContext(module.WithCaller(r.Context(), c)), nil
}

func (s *server) identify(w http.ResponseWriter, r *http.Request, level module.Access) (module.Caller, error) {
	if scheme, text, _ := strings.Cut(r.Header.Get("Authorization"), " "); strings.EqualFold(scheme, "Bearer") {
		c, found, err := s.Auth.Bearer(r.Context(), text)

		switch {
		case err != nil:
			return c, err
		case !found:
			return c, &module.Refusal{Status: http.StatusUnauthorized, Text: "the API token is unknown or revoked"}
		case level == module.Owner:
			return c, &module.Refusal{Status: http.StatusForbidden, Text: "no API token can reach this route; sign in to the web app"}
		case level == module.Write && !c.Write:
			return c, &module.Refusal{Status: http.StatusForbidden, Text: "this route changes something, and the API token may only read"}
		}

		return c, nil
	}

	cookie, err := r.Cookie(cookieName)
	if err != nil {
		return module.Caller{}, errNoAuthority
	}

	// SameSite does not tell ports apart, so a page on another port of this host
	// sends the cookie too; it cannot add a header without CORS, which is never granted.
	if r.Method != http.MethodGet && r.Method != http.MethodHead && r.Header.Get(csrfHeader) == "" {
		return module.Caller{}, &module.Refusal{Status: http.StatusForbidden, Text: "a request with the session cookie that changes something needs the " + csrfHeader + " header"}
	}

	c, renewed, ok, err := s.Auth.Session(r.Context(), cookie.Value)
	if err != nil {
		return c, err
	}

	if !ok {
		return c, errNoAuthority
	}

	if !renewed.IsZero() {
		http.SetCookie(w, s.cookie(cookie.Value, renewed))
	}

	return c, nil
}

// caller is the caller authorize put in r; a route that is not open always has one.
func caller(r *http.Request) module.Caller {
	c, _ := module.CallerFrom(r.Context())

	return c
}

func (s *server) cookie(value string, expires time.Time) *http.Cookie {
	return &http.Cookie{ //nolint:gosec // Secure follows public_url: plain http has no https to protect
		Name:     cookieName,
		Value:    value,
		Path:     "/",
		Expires:  expires,
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
		Secure:   s.secure,
	}
}

func (s *server) login(w http.ResponseWriter, r *http.Request) error {
	var in struct {
		Name     string `json:"name"`
		Password string `json:"password"`
	}

	if err := module.Decode(w, r, &in); err != nil {
		return err
	}

	// The limiter counts by the host alone: the port is new on every
	// connection, and a forwarded header is whatever the client wrote.
	host, _, _ := net.SplitHostPort(r.RemoteAddr)

	value, expires, err := s.Auth.Login(r.Context(), in.Name, in.Password, host)
	if limited, ok := errors.AsType[*auth.LimitedError](err); ok {
		w.Header().Set("Retry-After", strconv.FormatInt(int64((limited.RetryAfter+time.Second-1)/time.Second), 10))

		return &module.Refusal{Status: http.StatusTooManyRequests, Text: "too many failed sign-ins; try again after Retry-After seconds"}
	}

	if errors.Is(err, auth.ErrBadLogin) {
		return &module.Refusal{Status: http.StatusUnauthorized, Text: "wrong name or password"}
	}

	if err != nil {
		return err
	}

	http.SetCookie(w, s.cookie(value, expires))
	w.WriteHeader(http.StatusNoContent)

	return nil
}

// logout ends the session of the cookie the request carries and clears the cookie.
func (s *server) logout(w http.ResponseWriter, r *http.Request) error {
	if c, err := r.Cookie(cookieName); err == nil {
		if err := s.Auth.Logout(r.Context(), c.Value); err != nil {
			return err
		}

		// The cleared cookie replaces the renewal authorize may have sent.
		w.Header().Del("Set-Cookie")

		cleared := s.cookie("", time.Time{}) //nolint:gosec // the attributes of the cookie it clears
		cleared.MaxAge = -1
		http.SetCookie(w, cleared)
	}

	w.WriteHeader(http.StatusNoContent)

	return nil
}

func (s *server) session(w http.ResponseWriter, r *http.Request) error {
	name, err := s.Auth.UserName(r.Context(), caller(r).UserID)
	if err != nil {
		return err
	}

	modules := []string{}
	for _, m := range s.Modules.On() {
		modules = append(modules, m.Name)
	}

	module.Reply(w, http.StatusOK, map[string]any{
		"user":        map[string]string{"name": name},
		"modules":     modules,
		"version":     s.Version,
		"timezone":    s.Timezone,
		"last_backup": nil,
	})

	return nil
}
