package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	sdkauth "github.com/modelcontextprotocol/go-sdk/auth"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/mhersson/recallsynapsis/internal/core"
	"github.com/mhersson/recallsynapsis/internal/module"
)

// The SDK writes a verifier's error into its 401 answer, so the text names no token.
var errUnknownToken = fmt.Errorf("the API token is unknown or revoked: %w", sdkauth.ErrInvalidToken)

// callerKey keys the caller in the info verify gives a token.
const callerKey = "caller"

// toolArea is an area as a tool gives it: a client names an area by its slug
// or its name, so it is given no id.
type toolArea struct {
	Slug  string `json:"slug"`
	Name  string `json:"name"`
	Kind  string `json:"kind"`
	Brief string `json:"brief"`
}

type areaList struct {
	Areas []toolArea `json:"areas"`
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
		&mcp.ServerOptions{Capabilities: &mcp.ServerCapabilities{Tools: &mcp.ToolCapabilities{}}})

	tools := []module.Tool{
		module.NewTool("list_areas", "List the areas things are kept in: the personal area and every project that is not archived, "+
			"each with its slug, its name, its kind (personal or project) and its brief, in Markdown. "+
			"To search one area alone, give its slug or its name as the area of search.",
			module.Read, s.listAreas),
		module.NewTool("search", "Search what is kept, in every area or in one. "+
			"query is the text to find; part of a word is enough, so a stem or one part of a compound word finds the whole word. "+
			"area, when given, keeps the search to one area: its slug or its name, in any case, as list_areas gives them. "+
			"The hits come grouped by the module that keeps them, such as notes, the best first in each group. "+
			"Each hit has its ref (module:id, such as notes:42), its kind, its title, a snippet, the slug of its area, "+
			"its state and when it last changed.",
			module.Read, s.searchTool),
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

func (s *server) listAreas(ctx context.Context, _ struct{}) (areaList, error) {
	c, _ := module.CallerFrom(ctx)

	list, err := s.Core.Areas(ctx, c.UserID, false)
	if err != nil {
		return areaList{}, err
	}

	out := areaList{Areas: make([]toolArea, len(list))}
	for i, a := range list {
		out.Areas[i] = toolArea{a.Slug, a.Name, a.Kind, a.Brief}
	}

	return out, nil
}

func (s *server) searchTool(ctx context.Context, in searchIn) (searchOut, error) {
	if in.Query == "" {
		return searchOut{}, &module.Refusal{Status: http.StatusBadRequest, Text: "query is required: the text to search for"}
	}

	groups, err := s.groups(ctx, in.Query, in.Area)

	return searchOut{groups}, err
}
