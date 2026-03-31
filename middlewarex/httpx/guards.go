package httpx

import (
	"context"
	"strings"

	"github.com/dkoshenkov/packages-go/middlewarex"
)

type headerConfig struct {
	value string
}

// HeaderOption customizes header guard.
type HeaderOption interface {
	apply(*headerConfig)
}

type headerOptionFunc func(*headerConfig)

func (f headerOptionFunc) apply(cfg *headerConfig) {
	f(cfg)
}

// WithHeaderValue requires exact header value.
func WithHeaderValue(value string) HeaderOption {
	return headerOptionFunc(func(cfg *headerConfig) {
		cfg.value = value
	})
}

// RequireHeader requires request header presence and optional value.
func RequireHeader(name string, opts ...HeaderOption) Middleware {
	cfg := headerConfig{}
	for _, opt := range opts {
		if opt == nil {
			continue
		}
		opt.apply(&cfg)
	}

	name = strings.TrimSpace(name)

	return func(next Handler) Handler {
		return func(ctx context.Context, exchange Exchange) (struct{}, error) {
			if name == "" {
				return struct{}{}, middlewarex.Internal(errHeaderNameEmpty)
			}

			value := exchange.Request.Header.Get(name)
			if value == "" {
				return struct{}{}, middlewarex.BadRequest(errHeaderValueMissing)
			}
			if cfg.value != "" && value != cfg.value {
				return struct{}{}, middlewarex.BadRequest(errHeaderValueMissing)
			}

			return next(ctx, exchange)
		}
	}
}

// RequireMethod requires one of HTTP methods.
func RequireMethod(methods ...string) Middleware {
	allowed := make(map[string]struct{}, len(methods))
	for _, method := range methods {
		method = strings.ToUpper(strings.TrimSpace(method))
		if method == "" {
			continue
		}
		allowed[method] = struct{}{}
	}

	return func(next Handler) Handler {
		return func(ctx context.Context, exchange Exchange) (struct{}, error) {
			if _, ok := allowed[exchange.Request.Method]; !ok {
				return struct{}{}, middlewarex.MethodNotAllowed(errMethodMissing)
			}
			return next(ctx, exchange)
		}
	}
}
