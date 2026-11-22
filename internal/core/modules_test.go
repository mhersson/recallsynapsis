package core

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mhersson/recallsynapsis/internal/config"
	"github.com/mhersson/recallsynapsis/internal/module"
	"github.com/mhersson/recallsynapsis/internal/store"
)

// table is a [modules.NAME] table as config reads one.
type table = map[string]any

// stub is a module named name: a table of its own, hits for every query, the
// items of items by id, and what links to a ref by its text in links.
func stub(name string, hits []module.Hit, items map[int64]string, links map[string][]module.Ref) func(module.Deps) module.Module {
	return func(module.Deps) module.Module {
		return module.Module{
			Name:       name,
			Migrations: []store.Migration{{Version: 1, SQL: "CREATE TABLE " + name + "_items (id INTEGER PRIMARY KEY)"}},
			Search: func(context.Context, module.Query) ([]module.Hit, error) {
				return hits, nil
			},
			Resolve: func(_ context.Context, id int64) (module.Item, error) {
				title, ok := items[id]
				if !ok {
					return module.Item{}, fmt.Errorf("item %d: %w", id, store.ErrNotFound)
				}

				return module.Item{Title: title}, nil
			},
			LinksTo: func(_ context.Context, ref module.Ref) ([]module.Ref, error) {
				return links[string(ref)], nil
			},
		}
	}
}

func (f *fixture) load(t *testing.T, list []func(module.Deps) module.Module, tables map[string]table) *Modules {
	t.Helper()

	ms, err := f.svc.LoadModules(t.Context(), list, &config.Config{Modules: tables}, module.Deps{})
	require.NoError(t, err)

	return ms
}

func (f *fixture) tables(t *testing.T) []string {
	t.Helper()

	rows, err := f.store.QueryContext(t.Context(), `SELECT name FROM sqlite_schema WHERE type = 'table' AND name LIKE '%_items' ORDER BY name`)
	require.NoError(t, err)

	defer rows.Close()

	out := []string{}

	for rows.Next() {
		var name string
		require.NoError(t, rows.Scan(&name))
		out = append(out, name)
	}

	require.NoError(t, rows.Err())

	return out
}

func names(ms *Modules) []string {
	out := []string{}
	for _, m := range ms.On() {
		out = append(out, m.Name)
	}

	return out
}

func TestLoadModules(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	ada := f.user(t, "ada")

	type settings struct {
		Size int `toml:"size"`
	}

	var (
		got  module.Deps
		tune = settings{Size: 1}
	)

	tuned := func(d module.Deps) module.Module {
		got = d

		return module.Module{Name: "tuned", Settings: &tune, Migrations: []store.Migration{
			{Owner: "another", Version: 1, SQL: "CREATE TABLE tuned_items (id INTEGER PRIMARY KEY)"},
		}}
	}
	clock := func() time.Time { return time.Unix(7, 0) }

	var heard []module.Change

	ms, err := f.svc.LoadModules(t.Context(),
		[]func(module.Deps) module.Module{stub("off", nil, nil, nil), tuned, stub("plain", nil, map[int64]string{1: "Beds"}, nil), stub("bare", nil, nil, nil)},
		&config.Config{Modules: map[string]table{"off": {"enabled": false}, "tuned": {"enabled": true, "size": int64(9)}, "plain": {"enabled": true}}},
		module.Deps{Now: clock, Dir: "/data", Changed: func(c module.Change) { heard = append(heard, c) }})
	require.NoError(t, err)

	assert.Equal(t, []string{"tuned", "plain"}, names(ms), "the modules on, in the list's order; one with no table is off")
	assert.Equal(t, settings{Size: 9}, tune, "its table is decoded into its settings")
	assert.Equal(t, []string{"bare_items", "off_items", "plain_items", "tuned_items"}, f.tables(t), "every module's migrations run, on or off")

	var owners string
	require.NoError(t, f.store.QueryRowContext(t.Context(),
		`SELECT group_concat(owner, ' ' ORDER BY owner) FROM schema_migrations WHERE owner NOT IN ('auth', 'core')`).Scan(&owners))
	assert.Equal(t, "bare off plain tuned", owners, "a module's migrations are owned by its name")

	assert.Same(t, f.store, got.Store)
	assert.Equal(t, time.Unix(7, 0), got.Now())
	assert.Equal(t, "/data", got.Dir)
	got.Changed(module.Change{Module: "tuned", Kind: "item", ID: 1, Area: 2})
	assert.Equal(t, []module.Change{{Module: "tuned", Kind: "item", ID: 1, Area: 2}}, heard)
	got.Notify(t.Context(), module.NewRef("tuned", 1), "hello")

	personal, err := got.FindArea(t.Context(), ada, "PERSONAL")
	require.NoError(t, err)
	assert.Equal(t, "personal", personal.Slug)

	it, err := got.Resolve(t.Context(), module.NewRef("plain", 1))
	require.NoError(t, err)
	assert.Equal(t, module.Item{Ref: "plain:1", Title: "Beds", Available: true}, it, "Resolve reaches the modules built after the one given it")
}

