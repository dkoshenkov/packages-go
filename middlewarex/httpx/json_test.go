package httpx

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/dkoshenkov/packages-go/configx"
	"github.com/dkoshenkov/packages-go/middlewarex"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
)

type jsonRequest struct {
	Name string `json:"name"`
}

type jsonResponse struct {
	Message string `json:"message"`
}

func TestJSONSuccess(t *testing.T) {
	t.Parallel()

	h := JSON(func(_ context.Context, req jsonRequest) (Response[jsonResponse], error) {
		return OK(jsonResponse{Message: "hello, " + req.Name}), nil
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/hello", bytes.NewBufferString(`{"name":"alice"}`))
	h.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "application/json; charset=utf-8", rec.Header().Get("Content-Type"))
	require.JSONEq(t, `{"message":"hello, alice"}`, rec.Body.String())
}

func TestJSONDecodeError(t *testing.T) {
	t.Parallel()

	h := JSON(func(_ context.Context, req jsonRequest) (Response[jsonResponse], error) {
		return OK(jsonResponse{Message: req.Name}), nil
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/hello", bytes.NewBufferString(`{"name":`))
	h.ServeHTTP(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Equal(t, "Bad Request\n", rec.Body.String())
}

func TestJSONWithMiddlewaresAndRuntime(t *testing.T) {
	t.Parallel()

	var events []middlewarex.Event
	logger := middlewarex.LoggerFunc(func(_ context.Context, event middlewarex.Event) {
		events = append(events, event)
	})

	h := JSON(
		func(ctx context.Context, req jsonRequest) (Response[jsonResponse], error) {
			requestID, ok := middlewarex.RequestIDFromContext(ctx)
			require.True(t, ok)
			return Created(jsonResponse{Message: req.Name + ":" + requestID}), nil
		},
		WithRuntime[jsonRequest, jsonResponse](Runtime{
			Logger: logger,
		}),
		WithMiddlewares[jsonRequest, jsonResponse](func(next middlewarex.Handler[jsonRequest, Response[jsonResponse]]) middlewarex.Handler[jsonRequest, Response[jsonResponse]] {
			return func(ctx context.Context, req jsonRequest) (Response[jsonResponse], error) {
				req.Name += "-mw"
				return next(ctx, req)
			}
		}),
	)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/hello", bytes.NewBufferString(`{"name":"alice"}`))
	req.Header.Set("X-Request-ID", "rid-42")
	h.ServeHTTP(rec, req)

	require.Equal(t, http.StatusCreated, rec.Code)
	require.Equal(t, "rid-42", rec.Header().Get("X-Request-ID"))
	require.JSONEq(t, `{"message":"alice-mw:rid-42"}`, rec.Body.String())
	require.Len(t, events, 2)
	require.Equal(t, "http", events[0].Name)
	require.Equal(t, "request started", events[0].Message)
	require.Equal(t, "request finished", events[1].Message)
	require.Equal(t, http.MethodPost, events[0].Fields["method"])
	require.Equal(t, "/hello", events[0].Fields["path"])
	require.Equal(t, http.StatusCreated, events[1].Fields["status"])
	require.Equal(t, http.MethodPost, events[1].Fields["method"])
	require.Equal(t, "/hello", events[1].Fields["path"])
}

func TestJSONUsesCustomStatusMapper(t *testing.T) {
	t.Parallel()

	var events []middlewarex.Event
	logger := middlewarex.LoggerFunc(func(_ context.Context, event middlewarex.Event) {
		events = append(events, event)
	})
	teapot := StatusMapperFunc(func(error) int { return http.StatusTeapot })
	h := JSON(
		func(_ context.Context, _ jsonRequest) (Response[jsonResponse], error) {
			return Response[jsonResponse]{}, errors.New("boom")
		},
		WithRuntime[jsonRequest, jsonResponse](Runtime{
			Logger: logger,
		}),
		WithStatusMapper(teapot),
	)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/hello", bytes.NewBufferString(`{"name":"alice"}`))
	h.ServeHTTP(rec, req)

	require.Equal(t, http.StatusTeapot, rec.Code)
	require.Equal(t, "I'm a teapot\n", rec.Body.String())
	require.Len(t, events, 2)
	require.Equal(t, http.StatusTeapot, events[1].Fields["status"])
	require.Equal(t, "/hello", events[1].Fields["path"])
}

func TestJSONWithRuntimeDefaultErrorEncoderDoesNotPanic(t *testing.T) {
	t.Parallel()

	rt, err := DefaultRuntime("svc", Config{})
	require.NoError(t, err)

	h := JSON(
		func(_ context.Context, _ jsonRequest) (Response[jsonResponse], error) {
			return Response[jsonResponse]{}, errors.New("boom")
		},
		WithRuntime[jsonRequest, jsonResponse](rt),
	)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/hello", bytes.NewBufferString(`{"name":"alice"}`))

	require.NotPanics(t, func() {
		h.ServeHTTP(rec, req)
	})
	require.Equal(t, http.StatusInternalServerError, rec.Code)
	require.Equal(t, "Internal Server Error\n", rec.Body.String())
}

func TestJSONSkipsRequestLogsWhenDisabled(t *testing.T) {
	t.Parallel()

	var events []middlewarex.Event
	logger := middlewarex.LoggerFunc(func(_ context.Context, event middlewarex.Event) {
		events = append(events, event)
	})

	h := JSON(
		func(_ context.Context, req jsonRequest) (Response[jsonResponse], error) {
			return OK(jsonResponse{Message: req.Name}), nil
		},
		WithRuntime[jsonRequest, jsonResponse](Runtime{
			Logger:            logger,
			logRequests:       false,
			logRequestsWasSet: true,
		}),
	)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/hello", bytes.NewBufferString(`{"name":"alice"}`))
	h.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Empty(t, events)
}

func TestJSONDoesNotWriteFallbackErrorAfterEncoderCommit(t *testing.T) {
	t.Parallel()

	h := JSON(
		func(_ context.Context, _ jsonRequest) (Response[jsonResponse], error) {
			return OK(jsonResponse{Message: "alice"}), nil
		},
		WithEncoder[jsonRequest, jsonResponse](func(w http.ResponseWriter, _ *http.Request, resp Response[jsonResponse]) error {
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			w.WriteHeader(http.StatusCreated)
			_, writeErr := w.Write([]byte(`{"message":"partial"`))
			require.NoError(t, writeErr)
			require.NotNil(t, resp.Body)
			return errors.New("encode failed")
		}),
	)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/hello", bytes.NewBufferString(`{"name":"alice"}`))
	h.ServeHTTP(rec, req)

	require.Equal(t, http.StatusCreated, rec.Code)
	require.Equal(t, `{"message":"partial"`, rec.Body.String())
}

func TestResponseHelpers(t *testing.T) {
	t.Parallel()

	ok := OK(jsonResponse{Message: "ok"})
	require.Equal(t, http.StatusOK, ok.Status)
	require.NotNil(t, ok.Body)
	require.Equal(t, "ok", ok.Body.Message)

	created := Created(jsonResponse{Message: "created"})
	require.Equal(t, http.StatusCreated, created.Status)
	require.NotNil(t, created.Body)

	noContent := NoContent()
	require.Equal(t, http.StatusNoContent, noContent.Status)
	require.Nil(t, noContent.Body)

	status := Status(http.StatusAccepted)
	require.Equal(t, http.StatusAccepted, status.Status)
	require.Nil(t, status.Body)
}

func TestDefaultRuntime(t *testing.T) {
	t.Parallel()

	rt, err := DefaultRuntime("svc", Config{
		RequestIDHeader: "X-Correlation-ID",
		Timeout:         5 * time.Second,
		LogRequests:     false,
		PrettyLogs:      true,
	})
	require.NoError(t, err)
	require.NotNil(t, rt.Logger)
	require.NotNil(t, rt.StatusMapper)
	require.NotNil(t, rt.ErrorEncoder)
	require.Equal(t, "X-Correlation-ID", rt.requestIDHeader)
	require.Equal(t, 5*time.Second, rt.timeout)
	require.False(t, rt.logRequests)
}

func TestLoadDefaultRuntime(t *testing.T) {
	t.Setenv("DEV_HTTP_REQUEST_ID_HEADER", "X-Correlation-ID")
	t.Setenv("DEV_HTTP_TIMEOUT", "5s")
	t.Setenv("DEV_HTTP_LOG_REQUESTS", "false")
	t.Setenv("DEV_HTTP_PRETTY_LOGS", "true")

	var cfg Config
	rt, err := LoadDefaultRuntime(context.Background(), "svc", &cfg, configx.WithProfile("dev"))
	require.NoError(t, err)
	require.Equal(t, "X-Correlation-ID", cfg.RequestIDHeader)
	require.Equal(t, 5*time.Second, cfg.Timeout)
	require.False(t, cfg.LogRequests)
	require.True(t, cfg.PrettyLogs)
	require.Equal(t, "X-Correlation-ID", rt.requestIDHeader)
	require.Equal(t, 5*time.Second, rt.timeout)
	require.False(t, rt.logRequests)
}

func TestLoadDefaultRuntimeWithoutExplicitProfile(t *testing.T) {
	t.Setenv("HTTP_REQUEST_ID_HEADER", "X-Correlation-ID")
	t.Setenv("HTTP_TIMEOUT", "7s")

	var cfg Config
	rt, err := LoadDefaultRuntime(context.Background(), "svc", &cfg)
	require.NoError(t, err)
	require.Equal(t, "X-Correlation-ID", cfg.RequestIDHeader)
	require.Equal(t, 7*time.Second, cfg.Timeout)
	require.Equal(t, "X-Correlation-ID", rt.requestIDHeader)
	require.Equal(t, 7*time.Second, rt.timeout)
}

func TestWithZerolog(t *testing.T) {
	t.Parallel()

	buf := bytes.NewBuffer(nil)
	logger := zerolog.New(buf)
	rt := NewRuntime(WithZerolog(logger))

	require.NotNil(t, rt.Logger)
	rt.Logger.Log(context.Background(), middlewarex.Event{
		Level:     "error",
		Name:      "http",
		Message:   "failed",
		RequestID: "rid-1",
		Subject:   "user-1",
		Fields: map[string]any{
			"method": "POST",
		},
		Err: errors.New("boom"),
	})

	require.Contains(t, buf.String(), `"message":"failed"`)
	require.Contains(t, buf.String(), `"request_id":"rid-1"`)
	require.Contains(t, buf.String(), `"subject":"user-1"`)
	require.Contains(t, buf.String(), `"method":"POST"`)
}
