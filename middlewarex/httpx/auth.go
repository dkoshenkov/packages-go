package httpx

import (
	"context"
	"strings"

	"github.com/dkoshenkov/packages-go/middlewarex"
)

type authConfig struct {
	headerName string
	scheme     string
}

// AuthOption customizes HTTP auth middleware.
type AuthOption interface {
	apply(*authConfig)
}

type authOptionFunc func(*authConfig)

func (f authOptionFunc) apply(cfg *authConfig) {
	f(cfg)
}

// WithAuthHeader sets header name used for token extraction.
func WithAuthHeader(name string) AuthOption {
	return authOptionFunc(func(cfg *authConfig) {
		cfg.headerName = strings.TrimSpace(name)
	})
}

// WithAuthScheme sets expected auth scheme.
func WithAuthScheme(scheme string) AuthOption {
	return authOptionFunc(func(cfg *authConfig) {
		cfg.scheme = strings.TrimSpace(scheme)
	})
}

// Auth verifies bearer token and stores identity in context.
func Auth(verifier middlewarex.Verifier, opts ...AuthOption) Middleware {
	cfg := authConfig{
		headerName: "Authorization",
		scheme:     "Bearer",
	}
	for _, opt := range opts {
		if opt == nil {
			continue
		}
		opt.apply(&cfg)
	}

	return func(next Handler) Handler {
		return func(ctx context.Context, exchange Exchange) (struct{}, error) {
			if verifier == nil {
				return struct{}{}, middlewarex.Internal(errVerifierIsNil)
			}

			token, err := bearerToken(exchange.Request.Header.Get(cfg.headerName), cfg.scheme)
			if err != nil {
				return struct{}{}, middlewarex.Unauthorized(err)
			}

			identity, err := verifier.Verify(ctx, token)
			if err != nil {
				return struct{}{}, middlewarex.Unauthorized(err)
			}
			identity.Token = token

			ctx = middlewarex.WithIdentity(ctx, identity)
			exchange.Request = exchange.Request.WithContext(ctx)
			return next(ctx, exchange)
		}
	}
}

func bearerToken(value string, scheme string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", errAuthorizationMissing
	}

	parts := strings.Fields(value)
	if len(parts) != 2 || !strings.EqualFold(parts[0], scheme) || strings.TrimSpace(parts[1]) == "" {
		return "", errAuthorizationInvalid
	}

	return parts[1], nil
}
