package httpx

import (
	"context"
	"net"
	"net/http"
	"net/url"
	"strings"

	"github.com/dkoshenkov/packages-go/middlewarex"
)

// CSRFConfig contains application-specific CSRF policy. An origin is allowed
// when it matches the request origin or one of TrustedOrigins.
type CSRFConfig struct {
	TrustedOrigins     []string
	CheckFetchMetadata bool
	RequireOrigin      bool
	AllowBearer        bool
	Exempt             func(*http.Request) bool
}

// CSRF checks unsafe HTTP methods before the handler reads the request body.
// Place it after Auth when AllowBearer is enabled so it can use the verified
// credential source rather than inspecting the Authorization header itself.
func CSRF(cfg CSRFConfig) Middleware {
	trusted := make(map[string]struct{}, len(cfg.TrustedOrigins))
	for _, origin := range cfg.TrustedOrigins {
		if normalized, ok := normalizeOrigin(origin); ok {
			trusted[normalized] = struct{}{}
		}
	}

	return func(next Handler) Handler {
		return func(ctx context.Context, exchange Exchange) (struct{}, error) {
			r := exchange.Request
			if isSafeMethod(r.Method) || (cfg.Exempt != nil && cfg.Exempt(r)) {
				return next(ctx, exchange)
			}
			if cfg.AllowBearer {
				if source, ok := AuthSourceFromContext(ctx); ok && source == AuthSourceBearer {
					return next(ctx, exchange)
				}
			}

			origin := strings.TrimSpace(r.Header.Get("Origin"))
			trustedOrigin := false
			if origin == "" {
				if cfg.RequireOrigin {
					return struct{}{}, middlewarex.Forbidden(errCSRFOriginMissing)
				}
			} else {
				normalized, valid := normalizeOrigin(origin)
				_, trustedOrigin = trusted[normalized]
				if !valid || !originAllowed(origin, r, trusted) {
					return struct{}{}, middlewarex.Forbidden(errCSRFOriginInvalid)
				}
			}

			if cfg.CheckFetchMetadata {
				site := strings.ToLower(strings.TrimSpace(r.Header.Get("Sec-Fetch-Site")))
				if site == "cross-site" && !trustedOrigin {
					return struct{}{}, middlewarex.Forbidden(errCSRFFetchSiteInvalid)
				}
				if site != "" && site != "same-origin" && site != "same-site" && site != "none" && site != "cross-site" {
					return struct{}{}, middlewarex.Forbidden(errCSRFFetchSiteInvalid)
				}
			}
			return next(ctx, exchange)
		}
	}
}

func isSafeMethod(method string) bool {
	switch strings.ToUpper(strings.TrimSpace(method)) {
	case "GET", "HEAD", "OPTIONS", "TRACE":
		return true
	default:
		return false
	}
}

func originAllowed(origin string, r *http.Request, trusted map[string]struct{}) bool {
	normalized, ok := normalizeOrigin(origin)
	if !ok {
		return false
	}
	if expected, ok := requestOrigin(r); ok && normalized == expected {
		return true
	}
	_, ok = trusted[normalized]
	return ok
}

func requestOrigin(r *http.Request) (string, bool) {
	scheme := strings.ToLower(strings.TrimSpace(r.URL.Scheme))
	if scheme == "" {
		if r.TLS != nil {
			scheme = "https"
		} else {
			scheme = "http"
		}
	}
	host := strings.TrimSpace(r.Host)
	if host == "" {
		host = r.URL.Host
	}
	if host == "" {
		return "", false
	}
	return normalizeOrigin(scheme + "://" + host)
}

func normalizeOrigin(value string) (string, bool) {
	u, err := url.Parse(strings.TrimSpace(value))
	if err != nil || u == nil || !u.IsAbs() || u.Opaque != "" || u.User != nil || u.Host == "" || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") {
		return "", false
	}
	scheme := strings.ToLower(u.Scheme)
	if scheme != "http" && scheme != "https" {
		return "", false
	}
	host := strings.ToLower(u.Hostname())
	if host == "" {
		return "", false
	}
	port := u.Port()
	if (scheme == "http" && port == "80") || (scheme == "https" && port == "443") {
		port = ""
	}
	if port != "" {
		host = net.JoinHostPort(host, port)
	} else if strings.Contains(host, ":") {
		host = "[" + host + "]"
	}
	return scheme + "://" + host, true
}
