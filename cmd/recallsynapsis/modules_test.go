package main

import (
	"context"
	"crypto/ecdh"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mhersson/recallsynapsis/internal/module"
	"github.com/mhersson/recallsynapsis/internal/store"
	"github.com/mhersson/recallsynapsis/internal/testkit"
)

// TestMain gives the binary the fake module, which the shipped one lacks.
func TestMain(m *testing.M) {
	modules = append(modules, testkit.New)

	os.Exit(m.Run())
}

const fakeOn = "[modules.fake]\nenabled = true\n"

// hasTable reports whether the data directory's database has the table.
func hasTable(t *testing.T, dir, table string) bool {
	t.Helper()

	st, err := store.Open(t.Context(), filepath.Join(dir, "recallsynapsis.db"), nil)
	require.NoError(t, err)

	defer st.Close()

	var n int
	require.NoError(t, st.QueryRowContext(t.Context(), `SELECT count(*) FROM sqlite_schema WHERE type = 'table' AND name = ?`, table).Scan(&n))

	return n == 1
}

// Not parallel: serve replaces the default logger.
func TestEveryVerbAppliesTheMigrationsOfAModuleThatIsOff(t *testing.T) {
	keepDefaultLogger(t)

	for _, verb := range []string{"serve", "password"} {
		t.Run(verb, func(t *testing.T) {
			dir := filepath.Join(t.TempDir(), "data")
			config := writeConfig(t, t.TempDir(), unbindable)

			code, _, stderr := runWithInput(rightPhrase, verb, "--data-dir", dir, "--config", config)
			if verb == "password" {
				require.Equal(t, 0, code, stderr)
			}

			assert.True(t, hasTable(t, dir, "fake_items"))
			assert.True(t, hasTable(t, dir, "notes"), "the binary's own modules are built too")
			assert.True(t, hasTable(t, dir, "tasks"))
			assert.True(t, hasTable(t, dir, "push_subscriptions"))
			assert.True(t, hasTable(t, dir, "push_key"))
		})
	}
}

func TestAModuleTableTheBinaryCannotUseStopsTheStart(t *testing.T) {
	t.Parallel()

	tests := []struct{ name, table, key string }{
		{"a module the binary lacks", fakeOn + "[modules.ghost]\nenabled = true\n", "modules.ghost"},
		{"an unknown key", fakeOn + "colour = \"red\"\n", "modules.fake.colour"},
		{"an unknown key of notes", "[modules.notes]\ncolour = \"red\"\n", "modules.notes.colour"},
		{"a first day of the week that is none", "[modules.tasks]\nweek_start = \"someday\"\n", "modules.tasks.week_start"},
	}
	for _, tt := range tests {
		for _, verb := range []string{"serve", "password"} {
			t.Run(tt.name+" "+verb, func(t *testing.T) {
				t.Parallel()

				dir := filepath.Join(t.TempDir(), "data")
				config := writeConfig(t, t.TempDir(), unbindable+tt.table)

				code, _, stderr := runWithInput(rightPhrase, verb, "--data-dir", dir, "--config", config)
				assert.Equal(t, 1, code)
				assert.Contains(t, strings.ReplaceAll(stderr, config, ""), tt.key)
			})
		}
	}
}

func TestAModuleTellsTheDayInTheConfiguredZone(t *testing.T) {
	t.Parallel()

	config := writeConfig(t, t.TempDir(), validConfig+"timezone = \"Pacific/Kiritimati\"\n[modules.tasks]\nenabled = true\n")

	d, err := openData(t.Context(), filepath.Join(t.TempDir(), "data"), config, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = d.store.Close() })

	on := d.modules.On()
	i := slices.IndexFunc(on, func(m module.Module) bool { return m.Name == "tasks" })
	require.GreaterOrEqual(t, i, 0, "tasks is on")
	j := slices.IndexFunc(on[i].Routes, func(r module.Route) bool { return r.Pattern == "GET /api/tasks" })
	require.GreaterOrEqual(t, j, 0)

	rec := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(module.WithCaller(t.Context(), module.Caller{UserID: 1}), http.MethodGet, "/api/tasks", nil)
	require.NoError(t, on[i].Routes[j].Serve(rec, req))

	var got struct{ Now struct{ Timezone string } }
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got), rec.Body.String())
	assert.Equal(t, "Pacific/Kiritimati", got.Now.Timezone)
}

