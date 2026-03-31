package middlewarex

import "context"

// Handler processes request and returns response or error.
type Handler[Req, Resp any] func(ctx context.Context, req Req) (Resp, error)

// Middleware wraps a handler with extra behavior.
type Middleware[Req, Resp any] func(Handler[Req, Resp]) Handler[Req, Resp]

// Chain applies middleware in declaration order, where the last middleware is closest to the handler.
func Chain[Req, Resp any](handler Handler[Req, Resp], middleware ...Middleware[Req, Resp]) Handler[Req, Resp] {
	wrapped := handler
	for i := len(middleware) - 1; i >= 0; i-- {
		mw := middleware[i]
		if mw == nil {
			continue
		}
		wrapped = mw(wrapped)
	}

	return wrapped
}

// Identity describes authenticated principal details.
type Identity struct {
	Subject string
	Roles   []string
	Scopes  []string
	Token   string
	Claims  map[string]any
}

// Verifier validates token and returns identity.
type Verifier interface {
	Verify(ctx context.Context, token string) (Identity, error)
}
