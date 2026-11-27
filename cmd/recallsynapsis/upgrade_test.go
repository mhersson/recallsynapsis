package main

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mhersson/recallsynapsis/internal/auth"
	"github.com/mhersson/recallsynapsis/internal/core"
	"github.com/mhersson/recallsynapsis/internal/module"
	"github.com/mhersson/recallsynapsis/internal/store"
)

func TestADatabaseFromBeforeTheBriefOpensWithEachDescriptionAsItsBrief(t *testing.T) {
	t.Parallel()

	dir := filepath.Join(t.TempDir(), "data")
	require.NoError(t, os.Mkdir(dir, 0o700))

	// The schema as the core's first migration and every module's leave it.
	st, err := store.Open(t.Context(), filepath.Join(dir, "recallsynapsis.db"), slices.Concat(auth.Migrations, core.Migrations[:1]))
	require.NoError(t, err)

	for _, build := range modules {
		m := build(module.Deps{})

		ms := slices.Clone(m.Migrations)
		for i := range ms {
			ms[i].Owner = m.Name
		}

		require.NoError(t, st.Migrate(t.Context(), ms))
	}

	for _, q := range []string{
		`INSERT INTO users (id, name, created_at) VALUES (1, 'ada', 0)`,
		`INSERT INTO areas (id, user_id, kind, name, name_fold, slug, description, archived) VALUES
			(1, 1, 'personal', 'Personal', 'personal', 'personal', '', 0),
			(2, 1, 'project', 'Hytta', 'hytta', 'hytta', 'Vedlikehold.' || char(10) || char(10) || 'Annekset og veden.', 0),
			(3, 1, 'project', 'Shed', 'shed', 'shed', '', 0),
			(4, 1, 'project', 'Old', 'old', 'old', 'Gone.', 1)`,
		`INSERT INTO notes (id, user_id, area_id, kind, title, body, inbox, revision, source, created_at, changed_at)
			VALUES (1, 1, 2, 'note', 'Takrennene', '', 0, 1, 'web', 1, 1)`,
		`INSERT INTO tasks (id, user_id, area_id, title, notes, planned_from, planned_to, planned_time, deadline, list, source, created_at, changed_at)
			VALUES (1, 1, 3, 'Oil the hinges', '', '', '', '', '', '', 'web', 1, 1)`,
	} {
		_, err := st.ExecContext(t.Context(), q)
		require.NoError(t, err, q)
	}

	require.NoError(t, st.Close())

	before := time.Now().Truncate(time.Millisecond)

	d, err := openData(t.Context(), dir, writeConfig(t, t.TempDir(), validConfig), nil)
	require.NoError(t, err)

	t.Cleanup(func() { _ = d.store.Close() })

	areas, err := d.areas.Areas(t.Context(), 1, true)
	require.NoError(t, err)
	require.Len(t, areas, 4)

	for _, a := range areas {
		if a.BriefRevision == 0 {
			continue
		}

		require.NotNil(t, a.BriefChangedAt, a.Name)
		assert.False(t, a.BriefChangedAt.Before(before), "written at the moment the migration ran")
		assert.False(t, a.BriefChangedAt.After(time.Now()))
	}

	at := func(i int) *time.Time { return areas[i].BriefChangedAt }

	assert.Equal(t, []module.Area{
		{ID: 1, Kind: "personal", Name: "Personal", Slug: "personal"},
		{ID: 2, Kind: "project", Name: "Hytta", Slug: "hytta", Brief: "Vedlikehold.\n\nAnnekset og veden.", BriefRevision: 1, BriefChangedAt: at(1), BriefChangedBy: "web"},
		{ID: 4, Kind: "project", Name: "Old", Slug: "old", Brief: "Gone.", BriefRevision: 1, BriefChangedAt: at(2), BriefChangedBy: "web", Archived: true},
		{ID: 3, Kind: "project", Name: "Shed", Slug: "shed"},
	}, areas, "every name, slug and archived mark as it was, and each description its brief")

	for _, a := range []module.Area{areas[1], areas[2]} {
		h, err := d.areas.History(t.Context(), 1, a.ID)
		require.NoError(t, err)
		assert.Equal(t, []core.Entry{{Revision: 1, At: *a.BriefChangedAt, By: "web"}}, h, "its one revision, as the area tells of it")

		v, err := d.areas.Revision(t.Context(), 1, a.ID, 1)
		require.NoError(t, err)
		assert.Equal(t, a.Brief, v.Brief)
	}

	for _, id := range []int64{1, 3} {
		h, err := d.areas.History(t.Context(), 1, id)
		require.NoError(t, err)
		assert.Empty(t, h, "an area with no description has no history")
	}

	// The query of the binary before the brief, which an image rolled back runs.
	var described int
	require.NoError(t, d.store.QueryRowContext(t.Context(),
		`SELECT count(*) FROM (SELECT id, kind, name, slug, description, archived FROM areas) WHERE description <> ''`).Scan(&described))
	assert.Equal(t, 2, described, "an image rolled back still reads the areas and their briefs")

	var noteArea, taskArea int64
	require.NoError(t, d.store.QueryRowContext(t.Context(), `SELECT area_id FROM notes WHERE id = 1`).Scan(&noteArea))
	require.NoError(t, d.store.QueryRowContext(t.Context(), `SELECT area_id FROM tasks WHERE id = 1`).Scan(&taskArea))
	assert.Equal(t, []int64{2, 3}, []int64{noteArea, taskArea}, "every note and task is still in its area")
}
