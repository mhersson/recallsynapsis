package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mhersson/recallsynapsis/internal/auth"
	"github.com/mhersson/recallsynapsis/internal/core"
	"github.com/mhersson/recallsynapsis/internal/store"
)

const validConfig = "listen = \"127.0.0.1:0\"\npublic_url = \"https://recall.example.org\"\nowner = \"ada\"\n"

// unbindable passes the configuration's checks but fails at listen, so a start
// that should have been refused before it fails fast instead of serving.
var unbindable = strings.Replace(validConfig, "127.0.0.1:0", "127.0.0.1:-1", 1)

// logBuffer is a writer the server and the test may use at once.
type logBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (l *logBuffer) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	return l.b.Write(p)
}

func (l *logBuffer) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()

	return l.b.String()
}

// keepDefaultLogger restores the default logger after t, and the log package
// that slog.SetDefault redirects.
func keepDefaultLogger(t *testing.T) {
	t.Helper()

	def, out, flags := slog.Default(), log.Writer(), log.Flags()

	t.Cleanup(func() {
		slog.SetDefault(def)
		log.SetOutput(out)
		log.SetFlags(flags)
	})
}

func writeConfig(t *testing.T, dir, content string) string {
	t.Helper()

	path := filepath.Join(dir, "recallsynapsis.toml")
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))

	return path
}

func TestDataDir(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("RECALLSYNAPSIS_DATA_DIR", "")
	t.Setenv("XDG_DATA_HOME", "")

	got, err := dataDir("")
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(home, ".local", "share", "recallsynapsis"), got)

	t.Setenv("XDG_DATA_HOME", "/xdg")

	got, err = dataDir("")
	require.NoError(t, err)
	assert.Equal(t, filepath.Join("/xdg", "recallsynapsis"), got)

	t.Setenv("RECALLSYNAPSIS_DATA_DIR", "/from-env")

	got, err = dataDir("")
	require.NoError(t, err)
	assert.Equal(t, "/from-env", got)

	got, err = dataDir("/from-flag")
	require.NoError(t, err)
	assert.Equal(t, "/from-flag", got)
}

func TestADataDirectoryOthersHaveAnyPermissionOnIsRefused(t *testing.T) {
	t.Parallel()

	for _, mode := range []os.FileMode{0o740, 0o720, 0o710, 0o704, 0o702, 0o701} {
		t.Run(mode.String(), func(t *testing.T) {
			t.Parallel()

			dir := filepath.Join(t.TempDir(), "data")
			require.NoError(t, os.Mkdir(dir, 0o700))
			writeConfig(t, dir, unbindable)
			require.NoError(t, os.Chmod(dir, mode))

			code, _, _ := runVerb("serve", "--data-dir", dir)
			assert.Equal(t, 1, code)

			entries, err := os.ReadDir(dir)
			require.NoError(t, err)
			require.Len(t, entries, 1, "a refused directory gains nothing")
			assert.Equal(t, "recallsynapsis.toml", entries[0].Name())
		})
	}
}

func TestServeRefusals(t *testing.T) {
	t.Parallel()

	t.Run("help", func(t *testing.T) {
		t.Parallel()

		code, stdout, _ := runVerb("serve", "-h")
		assert.Equal(t, 0, code)
		assert.Empty(t, stdout)
	})

	t.Run("an argument", func(t *testing.T) {
		t.Parallel()

		dir := filepath.Join(t.TempDir(), "data")
		code, _, _ := runVerb("serve", "--data-dir", dir, "extra")
		assert.Equal(t, 1, code)
		assert.NoDirExists(t, dir)
	})

	t.Run("no configuration file", func(t *testing.T) {
		t.Parallel()

		dir := filepath.Join(t.TempDir(), "data")
		code, _, stderr := runVerb("serve", "--data-dir", dir)
		assert.Equal(t, 1, code)
		assert.Contains(t, stderr, filepath.Join(dir, "recallsynapsis.toml"), "the refusal names the file")
		assert.NoDirExists(t, dir, "a refused start makes nothing")
	})

	t.Run("a configuration file it refuses", func(t *testing.T) {
		t.Parallel()

		dir := filepath.Join(t.TempDir(), "data")
		require.NoError(t, os.Mkdir(dir, 0o700))
		writeConfig(t, dir, "public_url = \"https://recall.example.org\"\n")

		code, _, _ := runVerb("serve", "--data-dir", dir)
		assert.Equal(t, 1, code)
		assert.NoFileExists(t, filepath.Join(dir, "recallsynapsis.db"))
	})
}

