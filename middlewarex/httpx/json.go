package httpx

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/dkoshenkov/packages-go/middlewarex"
	"github.com/google/uuid"
)

// Response describes typed HTTP response envelope.
type Response[T any] struct {
	Status int
	Body   *T
}

// OK returns 200 response with JSON body.
func OK[T any](body T) Response[T] {
	return WithStatus(http.StatusOK, body)
}

// Created returns 201 response with JSON body.
func Created[T any](body T) Response[T] {
	return WithStatus(http.StatusCreated, body)
}

// NoContent returns 204 response without body.
func NoContent() Response[struct{}] {
	return Response[struct{}]{Status: http.StatusNoContent}
}

// Status returns custom status without body.
func Status(code int) Response[struct{}] {
	return Response[struct{}]{Status: code}
}

// WithStatus returns response with custom status and JSON body.
func WithStatus[T any](code int, body T) Response[T] {
	return Response[T]{
		Status: code,
		Body:   &body,
	}
}

// DecodeFunc decodes HTTP request into typed payload.
type DecodeFunc[Req any] func(*http.Request) (Req, error)

// EncodeFunc encodes typed response into HTTP response.
type EncodeFunc[Resp any] func(http.ResponseWriter, *http.Request, Response[Resp]) error

// Option customizes typed JSON adapter behavior.
type Option[Req, Resp any] interface {
	optionMarker
}

type decoderOption[Req, Resp any] struct {
	fn DecodeFunc[Req]
}

func (decoderOption[Req, Resp]) option() {}

type encoderOption[Req, Resp any] struct {
	fn EncodeFunc[Resp]
}

func (encoderOption[Req, Resp]) option() {}

type middlewaresOption[Req, Resp any] struct {
	middlewares []middlewarex.Middleware[Req, Response[Resp]]
}

func (middlewaresOption[Req, Resp]) option() {}

type runtimeOption[Req, Resp any] struct {
	runtime Runtime
}

func (runtimeOption[Req, Resp]) option() {}

type jsonConfig[Req, Resp any] struct {
	decoder      DecodeFunc[Req]
	encoder      EncodeFunc[Resp]
	middlewares  []middlewarex.Middleware[Req, Response[Resp]]
	runtime      *Runtime
	statusMapper StatusMapper
	errorEncoder ErrorEncoder

	statusMapperWasSet bool
	errorEncoderWasSet bool
}

// WithDecoder sets request decoder for JSON adapter.
func WithDecoder[Req, Resp any](fn DecodeFunc[Req]) Option[Req, Resp] {
	return decoderOption[Req, Resp]{fn: fn}
}

// WithEncoder sets response encoder for JSON adapter.
func WithEncoder[Req, Resp any](fn EncodeFunc[Resp]) Option[Req, Resp] {
	return encoderOption[Req, Resp]{fn: fn}
}

// WithMiddlewares applies typed middleware chain before handler execution.
func WithMiddlewares[Req, Resp any](mw ...middlewarex.Middleware[Req, Response[Resp]]) Option[Req, Resp] {
	return middlewaresOption[Req, Resp]{middlewares: mw}
}

// WithRuntime sets shared runtime defaults for JSON adapter.
func WithRuntime[Req, Resp any](runtime Runtime) Option[Req, Resp] {
	return runtimeOption[Req, Resp]{runtime: runtime}
}

// DecodeJSON decodes JSON body into request value.
func DecodeJSON[Req any](r *http.Request) (Req, error) {
	var req Req
	if r == nil || r.Body == nil || r.Body == http.NoBody || r.ContentLength == 0 {
		return req, nil
	}

	decoder := json.NewDecoder(r.Body)
	if err := decoder.Decode(&req); err != nil {
		return req, middlewarex.BadRequest(err)
	}
	if err := decoder.Decode(new(struct{})); err != io.EOF {
		if err == nil {
			return req, middlewarex.BadRequest(errors.New("request body must contain a single JSON value"))
		}
		return req, middlewarex.BadRequest(err)
	}

	return req, nil
}

