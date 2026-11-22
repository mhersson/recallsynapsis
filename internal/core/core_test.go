package core

import (
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mhersson/recallsynapsis/internal/auth"
	"github.com/mhersson/recallsynapsis/internal/module"
	"github.com/mhersson/recallsynapsis/internal/store"
)

type fixture struct {
	svc   *Service
	store *store.Store
}

// newFixture is a Service over a new database with auth's tables.
func newFixture(t *testing.T) *fixture {
	t.Helper()

	s, err := store.Open(t.Context(), filepath.Join(t.TempDir(), "recallsynapsis.db"), slices.Concat(auth.Migrations, Migrations))
	require.NoError(t, err)

	t.Cleanup(func() { _ = s.Close() })

	return &fixture{svc: New(s), store: s}
}

// user makes a user named name with their personal area.
func (f *fixture) user(t *testing.T, name string) int64 {
	t.Helper()

	var id int64
	require.NoError(t, f.store.QueryRowContext(t.Context(), `INSERT INTO users (name, created_at) VALUES (?, 0) RETURNING id`, name).Scan(&id))
	require.NoError(t, f.svc.EnsurePersonal(t.Context(), id))

	return id
}

func (f *fixture) project(t *testing.T, user int64, name string) module.Area {
	t.Helper()

	a, err := f.svc.CreateProject(t.Context(), user, name, "")
	require.NoError(t, err)

	return a
}

func (f *fixture) names(t *testing.T, user int64, archived bool) []string {
	t.Helper()

	list, err := f.svc.Areas(t.Context(), user, archived)
	require.NoError(t, err)

	out := []string{}
	for _, a := range list {
		out = append(out, a.Name)
	}

	return out
}

func TestEachUserHasOnePersonalArea(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	ada := f.user(t, "ada")

	require.NoError(t, f.svc.EnsurePersonal(t.Context(), ada))

	list, err := f.svc.Areas(t.Context(), ada, true)
	require.NoError(t, err)
	require.Len(t, list, 1)
	assert.Equal(t, module.Area{ID: list[0].ID, Kind: personal, Name: "Personal", Slug: "personal"}, list[0])

	_, err = f.store.ExecContext(t.Context(), `INSERT INTO areas (user_id, kind, name, name_fold, slug) VALUES (?, 'personal', 'Other', 'other', 'other')`, ada)
	assert.True(t, store.IsUniqueViolation(err), "the schema holds each user to one")
}

func TestTheListGivesThePersonalAreaFirstThenTheProjectsByNameInAnyCase(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	ada := f.user(t, "ada")

	for _, name := range []string{"beta", "Ærlig", "Alpha", "cavu", "Zebra"} {
		f.project(t, ada, name)
	}

	assert.Equal(t, []string{"Personal", "Alpha", "beta", "cavu", "Zebra", "Ærlig"}, f.names(t, ada, false))
}

func TestAProjectNameIsUniqueInAnyCase(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	ada := f.user(t, "ada")
	hytta := f.project(t, ada, "Hytta")
	f.project(t, ada, "Ærlig")

	for _, name := range []string{"hytta", "HYTTA", " Hytta ", "ærlig", "personal"} {
		_, err := f.svc.CreateProject(t.Context(), ada, name, "")
		require.ErrorIs(t, err, store.ErrConflict, name)
	}

	_, err := f.svc.ChangeArea(t.Context(), ada, hytta.ID, Change{Name: new("ÆRLIG")})
	require.ErrorIs(t, err, store.ErrConflict)

	renamed, err := f.svc.ChangeArea(t.Context(), ada, hytta.ID, Change{Name: new("hytta")})
	require.NoError(t, err, "a project may change the case of its own name")
	assert.Equal(t, "hytta", renamed.Name)
	assert.Equal(t, hytta.Slug, renamed.Slug, "and keeps its slug")

	assert.Equal(t, []string{"Personal", "hytta", "Ærlig"}, f.names(t, ada, true), "the refused writes made nothing")
}