// Not parallel: it signals the test process, and serve replaces the default logger.
func TestServeAnswersAndStopsOnASignal(t *testing.T) {
	keepDefaultLogger(t)

	// The data directory does not exist yet, and holds no configuration file.
	dir := filepath.Join(t.TempDir(), "nested", "data")
	config := writeConfig(t, t.TempDir(), validConfig)

	var stderr logBuffer

	done := make(chan int, 1)

	go func() {
		done <- run([]string{"serve", "--data-dir", dir, "--config", config}, strings.NewReader(""), io.Discard, &stderr)
	}()

	address := regexp.MustCompile(`address=(\S+)`)

	require.Eventually(t, func() bool { return address.MatchString(stderr.String()) }, 5*time.Second, 10*time.Millisecond)

	url := "http://" + address.FindStringSubmatch(stderr.String())[1]

	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, url+"/api/health", nil)
	require.NoError(t, err)

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)

	var health map[string]string
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&health))
	require.NoError(t, resp.Body.Close())
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, map[string]string{"status": "ok", "version": version}, health)

	// A page path gets the web app, or the placeholder in a build without it, and its own log line.
	req, err = http.NewRequestWithContext(t.Context(), http.MethodGet, url+"/settings", nil)
	require.NoError(t, err)

	resp, err = http.DefaultClient.Do(req)
	require.NoError(t, err)

	page, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "text/html; charset=utf-8", resp.Header.Get("Content-Type"))
	assert.NotEmpty(t, resp.Header.Get("Content-Security-Policy"))
	assert.Contains(t, string(page), "<html")
	assert.Eventually(t, func() bool { return strings.Contains(stderr.String(), "path=/settings status=200") }, time.Second, 10*time.Millisecond)

	info, err := os.Stat(dir)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o700), info.Mode().Perm())
	assert.FileExists(t, filepath.Join(dir, "recallsynapsis.db"))

	signalled := time.Now()

	require.NoError(t, syscall.Kill(syscall.Getpid(), syscall.SIGTERM))

	assert.Equal(t, 0, within(t, done, "serve did not stop on the signal"), stderr.String())
	assert.Less(t, time.Since(signalled), shutdownGrace)
	// SQLite removes the write-ahead log when the last connection closes cleanly.
	assert.NoFileExists(t, filepath.Join(dir, "recallsynapsis.db-wal"))
}

// serveOn runs serve with h and grace on a spare port, and returns its address,
// the cancel that stands for the signal, and what serve returns.
func serveOn(t *testing.T, h http.Handler, grace time.Duration) (string, context.CancelFunc, <-chan error) {
	t.Helper()

	ln, err := (&net.ListenConfig{}).Listen(t.Context(), "tcp", "127.0.0.1:0")
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(t.Context())
	served := make(chan error, 1)

	go func() { served <- serve(ctx, ln, h, grace, slog.New(slog.DiscardHandler)) }()

	return ln.Addr().String(), cancel, served
}

// status sends a GET to addr and yields the status of the answer, or 0 when none came.
func status(t *testing.T, addr string) <-chan int {
	t.Helper()

	got := make(chan int, 1)

	go func() {
		req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, "http://"+addr+"/", nil)
		if err != nil {
			got <- 0

			return
		}

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			got <- 0

			return
		}

		_ = resp.Body.Close()
		got <- resp.StatusCode
	}()

	return got
}

// within gives what ch yields, and fails t with what when nothing comes in time.
func within[T any](t *testing.T, ch <-chan T, what string) T {
	t.Helper()

	select {
	case v := <-ch:
		return v
	case <-time.After(2 * shutdownGrace):
		t.Fatal(what)

		panic("unreachable")
	}
}

func TestOpenRequestsGetTheGrace(t *testing.T) {
	entered, release := make(chan struct{}), make(chan struct{})
	addr, cancel, served := serveOn(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		close(entered)
		<-release
		w.WriteHeader(http.StatusNoContent)
	}), shutdownGrace)

	answered := status(t, addr)

	within(t, entered, "the request never reached the handler")
	cancel()

	// A refused connection shows the shutdown has begun with the request still open.
	require.Eventually(t, func() bool {
		conn, err := (&net.Dialer{}).DialContext(t.Context(), "tcp", addr)
		if err == nil {
			_ = conn.Close()
		}

		return err != nil
	}, shutdownGrace, 10*time.Millisecond)
	close(release)

	assert.Equal(t, http.StatusNoContent, within(t, answered, "the open request got no answer"))
	require.NoError(t, within(t, served, "serve did not stop"))
}

func TestOpenRequestsAreCutAfterTheGrace(t *testing.T) {
	entered, ended := make(chan struct{}), make(chan struct{})
	addr, cancel, served := serveOn(t, http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		close(entered)
		<-r.Context().Done()
		close(ended)
	}), 100*time.Millisecond)

	status(t, addr)
	within(t, entered, "the request never reached the handler")
	cancel()

	require.NoError(t, within(t, served, "serve waited on the open request past its grace"))
	within(t, ended, "the cut request's context never ended")
}

