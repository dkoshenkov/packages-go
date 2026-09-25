package httpx

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/dkoshenkov/packages-go/middlewarex"
)

type authConfig struct {
	headerName string
	scheme     string
	cookieName string
	priority   []AuthSource
}

// AuthSource identifies where an authenticated credential was extracted.
type AuthSource string

const (
	AuthSourceBearer AuthSource = "bearer"
	AuthSourceCookie AuthSource = "cookie"
)

type authSourceContextKey struct{}

// AuthSourceFromContext reports the credential source selected by Auth.
func AuthSourceFromContext(ctx context.Context) (AuthSource, bool) {
	source, ok := ctx.Value(authSourceContextKey{}).(AuthSource)
	return source, ok
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

// WithAuthCookie enables extraction from a cookie with the given name.
// Include AuthSourceCookie in WithAuthPriority to use it.
func WithAuthCookie(name string) AuthOption {
	return authOptionFunc(func(cfg *authConfig) {
		cfg.cookieName = strings.TrimSpace(name)
	})
}

// WithAuthPriority sets credential source precedence. Sources absent from the
// request are skipped; once a present source is selected, failed verification
// does not fall back to another source.
func WithAuthPriority(sources ...AuthSource) AuthOption {
	return authOptionFunc(func(cfg *authConfig) {
		cfg.priority = append([]AuthSource(nil), sources...)
	})
}

// Auth verifies a configured request credential and stores its identity/source
// in context. With no options, it retains Bearer-only behavior.
func Auth(verifier middlewarex.Verifier, opts ...AuthOption) Middleware {
	cfg := authConfig{
		headerName: "Authorization",
		scheme:     "Bearer",
		priority:   []AuthSource{AuthSourceBearer},
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

			source, token, err := selectCredential(exchange.Request, cfg)
			if err != nil {
				return struct{}{}, middlewarex.Unauthorized(err)
			}

			identity, err := verifier.Verify(ctx, token)
			if err != nil {
				return struct{}{}, middlewarex.Unauthorized(err)
			}
			identity.Token = token

			ctx = context.WithValue(ctx, authSourceContextKey{}, source)
			ctx = middlewarex.WithIdentity(ctx, identity)
			exchange.Request = exchange.Request.WithContext(ctx)
			return next(ctx, exchange)
		}
	}
}

func selectCredential(r *http.Request, cfg authConfig) (AuthSource, string, error) {
	for _, source := range cfg.priority {
		switch source {
		case AuthSourceBearer:
			value := r.Header.Get(cfg.headerName)
			if strings.TrimSpace(value) == "" {
				continue
			}
			token, err := bearerToken(value, cfg.scheme)
			return source, token, err
		case AuthSourceCookie:
			if cfg.cookieName == "" {
				return "", "", errAuthCookieNameEmpty
			}
			cookie, err := r.Cookie(cfg.cookieName)
			if errors.Is(err, http.ErrNoCookie) {
				continue
			}
			if err != nil {
				return source, "", err
			}
			if cookie.Value == "" {
				return source, "", errAuthorizationMissing
			}
			return source, cookie.Value, nil
		default:
			return "", "", errAuthSourceInvalid
		}
	}
	return "", "", errAuthorizationMissing
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
