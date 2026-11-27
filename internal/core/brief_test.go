package core

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mhersson/recallsynapsis/internal/module"
	"github.com/mhersson/recallsynapsis/internal/store"
)

// history is the history of the brief of the user's area with the id.
func (f *fixture) history(t *testing.T, user, id int64) []Entry {
	t.Helper()

	h, err := f.svc.History(t.Context(), user, id)
	require.NoError(t, err)

	return h
}

// brief changes the brief of the user's area with the id from the revision from, as by.
func (f *fixture) brief(t *testing.T, by module.Caller, id int64, text string, from int64) module.Area {
	t.Helper()

	a, err := f.svc.ChangeArea(t.Context(), by, id, Change{Brief: &text, BriefRevision: &from})
	require.NoError(t, err)

	return a
}

func TestAProjectMadeWithABriefIsAtItsFirstRevision(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	ada := f.user(t, "ada")
	laptop := module.Caller{UserID: ada, Source: "laptop", Write: true}

	garden, err := f.svc.CreateProject(t.Context(), laptop, "Garden", "The beds.")
	require.NoError(t, err)
	require.NotNil(t, garden.BriefChangedAt)
	assert.Equal(t, module.Area{
		ID: garden.ID, Kind: project, Name: "Garden", Slug: "garden",
		Brief: "The beds.", BriefRevision: 1, BriefChangedAt: garden.BriefChangedAt, BriefChangedBy: "laptop",
	}, garden)
	assert.Equal(t, []Entry{{Revision: 1, At: *garden.BriefChangedAt, By: "laptop"}}, f.history(t, ada, garden.ID))

	shed := f.project(t, ada, "Shed")
	assert.Equal(t, module.Area{ID: shed.ID, Kind: project, Name: "Shed", Slug: "shed"}, shed, "one made with none is at 0")
	assert.Equal(t, []Entry{}, f.history(t, ada, shed.ID))
}

func TestAChangeOfTheBriefNamesTheRevisionItWasMadeFrom(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	ada := f.user(t, "ada")
	garden := f.project(t, ada, "Garden")

	first := f.brief(t, web(ada), garden.ID, "The beds.", 0)
	assert.Equal(t, int64(1), first.BriefRevision)

	laptop := module.Caller{UserID: ada, Source: "laptop", Write: true}
	second := f.brief(t, laptop, garden.ID, "The beds and the greenhouse.", 1)
	assert.Equal(t, int64(2), second.BriefRevision)
	assert.Equal(t, "laptop", second.BriefChangedBy, "by the caller")

	for _, tt := range []struct {
		name string
		ch   Change
		want error
	}{
		{"from an older revision", Change{Brief: new("Stale."), BriefRevision: new(int64(1))}, store.ErrConflict},
		{"from a revision it never had", Change{Brief: new("Ahead."), BriefRevision: new(int64(3))}, store.ErrConflict},
		{"a rename beside one from an older revision", Change{Name: new("Orchard"), Brief: new("Stale."), BriefRevision: new(int64(1))}, store.ErrConflict},
		{"naming none", Change{Brief: new("Blind.")}, store.ErrInvalid},
		{"a revision with no brief", Change{Name: new("Orchard"), BriefRevision: new(int64(2))}, store.ErrInvalid},
	} {
		_, err := f.svc.ChangeArea(t.Context(), web(ada), garden.ID, tt.ch)
		require.ErrorIs(t, err, tt.want, tt.name)
	}

	got, err := f.svc.Area(t.Context(), ada, garden.ID)
	require.NoError(t, err)
	assert.Equal(t, second, got, "a refused change changes nothing")

	renamed, err := f.svc.ChangeArea(t.Context(), web(ada), garden.ID, Change{Name: new("Orchard")})
	require.NoError(t, err)

	archived, err := f.svc.ChangeArea(t.Context(), web(ada), garden.ID, Change{Archived: new(true)})
	require.NoError(t, err)

	for _, a := range []module.Area{renamed, archived} {
		assert.Equal(t, second.BriefRevision, a.BriefRevision, "a rename or an archiving names no revision")
		assert.Equal(t, second.BriefChangedAt, a.BriefChangedAt)
	}

	assert.Len(t, f.history(t, ada, garden.ID), 2, "and writes none")
}

func TestTheHistoryListsEveryWordingAndARestoreBringsOneBack(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	ada := f.user(t, "ada")
	garden := f.project(t, ada, "Garden")
	laptop := module.Caller{UserID: ada, Source: "laptop", Write: true}

	f.brief(t, web(ada), garden.ID, "One.", 0)
	f.brief(t, laptop, garden.ID, "One.\n\nTwo.", 1)
	third := f.brief(t, web(ada), garden.ID, "", 2)

	h := f.history(t, ada, garden.ID)
	require.Len(t, h, 3)
	assert.Equal(t, []int64{3, 2, 1}, []int64{h[0].Revision, h[1].Revision, h[2].Revision}, "the newest first")
	assert.Equal(t, []string{"web", "laptop", "web"}, []string{h[0].By, h[1].By, h[2].By})
	assert.Equal(t, *third.BriefChangedAt, h[0].At)

	for rev, want := range map[int64]string{1: "One.", 2: "One.\n\nTwo.", 3: ""} {
		v, err := f.svc.Revision(t.Context(), ada, garden.ID, rev)
		require.NoError(t, err)
		assert.Equal(t, Version{h[3-rev], want}, v, "each is read whole")
	}

	restored, err := f.svc.Restore(t.Context(), web(ada), garden.ID, 2)
	require.NoError(t, err)
	assert.Equal(t, "One.\n\nTwo.", restored.Brief)
	assert.Equal(t, int64(4), restored.BriefRevision)

	v, err := f.svc.Revision(t.Context(), ada, garden.ID, 4)
	require.NoError(t, err)
	assert.Equal(t, Version{Entry{Revision: 4, At: *restored.BriefChangedAt, By: "web", RestoredFrom: new(int64(2))}, "One.\n\nTwo."}, v,
		"a restore writes a new revision that names the one it brought back")
	assert.Equal(t, v.Entry, f.history(t, ada, garden.ID)[0])

	_, err = f.svc.Revision(t.Context(), ada, garden.ID, 9)
	require.ErrorIs(t, err, store.ErrNotFound)

	_, err = f.svc.Restore(t.Context(), web(ada), garden.ID, 9)
	require.ErrorIs(t, err, store.ErrNotFound)

	_, err = f.svc.History(t.Context(), ada, 999)
	require.ErrorIs(t, err, store.ErrNotFound)

	got, err := f.svc.Area(t.Context(), ada, garden.ID)
	require.NoError(t, err)
	assert.Equal(t, restored, got, "a refused restore changes nothing")
	assert.Len(t, f.history(t, ada, garden.ID), 4)
}