func TestThePersonalAreaKeepsItsNameAndIsNeverArchived(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	ada := f.user(t, "ada")

	p, err := f.svc.Find(t.Context(), ada, "personal")
	require.NoError(t, err)

	for _, ch := range []Change{
		{Name: new("Home")},
		{Name: new("personal")},
		{Archived: new(true)},
		{Name: new("Home"), Description: new("refused")},
		{Archived: new(true), Description: new("refused")},
	} {
		_, err := f.svc.ChangeArea(t.Context(), ada, p.ID, ch)
		require.ErrorIs(t, err, store.ErrInvalid)
	}

	unchanged, err := f.svc.Area(t.Context(), ada, p.ID)
	require.NoError(t, err)
	assert.Equal(t, p, unchanged, "a refused change writes nothing")

	got, err := f.svc.ChangeArea(t.Context(), ada, p.ID, Change{Name: new(" Personal "), Description: new(" mine "), Archived: new(false)})
	require.NoError(t, err, "its own name and staying unarchived change nothing")
	assert.Equal(t, module.Area{ID: p.ID, Kind: personal, Name: "Personal", Slug: "personal", Description: "mine"}, got)

	again, err := f.svc.Area(t.Context(), ada, p.ID)
	require.NoError(t, err)
	assert.Equal(t, got, again)
}

func TestAnArchivedProjectIsLeftOutUnlessAskedForAndCanBeRestored(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	ada := f.user(t, "ada")
	garden := f.project(t, ada, "garden")
	f.project(t, ada, "shed")

	archived, err := f.svc.ChangeArea(t.Context(), ada, garden.ID, Change{Archived: new(true)})
	require.NoError(t, err)
	assert.True(t, archived.Archived)

	assert.Equal(t, []string{"Personal", "shed"}, f.names(t, ada, false))
	assert.Equal(t, []string{"Personal", "garden", "shed"}, f.names(t, ada, true))

	_, err = f.svc.CreateProject(t.Context(), ada, "Garden", "")
	require.ErrorIs(t, err, store.ErrConflict, "an archived project keeps its name")

	restored, err := f.svc.ChangeArea(t.Context(), ada, garden.ID, Change{Archived: new(false)})
	require.NoError(t, err)
	assert.Equal(t, garden, restored)
	assert.Equal(t, []string{"Personal", "garden", "shed"}, f.names(t, ada, false))
}

func TestSlugs(t *testing.T) {
	t.Parallel()

	for name, want := range map[string]string{
		"Recall Synapsis":        "recall-synapsis",
		"  Hytte på fjellet!  ":  "hytte-på-fjellet",
		"--a  --  b--":           "a-b",
		"2027 plan":              "2027-plan",
		"🌱":                      "project",
		"ÆRLIG\u200bsak":         "ærlig-sak",
		"tab\tand newline-ish":   "tab-and-newline-ish",
		"C++ & Go: the rewrite.": "c-go-the-rewrite",
	} {
		assert.Equal(t, want, slug(name), name)
	}
}

func TestTwoNamesThatGiveTheSameSlugGetDifferentSlugs(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	ada := f.user(t, "ada")

	got := []string{}
	for _, name := range []string{"Garden", "garden!", "GARDEN?", "Personal!"} {
		got = append(got, f.project(t, ada, name).Slug)
	}

	assert.Equal(t, []string{"garden", "garden-2", "garden-3", "personal-2"}, got)

	// A rename makes the slug again from the new name, so a slug a rename frees is taken again.
	first, err := f.svc.Find(t.Context(), ada, "garden")
	require.NoError(t, err)

	moved, err := f.svc.ChangeArea(t.Context(), ada, first.ID, Change{Name: new("Allotment")})
	require.NoError(t, err)
	assert.Equal(t, "allotment", moved.Slug)

	second, err := f.svc.Find(t.Context(), ada, "garden-2")
	require.NoError(t, err)

	cased, err := f.svc.ChangeArea(t.Context(), ada, second.ID, Change{Name: new("GARDEN!")})
	require.NoError(t, err)
	assert.Equal(t, "garden-2", cased.Slug, "a change of case alone keeps the slug")

	third, err := f.svc.Find(t.Context(), ada, "garden-3")
	require.NoError(t, err)

	back, err := f.svc.ChangeArea(t.Context(), ada, third.ID, Change{Name: new("Garden")})
	require.NoError(t, err)
	assert.Equal(t, "garden", back.Slug)
}

