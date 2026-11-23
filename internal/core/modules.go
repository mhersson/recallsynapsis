package core

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"

	"github.com/mhersson/recallsynapsis/internal/config"
	"github.com/mhersson/recallsynapsis/internal/module"
	"github.com/mhersson/recallsynapsis/internal/store"
)

// Modules are the feature modules of the binary that are switched on, and
// what the core asks of them on behalf of a caller.
type Modules struct {
	areas *Service
	on    []module.Module
}

// Group is one module's hits.
type Group struct {
	Module string       `json:"module"`
	Hits   []module.Hit `json:"hits"`
}

// LoadModules builds each module of list with d, which it completes with the
// store, the area lookups, Resolve, LinksTo and Notify, decodes the module's
// table of cfg, and applies every module's migrations, on or off, after
// auth's and core's. A table no module of list has is refused, and so are,
// on or off, settings their own check refuses, an open module route and a
// module tool that asks for neither read nor write.
func (c *Service) LoadModules(ctx context.Context, list []func(module.Deps) module.Module, cfg *config.Config, d module.Deps) (*Modules, error) {
	ms := &Modules{areas: c}
	d.Store, d.FindArea = c.store, c.Find
	d.Areas = func(ctx context.Context, userID int64) ([]module.Area, error) { return c.Areas(ctx, userID, true) }
	d.Resolve, d.LinksTo = ms.Resolve, ms.LinksTo
	d.Notify = ms.Notify

	var (
		names      []string
		migrations []store.Migration
	)

	for _, build := range list {
		m := build(d)

		on, err := cfg.Module(m.Name, m.Settings)
		if err != nil {
			return nil, err
		}

		if s, ok := m.Settings.(interface{ Check() error }); ok {
			if err := s.Check(); err != nil {
				return nil, fmt.Errorf("key modules.%s.%w", m.Name, err)
			}
		}

		// Left out, Access is Open, which would serve the route to anyone.
		for _, rt := range m.Routes {
			if rt.Access == module.Open {
				return nil, fmt.Errorf("module %s: the route %s is open; a module route asks for read, write or owner", m.Name, rt.Pattern)
			}
		}

		// Only an API token reaches a tool: open would let a token that may only read
		// call a tool that writes, and owner would let none call it.
		for _, t := range m.Tools {
			if t.Access != module.Read && t.Access != module.Write {
				return nil, fmt.Errorf("module %s: the tool %s asks for neither read nor write, the access an API token has", m.Name, t.Name)
			}
		}

		names = append(names, m.Name)

		for _, mg := range m.Migrations {
			mg.Owner = m.Name
			migrations = append(migrations, mg)
		}

		if on {
			ms.on = append(ms.on, m)
		}
	}

	for _, name := range slices.Sorted(maps.Keys(cfg.Modules)) {
		if !slices.Contains(names, name) {
			return nil, fmt.Errorf("unknown key modules.%s: this binary has no module %s", name, name)
		}
	}

	if err := c.store.Migrate(ctx, migrations); err != nil {
		return nil, err
	}

	return ms, nil
}

// On is the modules switched on, in the order of the list they were built from.
func (ms *Modules) On() []module.Module {
	return ms.on
}

// Search asks each module that searches for q's hits, and gives a group for
// each one with hits, in the modules' order. One module's failure fails all.
func (ms *Modules) Search(ctx context.Context, q module.Query) ([]Group, error) {
	out := []Group{}

	for _, m := range ms.on {
		if m.Search == nil {
			continue
		}

		hits, err := m.Search(ctx, q)
		if err != nil {
			return nil, fmt.Errorf("search %s: %w", m.Name, err)
		}

		if len(hits) > 0 {
			out = append(out, Group{Module: m.Name, Hits: hits})
		}
	}

	if len(out) == 0 {
		return out, nil
	}

	c, _ := module.CallerFrom(ctx)

	areas, err := ms.areas.Areas(ctx, c.UserID, true)
	if err != nil {
		return nil, err
	}

	slugs := make(map[int64]string, len(areas))
	for _, a := range areas {
		slugs[a.ID] = a.Slug
	}

	// A hit outside the caller's areas is another user's, which no module may give.
	for _, g := range out {
		for i, h := range g.Hits {
			slug, ok := slugs[h.AreaID]
			if !ok {
				return nil, fmt.Errorf("search %s: %s is in no area of the caller", g.Module, h.Ref)
			}

			g.Hits[i].Area = slug
		}
	}

	return out, nil
}

// Resolve gives the item ref names. One that no module switched on has, or
// that its module does not find, is unavailable rather than an error.
func (ms *Modules) Resolve(ctx context.Context, ref module.Ref) (module.Item, error) {
	unavailable := module.Item{Ref: ref}
	name, id, ok := ref.Split()

	i := slices.IndexFunc(ms.on, func(m module.Module) bool { return m.Name == name && m.Resolve != nil })
	if !ok || i < 0 {
		return unavailable, nil
	}

	it, err := ms.on[i].Resolve(ctx, id)
	if errors.Is(err, store.ErrNotFound) {
		return unavailable, nil
	}

	if err != nil {
		return unavailable, fmt.Errorf("resolve %s: %w", ref, err)
	}

	it.Ref, it.Available = ref, true

	return it, nil
}

// Notify hands m for the user to each module switched on that delivers.
func (ms *Modules) Notify(ctx context.Context, userID int64, m module.Message) {
	for _, mod := range ms.on {
		if mod.Deliver != nil {
			mod.Deliver(ctx, userID, m)
		}
	}
}

// LinksTo gives the items of every module switched on that link to ref.
func (ms *Modules) LinksTo(ctx context.Context, ref module.Ref) ([]module.Ref, error) {
	out := []module.Ref{}

	for _, m := range ms.on {
		if m.LinksTo == nil {
			continue
		}

		refs, err := m.LinksTo(ctx, ref)
		if err != nil {
			return nil, fmt.Errorf("links to %s in %s: %w", ref, m.Name, err)
		}

		out = append(out, refs...)
	}

	return out, nil
}
