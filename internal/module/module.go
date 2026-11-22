// Package module is what the core and its feature modules share: the
// contract a feature module fulfils and what it is built with, the caller of
// a request, the access a route or a tool asks of it, and the refusal, the
// reply and the strict decoding of a body that every route uses.
package module

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/mhersson/recallsynapsis/internal/store"
)

// Module is what a feature module offers. Everything in it but its
// migrations is gated by the enabled key of its [modules.NAME] table.
type Module struct {
	Name       string            // the key under [modules], and the owner of its migrations
	Settings   any               // nil, or a pointer to its settings with their defaults; its table is decoded into them once it is built
	Migrations []store.Migration // its own tables, applied whether it is on or not; their Owner is its Name
	Routes     []Route
	Tools      []Tool
	Jobs       []Job

	// Optional. Nil when the module has nothing to offer. Each acts for the caller its context carries.
	Search  func(context.Context, Query) ([]Hit, error) // its hits, ranked
	Resolve func(context.Context, int64) (Item, error)  // one item as a link shows it; store.ErrNotFound when there is none
	LinksTo func(context.Context, Ref) ([]Ref, error)   // its items that link to a ref
}

// Route is one REST route of a module. It is served behind the same access
// check as a core route, and its error is answered as a core route's is. A
// module route is never open, so one that leaves Access out stops the start.
type Route struct {
	Pattern string // an http.ServeMux pattern under /api/NAME/, such as "GET /api/notes/{id}"
	Access  Access
	Serve   func(http.ResponseWriter, *http.Request) error
}

// Job is work a module does on a tick while the server runs. Its context
// ends when the server stops, and its error is logged.
type Job struct {
	Name  string
	Every time.Duration
	Run   func(context.Context) error
}

// Query is what a search asks: its text, and the id of the area it is
// limited to, or 0 for every area.
type Query struct {
	Text   string
	AreaID int64
}

// Hit is one item a search found.
type Hit struct {
	Ref       Ref       `json:"ref"`
	Kind      string    `json:"kind"`
	Title     string    `json:"title"`
	Snippet   string    `json:"snippet"`
	AreaID    int64     `json:"-"`    // set by the module: the id of the item's area
	Area      string    `json:"area"` // set by the core: that area's slug
	State     string    `json:"state"`
	ChangedAt time.Time `json:"changed_at"` // in UTC
}

// Ref names an item of a module as module:id, such as notes:42. It is a
// string, so a tool's schema gives it as one.
type Ref string

// NewRef is the reference to the item id of the module name.
func NewRef(name string, id int64) Ref {
	return Ref(name + ":" + strconv.FormatInt(id, 10))
}

// Split gives the module and the id r names; false when r is not module:id.
func (r Ref) Split() (string, int64, bool) {
	name, id, _ := strings.Cut(string(r), ":")
	n, err := strconv.ParseInt(id, 10, 64)

	return name, n, name != "" && err == nil
}

// Item is what a link shows of the item it names: its kind and state as a
// hit of it gives them, and its title. The core sets Ref and Available; the
// module that has the item gives the rest.
type Item struct {
	Ref       Ref    `json:"ref"`
	Kind      string `json:"kind"`
	State     string `json:"state"`
	Title     string `json:"title"`
	Available bool   `json:"available"` // false when no enabled module has the item
}

// Change is what a module changed, as an open page hears of it.
type Change struct {
	Module string `json:"module"`
	Kind   string `json:"kind"`
	ID     int64  `json:"id"`
	Area   int64  `json:"area"`
}

// Area is a user's personal area or one of their projects.
type Area struct {
	ID          int64  `json:"id"`
	Kind        string `json:"kind"` // "personal" or "project"
	Name        string `json:"name"`
	Slug        string `json:"slug"`
	Description string `json:"description"`
	Archived    bool   `json:"archived"`
}

// Deps is what a module is built with. Resolve and LinksTo reach only the
// enabled modules and act for the caller their context carries.
type Deps struct {
	Store    *store.Store
	FindArea func(ctx context.Context, userID int64, ref string) (Area, error) // the area whose slug or name is ref, in any case
	Areas    func(ctx context.Context, userID int64) ([]Area, error)           // every area of the user, the archived ones too, the personal area first
	Now      func() time.Time
	// Dir is the data directory. A file under its files/ appears whole, by a
	// rename, and never changes, since a backup never copies one it has again.
	Dir string

	Resolve func(context.Context, Ref) (Item, error)          // the item ref names, unavailable rather than an error when no enabled module has it
	LinksTo func(context.Context, Ref) ([]Ref, error)         // the items of every enabled module that link to ref
	Notify  func(ctx context.Context, about Ref, text string) // tells the caller text about an item; no module delivers it, so it does nothing
	// Changed tells the open pages of the caller its context carries what
	// changed, once the change is stored; with no caller it tells no page.
	Changed func(context.Context, Change)
}

// Caller is who an authorised request acts for.
type Caller struct {
	UserID int64
	Source string // "web" for the session cookie, else the token's name
	Write  bool   // whether the request may change something
}

type callerKey struct{}

// WithCaller returns ctx carrying c.
func WithCaller(ctx context.Context, c Caller) context.Context {
	return context.WithValue(ctx, callerKey{}, c)
}

// CallerFrom returns the caller ctx carries; false when it carries none.
func CallerFrom(ctx context.Context) (Caller, bool) {
	c, ok := ctx.Value(callerKey{}).(Caller)

	return c, ok
}
