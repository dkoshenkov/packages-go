package httpx

import (
	"context"

	"github.com/dkoshenkov/packages-go/middlewarex"
)

// Logging logs HTTP request lifecycle using generic logger.
func Logging(logger middlewarex.Logger) Middleware {
	return middlewarex.Logging[Exchange, struct{}](logger,
		middlewarex.WithLogName[Exchange, struct{}]("http"),
		middlewarex.WithLogMessages[Exchange, struct{}]("request started", "request finished"),
		middlewarex.WithLogFields[Exchange, struct{}](func(ctx context.Context, exchange Exchange, _ struct{}, err error) map[string]any {
			fields := map[string]any{
				"method": exchange.Request.Method,
				"path":   exchange.Request.URL.Path,
			}
			if err != nil {
				fields["status"] = DefaultStatusMapper(err)
			}
			return fields
		}),
	)
}

// LoggingContext logs HTTP request lifecycle using the logger stored in context.
func LoggingContext() Middleware {
	return middlewarex.LoggingContext[Exchange, struct{}](
		middlewarex.WithLogName[Exchange, struct{}]("http"),
		middlewarex.WithLogMessages[Exchange, struct{}]("request started", "request finished"),
		middlewarex.WithLogFields[Exchange, struct{}](func(_ context.Context, exchange Exchange, _ struct{}, err error) map[string]any {
			fields := map[string]any{
				"method": exchange.Request.Method,
				"path":   exchange.Request.URL.Path,
			}
			if err != nil {
				fields["status"] = DefaultStatusMapper(err)
			}
			return fields
		}),
	)
}
