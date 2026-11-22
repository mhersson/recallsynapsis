package module

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Tool is one MCP tool of a module. NewTool makes one.
type Tool struct {
	Name        string
	Description string
	Access      Access
	add         func(*mcp.Server, Guard)
}

// Guard runs one call of a tool that asks for access: it may refuse the
// call, give run the context to run in, and change the error run returns.
type Guard func(ctx context.Context, req *mcp.CallToolRequest, access Access, run func(context.Context) error) error

// NewTool returns the tool that do serves, whose input and output schemas
// the SDK infers from In and Out, so a module never touches the SDK.
func NewTool[In, Out any](name, description string, access Access, do func(context.Context, In) (Out, error)) Tool {
	return Tool{Name: name, Description: description, Access: access, add: func(srv *mcp.Server, guard Guard) {
		mcp.AddTool(srv, &mcp.Tool{Name: name, Description: description},
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

// AddTo adds t to srv, with each of its calls run by guard.
func (t Tool) AddTo(srv *mcp.Server, guard Guard) { t.add(srv, guard) }
