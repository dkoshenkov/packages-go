package middlewarex

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestChainAppliesMiddlewareInOrder(t *testing.T) {
	t.Parallel()

	var steps []string

	handler := Chain(func(_ context.Context, req string) (string, error) {
		steps = append(steps, "handler:"+req)
		return req + "-handled", nil
	},
		func(next Handler[string, string]) Handler[string, string] {
			return func(ctx context.Context, req string) (string, error) {
				steps = append(steps, "mw1-before")
				resp, err := next(ctx, req+"-mw1")
				steps = append(steps, "mw1-after")
				return resp, err
			}
		},
		func(next Handler[string, string]) Handler[string, string] {
			return func(ctx context.Context, req string) (string, error) {
				steps = append(steps, "mw2-before")
				resp, err := next(ctx, req+"-mw2")
				steps = append(steps, "mw2-after")
				return resp, err
			}
		},
	)

	resp, err := handler(context.Background(), "req")
	require.NoError(t, err)
	require.Equal(t, "req-mw1-mw2-handled", resp)
	require.Equal(t, []string{"mw1-before", "mw2-before", "handler:req-mw1-mw2", "mw2-after", "mw1-after"}, steps)
}

func TestChainShortCircuitsOnError(t *testing.T) {
	t.Parallel()

	handler := Chain(func(_ context.Context, req string) (string, error) {
		return req, nil
	}, func(_ Handler[string, string]) Handler[string, string] {
		return func(_ context.Context, _ string) (string, error) {
			return "", errors.New("boom")
		}
	})

	_, err := handler(context.Background(), "req")
	require.EqualError(t, err, "boom")
}

func TestChainPropagatesContextAndRequest(t *testing.T) {
	t.Parallel()

	type contextKey string

	handler := Chain(func(ctx context.Context, req string) (string, error) {
		return ctx.Value(contextKey("trace")).(string) + ":" + req, nil
	}, func(next Handler[string, string]) Handler[string, string] {
		return func(ctx context.Context, req string) (string, error) {
			ctx = context.WithValue(ctx, contextKey("trace"), "ok")
			return next(ctx, req+"-next")
		}
	})

	resp, err := handler(context.Background(), "req")
	require.NoError(t, err)
	require.Equal(t, "ok:req-next", resp)
}

func TestRequireAuthAndIdentityHelpers(t *testing.T) {
	t.Parallel()

	handler := Chain(func(ctx context.Context, _ string) (string, error) {
		identity, ok := IdentityFromContext(ctx)
		require.True(t, ok)
		return identity.Subject, nil
	}, RequireAuth[string, string]())

	_, err := handler(context.Background(), "")
	require.True(t, IsUnauthorized(err))

	ctx := WithIdentity(context.Background(), Identity{Subject: "user-1"})
	resp, err := handler(ctx, "")
	require.NoError(t, err)
	require.Equal(t, "user-1", resp)
}

func TestRequireRoles(t *testing.T) {
	t.Parallel()

	handler := Chain(func(_ context.Context, _ string) (string, error) {
		return "ok", nil
	}, RequireRoles[string, string]("admin", "support"))

	ctx := WithIdentity(context.Background(), Identity{Roles: []string{"viewer", "support"}})
	resp, err := handler(ctx, "")
	require.NoError(t, err)
	require.Equal(t, "ok", resp)

	ctx = WithIdentity(context.Background(), Identity{Roles: []string{"viewer"}})
	_, err = handler(ctx, "")
	require.True(t, IsForbidden(err))
}

func TestRequireScopes(t *testing.T) {
	t.Parallel()

	handler := Chain(func(_ context.Context, _ string) (string, error) {
		return "ok", nil
	}, RequireScopes[string, string]("read", "write"))

	ctx := WithIdentity(context.Background(), Identity{Scopes: []string{"read", "write", "admin"}})
	resp, err := handler(ctx, "")
	require.NoError(t, err)
	require.Equal(t, "ok", resp)

	ctx = WithIdentity(context.Background(), Identity{Scopes: []string{"read"}})
	_, err = handler(ctx, "")
	require.True(t, IsForbidden(err))
}

func TestTimeout(t *testing.T) {
	t.Parallel()

	handler := Chain(func(ctx context.Context, _ string) (string, error) {
		<-ctx.Done()
		return "", ctx.Err()
	}, Timeout[string, string](10*time.Millisecond))

	_, err := handler(context.Background(), "")
	require.True(t, IsTimeout(err))
}

func TestRecovery(t *testing.T) {
	t.Parallel()

	var events []Event
	logger := LoggerFunc(func(_ context.Context, event Event) {
		events = append(events, event)
	})

	handler := Chain(func(_ context.Context, _ string) (string, error) {
		panic("kaboom")
	}, Recovery[string, string](logger))

	_, err := handler(context.Background(), "")
	require.True(t, IsInternal(err))
	require.Len(t, events, 1)
	require.Equal(t, "recovery", events[0].Name)
}

func TestLogging(t *testing.T) {
	t.Parallel()

	var events []Event
	logger := LoggerFunc(func(_ context.Context, event Event) {
		events = append(events, event)
	})

	handler := Chain(func(_ context.Context, req string) (string, error) {
		return req + "-ok", nil
	}, Logging[string, string](logger, WithLogName[string, string]("test"), WithLogFields[string, string](func(_ context.Context, req string, resp string, err error) map[string]any {
		return map[string]any{"req": req, "resp": resp, "err_nil": err == nil}
	})))

	resp, err := handler(WithRequestID(context.Background(), "r1"), "payload")
	require.NoError(t, err)
	require.Equal(t, "payload-ok", resp)
	require.Len(t, events, 2)
	require.Equal(t, "test", events[0].Name)
	require.Equal(t, "r1", events[0].RequestID)
	require.Equal(t, "payload", events[1].Fields["req"])
}
