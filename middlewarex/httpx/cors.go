package httpx

import (
	"context"
	"net/http"

	rscors "github.com/rs/cors"
)

type CORSOption interface {
	apply(*corsConfig)
}

type corsOptionFunc func(*corsConfig)

func (f corsOptionFunc) apply(cfg *corsConfig) {
	f(cfg)
}

type corsConfig struct {
	allowedOrigins []string
	allowedMethods []string
	allowedHeaders []string
}

// WithAllowedOrigins sets allowed origins.
func WithAllowedOrigins(origins ...string) CORSOption {
	return corsOptionFunc(func(cfg *corsConfig) {
		cfg.allowedOrigins = append([]string(nil), origins...)
	})
}

// WithAllowedMethods sets allowed methods.
func WithAllowedMethods(methods ...string) CORSOption {
	return corsOptionFunc(func(cfg *corsConfig) {
		cfg.allowedMethods = append([]string(nil), methods...)
	})
}

// WithAllowedHeaders sets allowed headers.
func WithAllowedHeaders(headers ...string) CORSOption {
	return corsOptionFunc(func(cfg *corsConfig) {
		cfg.allowedHeaders = append([]string(nil), headers...)
	})
}

// CORS handles basic CORS headers and preflight requests.
func CORS(opts ...CORSOption) Middleware {
	cfg := corsConfig{
		allowedMethods: []string{http.MethodGet, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete, http.MethodOptions},
		allowedHeaders: []string{"Authorization", "Content-Type", "X-Request-ID"},
	}
	for _, opt := range opts {
		if opt == nil {
			continue
		}
		opt.apply(&cfg)
	}

	handler := rscors.New(rscors.Options{
		AllowedOrigins: cfg.allowedOrigins,
		AllowedMethods: cfg.allowedMethods,
		AllowedHeaders: cfg.allowedHeaders,
	})

	return func(next Handler) Handler {
		return func(ctx context.Context, exchange Exchange) (struct{}, error) {
			var callErr error
			handler.Handler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				exchange.Writer = w
				exchange.Request = r
				_, callErr = next(ctx, exchange)
			})).ServeHTTP(exchange.Writer, exchange.Request)
			return struct{}{}, callErr
		}
	}
}
