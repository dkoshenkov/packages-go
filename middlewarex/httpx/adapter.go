package httpx

import (
	"net/http"

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

type adaptConfig struct {
	statusMapper StatusMapper
	errorEncoder ErrorEncoder
}

// AdaptOption customizes adapter behavior.
type AdaptOption interface {
	apply(*adaptConfig)
}

type adaptOptionFunc func(*adaptConfig)

func (f adaptOptionFunc) apply(cfg *adaptConfig) {
	f(cfg)
}

// WithStatusMapper sets error status resolver.
func WithStatusMapper(statusMapper StatusMapper) AdaptOption {
	return adaptOptionFunc(func(cfg *adaptConfig) {
		cfg.statusMapper = statusMapper
	})
}

// WithErrorEncoder sets custom error encoder.
func WithErrorEncoder(errorEncoder ErrorEncoder) AdaptOption {
	return adaptOptionFunc(func(cfg *adaptConfig) {
		cfg.errorEncoder = errorEncoder
	})
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

// Adapt converts error-returning handler to http.Handler.
func Adapt(handler Handler, opts ...AdaptOption) http.Handler {
	cfg := adaptConfig{
		statusMapper: StatusMapperFunc(DefaultStatusMapper),
	}
	for _, opt := range opts {
		if opt == nil {
			continue
		}
		opt.apply(&cfg)
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
