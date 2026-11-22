package auth

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mhersson/recallsynapsis/internal/module"
	"github.com/mhersson/recallsynapsis/internal/store"
)

const (
	day      = 24 * time.Hour
	password = "correct horse battery"
)

// passwordHash is password's hash, made once since each costs an argon2 run.
var passwordHash = sync.OnceValue(func() string {
	h, err := HashPassword(password)
	if err != nil {
		panic(err)
	}

	return h
})

type fixture struct {
	svc   *Service
	store *store.Store
	dir   string
	clock *time.Time
	user  int64
}

// newFixture is a Service over a new database, with the user ada whose password is password.
func newFixture(t *testing.T) *fixture {
	t.Helper()

	dir := t.TempDir()

	s, err := store.Open(t.Context(), filepath.Join(dir, "recallsynapsis.db"), Migrations)
	require.NoError(t, err)

	t.Cleanup(func() { _ = s.Close() })

	clock := time.Unix(1_000_000, 0)
	f := &fixture{svc: New(s, func() time.Time { return clock }), store: s, dir: dir, clock: &clock}

	f.user, err = f.svc.EnsureUser(t.Context(), "ada")
	require.NoError(t, err)
	require.NoError(t, f.svc.SetPasswordHash(t.Context(), f.user, passwordHash()))

	return f
}

func (f *fixture) login(t *testing.T, name, password, remote string) (string, error) {
	t.Helper()

	cookie, expires, err := f.svc.Login(t.Context(), name, password, remote)
	if err == nil {
		assert.Equal(t, f.clock.Add(30*day), expires)
	}

	return cookie, err
}

func (f *fixture) live(t *testing.T, cookie string) bool {
	t.Helper()

	c, _, ok, err := f.svc.Session(t.Context(), cookie)
	require.NoError(t, err)

	if ok {
		assert.Equal(t, module.Caller{UserID: f.user, Source: "web", Write: true}, c)
	}

	return ok
}

func TestLogin(t *testing.T) {
	t.Parallel()

	f := newFixture(t)

	cookie, err := f.login(t, "ada", password, "192.0.2.1")
	require.NoError(t, err)
	assert.True(t, f.live(t, cookie))
	assert.False(t, f.live(t, "not-a-cookie"))

	_, err = f.login(t, "ada", "wrong horse battery", "192.0.2.1")
	require.ErrorIs(t, err, ErrBadLogin)

	_, err = f.login(t, "grace", password, "192.0.2.1")
	require.ErrorIs(t, err, ErrBadLogin)

	_, err = f.login(t, "Ada", password, "192.0.2.1")
	require.ErrorIs(t, err, ErrBadLogin, "the name is matched exactly")

	other, err := f.login(t, "ada", password, "192.0.2.2")
	require.NoError(t, err)

	require.NoError(t, f.svc.Logout(t.Context(), cookie))
	assert.False(t, f.live(t, cookie))
	assert.True(t, f.live(t, other), "ending one session leaves the other")
}

func TestAUserWithNoPasswordCannotSignIn(t *testing.T) {
	t.Parallel()

	f := newFixture(t)

	id, err := f.svc.EnsureUser(t.Context(), "grace")
	require.NoError(t, err)

	_, err = f.store.ExecContext(t.Context(), `UPDATE users SET password_hash = '' WHERE id = ?`, id)
	require.NoError(t, err)

	for _, pw := range []string{"", password} {
		_, err = f.login(t, "grace", pw, "192.0.2.1")
		require.ErrorIs(t, err, ErrBadLogin)
	}
}

// Not parallel: it reads what the whole process allocates.
func TestAWrongNameCostsWhatAWrongPasswordDoes(t *testing.T) {
	f := newFixture(t)

	// Each verify allocates argon2's memory afresh, so the bytes a sign-in
	// allocates count its verifies, and unlike its time no load moves them.
	verify := uint64(argonMemory) * 1024

	for _, name := range []string{"ada", "grace"} {
		var before, after runtime.MemStats

		runtime.ReadMemStats(&before)

		_, err := f.login(t, name, "wrong horse battery", "192.0.2.1")

		runtime.ReadMemStats(&after)
		require.ErrorIs(t, err, ErrBadLogin)

		got := after.TotalAlloc - before.TotalAlloc
		assert.GreaterOrEqual(t, got, verify, "%s is answered without the verify", name)
		assert.Less(t, got, 2*verify, "%s is answered with more than one verify", name)
	}
}

