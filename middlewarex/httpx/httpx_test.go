package httpx

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/dkoshenkov/packages-go/middlewarex"
	"github.com/stretchr/testify/require"
)

type verifierFunc func(ctx context.Context, token string) (middlewarex.Identity, error)

func (f verifierFunc) Verify(ctx context.Context, token string) (middlewarex.Identity, error) {
	return f(ctx, token)
}

func TestAuthSuccess(t *testing.T) {
	t.Parallel()

	h := Adapt(Chain(FromHTTPFunc(func(w http.ResponseWriter, r *http.Request) error {
		identity, ok := middlewarex.IdentityFromContext(r.Context())
		require.True(t, ok)
		_, _ = w.Write([]byte(identity.Subject))
		return nil
	}), Auth(verifierFunc(func(_ context.Context, token string) (middlewarex.Identity, error) {
		require.Equal(t, "good", token)
		return middlewarex.Identity{Subject: "user-1"}, nil
	}))))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/secure", nil)
	req.Header.Set("Authorization", "Bearer good")
	h.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "user-1", rec.Body.String())
}

func TestAuthRejectsMissingAndInvalidHeader(t *testing.T) {
	t.Parallel()

	h := Adapt(Chain(FromHTTP(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})), Auth(verifierFunc(func(_ context.Context, token string) (middlewarex.Identity, error) {
		return middlewarex.Identity{Subject: token}, nil
	}))))

	for _, header := range []string{"", "Basic test", "Bearer"} {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/secure", nil)
		if header != "" {
			req.Header.Set("Authorization", header)
		}
		h.ServeHTTP(rec, req)
		require.Equal(t, http.StatusUnauthorized, rec.Code)
	}
}

func TestAuthRejectsVerifierError(t *testing.T) {
	t.Parallel()

	h := Adapt(Chain(FromHTTP(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})), Auth(verifierFunc(func(_ context.Context, _ string) (middlewarex.Identity, error) {
		return middlewarex.Identity{}, errors.New("bad token")
	}))))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/secure", nil)
	req.Header.Set("Authorization", "Bearer bad")
	h.ServeHTTP(rec, req)
	require.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestRequireAuthRolesScopes(t *testing.T) {
	t.Parallel()

	h := Adapt(Chain(FromHTTPFunc(func(w http.ResponseWriter, _ *http.Request) error {
		w.WriteHeader(http.StatusNoContent)
		return nil
	}),
		Auth(verifierFunc(func(_ context.Context, _ string) (middlewarex.Identity, error) {
			return middlewarex.Identity{Subject: "u1", Roles: []string{"admin"}, Scopes: []string{"read", "write"}}, nil
		})),
		middlewarex.RequireAuth[Exchange, struct{}](),
		middlewarex.RequireRoles[Exchange, struct{}]("admin"),
		middlewarex.RequireScopes[Exchange, struct{}]("read", "write"),
	))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/secure", nil)
	req.Header.Set("Authorization", "Bearer good")
	h.ServeHTTP(rec, req)
	require.Equal(t, http.StatusNoContent, rec.Code)
}

func TestRequireRolesForbidden(t *testing.T) {
	t.Parallel()

	h := Adapt(Chain(FromHTTPFunc(func(w http.ResponseWriter, _ *http.Request) error {
		w.WriteHeader(http.StatusNoContent)
		return nil
	}),
		Auth(verifierFunc(func(_ context.Context, _ string) (middlewarex.Identity, error) {
			return middlewarex.Identity{Subject: "u1", Roles: []string{"viewer"}}, nil
		})),
		middlewarex.RequireRoles[Exchange, struct{}]("admin"),
	))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/secure", nil)
	req.Header.Set("Authorization", "Bearer good")
	h.ServeHTTP(rec, req)
	require.Equal(t, http.StatusForbidden, rec.Code)
}

func TestRequireHeader(t *testing.T) {
	t.Parallel()

	h := Adapt(Chain(FromHTTP(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})), RequireHeader("X-Tenant-ID", WithHeaderValue("tenant-1"))))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Tenant-ID", "tenant-1")
	h.ServeHTTP(rec, req)
	require.Equal(t, http.StatusNoContent, rec.Code)

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Tenant-ID", "tenant-2")
	h.ServeHTTP(rec, req)
	require.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestRequireMethod(t *testing.T) {
	t.Parallel()

	h := Adapt(Chain(FromHTTP(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})), RequireMethod(http.MethodPost, http.MethodPut)))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/", nil)
	h.ServeHTTP(rec, req)
	require.Equal(t, http.StatusNoContent, rec.Code)

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/", nil)
	h.ServeHTTP(rec, req)
	require.Equal(t, http.StatusMethodNotAllowed, rec.Code)
}