// EncodeJSON encodes typed response as JSON.
func EncodeJSON[T any](w http.ResponseWriter, _ *http.Request, resp Response[T]) error {
	status := responseStatus(resp.Status, resp.Body)
	if resp.Body == nil {
		w.WriteHeader(status)
		return nil
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	return json.NewEncoder(w).Encode(resp.Body)
}

// JSON adapts typed middlewarex handler to http.Handler.
func JSON[Req, Resp any](handler middlewarex.Handler[Req, Response[Resp]], opts ...Option[Req, Resp]) http.Handler {
	cfg := jsonConfig[Req, Resp]{
		decoder: DecodeJSON[Req],
		encoder: EncodeJSON[Resp],
	}

	for _, opt := range opts {
		switch opt := opt.(type) {
		case nil:
			continue
		case decoderOption[Req, Resp]:
			cfg.decoder = opt.fn
		case encoderOption[Req, Resp]:
			cfg.encoder = opt.fn
		case middlewaresOption[Req, Resp]:
			cfg.middlewares = append(cfg.middlewares, opt.middlewares...)
		case runtimeOption[Req, Resp]:
			runtime := opt.runtime
			cfg.runtime = &runtime
		case statusMapperOption:
			cfg.statusMapper = opt.statusMapper
			cfg.statusMapperWasSet = true
		case errorEncoderOption:
			cfg.errorEncoder = opt.errorEncoder
			cfg.errorEncoderWasSet = true
		}
	}

	runtime := runtimeOrDefault(cfg.runtime)
	if cfg.statusMapper == nil {
		cfg.statusMapper = runtime.StatusMapper
	}
	if cfg.errorEncoder == nil {
		if cfg.runtime != nil && cfg.runtime.ErrorEncoder != nil && !cfg.statusMapperWasSet && !cfg.errorEncoderWasSet {
			cfg.errorEncoder = runtime.ErrorEncoder
		} else {
			cfg.errorEncoder = DefaultErrorEncoder(cfg.statusMapper)
		}
	}
	runtime.StatusMapper = cfg.statusMapper
	runtime.ErrorEncoder = cfg.errorEncoder

	middleware := runtimeMiddlewares[Req, Resp](runtime)
	middleware = append(middleware, cfg.middlewares...)
	wrapped := middlewarex.Chain(handler, middleware...)

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writer := &trackingResponseWriter{ResponseWriter: w}
		r = runtime.prepareRequest(writer, r)
		startedAt := time.Now()
		if runtime.logRequests {
			logRequestStart(runtime.Logger, r)
		}

		req, err := cfg.decoder(r)
		if err != nil {
			if runtime.logRequests {
				logRequestFinish(runtime.Logger, r, startedAt, cfg.statusMapper.Status(err), err)
			}
			WriteError(writer, r, err, statusMapperOption{statusMapper: cfg.statusMapper}, errorEncoderOption{errorEncoder: cfg.errorEncoder})
			return
		}

		resp, err := wrapped(r.Context(), req)
		if err != nil {
			if runtime.logRequests {
				logRequestFinish(runtime.Logger, r, startedAt, cfg.statusMapper.Status(err), err)
			}
			WriteError(writer, r, err, statusMapperOption{statusMapper: cfg.statusMapper}, errorEncoderOption{errorEncoder: cfg.errorEncoder})
			return
		}
		if err := cfg.encoder(writer, r, resp); err != nil {
			if runtime.logRequests {
				logRequestFinish(runtime.Logger, r, startedAt, cfg.statusMapper.Status(err), err)
			}
			if !writer.Written() {
				WriteError(writer, r, err, statusMapperOption{statusMapper: cfg.statusMapper}, errorEncoderOption{errorEncoder: cfg.errorEncoder})
			}
			return
		}
		if runtime.logRequests {
			logRequestFinish(runtime.Logger, r, startedAt, responseStatus(resp.Status, resp.Body), nil)
		}
	})
}

func responseStatus[T any](status int, body *T) int {
	if status > 0 {
		return status
	}
	if body == nil {
		return http.StatusNoContent
	}
	return http.StatusOK
}

type trackingResponseWriter struct {
	http.ResponseWriter
	written bool
}

func (w *trackingResponseWriter) WriteHeader(statusCode int) {
	w.written = true
	w.ResponseWriter.WriteHeader(statusCode)
}

func (w *trackingResponseWriter) Write(p []byte) (int, error) {
	w.written = true
	return w.ResponseWriter.Write(p)
}

func (w *trackingResponseWriter) Written() bool {
	if w == nil {
		return false
	}
	return w.written
}

func runtimeOrDefault(runtime *Runtime) Runtime {
	if runtime == nil {
		return NewRuntime()
	}

	result := *runtime
	if result.StatusMapper == nil {
		result.StatusMapper = StatusMapperFunc(DefaultStatusMapper)
	}
	if strings.TrimSpace(result.requestIDHeader) == "" {
		result.requestIDHeader = defaultRequestIDHeader
	}
	if result.timeout == 0 {
		result.timeout = defaultTimeout
	}
	if result.ErrorEncoder == nil {
		result.ErrorEncoder = DefaultErrorEncoder(result.StatusMapper)
	}
	if !result.logRequestsWasSet {
		result.logRequests = true
	}
	return result
}

func (rt Runtime) prepareRequest(w http.ResponseWriter, r *http.Request) *http.Request {
	headerName := strings.TrimSpace(rt.requestIDHeader)
	if headerName == "" {
		headerName = defaultRequestIDHeader
	}

	requestID := strings.TrimSpace(r.Header.Get(headerName))
	if requestID == "" {
		requestID = uuid.NewString()
	}

	w.Header().Set(headerName, requestID)
	ctx := middlewarex.WithRequestID(r.Context(), requestID)
	return r.WithContext(ctx)
}

func runtimeMiddlewares[Req, Resp any](rt Runtime) []middlewarex.Middleware[Req, Response[Resp]] {
	var result []middlewarex.Middleware[Req, Response[Resp]]
	result = append(result, middlewarex.Recovery[Req, Response[Resp]](rt.Logger))
	if rt.timeout > 0 {
		result = append(result, middlewarex.Timeout[Req, Response[Resp]](rt.timeout))
	}
	return result
}

func logRequestStart(logger middlewarex.Logger, r *http.Request) {
	if logger == nil || r == nil {
		return
	}

	logger.Log(r.Context(), middlewarex.Event{
		Level:   "info",
		Name:    "http",
		Message: "request started",
		Fields: map[string]any{
			"method": r.Method,
			"path":   r.URL.Path,
		},
	})
}

func logRequestFinish(logger middlewarex.Logger, r *http.Request, startedAt time.Time, status int, err error) {
	if logger == nil || r == nil {
		return
	}

	level := "info"
	if err != nil {
		level = "error"
	}
	logger.Log(r.Context(), middlewarex.Event{
		Level:    level,
		Name:     "http",
		Message:  "request finished",
		Duration: time.Since(startedAt),
		Err:      err,
		Fields: map[string]any{
			"method": r.Method,
			"path":   r.URL.Path,
			"status": status,
		},
	})
}
