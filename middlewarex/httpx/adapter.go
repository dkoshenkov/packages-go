package httpx

import (
	"net/http"
	"time"

	"github.com/dkoshenkov/packages-go/middlewarex"
)

// StatusMapper resolves HTTP status code for error.
type StatusMapper interface {
	Status(err error) int
}

// StatusMapperFunc adapts function to StatusMapper.
type StatusMapperFunc func(err error) int

// Status returns HTTP status code.
func (f StatusMapperFunc) Status(err error) int {
	return f(err)
}

// ErrorEncoder writes error response.
type ErrorEncoder interface {
	Encode(w http.ResponseWriter, r *http.Request, err error)
}

// ErrorEncoderFunc adapts function to ErrorEncoder.
type ErrorEncoderFunc func(w http.ResponseWriter, r *http.Request, err error)

// Encode writes error response.
func (f ErrorEncoderFunc) Encode(w http.ResponseWriter, r *http.Request, err error) {
	f(w, r, err)
}

type optionMarker interface {
	option()
}

type adaptConfig struct {
	statusMapper StatusMapper
	errorEncoder ErrorEncoder
}

// AdaptOption customizes adapter behavior.
type AdaptOption interface {
	applyAdapt(*adaptConfig)
}

// ErrorOption customizes error writing behavior.
type ErrorOption interface {
	applyError(*errorConfig)
}

// RuntimeOption customizes runtime defaults.
type RuntimeOption interface {
	applyRuntime(*runtimeConfig)
}

type adaptOptionFunc func(*adaptConfig)

func (f adaptOptionFunc) applyAdapt(cfg *adaptConfig) {
	f(cfg)
}

type errorConfig struct {
	statusMapper StatusMapper
	errorEncoder ErrorEncoder
}

type runtimeConfig struct {
	logger          middlewarex.Logger
	statusMapper    StatusMapper
	errorEncoder    ErrorEncoder
	timeout         time.Duration
	requestIDHeader string
	logRequests     bool
}

type statusMapperOption struct {
	statusMapper StatusMapper
}

func (statusMapperOption) option() {}

func (o statusMapperOption) applyAdapt(cfg *adaptConfig) {
	cfg.statusMapper = o.statusMapper
}

func (o statusMapperOption) applyError(cfg *errorConfig) {
	cfg.statusMapper = o.statusMapper
}

func (o statusMapperOption) applyRuntime(cfg *runtimeConfig) {
	cfg.statusMapper = o.statusMapper
}

type errorEncoderOption struct {
	errorEncoder ErrorEncoder
}

func (errorEncoderOption) option() {}

func (o errorEncoderOption) applyAdapt(cfg *adaptConfig) {
	cfg.errorEncoder = o.errorEncoder
}

func (o errorEncoderOption) applyError(cfg *errorConfig) {
	cfg.errorEncoder = o.errorEncoder
}

func (o errorEncoderOption) applyRuntime(cfg *runtimeConfig) {
	cfg.errorEncoder = o.errorEncoder
}

// WithStatusMapper sets error status resolver.
func WithStatusMapper(statusMapper StatusMapper) statusMapperOption {
	return statusMapperOption{statusMapper: statusMapper}
}

// WithErrorEncoder sets custom error encoder.
func WithErrorEncoder(errorEncoder ErrorEncoder) errorEncoderOption {
	return errorEncoderOption{errorEncoder: errorEncoder}
}

// DefaultStatusMapper returns default HTTP status for classified error.
func DefaultStatusMapper(err error) int {
	switch {
	case err == nil:
		return http.StatusOK
	case middlewarex.IsUnauthorized(err):
		return http.StatusUnauthorized
	case middlewarex.IsForbidden(err):
		return http.StatusForbidden
	case middlewarex.IsBadRequest(err):
		return http.StatusBadRequest
	case middlewarex.IsMethodNotAllowed(err):
		return http.StatusMethodNotAllowed
	case middlewarex.IsTimeout(err):
		return http.StatusServiceUnavailable
	default:
		return http.StatusInternalServerError
	}
}

// DefaultErrorEncoder writes status text body.
func DefaultErrorEncoder(statusMapper StatusMapper) ErrorEncoder {
	return ErrorEncoderFunc(func(w http.ResponseWriter, _ *http.Request, err error) {
		if statusMapper == nil {
			statusMapper = StatusMapperFunc(DefaultStatusMapper)
		}
		status := statusMapper.Status(err)
		http.Error(w, http.StatusText(status), status)
	})
}

// WriteError writes error response using configured or default encoder.
func WriteError(w http.ResponseWriter, r *http.Request, err error, opts ...ErrorOption) {
	cfg := errorConfig{
		statusMapper: StatusMapperFunc(DefaultStatusMapper),
	}
	for _, opt := range opts {
		if opt == nil {
			continue
		}
		opt.applyError(&cfg)
	}
	if cfg.statusMapper == nil {
		cfg.statusMapper = StatusMapperFunc(DefaultStatusMapper)
	}
	if cfg.errorEncoder == nil {
		cfg.errorEncoder = DefaultErrorEncoder(cfg.statusMapper)
	}

	cfg.errorEncoder.Encode(w, r, err)
}

// Adapt converts error-returning handler to http.Handler.
func Adapt(handler Handler, opts ...AdaptOption) http.Handler {
	cfg := adaptConfig{
		statusMapper: StatusMapperFunc(DefaultStatusMapper),
	}
	for _, opt := range opts {
		if opt == nil {
			continue
		}
		opt.applyAdapt(&cfg)
	}
	if cfg.statusMapper == nil {
		cfg.statusMapper = StatusMapperFunc(DefaultStatusMapper)
	}
	if cfg.errorEncoder == nil {
		cfg.errorEncoder = DefaultErrorEncoder(cfg.statusMapper)
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, err := handler(r.Context(), Exchange{Writer: w, Request: r})
		if err != nil {
			cfg.errorEncoder.Encode(w, r, err)
		}
	})
}

// Wrap applies middleware to plain http.Handler.
func Wrap(next http.Handler, middleware ...Middleware) http.Handler {
	return Adapt(Chain(FromHTTP(next), middleware...))
}

// WrapFunc applies middleware to plain http.HandlerFunc.
func WrapFunc(next http.HandlerFunc, middleware ...Middleware) http.Handler {
	return Wrap(next, middleware...)
}
