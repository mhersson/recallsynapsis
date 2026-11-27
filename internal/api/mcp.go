package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"time"

	sdkauth "github.com/modelcontextprotocol/go-sdk/auth"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/mhersson/recallsynapsis/internal/core"
	"github.com/mhersson/recallsynapsis/internal/module"
)

// The SDK writes a verifier's error into its 401 answer, so the text names no token.
var errUnknownToken = fmt.Errorf("the API token is unknown or revoked: %w", sdkauth.ErrInvalidToken)

const (
	// callerKey keys the caller in the info verify gives a token.
	callerKey = "caller"
	// maxSaid caps, in characters, the owner's words update_area keeps.
	maxSaid = 4000
	// instructions are the same with any module on or off. A gateway such as
	// Portal drops them, so the descriptions of get_knowledge and list_areas say the same.
	instructions = "When a discussion of one of the owner's projects begins, read what is kept of it first: " +
		"call get_knowledge with area set to the project's slug or name, " +
		"or list_areas with the same area when get_knowledge is not listed."
)

// toolArea is an area as a tool gives it: a client names an area by its slug
// or its name, so it is given no id. Only an area named alone gives the
// brief's revision and its change, so a list's schema cannot require them.
type toolArea struct {
	Slug           string     `json:"slug"`
	Name           string     `json:"name"`
	Kind           string     `json:"kind"`
	Brief          string     `json:"brief"`
	BriefRevision  *int64     `json:"brief_revision,omitempty"`
	BriefChangedAt *time.Time `json:"brief_changed_at,omitempty"`
	BriefChangedBy string     `json:"brief_changed_by,omitempty"`
}

type areaList struct {
	Areas []toolArea `json:"areas"`
}

type listAreasIn struct {
	Area string `json:"area,omitempty" jsonschema:"the slug or the name of one area, to read it whole"`
}

// blankLine ends a brief's first paragraph: as Markdown has it, a line of
// nothing but spaces and tabs is blank.
var blankLine = regexp.MustCompile(`\n[ \t]*\n`)

type updateAreaIn struct {
	Area          string `json:"area" jsonschema:"the slug or the name of the area, as list_areas gives them"`
	Brief         string `json:"brief" jsonschema:"the whole new brief, in Markdown"`
	BriefRevision int64  `json:"brief_revision" jsonschema:"the revision of the brief the new text was written from, as list_areas gives it"`
	Said          string `json:"said,omitempty" jsonschema:"the owner's own words agreeing to the change"`
}

type searchIn struct {
	Query string `json:"query" jsonschema:"the text to find; part of a word is enough"`
	Area  string `json:"area,omitempty" jsonschema:"the slug or the name of the one area to search, as list_areas gives them; left out, every area is searched"`
}

type searchOut struct {
	Groups []core.Group `json:"groups"`
}

// mcp returns the handler of /mcp: the core's tools and those of the modules
// switched on, each call run by guard, behind the SDK's bearer check, so an
// API token is the only authority there.
func (s *server) mcp() http.Handler {
	// The SDK's defaults advertise listChanged and logging, which makes a current
	// client open a stream that lasts its whole connection. The list never changes.
	srv := mcp.NewServer(&mcp.Implementation{Name: "recallsynapsis", Version: s.Version},
		&mcp.ServerOptions{Instructions: instructions, Capabilities: &mcp.ServerCapabilities{Tools: &mcp.ToolCapabilities{}}})

	tools := []module.Tool{
		module.NewTool("list_areas", "List the owner's areas, the personal area and the projects, "+
			"each with its slug, its name, its kind and the first paragraph of its brief, which says what it is. "+
			"With area, a slug or a name, it gives that one area with its whole brief, in Markdown, and brief_revision, which update_area needs. "+
			"When get_knowledge is not listed, read a project's whole brief here when a discussion of the project begins.",
			module.Read, s.listAreas),
		module.NewTool("search", "Search what is kept, in every area or in one. "+
			"query is the text to find; part of a word is enough, so a stem or one part of a compound word finds the whole word. "+
			"area keeps the search to one project or area, by its slug or name as list_areas gives them. "+
			"Each hit has a ref, such as notes:42, to read it by.",
			module.Read, s.searchTool),
		module.NewTool("update_area", "Replace the brief of one of the owner's areas, and only on the owner's word: "+
			"show them the whole new text in the conversation first, and call this once they have agreed to it. "+
			"A brief is Markdown that says what the project is, why it exists, what it should do and what shapes it; "+
			"single decisions, plans and tasks do not belong in it. "+
			"Its first paragraph says what the project is in a few lines, since lists show that paragraph alone. "+
			"brief replaces the whole text. "+
			"brief_revision is the revision you read with list_areas and the area; "+
			"if the call is refused because the brief has changed since, read it again and draft again. "+
			"said is the owner's own words agreeing to the change, and may be left out.",
			module.Write, s.updateArea).Asked(),
	}

	for _, m := range s.Modules.On() {
		tools = append(tools, m.Tools...)
	}

	names := map[string]bool{}

	for _, t := range tools {
		// The SDK replaces a tool of a name it has, so a module's would hide the core's.
		if names[t.Name] {
			panic("two tools are named " + t.Name)
		}

		names[t.Name] = true
		t.AddTo(srv, s.guard)
	}

	h := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return srv }, &mcp.StreamableHTTPOptions{
		Stateless: true, JSONResponse: true, MaxRequestBodyBytes: module.MaxBody,
		// A page holds no bearer token, which defeats DNS rebinding; the protection would
		// refuse a TLS proxy on this host, which forwards its public Host to 127.0.0.1.
		DisableLocalhostProtection: true,
	})

	// An API token lasts until it is revoked, so it has no expiry to check.
	return sdkauth.RequireBearerToken(s.verify, &sdkauth.RequireBearerTokenOptions{AllowMissingExpiration: true})(h)
}

