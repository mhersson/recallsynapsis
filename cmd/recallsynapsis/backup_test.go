package main

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mhersson/recallsynapsis/internal/auth"
	"github.com/mhersson/recallsynapsis/internal/core"
	"github.com/mhersson/recallsynapsis/internal/module"
	"github.com/mhersson/recallsynapsis/internal/store"
)

// backupConfig is validConfig with backup.dir set to dir.
func backupConfig(dir string) string {
	return validConfig + "\n[backup]\ndir = \"" + dir + "\"\n"
}

// ownedData makes a data directory whose owner ada has rightPhrase as her
// password and a project named Garden, and returns it.
func ownedData(t *testing.T, config string) string {
	t.Helper()

	dir := filepath.Join(t.TempDir(), "data")

	code, _, stderr := runWithInput(rightPhrase+"\n", "password", "--data-dir", dir, "--config", config)
	require.Equal(t, 0, code, stderr)

	st, err := store.Open(t.Context(), filepath.Join(dir, "recallsynapsis.db"), migrations)
	require.NoError(t, err)

	defer st.Close()

	owner, err := auth.New(st, time.Now).EnsureUser(t.Context(), "ada")
	require.NoError(t, err)

	_, err = core.New(st).CreateProject(t.Context(), owner, "Garden", "")
	require.NoError(t, err)

	return dir
}

func TestBackupVerb(t *testing.T) {
	t.Parallel()

	into := filepath.Join(t.TempDir(), "backup")
	config := writeConfig(t, t.TempDir(), backupConfig(into))
	dir := ownedData(t, config)

	code, stdout, stderr := runVerb("backup", "--data-dir", dir, "--config", config)
	require.Equal(t, 0, code, stderr)

	path := strings.TrimSuffix(stdout, "\n")
	assert.Equal(t, into, filepath.Dir(path))

	// The snapshot is the whole database: the owner signs in to it with her password.
	code, _ = signIn(t, openSite(t, restore(t, path)), rightPhrase)
	assert.Equal(t, http.StatusNoContent, code)

	// --to backs up into another directory instead.
	other := filepath.Join(t.TempDir(), "other")

	code, stdout, stderr = runVerb("backup", "--data-dir", dir, "--config", config, "--to", other)
	require.Equal(t, 0, code, stderr)
	assert.Equal(t, other, filepath.Dir(strings.TrimSuffix(stdout, "\n")))
	assert.Equal(t, []string{filepath.Base(path)}, dirNames(t, into), "backup.dir gains nothing")
}

func TestBackupRefusals(t *testing.T) {
	t.Parallel()

	t.Run("an argument", func(t *testing.T) {
		t.Parallel()

		into := filepath.Join(t.TempDir(), "backup")
		config := writeConfig(t, t.TempDir(), backupConfig(into))
		dir := ownedData(t, config)

		code, _, _ := runVerb("backup", "--data-dir", dir, "--config", config, "extra")
		assert.Equal(t, 1, code)
		assert.NoDirExists(t, into)
	})

	t.Run("no backup directory", func(t *testing.T) {
		t.Parallel()

		config := writeConfig(t, t.TempDir(), validConfig)
		dir := ownedData(t, config)

		code, stdout, stderr := runVerb("backup", "--data-dir", dir, "--config", config)
		assert.Equal(t, 1, code)
		assert.Empty(t, stdout)
		assert.Equal(t, 1, strings.Count(stderr, "\n"), "one line")
	})

	t.Run("no database", func(t *testing.T) {
		t.Parallel()

		into := filepath.Join(t.TempDir(), "backup")
		dir := filepath.Join(t.TempDir(), "data")
		config := writeConfig(t, t.TempDir(), backupConfig(into))

		code, _, _ := runVerb("backup", "--data-dir", dir, "--config", config)
		assert.Equal(t, 1, code)
		assert.NoDirExists(t, dir, "no empty database is made")
		assert.NoDirExists(t, into)
	})
}

// restore copies the snapshot at path in as the database of a new data
// directory, and returns the directory.
func restore(t *testing.T, path string) string {
	t.Helper()

	snapshot, err := os.ReadFile(path)
	require.NoError(t, err)

	dir := filepath.Join(t.TempDir(), "restored")
	require.NoError(t, os.Mkdir(dir, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "recallsynapsis.db"), snapshot, 0o600)) //nolint:gosec // a file in the test's own temporary directory

	return dir
}

func dirNames(t *testing.T, dir string) []string {
	t.Helper()

	entries, err := os.ReadDir(dir)
	require.NoError(t, err)

	list := make([]string, len(entries))
	for i, e := range entries {
		list[i] = e.Name()
	}

	return list
}

// Not parallel: it signals the test process, and serve replaces the default logger.
func TestARestoredSnapshotServes(t *testing.T) {
	keepDefaultLogger(t)

	into := filepath.Join(t.TempDir(), "backup")
	config := writeConfig(t, t.TempDir(), backupConfig(into))
	dir := ownedData(t, config)

	code, stdout, stderr := runVerb("backup", "--data-dir", dir, "--config", config)
	require.Equal(t, 0, code, stderr)

	restored := restore(t, strings.TrimSuffix(stdout, "\n"))

	var logged logBuffer

	done := make(chan int, 1)

	go func() {
		done <- run([]string{"serve", "--data-dir", restored, "--config", config}, strings.NewReader(""), io.Discard, &logged)
	}()

	address := regexp.MustCompile(`address=(\S+)`)

	require.Eventually(t, func() bool { return address.MatchString(logged.String()) }, 5*time.Second, 10*time.Millisecond)

	url := "http://" + address.FindStringSubmatch(logged.String())[1]

	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, url+"/api/auth/login", strings.NewReader(`{"name":"ada","password":"`+rightPhrase+`"}`))
	require.NoError(t, err)

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	require.NotEmpty(t, resp.Cookies())

	req, err = http.NewRequestWithContext(t.Context(), http.MethodGet, url+"/api/areas", nil)
	require.NoError(t, err)
	req.AddCookie(resp.Cookies()[0])

	resp, err = http.DefaultClient.Do(req)
	require.NoError(t, err)

	var got struct {
		Areas []module.Area `json:"areas"`
	}

	require.NoError(t, json.NewDecoder(resp.Body).Decode(&got))
	require.NoError(t, resp.Body.Close())
	require.Len(t, got.Areas, 2)
	assert.Equal(t, "Garden", got.Areas[1].Name)

	// The nightly backup waits until three at night, and stops with the server.
	signalled := time.Now()

	require.NoError(t, syscall.Kill(syscall.Getpid(), syscall.SIGTERM))
	assert.Equal(t, 0, within(t, done, "serve did not stop on the signal"), logged.String())
	assert.Less(t, time.Since(signalled), shutdownGrace)
}
