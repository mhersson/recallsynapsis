package module_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mhersson/recallsynapsis/internal/module"
)

type echoIn struct {
	Text string `json:"text"`
}

type echoOut struct {
	Text   string     `json:"text"`
	Source string     `json:"source"`
	Hit    module.Hit `json:"hit"`
}

var errTool = errors.New("the tool failed")

// session connects a client to a server that holds tool, each call run by guard.
func session(t *testing.T, tool module.Tool, guard module.Guard) *mcp.ClientSession {
	t.Helper()

	srv := mcp.NewServer(&mcp.Implementation{Name: "test"}, nil)
	tool.AddTo(srv, guard)

	serverEnd, clientEnd := mcp.NewInMemoryTransports()
	ss, err := srv.Connect(t.Context(), serverEnd, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = ss.Close() })

	cs, err := mcp.NewClient(&mcp.Implementation{Name: "client"}, nil).Connect(t.Context(), clientEnd, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = cs.Close() })

	return cs
}

func TestNewTool(t *testing.T) {
	t.Parallel()

	tool := module.NewTool("echo", "Say the text back.", module.Write, func(ctx context.Context, in echoIn) (echoOut, error) {
		if in.Text == "fail" {
			return echoOut{}, fmt.Errorf("echo: %w", errTool)
		}

		c, _ := module.CallerFrom(ctx)

		return echoOut{Text: in.Text, Source: c.Source, Hit: module.Hit{Ref: module.NewRef("notes", 42), Area: "garden"}}, nil
	})
	assert.Equal(t, "echo", tool.Name)
	assert.Equal(t, "Say the text back.", tool.Description)
	assert.Equal(t, module.Write, tool.Access)

	var failed error

	cs := session(t, tool, func(ctx context.Context, req *mcp.CallToolRequest, access module.Access, run func(context.Context) error) error {
		if req.Params.Name != "echo" || access != module.Write {
			return errors.New("another tool")
		}

		if failed = run(module.WithCaller(ctx, module.Caller{Source: "phone"})); failed != nil {
			return errors.New("refused")
		}

		return nil
	})

	list, err := cs.ListTools(t.Context(), nil)
	require.NoError(t, err)
	require.Len(t, list.Tools, 1)
	assert.Equal(t, "echo", list.Tools[0].Name)
	assert.Equal(t, "Say the text back.", list.Tools[0].Description)
	assert.Contains(t, list.Tools[0].InputSchema.(map[string]any)["properties"], "text")

	res, err := cs.CallTool(t.Context(), &mcp.CallToolParams{Name: "echo", Arguments: map[string]any{"text": "hello"}})
	require.NoError(t, err)
	require.False(t, res.IsError, res.Content)
	assert.Equal(t, "hello", res.StructuredContent.(map[string]any)["text"])
	assert.Equal(t, "phone", res.StructuredContent.(map[string]any)["source"], "the guard's context reaches the tool")
	assert.Equal(t, "notes:42", res.StructuredContent.(map[string]any)["hit"].(map[string]any)["ref"], "a ref is a string to a client")

	res, err = cs.CallTool(t.Context(), &mcp.CallToolParams{Name: "echo", Arguments: map[string]any{"text": "fail"}})
	require.NoError(t, err)
	assert.True(t, res.IsError)
	require.ErrorIs(t, failed, errTool, "the guard sees the tool's error")
	require.Len(t, res.Content, 1)
	assert.Equal(t, "refused", res.Content[0].(*mcp.TextContent).Text, "the client gets the guard's error")
}

func TestAGuardThatRefusesRunsNothing(t *testing.T) {
	t.Parallel()

	ran := false
	tool := module.NewTool("echo", "Say the text back.", module.Read, func(_ context.Context, in echoIn) (echoOut, error) {
		ran = true

		return echoOut{Text: in.Text}, nil
	})

	cs := session(t, tool, func(context.Context, *mcp.CallToolRequest, module.Access, func(context.Context) error) error {
		return errors.New("refused")
	})

	res, err := cs.CallTool(t.Context(), &mcp.CallToolParams{Name: "echo", Arguments: map[string]any{"text": "hello"}})
	require.NoError(t, err)
	assert.True(t, res.IsError)
	assert.False(t, ran)
}