func TestFind(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	ada := f.user(t, "ada")
	garden := f.project(t, ada, "Hage på taket")
	other := f.project(t, ada, "Hage på taket!")
	// Its name is the slug of another project, which the slug wins.
	f.project(t, ada, "hage-på-taket-2")

	for _, ref := range []string{"hage-på-taket", "HAGE-PÅ-TAKET", "Hage på taket", " hage PÅ taket "} {
		a, err := f.svc.Find(t.Context(), ada, ref)
		require.NoError(t, err, ref)
		assert.Equal(t, garden, a, ref)
	}

	a, err := f.svc.Find(t.Context(), ada, "hage-på-taket-2")
	require.NoError(t, err)
	assert.Equal(t, other, a)

	byID, err := f.svc.Area(t.Context(), ada, garden.ID)
	require.NoError(t, err)
	assert.Equal(t, garden, byID)

	_, err = f.svc.Find(t.Context(), ada, "hage")
	require.ErrorIs(t, err, store.ErrNotFound)

	_, err = f.svc.Area(t.Context(), ada, 999)
	require.ErrorIs(t, err, store.ErrNotFound)
}

func TestAUsersAreasAreInvisibleToAnother(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	ada, grace := f.user(t, "ada"), f.user(t, "grace")
	garden := f.project(t, ada, "garden")

	assert.Equal(t, []string{"Personal"}, f.names(t, grace, true))

	_, err := f.svc.Area(t.Context(), grace, garden.ID)
	require.ErrorIs(t, err, store.ErrNotFound)

	_, err = f.svc.Find(t.Context(), grace, "garden")
	require.ErrorIs(t, err, store.ErrNotFound)

	_, err = f.svc.ChangeArea(t.Context(), grace, garden.ID, Change{Archived: new(true)})
	require.ErrorIs(t, err, store.ErrNotFound)

	theirs := f.project(t, grace, "garden")
	assert.Equal(t, "garden", theirs.Slug, "names and slugs are unique per user")

	got, err := f.svc.Area(t.Context(), ada, garden.ID)
	require.NoError(t, err)
	assert.Equal(t, garden, got, "and nothing of ada's changed")
}

func TestNamesAndDescriptionsAreChecked(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	ada := f.user(t, "ada")

	a, err := f.svc.CreateProject(t.Context(), ada, "  garden  ", "  The beds.\n\tAnd the greenhouse.  ")
	require.NoError(t, err)
	assert.Equal(t, module.Area{ID: a.ID, Kind: project, Name: "garden", Slug: "garden", Description: "The beds.\n\tAnd the greenhouse."}, a)

	_, err = f.svc.CreateProject(t.Context(), ada, strings.Repeat("å", maxName), strings.Repeat("ø", maxDescription))
	require.NoError(t, err, "the limits count characters")

	for _, name := range []string{"", "   ", strings.Repeat("a", maxName+1), "a\nb", "a\x00b"} {
		_, err := f.svc.CreateProject(t.Context(), ada, name, "")
		require.ErrorIs(t, err, store.ErrInvalid, name)

		_, err = f.svc.ChangeArea(t.Context(), ada, a.ID, Change{Name: &name})
		require.ErrorIs(t, err, store.ErrInvalid, name)
	}

	for _, description := range []string{strings.Repeat("a", maxDescription+1), "a\x1b[2Jb", "a\rb", "a\x00b"} {
		_, err = f.svc.CreateProject(t.Context(), ada, "shed", description)
		require.ErrorIs(t, err, store.ErrInvalid, description)

		_, err = f.svc.ChangeArea(t.Context(), ada, a.ID, Change{Description: &description})
		require.ErrorIs(t, err, store.ErrInvalid, description)
	}

	got, err := f.svc.Area(t.Context(), ada, a.ID)
	require.NoError(t, err)
	assert.Equal(t, a, got, "a refused change changes nothing")
}