func TestLoadModulesRefusesATableItCannotUse(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		tables map[string]table
		key    string
	}{
		{"a module the binary lacks", map[string]table{"plain": {"enabled": true}, "ghost": {"enabled": true}}, "modules.ghost"},
		{"an unknown key", map[string]table{"plain": {"enabled": true, "colour": "red"}}, "modules.plain.colour"},
		{"enabled not a boolean", map[string]table{"plain": {"enabled": "yes"}}, "modules.plain.enabled"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f := newFixture(t)

			_, err := f.svc.LoadModules(t.Context(), []func(module.Deps) module.Module{stub("plain", nil, nil, nil)},
				&config.Config{Modules: tt.tables}, module.Deps{})
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.key)
			assert.Empty(t, f.tables(t), "a refused start migrates no module")
		})
	}
}

func TestLoadModulesRefusesAnOpenModuleRoute(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	careless := func(module.Deps) module.Module {
		return module.Module{Name: "careless", Routes: []module.Route{{Pattern: "GET /api/careless/items", Serve: func(http.ResponseWriter, *http.Request) error {
			return nil
		}}}}
	}

	for _, tables := range []map[string]table{{"careless": {"enabled": true}}, nil} {
		_, err := f.svc.LoadModules(t.Context(), []func(module.Deps) module.Module{careless}, &config.Config{Modules: tables}, module.Deps{})
		require.Error(t, err, "on or off")
		assert.Contains(t, err.Error(), "GET /api/careless/items")
	}
}

func TestLoadModulesRefusesAModuleToolNoTokenFits(t *testing.T) {
	t.Parallel()

	f := newFixture(t)

	for _, access := range []module.Access{module.Open, module.Owner} {
		careless := func(module.Deps) module.Module {
			return module.Module{Name: "careless", Tools: []module.Tool{module.NewTool("wipe", "Wipe it all.", access,
				func(context.Context, struct{}) (struct{}, error) { return struct{}{}, nil })}}
		}

		for _, tables := range []map[string]table{{"careless": {"enabled": true}}, nil} {
			_, err := f.svc.LoadModules(t.Context(), []func(module.Deps) module.Module{careless}, &config.Config{Modules: tables}, module.Deps{})
			require.Error(t, err, "on or off")
			assert.Contains(t, err.Error(), "wipe")
		}
	}

	for _, access := range []module.Access{module.Read, module.Write} {
		fine := func(module.Deps) module.Module {
			return module.Module{Name: "fine", Tools: []module.Tool{module.NewTool("look", "Look.", access,
				func(context.Context, struct{}) (struct{}, error) { return struct{}{}, nil })}}
		}

		f.load(t, []func(module.Deps) module.Module{fine}, map[string]table{"fine": {"enabled": true}})
	}
}

