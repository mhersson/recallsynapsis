// Package module is what the core and its feature modules share: the caller
// of a request, the access a route asks of it, and the refusal, the reply and
// the strict decoding of a body that every route uses.
package module

import "context"

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
