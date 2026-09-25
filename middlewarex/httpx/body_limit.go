package httpx

import (
	"context"
	"errors"
	"net/http"

	"github.com/dkoshenkov/packages-go/middlewarex"
)

// LimitBody limits bytes read from the request body. It should be placed
// before the handler or decoder that consumes the body.
func LimitBody(maxBytes int64) Middleware {
	return func(next Handler) Handler {
		return func(ctx context.Context, exchange Exchange) (struct{}, error) {
			if maxBytes <= 0 {
				return struct{}{}, middlewarex.Internal(errBodyLimitNonPositive)
			}
			if exchange.Request != nil && exchange.Request.Body != nil && exchange.Request.Body != http.NoBody {
				exchange.Request = exchange.Request.WithContext(ctx)
				exchange.Request.Body = http.MaxBytesReader(unwrapResponseWriter(exchange.Writer), exchange.Request.Body, maxBytes)
			}

			result, err := next(ctx, exchange)
			if isBodyTooLarge(err) {
				return result, middlewarex.PayloadTooLarge(err)
			}
			return result, err
		}
	}
}

func unwrapResponseWriter(w http.ResponseWriter) http.ResponseWriter {
	for w != nil {
		unwrapper, ok := w.(interface{ Unwrap() http.ResponseWriter })
		if !ok {
			return w
		}
		unwrapped := unwrapper.Unwrap()
		if unwrapped == nil || unwrapped == w {
			return w
		}
		w = unwrapped
	}
	return nil
}

func isBodyTooLarge(err error) bool {
	var maxBytesErr *http.MaxBytesError
	return errors.As(err, &maxBytesErr) || middlewarex.IsPayloadTooLarge(err)
}