func TestRequestID(t *testing.T) {
	t.Parallel()

	h := Adapt(Chain(FromHTTPFunc(func(w http.ResponseWriter, r *http.Request) error {
		requestID, ok := middlewarex.RequestIDFromContext(r.Context())
		require.True(t, ok)
		_, _ = w.Write([]byte(requestID))
		return nil
	}), RequestID(WithRequestIDGenerator(func() string { return "generated-id" }))))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Request-ID", "incoming-id")
	h.ServeHTTP(rec, req)
	require.Equal(t, "incoming-id", rec.Header().Get("X-Request-ID"))
	require.Equal(t, "incoming-id", rec.Body.String())

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/", nil)
	h.ServeHTTP(rec, req)
	require.Equal(t, "generated-id", rec.Header().Get("X-Request-ID"))
	require.Equal(t, "generated-id", rec.Body.String())
}

func TestRecoveryAndTimeout(t *testing.T) {
	t.Parallel()

	h := Adapt(Chain(FromHTTPFunc(func(_ http.ResponseWriter, r *http.Request) error {
		<-r.Context().Done()
		return r.Context().Err()
	}), middlewarex.Recovery[Exchange, struct{}](nil), middlewarex.Timeout[Exchange, struct{}](10*time.Millisecond)))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	h.ServeHTTP(rec, req)
	require.Equal(t, http.StatusServiceUnavailable, rec.Code)

	h = Adapt(Chain(func(_ context.Context, _ Exchange) (struct{}, error) {
		panic("boom")
	}, middlewarex.Recovery[Exchange, struct{}](nil)))

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/", nil)
	h.ServeHTTP(rec, req)
	require.Equal(t, http.StatusInternalServerError, rec.Code)
}

func TestCORS(t *testing.T) {
	t.Parallel()

	h := Adapt(Chain(FromHTTP(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})), CORS(WithAllowedOrigins("https://example.com"), WithAllowedMethods(http.MethodGet), WithAllowedHeaders("Authorization"))))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodOptions, "/", nil)
	req.Header.Set("Origin", "https://example.com")
	req.Header.Set("Access-Control-Request-Method", http.MethodGet)
	h.ServeHTTP(rec, req)
	require.Equal(t, http.StatusNoContent, rec.Code)
	require.Equal(t, "https://example.com", rec.Header().Get("Access-Control-Allow-Origin"))

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Origin", "https://example.com")
	h.ServeHTTP(rec, req)
	require.Equal(t, http.StatusNoContent, rec.Code)
	require.Equal(t, "https://example.com", rec.Header().Get("Access-Control-Allow-Origin"))

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Origin", "https://blocked.example.com")
	h.ServeHTTP(rec, req)
	require.Equal(t, http.StatusNoContent, rec.Code)
	require.Empty(t, rec.Header().Get("Access-Control-Allow-Origin"))
}

func TestLoggingAndEndToEnd(t *testing.T) {
	t.Parallel()

	var events []middlewarex.Event
	logger := middlewarex.LoggerFunc(func(_ context.Context, event middlewarex.Event) {
		events = append(events, event)
	})

	h := Adapt(Chain(FromHTTPFunc(func(w http.ResponseWriter, r *http.Request) error {
		identity, _ := middlewarex.IdentityFromContext(r.Context())
		requestID, _ := middlewarex.RequestIDFromContext(r.Context())
		_, _ = w.Write([]byte(identity.Subject + ":" + requestID))
		return nil
	}),
		RequestID(WithRequestIDGenerator(func() string { return "rid-1" })),
		Auth(verifierFunc(func(_ context.Context, _ string) (middlewarex.Identity, error) {
			return middlewarex.Identity{Subject: "user-1", Roles: []string{"admin"}, Scopes: []string{"read"}}, nil
		})),
		Logging(logger),
		middlewarex.RequireAuth[Exchange, struct{}](),
		middlewarex.RequireRoles[Exchange, struct{}]("admin"),
		middlewarex.RequireScopes[Exchange, struct{}]("read"),
		RequireHeader("X-Tenant-ID"),
		RequireMethod(http.MethodGet),
	))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/secure", nil)
	req.Header.Set("Authorization", "Bearer ok")
	req.Header.Set("X-Tenant-ID", "tenant-1")
	h.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "user-1:rid-1", rec.Body.String())
	require.Len(t, events, 2)
	require.Equal(t, "http", events[0].Name)
	require.Equal(t, "rid-1", events[1].RequestID)
	require.Equal(t, "user-1", events[1].Subject)
	status, ok := events[1].Fields["status"]
	require.False(t, ok)
	require.Nil(t, status)
}