func TestPushSignsWithThePublicURL(t *testing.T) {
	t.Parallel()

	got := make(chan string, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got <- r.Header.Get("Authorization")

		w.WriteHeader(http.StatusCreated)
	}))
	t.Cleanup(srv.Close)

	config := writeConfig(t, t.TempDir(), validConfig+"[modules.push]\nenabled = true\n")

	d, err := openData(t.Context(), filepath.Join(t.TempDir(), "data"), config, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = d.store.Close() })

	browser, err := ecdh.P256().GenerateKey(rand.Reader)
	require.NoError(t, err)

	// A browser at a push service of the test, in plain HTTP, which a registration would refuse.
	b64 := base64.RawURLEncoding
	_, err = d.store.ExecContext(t.Context(), `INSERT INTO users (id, name, created_at) VALUES (1, 'ada', 0)`)
	require.NoError(t, err)
	_, err = d.store.ExecContext(t.Context(), `INSERT INTO push_subscriptions (user_id, endpoint, p256dh, auth, created_at) VALUES (1, ?, ?, ?, 0)`,
		srv.URL+"/push/one", b64.EncodeToString(browser.PublicKey().Bytes()), b64.EncodeToString(make([]byte, 16)))
	require.NoError(t, err)

	d.modules.Notify(t.Context(), 1, module.Message{Title: "Rake"})

	var header string

	select {
	case header = <-got:
	default:
		require.FailNow(t, "push sent nothing")
	}

	token, _, _ := strings.Cut(strings.TrimPrefix(header, "vapid t="), ", k=")
	claims, err := b64.DecodeString(strings.Split(token, ".")[1])
	require.NoError(t, err)

	var c struct{ Sub string }
	require.NoError(t, json.Unmarshal(claims, &c))
	assert.Equal(t, "https://recall.example.org", c.Sub)
}

// fakeRoute is the status of a POST with no authority to the fake module's
// route on the server at url, a refusal either way: 401 when the route is
// served, else 404.
func fakeRoute(t *testing.T, url string) int {
	t.Helper()

	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, url+"/api/fake/items", strings.NewReader("{}"))
	require.NoError(t, err)

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)

	// Read to its end, so the connection goes back idle and the shutdown need not wait on it.
	var got map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&got))
	require.NoError(t, resp.Body.Close())
	assert.Equal(t, "application/json", resp.Header.Get("Content-Type"))
	assert.Len(t, got, 1)
	assert.NotEmpty(t, got["error"])

	return resp.StatusCode
}

// Not parallel: it signals the test process, and serve replaces the default logger.
func TestServeServesAModuleThatIsOn(t *testing.T) {
	keepDefaultLogger(t)

	dir := filepath.Join(t.TempDir(), "data")
	config := writeConfig(t, t.TempDir(), validConfig+fakeOn)

	var stderr logBuffer

	done := make(chan int, 1)

	go func() {
		done <- run([]string{"serve", "--data-dir", dir, "--config", config}, strings.NewReader(""), io.Discard, &stderr)
	}()

	// The fake's job fails on every tick, and the next tick runs all the same.
	ran := regexp.MustCompile(`module=fake job=tick`)

	require.Eventually(t, func() bool { return len(ran.FindAllString(stderr.String(), -1)) >= 2 }, 5*time.Second, 10*time.Millisecond)

	address := regexp.MustCompile(`address=(\S+)`).FindStringSubmatch(stderr.String())
	require.Len(t, address, 2)
	assert.Equal(t, http.StatusUnauthorized, fakeRoute(t, "http://"+address[1]), "its route is served behind the access check")

	require.NoError(t, syscall.Kill(syscall.Getpid(), syscall.SIGTERM))
	assert.Equal(t, 0, within(t, done, "serve did not stop on the signal"), stderr.String())
	assert.NoFileExists(t, filepath.Join(dir, "recallsynapsis.db-wal"), "the store closed cleanly after the jobs")
}

func TestRunJobs(t *testing.T) {
	t.Parallel()

	var (
		logged   logBuffer
		runs     atomic.Int32
		inFlight = make(chan struct{})
		ended    = make(chan struct{})
		other    = make(chan struct{}, 1)
	)

	ms := []module.Module{
		{Name: "alpha", Jobs: []module.Job{{Name: "sweep", Every: time.Millisecond, Run: func(ctx context.Context) error {
			switch runs.Add(1) {
			case 1:
				return errors.New("swept nothing")
			case 2:
				return nil
			case 3:
				close(inFlight)
				<-ctx.Done()
				close(ended)

				return ctx.Err()
			}

			return nil
		}}}},
		{Name: "beta", Jobs: []module.Job{{Name: "count", Every: time.Millisecond, Run: func(context.Context) error {
			select {
			case other <- struct{}{}:
			default:
			}

			return nil
		}}}},
	}

	stop := runJobs(t.Context(), ms, slog.New(slog.NewTextHandler(&logged, nil)))

	within(t, inFlight, "the third tick never came after a failed run")
	within(t, other, "the job of another module never ran")
	stop()

	select {
	case <-ended:
	default:
		t.Fatal("stop returned before the run in flight ended")
	}

	assert.Equal(t, 1, strings.Count(logged.String(), "module=alpha job=sweep"), "the failed run is logged, the one the stop cut short is not")
}
