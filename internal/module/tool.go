package module

import (
	"context"
	"fmt"
	"unicode/utf8"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/mhersson/recallsynapsis/internal/store"
)

// Tool is one MCP tool of a module. NewTool makes one. Only an API token
// reaches a tool, so a module tool asks for read or write, and one that asks
// for neither stops the start.
type Tool struct {
	Name        string
	Description string
	Access      Access
	asked       bool
	add         func(*mcp.Server, *mcp.Tool, Guard)
}

// Guard runs one call of a tool that asks for access: it may refuse the
// call, give run the context to run in, and change the error run returns.
type Guard func(ctx context.Context, req *mcp.CallToolRequest, access Access, run func(context.Context) error) error

// NewTool returns the tool that do serves, whose input and output schemas
// the SDK infers from In and Out, so a module never touches the SDK.
func NewTool[In, Out any](name, description string, access Access, do func(context.Context, In) (Out, error)) Tool {
	return Tool{Name: name, Description: description, Access: access, add: func(srv *mcp.Server, tool *mcp.Tool, guard Guard) {
		mcp.AddTool(srv, tool,
			func(ctx context.Context, req *mcp.CallToolRequest, in In) (*mcp.CallToolResult, Out, error) {
				var out Out

				err := guard(ctx, req, access, func(ctx context.Context) (err error) {
					out, err = do(ctx, in)

					return err
				})

				return nil, out, err
			})
	}}
}

// Asked returns t as a tool Claude Code asks its user about before every call.
func (t Tool) Asked() Tool {
	t.asked = true

	return t
}

// AddTo adds t to srv, with each of its calls run by guard.
func (t Tool) AddTo(srv *mcp.Server, guard Guard) {
	tool := &mcp.Tool{Name: t.Name, Description: t.Description}
	if t.asked {
		// Claude Code prompts for every call of a tool that carries it, in auto
		// mode too, and refuses the call where nobody is there to answer.
		tool.Meta = mcp.Meta{"anthropic/requiresUserInteraction": true}
	}

	t.add(srv, tool, guard)
}

// Capped refuses a text given to a tool as field when it is longer than
// limit, which counts characters, not bytes. A nil text was not given.
func Capped(field string, text *string, limit int) error {
	if text == nil {
		return nil
	}

	if count := utf8.RuneCountInString(*text); count > limit {
		return fmt.Errorf("%s is %d characters, over the cap of %d on a text a tool keeps: %w", field, count, limit, store.ErrInvalid)
	}

	return nil
}
