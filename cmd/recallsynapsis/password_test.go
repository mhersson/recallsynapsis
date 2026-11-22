package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mhersson/recallsynapsis/internal/api"
	"github.com/mhersson/recallsynapsis/internal/auth"
	"github.com/mhersson/recallsynapsis/internal/core"
	"github.com/mhersson/recallsynapsis/internal/store"
)

const (
	rightPhrase = "made-up-test-password"
	otherPhrase = "another-made-up-password"
)

func runWithInput(stdin string, args ...string) (code int, stdout, stderr string) {
	var out, errOut bytes.Buffer

	code = run(args, strings.NewReader(stdin), &out, &errOut)

	return code, out.String(), errOut.String()
}

// openSite serves the API over the data directory's database, as serve would
// from its own process.
func openSite(t *testing.T, dir string) http.Handler {
	t.Helper()

	st, err := store.Open(t.Context(), filepath.Join(dir, "recallsynapsis.db"), migrations)
	require.NoError(t, err)

	t.Cleanup(func() { _ = st.Close() })

	return api.New(api.Deps{Auth: auth.New(st, time.Now), Core: core.New(st), Modules: &core.Modules{}, PublicURL: "https://recall.example.org"})
}

// signIn signs ada in and returns the status and the Cookie header of her session.
func signIn(t *testing.T, h http.Handler, password string) (int, string) {
	t.Helper()

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/api/auth/login",
		strings.NewReader(`{"name":"ada","password":"`+password+`"}`)))

	for _, c := range rec.Result().Cookies() {
		return rec.Code, "recallsynapsis_session=" + c.Value
	}

	return rec.Code, ""
}

func sessionStatus(t *testing.T, h http.Handler, cookie string) int {
	t.Helper()

	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/api/session", nil)
	req.Header.Set("Cookie", cookie)

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	return rec.Code
}

func TestPasswordVerb(t *testing.T) {
	t.Parallel()

	dir := filepath.Join(t.TempDir(), "data")
	config := writeConfig(t, t.TempDir(), validConfig)

	code, stdout, stderr := runWithInput(rightPhrase+"\r\n", "password", "--data-dir", dir, "--config", config)
	require.Equal(t, 0, code, stderr)
	assert.NotEmpty(t, stdout)
	assert.NotContains(t, stdout+stderr, rightPhrase)
	assert.NotContains(t, stdout+stderr, "$argon2id", "only --hash prints the hash")

	info, err := os.Stat(dir)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o700), info.Mode().Perm())

	// This handle stays open as a running serve's would.
	site := openSite(t, dir)

	code, cookie := signIn(t, site, rightPhrase)
	require.Equal(t, http.StatusNoContent, code)
	assert.Equal(t, http.StatusOK, sessionStatus(t, site, cookie))

	code, _, stderr = runWithInput(otherPhrase+"\n", "password", "--data-dir", dir, "--config", config)
	require.Equal(t, 0, code, stderr)

	assert.Equal(t, http.StatusUnauthorized, sessionStatus(t, site, cookie), "a new password ends every session")

	code, _ = signIn(t, site, rightPhrase)
	assert.Equal(t, http.StatusUnauthorized, code)

	code, _ = signIn(t, site, otherPhrase)
	assert.Equal(t, http.StatusNoContent, code)
}

func TestPasswordVerbRefusals(t *testing.T) {
	t.Parallel()

	config := writeConfig(t, t.TempDir(), validConfig)

	t.Run("help", func(t *testing.T) {
		t.Parallel()

		code, stdout, _ := runWithInput("", "password", "-h")
		assert.Equal(t, 0, code)
		assert.Empty(t, stdout)
	})

	t.Run("a short password", func(t *testing.T) {
		t.Parallel()

		dir := filepath.Join(t.TempDir(), "data")
		code, stdout, stderr := runWithInput("elevenchars\n", "password", "--data-dir", dir, "--config", config)
		assert.Equal(t, 1, code)
		assert.Empty(t, stdout)
		assert.NotEmpty(t, stderr)
		assert.NoDirExists(t, dir, "a refused password makes nothing")
	})

	t.Run("the password as an argument", func(t *testing.T) {
		t.Parallel()

		dir := filepath.Join(t.TempDir(), "data")
		code, _, stderr := runWithInput(rightPhrase, "password", "--data-dir", dir, "--config", config, rightPhrase)
		assert.Equal(t, 1, code)
		assert.NotContains(t, stderr, rightPhrase)
		assert.NoDirExists(t, dir)
	})

	t.Run("a terminal", func(t *testing.T) {
		t.Parallel()

		tty, err := os.Open(os.DevNull) // a character device, as a terminal is
		require.NoError(t, err)

		t.Cleanup(func() { _ = tty.Close() })

		dir := filepath.Join(t.TempDir(), "data")

		var out, errOut bytes.Buffer

		assert.Equal(t, 1, run([]string{"password", "--data-dir", dir, "--config", config}, tty, &out, &errOut))
		assert.NotEmpty(t, errOut.String())
		assert.NoDirExists(t, dir, "the terminal is refused before anything is made")
	})
}

func TestPasswordVerbPrintsAHash(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "data")
	t.Setenv("RECALLSYNAPSIS_DATA_DIR", dir)

	code, stdout, stderr := runWithInput(rightPhrase+"\n", "password", "--hash")
	require.Equal(t, 0, code, stderr)

	ok, err := auth.VerifyPassword(rightPhrase, strings.TrimSuffix(stdout, "\n"))
	require.NoError(t, err)
	assert.True(t, ok)

	code, stdout, _ = runWithInput("elevenchars\n", "password", "--hash")
	assert.Equal(t, 1, code)
	assert.Empty(t, stdout)

	assert.NoDirExists(t, dir, "no data directory is made")
}