// verify is the SDK's check of a bearer token: it keeps the token's caller in
// the token's info. An error that is not the caller's reaches the answer as
// the line REST would give it.
func (s *server) verify(ctx context.Context, text string, _ *http.Request) (*sdkauth.TokenInfo, error) {
	c, found, err := s.Auth.Bearer(ctx, text)
	if err != nil {
		_, line := classify(ctx, "/mcp", err)

		return nil, errors.New(line)
	}

	if !found {
		return nil, errUnknownToken
	}

	return &sdkauth.TokenInfo{Extra: map[string]any{callerKey: c}}, nil
}

// guard runs a call for the token's caller once the tool's access admits the
// token, and cuts an error to the one line REST would answer.
func (s *server) guard(ctx context.Context, req *mcp.CallToolRequest, access module.Access, run func(context.Context) error) error {
	// Set by verify: the bearer check runs before the server does.
	c, _ := req.Extra.TokenInfo.Extra[callerKey].(module.Caller)

	var err error

	switch {
	case access == module.Owner:
		err = &module.Refusal{Status: http.StatusForbidden, Text: "no API token can call this tool; it is the web app's alone"}
	case access == module.Write && !c.Write:
		err = &module.Refusal{Status: http.StatusForbidden, Text: "this tool changes something, and the API token may only read"}
	default:
		err = run(module.WithCaller(ctx, c))
	}

	if err == nil {
		return nil
	}

	_, line := classify(ctx, "/mcp "+req.Params.Name, err)

	return errors.New(line)
}

func (s *server) listAreas(ctx context.Context, in listAreasIn) (areaList, error) {
	c, _ := module.CallerFrom(ctx)

	if in.Area != "" {
		a, err := s.Core.Find(ctx, c.UserID, in.Area)

		return areaList{[]toolArea{named(a)}}, err
	}

	list, err := s.Core.Areas(ctx, c.UserID, false)
	if err != nil {
		return areaList{}, err
	}

	out := areaList{Areas: make([]toolArea, len(list))}
	for i, a := range list {
		out.Areas[i] = toolArea{Slug: a.Slug, Name: a.Name, Kind: a.Kind, Brief: blankLine.Split(a.Brief, 2)[0]}
	}

	return out, nil
}

// named is a as a tool gives an area it is given: with the brief whole, its
// revision, when it changed and what changed it.
func named(a module.Area) toolArea {
	return toolArea{a.Slug, a.Name, a.Kind, a.Brief, &a.BriefRevision, a.BriefChangedAt, a.BriefChangedBy}
}

// updateArea changes the brief as PATCH /api/areas/{id} does, and keeps said
// on the revision it writes.
func (s *server) updateArea(ctx context.Context, in updateAreaIn) (toolArea, error) {
	if err := module.Capped("said", &in.Said, maxSaid); err != nil {
		return toolArea{}, err
	}

	c, _ := module.CallerFrom(ctx)

	a, err := s.Core.Find(ctx, c.UserID, in.Area)
	if err != nil {
		return toolArea{}, err
	}

	a, err = s.Core.ChangeArea(ctx, c, a.ID, core.Change{Brief: &in.Brief, BriefRevision: &in.BriefRevision, Said: in.Said})
	if err != nil {
		return toolArea{}, err
	}

	s.areaChanged(ctx, a)

	return named(a), nil
}

func (s *server) searchTool(ctx context.Context, in searchIn) (searchOut, error) {
	if in.Query == "" {
		return searchOut{}, &module.Refusal{Status: http.StatusBadRequest, Text: "query is required: the text to search for"}
	}

	groups, err := s.groups(ctx, in.Query, in.Area)

	return searchOut{groups}, err
}
