package module

import (
	"context"
	"fmt"
	"slices"

	"github.com/mhersson/recallsynapsis/internal/store"
)

// maxLinks is the most links an item keeps, since it is read with a query for each.
const maxLinks = 100

// CleanLinks gives each of links once, in the order given and written as
// NewRef writes it, so a link to an item reads the same however it was
// given. A link that is not module:id is refused, and so are more than
// maxLinks.
func CleanLinks(links []Ref) ([]Ref, error) {
	if len(links) > maxLinks {
		return nil, fmt.Errorf("an item has at most %d links: %w", maxLinks, store.ErrInvalid)
	}

	out := make([]Ref, 0, len(links))

	for _, l := range links {
		mod, id, ok := l.Split()
		if !ok {
			return nil, fmt.Errorf("the link %q is not a reference, module:id: %w", l, store.ErrInvalid)
		}

		if r := NewRef(mod, id); !slices.Contains(out, r) {
			out = append(out, r)
		}
	}

	return out, nil
}

// Items gives each of refs as Resolve gives it.
func (d Deps) Items(ctx context.Context, refs []Ref) ([]Item, error) {
	out := make([]Item, 0, len(refs))

	for _, r := range refs {
		it, err := d.Resolve(ctx, r)
		if err != nil {
			return nil, err
		}

		out = append(out, it)
	}

	return out, nil
}