func TestTheThirdFailureStartsTheWait(t *testing.T) {
	t.Parallel()

	f := newFixture(t)

	for range 3 {
		_, err := f.login(t, "ada", "wrong horse battery", "192.0.2.1")
		require.ErrorIs(t, err, ErrBadLogin)
	}

	_, err := f.login(t, "ada", password, "192.0.2.1")
	limited, ok := errors.AsType[*LimitedError](err)
	require.True(t, ok, "the right password is refused while the wait lasts")
	assert.Equal(t, time.Second, limited.RetryAfter)

	_, err = f.login(t, "ada", password, "192.0.2.2")
	require.NoError(t, err, "another address does not wait")

	// A name no user has is counted as ada's is, so a wait tells no name apart.
	for range 3 {
		_, err = f.login(t, "grace", "wrong horse battery", "192.0.2.1")
		require.ErrorIs(t, err, ErrBadLogin, "another name from the address does not wait")
	}

	_, err = f.login(t, "grace", "wrong horse battery", "192.0.2.1")
	_, ok = errors.AsType[*LimitedError](err)
	require.True(t, ok, "a name no user has waits from its third failure too")

	*f.clock = f.clock.Add(limited.RetryAfter)

	_, err = f.login(t, "ada", password, "192.0.2.1")
	require.NoError(t, err)

	for range 2 {
		_, err := f.login(t, "ada", "wrong horse battery", "192.0.2.1")
		require.ErrorIs(t, err, ErrBadLogin, "a success cleared the count")
	}
}

func TestParallelFailuresAreAllCounted(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	errs := make(chan error, 6)

	for range cap(errs) {
		go func() {
			_, _, err := f.svc.Login(t.Context(), "ada", "wrong horse battery", "192.0.2.1")
			errs <- err
		}()
	}

	var bad, limited int

	for range cap(errs) {
		err := <-errs
		if _, ok := errors.AsType[*LimitedError](err); ok {
			limited++
		} else if errors.Is(err, ErrBadLogin) {
			bad++
		}
	}

	assert.Equal(t, 3, bad)
	assert.Equal(t, 3, limited)
}

func TestTheLimiterKeyHasAFixedSize(t *testing.T) {
	t.Parallel()

	f := newFixture(t)

	for _, name := range []string{strings.Repeat("a", 1<<20), "ada"} {
		_, err := f.login(t, name, "wrong horse battery", "192.0.2.1")
		require.ErrorIs(t, err, ErrBadLogin)
	}

	require.Len(t, f.svc.limiter.entries, 2)

	var sizes []int
	for key := range f.svc.limiter.entries {
		sizes = append(sizes, len(key))
	}

	assert.Equal(t, sizes[0], sizes[1], "a key's size does not follow the name's")
}

func TestASessionSlides(t *testing.T) {
	t.Parallel()

	f := newFixture(t)

	cookie, err := f.login(t, "ada", password, "192.0.2.1")
	require.NoError(t, err)

	_, renewed, _, err := f.svc.Session(t.Context(), cookie)
	require.NoError(t, err)
	assert.Zero(t, renewed, "a new session is not written again")

	for range 4 {
		*f.clock = f.clock.Add(20 * day)

		_, renewed, ok, err := f.svc.Session(t.Context(), cookie)
		require.NoError(t, err)
		require.True(t, ok, "a session used every 20 days lives on")
		assert.Equal(t, f.clock.Add(30*day), renewed)
	}

	*f.clock = f.clock.Add(30 * day)
	assert.False(t, f.live(t, cookie))

	var n int
	require.NoError(t, f.store.QueryRowContext(t.Context(), `SELECT count(*) FROM sessions`).Scan(&n))
	assert.Equal(t, 1, n)

	_, err = f.login(t, "ada", password, "192.0.2.1")
	require.NoError(t, err)

	require.NoError(t, f.store.QueryRowContext(t.Context(), `SELECT count(*) FROM sessions`).Scan(&n))
	assert.Equal(t, 1, n, "the next sign-in removes the expired session")
}

