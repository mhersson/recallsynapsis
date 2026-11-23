package push

import (
	"net/http"

	"github.com/mhersson/recallsynapsis/internal/module"
)

// WithClient builds the module as New does, posting with client.
func WithClient(client *http.Client) func(module.Deps) module.Module {
	return func(d module.Deps) module.Module { return build(d, client) }
}
