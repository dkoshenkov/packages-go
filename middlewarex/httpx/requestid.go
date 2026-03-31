package httpx

import (
	"context"
	"strings"

	"github.com/dkoshenkov/packages-go/middlewarex"
	"github.com/google/uuid"
)

type requestIDConfig struct {
	headerName string
	generator  func() string
}

// RequestIDOption customizes request ID middleware.
type RequestIDOption interface {
	apply(*requestIDConfig)
}

type requestIDOptionFunc func(*requestIDConfig)

func (f requestIDOptionFunc) apply(cfg *requestIDConfig) {
	f(cfg)
}

// WithRequestIDHeader sets header name used for request ID.
func WithRequestIDHeader(name string) RequestIDOption {
	return requestIDOptionFunc(func(cfg *requestIDConfig) {
		cfg.headerName = strings.TrimSpace(name)
	})
}

// WithRequestIDGenerator sets request ID generator.
func WithRequestIDGenerator(generator func() string) RequestIDOption {
	return requestIDOptionFunc(func(cfg *requestIDConfig) {
		cfg.generator = generator
	})
}

// RequestID stores request ID in context and response header.
func RequestID(opts ...RequestIDOption) Middleware {
	cfg := requestIDConfig{
		headerName: "X-Request-ID",
		generator:  newRequestID,
	}
	for _, opt := range opts {
		if opt == nil {
			continue
		}
		opt.apply(&cfg)
	}

	return func(next Handler) Handler {
		return func(ctx context.Context, exchange Exchange) (struct{}, error) {
			if cfg.generator == nil {
				return struct{}{}, middlewarex.Internal(errRequestIDGeneratorNil)
			}

			requestID := strings.TrimSpace(exchange.Request.Header.Get(cfg.headerName))
			if requestID == "" {
				requestID = cfg.generator()
			}
			ctx = middlewarex.WithRequestID(ctx, requestID)
			exchange.Request = exchange.Request.WithContext(ctx)
			exchange.Writer.Header().Set(cfg.headerName, requestID)
			return next(ctx, exchange)
		}
	}
}

func newRequestID() string {
	return uuid.NewString()
}