func TestSetPasswordHash(t *testing.T) {
	t.Parallel()

	f := newFixture(t)

	cookie, err := f.login(t, "ada", password, "192.0.2.1")
	require.NoError(t, err)

	require.NoError(t, f.svc.SetPasswordHash(t.Context(), f.user, passwordHash()))
	assert.True(t, f.live(t, cookie), "the same hash changes nothing")

	// The parameters a sign-in would verify with, other than this binary's own.
	weaker := strings.Replace(passwordHash(), "m=65536,t=3,p=4", "m=4096,t=3,p=1", 1)

	for _, bad := range []string{"", password, "$argon2id$v=19$m=65536,t=3,p=4$c2FsdA", weaker} {
		err := f.svc.SetPasswordHash(t.Context(), f.user, bad)
		require.ErrorIs(t, err, ErrInvalidHash)

		if bad != "" {
			assert.NotContains(t, err.Error(), bad)
		}
	}

	assert.True(t, f.live(t, cookie), "a refused hash changes nothing")

	next, err := HashPassword("another long password")
	require.NoError(t, err)
	require.NoError(t, f.svc.SetPasswordHash(t.Context(), f.user, next))
	assert.False(t, f.live(t, cookie), "a new hash ends every session")

	_, err = f.login(t, "ada", password, "192.0.2.1")
	require.ErrorIs(t, err, ErrBadLogin)

	_, err = f.login(t, "ada", "another long password", "192.0.2.1")
	require.NoError(t, err)
}

func TestASignInThatRacesAPasswordChangeMakesNoSession(t *testing.T) {
	t.Parallel()

	f := newFixture(t)

	id, hash, err := f.svc.check(t.Context(), "ada", password, "key")
	require.NoError(t, err)

	next, err := HashPassword("another long password")
	require.NoError(t, err)
	require.NoError(t, f.svc.SetPasswordHash(t.Context(), f.user, next))

	_, _, err = f.svc.startSession(t.Context(), id, hash)
	require.ErrorIs(t, err, ErrBadLogin)

	var n int
	require.NoError(t, f.store.QueryRowContext(t.Context(), `SELECT count(*) FROM sessions`).Scan(&n))
	assert.Zero(t, n)
}

func TestEnsureUser(t *testing.T) {
	t.Parallel()

	f := newFixture(t)

	again, err := f.svc.EnsureUser(t.Context(), "ada")
	require.NoError(t, err)
	assert.Equal(t, f.user, again)

	cookie, err := f.login(t, "ada", password, "192.0.2.1")
	require.NoError(t, err)

	_, text, err := f.svc.CreateToken(t.Context(), f.user, "phone", "read")
	require.NoError(t, err)

	renamed, err := f.svc.EnsureUser(t.Context(), "grace")
	require.NoError(t, err)
	assert.Equal(t, f.user, renamed, "the one user is renamed")

	name, err := f.svc.UserName(t.Context(), f.user)
	require.NoError(t, err)
	assert.Equal(t, "grace", name)

	assert.True(t, f.live(t, cookie), "a rename keeps the sessions")

	_, ok, err := f.svc.Bearer(t.Context(), text)
	require.NoError(t, err)
	assert.True(t, ok, "a rename keeps the tokens")

	_, err = f.login(t, "ada", password, "192.0.2.1")
	require.ErrorIs(t, err, ErrBadLogin)

	_, err = f.login(t, "grace", password, "192.0.2.1")
	require.NoError(t, err)
}

func TestEnsureUserMakesTheFirst(t *testing.T) {
	t.Parallel()

	s, err := store.Open(t.Context(), filepath.Join(t.TempDir(), "recallsynapsis.db"), Migrations)
	require.NoError(t, err)

	t.Cleanup(func() { _ = s.Close() })

	svc := New(s, time.Now)

	id, err := svc.EnsureUser(t.Context(), "ada")
	require.NoError(t, err)

	name, err := svc.UserName(t.Context(), id)
	require.NoError(t, err)
	assert.Equal(t, "ada", name)

	has, err := svc.HasPassword(t.Context(), id)
	require.NoError(t, err)
	assert.False(t, has)

	_, _, err = svc.Login(t.Context(), "ada", "", "192.0.2.1")
	require.ErrorIs(t, err, ErrBadLogin, "the user has no password yet")

	require.NoError(t, svc.SetPasswordHash(t.Context(), id, passwordHash()))

	has, err = svc.HasPassword(t.Context(), id)
	require.NoError(t, err)
	assert.True(t, has)
}