func TestSearchGroupsTheHitsOfTheModulesOn(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	ada := f.user(t, "ada")
	garden := f.project(t, ada, "Garden")

	hit := func(name string, id int64) module.Hit {
		return module.Hit{Ref: module.NewRef(name, id), Kind: "item", Title: name, AreaID: garden.ID}
	}

	var asked module.Query

	failing := func(module.Deps) module.Module {
		return module.Module{Name: "failing", Search: func(_ context.Context, q module.Query) ([]module.Hit, error) {
			asked = q

			return nil, errors.New("index broken")
		}}
	}

	list := []func(module.Deps) module.Module{
		stub("beta", []module.Hit{hit("beta", 2), hit("beta", 1)}, nil, nil),
		stub("quiet", nil, nil, nil),
		stub("off", []module.Hit{hit("off", 1)}, nil, nil),
		func(module.Deps) module.Module { return module.Module{Name: "blind"} },
		stub("alpha", []module.Hit{hit("alpha", 1)}, nil, nil),
		failing,
	}
	on := map[string]table{"beta": {"enabled": true}, "quiet": {"enabled": true}, "blind": {"enabled": true}, "alpha": {"enabled": true}}
	ctx := module.WithCaller(t.Context(), module.Caller{UserID: ada})

	groups, err := f.load(t, list, on).Search(ctx, module.Query{Text: "x"})
	require.NoError(t, err)

	want := func(name string, ids ...int64) Group {
		g := Group{Module: name}
		for _, id := range ids {
			h := hit(name, id)
			h.Area = "garden"
			g.Hits = append(g.Hits, h)
		}

		return g
	}
	assert.Equal(t, []Group{want("beta", 2, 1), want("alpha", 1)}, groups, "in the modules' order, each ranked by its module, with the area's slug")

	groups, err = f.load(t, list, map[string]table{"quiet": {"enabled": true}}).Search(ctx, module.Query{Text: "x"})
	require.NoError(t, err)
	assert.NotNil(t, groups)
	assert.Empty(t, groups)

	grace := f.user(t, "grace")
	theirs := f.project(t, grace, "Garden")
	leaky := stub("leaky", []module.Hit{{Ref: "leaky:1", AreaID: theirs.ID}}, nil, nil)

	_, err = f.load(t, []func(module.Deps) module.Module{leaky}, map[string]table{"leaky": {"enabled": true}}).Search(ctx, module.Query{Text: "x"})
	require.Error(t, err, "a hit in another user's area fails the search")

	on["failing"] = table{"enabled": true}
	_, err = f.load(t, list, on).Search(ctx, module.Query{Text: "x", AreaID: garden.ID})
	require.Error(t, err)
	assert.Equal(t, module.Query{Text: "x", AreaID: garden.ID}, asked, "a module is asked the query as given")
}

func TestResolveAndLinksTo(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	target := module.NewRef("alpha", 1)

	broken := func(module.Deps) module.Module {
		return module.Module{Name: "broken", Resolve: func(context.Context, int64) (module.Item, error) {
			return module.Item{}, errors.New("disk on fire")
		}}
	}

	ms := f.load(t, []func(module.Deps) module.Module{
		stub("alpha", nil, map[int64]string{0: "Zero", 1: "Beds"}, map[string][]module.Ref{"beta:3": {module.NewRef("alpha", 2)}}),
		stub("beta", nil, map[int64]string{3: "Seeds"}, map[string][]module.Ref{"alpha:1": {module.NewRef("beta", 3), module.NewRef("beta", 4)}}),
		stub("off", nil, map[int64]string{1: "Hidden"}, map[string][]module.Ref{"alpha:1": {module.NewRef("off", 1)}}),
		broken,
	}, map[string]table{"alpha": {"enabled": true}, "beta": {"enabled": true}, "broken": {"enabled": true}})

	it, err := ms.Resolve(t.Context(), target)
	require.NoError(t, err)
	assert.Equal(t, module.Item{Ref: target, Title: "Beds", Available: true}, it)

	for _, ref := range []module.Ref{"off:1", "ghost:1", "alpha:9", "alpha", "alpha:x"} {
		it, err := ms.Resolve(t.Context(), ref)
		require.NoError(t, err, ref)
		assert.Equal(t, module.Item{Ref: ref}, it, "%s is unavailable", ref)
	}

	_, err = ms.Resolve(t.Context(), module.NewRef("broken", 1))
	require.Error(t, err)

	refs, err := ms.LinksTo(t.Context(), target)
	require.NoError(t, err)
	assert.Equal(t, []module.Ref{module.NewRef("beta", 3), module.NewRef("beta", 4)}, refs, "a module that is off is not asked")

	refs, err = ms.LinksTo(t.Context(), module.NewRef("alpha", 2))
	require.NoError(t, err)
	assert.NotNil(t, refs)
	assert.Empty(t, refs)
}
