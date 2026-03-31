package httpx

import (
	"context"
	"net/http"

	"github.com/dkoshenkov/packages-go/middlewarex"
)

// Exchange keeps HTTP writer and request together for generic middleware chains.
type Exchange struct {
	Writer  http.ResponseWriter
	Request *http.Request
}

// Handler processes HTTP exchange and returns error for centralized encoding.
type Handler = middlewarex.Handler[Exchange, struct{}]

// Middleware wraps HTTP exchange handler.
type Middleware = middlewarex.Middleware[Exchange, struct{}]

// Chain applies HTTP middleware in declaration order.
func Chain(handler Handler, middleware ...Middleware) Handler {
	return middlewarex.Chain(handler, middleware...)
}

// FromHTTP wraps plain http.Handler as middlewarex handler.
func FromHTTP(next http.Handler) Handler {
	return func(ctx context.Context, exchange Exchange) (struct{}, error) {
		next.ServeHTTP(exchange.Writer, exchange.Request.WithContext(ctx))
		return struct{}{}, nil
	}
}

// FromHTTPFunc wraps error-returning function as middlewarex handler.
func FromHTTPFunc(next func(http.ResponseWriter, *http.Request) error) Handler {
	return func(ctx context.Context, exchange Exchange) (struct{}, error) {
		return struct{}{}, next(exchange.Writer, exchange.Request.WithContext(ctx))
	}
}