func TestTokens(t *testing.T) {
	t.Parallel()

	f := newFixture(t)

	tok, text, err := f.svc.CreateToken(t.Context(), f.user, "phone", "read")
	require.NoError(t, err)
	assert.Len(t, text, len("recall_")+43)
	assert.True(t, strings.HasPrefix(text, "recall_"))
	assert.Equal(t, Token{ID: tok.ID, Name: "phone", Access: "read", CreatedAt: f.clock.UTC()}, tok)

	_, _, err = f.svc.CreateToken(t.Context(), f.user, "phone", "write")
	require.ErrorIs(t, err, store.ErrConflict)

	grace, err := f.store.ExecContext(t.Context(), `INSERT INTO users (name, created_at) VALUES ('grace', 0)`)
	require.NoError(t, err)

	graceID, err := grace.LastInsertId()
	require.NoError(t, err)

	_, _, err = f.svc.CreateToken(t.Context(), graceID, "phone", "write")
	require.NoError(t, err, "a name is unique per user")

	_, laptop, err := f.svc.CreateToken(t.Context(), f.user, "laptop", "write")
	require.NoError(t, err)

	*f.clock = f.clock.Add(time.Hour)

	c, ok, err := f.svc.Bearer(t.Context(), text)
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, module.Caller{UserID: f.user, Source: "phone", Write: false}, c)

	c, ok, err = f.svc.Bearer(t.Context(), laptop)
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, module.Caller{UserID: f.user, Source: "laptop", Write: true}, c)

	cookie, err := f.login(t, "ada", password, "192.0.2.1")
	require.NoError(t, err)

	for _, other := range []string{text[:len(text)-1], "", cookie, strings.TrimPrefix(text, "recall_")} {
		_, ok, err := f.svc.Bearer(t.Context(), other)
		require.NoError(t, err)
		assert.False(t, ok)
	}

	list, err := f.svc.Tokens(t.Context(), f.user)
	require.NoError(t, err)
	require.Len(t, list, 2)
	assert.Equal(t, []string{"laptop", "phone"}, []string{list[0].Name, list[1].Name})
	require.NotNil(t, list[1].LastUsedAt)
	assert.Equal(t, f.clock.UTC(), *list[1].LastUsedAt)

	require.ErrorIs(t, f.svc.DeleteToken(t.Context(), graceID, tok.ID), store.ErrNotFound, "another user's token is not theirs to revoke")
	require.NoError(t, f.svc.DeleteToken(t.Context(), f.user, tok.ID))
	require.ErrorIs(t, f.svc.DeleteToken(t.Context(), f.user, tok.ID), store.ErrNotFound)

	_, ok, err = f.svc.Bearer(t.Context(), text)
	require.NoError(t, err)
	assert.False(t, ok, "a revoked token stands for nobody")

	none, err := f.svc.Tokens(t.Context(), 999)
	require.NoError(t, err)
	assert.NotNil(t, none)
	assert.Empty(t, none)
}

func TestOnlyHashesAreStored(t *testing.T) {
	t.Parallel()

	f := newFixture(t)

	_, text, err := f.svc.CreateToken(t.Context(), f.user, "phone", "write")
	require.NoError(t, err)

	cookie, err := f.login(t, "ada", password, "192.0.2.1")
	require.NoError(t, err)

	require.NoError(t, f.store.Close())

	entries, err := os.ReadDir(f.dir)
	require.NoError(t, err)

	var all []byte

	for _, e := range entries {
		b, err := os.ReadFile(filepath.Join(f.dir, e.Name()))
		require.NoError(t, err)

		all = append(all, b...)
	}

	assert.Contains(t, string(all), "phone", "positive control: a stored value is found")
	assert.Contains(t, string(all), hashToken(text), "positive control: the hash is stored")
	assert.NotContains(t, string(all), text)
	assert.NotContains(t, string(all), strings.TrimPrefix(text, "recall_"))
	assert.NotContains(t, string(all), cookie)
	assert.NotContains(t, string(all), password)
}