func TestTheRequestLog(t *testing.T) {
	t.Parallel()

	var logged bytes.Buffer

	h := logRequests(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)

		if r.URL.Path == "/teapot" {
			w.WriteHeader(http.StatusTeapot)
		}

		_, _ = io.WriteString(w, "answer")
	}), slog.New(slog.NewJSONHandler(&logged, nil)))

	for _, target := range []string{"/plain?token=query-secret", "/teapot?token=query-secret"} {
		req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, target, strings.NewReader("body-secret"))
		req.Header.Set("Authorization", "Bearer header-secret")
		req.Header.Set("Cookie", "recallsynapsis_session=cookie-secret")
		h.ServeHTTP(httptest.NewRecorder(), req)
	}

	for _, secret := range []string{"query-secret", "body-secret", "header-secret", "cookie-secret"} {
		assert.NotContains(t, logged.String(), secret)
	}

	type line struct {
		Method string        `json:"method"`
		Path   string        `json:"path"`
		Status int           `json:"status"`
		Took   time.Duration `json:"took"`
	}

	var lines []line

	scan := bufio.NewScanner(&logged)
	for scan.Scan() {
		var l line
		require.NoError(t, json.Unmarshal(scan.Bytes(), &l))

		assert.Positive(t, l.Took)

		l.Took = 0
		lines = append(lines, l)
	}

	assert.Equal(t, []line{
		{Method: http.MethodPost, Path: "/plain", Status: http.StatusOK},
		{Method: http.MethodPost, Path: "/teapot", Status: http.StatusTeapot},
	}, lines)
}

// Not parallel: it sets the environment, and serve replaces the default logger.
func TestThePasswordHashFromTheEnvironment(t *testing.T) {
	keepDefaultLogger(t)

	dir := filepath.Join(t.TempDir(), "data")
	config := writeConfig(t, t.TempDir(), unbindable)

	// Each start runs every step up to listening, which unbindable then fails.
	start := func(value string) string {
		t.Setenv("RECALLSYNAPSIS_PASSWORD_HASH", value)

		code, _, stderr := runVerb("serve", "--data-dir", dir, "--config", config)
		require.Equal(t, 1, code)
		assert.NotContains(t, stderr, value, "the value never reaches the output")

		return stderr
	}

	first, err := auth.HashPassword(rightPhrase)
	require.NoError(t, err)
	start(first)

	// The owner was made at start, and signs in with the hash's password.
	site := openSite(t, dir)

	code, cookie := signIn(t, site, rightPhrase)
	require.Equal(t, http.StatusNoContent, code)

	start(first)
	assert.Equal(t, http.StatusOK, sessionStatus(t, site, cookie), "the same hash changes nothing")

	second, err := auth.HashPassword(otherPhrase)
	require.NoError(t, err)
	start(second)
	assert.Equal(t, http.StatusUnauthorized, sessionStatus(t, site, cookie), "a new hash ends every session")

	code, cookie = signIn(t, site, otherPhrase)
	require.Equal(t, http.StatusNoContent, code)

	assert.Contains(t, start(rightPhrase), "RECALLSYNAPSIS_PASSWORD_HASH", "a value that is not a hash is refused by the variable's name")
	assert.Equal(t, http.StatusOK, sessionStatus(t, site, cookie), "and changes nothing")
}

// Not parallel: it sets the environment, and serve replaces the default logger.
func TestAStartWarnsWhileTheOwnerHasNoPassword(t *testing.T) {
	keepDefaultLogger(t)
	t.Setenv("RECALLSYNAPSIS_PASSWORD_HASH", "")

	dir := filepath.Join(t.TempDir(), "data")
	config := writeConfig(t, t.TempDir(), unbindable)

	_, _, stderr := runVerb("serve", "--data-dir", dir, "--config", config)
	assert.Contains(t, stderr, "level=WARN")

	code, _, _ := runWithInput(rightPhrase+"\n", "password", "--data-dir", dir, "--config", config)
	require.Equal(t, 0, code)

	_, _, stderr = runVerb("serve", "--data-dir", dir, "--config", config)
	assert.NotContains(t, stderr, "level=WARN")
}

// Not parallel: serve replaces the default logger.
func TestAStartMakesTheOwnersPersonalArea(t *testing.T) {
	keepDefaultLogger(t)

	dir := filepath.Join(t.TempDir(), "data")
	config := writeConfig(t, t.TempDir(), unbindable)

	personal := func(owner string) []core.Area {
		st, err := store.Open(t.Context(), filepath.Join(dir, "recallsynapsis.db"), migrations)
		require.NoError(t, err)

		defer st.Close()

		id, err := auth.New(st, time.Now).EnsureUser(t.Context(), owner)
		require.NoError(t, err)

		list, err := core.New(st).Areas(t.Context(), id, true)
		require.NoError(t, err)

		return list
	}

	runVerb("serve", "--data-dir", dir, "--config", config)

	made := personal("ada")
	require.Len(t, made, 1)
	assert.Equal(t, "personal", made[0].Kind)

	// The owner renamed keeps the area, and no start makes a second.
	writeConfig(t, filepath.Dir(config), strings.Replace(unbindable, `"ada"`, `"grace"`, 1))
	runVerb("serve", "--data-dir", dir, "--config", config)
	runVerb("serve", "--data-dir", dir, "--config", config)

	assert.Equal(t, made, personal("grace"))
}
