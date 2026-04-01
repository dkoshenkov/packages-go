package middlewarex

import (
	"context"
	"strings"
	"time"

	"github.com/rs/zerolog"
)

// LoggingContext logs handler lifecycle using the logger stored in context.
func LoggingContext[Req, Resp any](opts ...LoggingOption[Req, Resp]) Middleware[Req, Resp] {
	cfg := loggingConfig[Req, Resp]{
		name:      "middleware",
		startMsg:  "started",
		finishMsg: "finished",
	}
	for _, opt := range opts {
		if opt == nil {
			continue
		}
		opt.apply(&cfg)
	}

	return func(next Handler[Req, Resp]) Handler[Req, Resp] {
		return func(ctx context.Context, req Req) (Resp, error) {
			logContextEvent(ctx, buildEvent(ctx, cfg, req, *new(Resp), nil, 0, "info", cfg.startMsg))

			startedAt := time.Now()
			resp, err := next(ctx, req)
			duration := time.Since(startedAt)
			level := "info"
			if err != nil {
				level = "error"
			}
			logContextEvent(ctx, buildEvent(ctx, cfg, req, resp, err, duration, level, cfg.finishMsg))
			return resp, err
		}
	}
}

func logContextEvent(ctx context.Context, event Event) {
	if ctx == nil {
		return
	}

	logger := zerolog.Ctx(ctx)
	if logger == nil {
		return
	}

	entry := logger.WithLevel(parseEventLevel(event.Level)).
		Str("name", event.Name).
		Dur("duration", event.Duration)
	if event.RequestID != "" {
		entry = entry.Str("request_id", event.RequestID)
	} else if requestID, ok := RequestIDFromContext(ctx); ok && requestID != "" {
		entry = entry.Str("request_id", requestID)
	}
	if event.Subject != "" {
		entry = entry.Str("subject", event.Subject)
	} else if identity, ok := IdentityFromContext(ctx); ok && identity.Subject != "" {
		entry = entry.Str("subject", identity.Subject)
	}
	for key, value := range event.Fields {
		entry = entry.Interface(key, value)
	}
	if event.Err != nil {
		entry = entry.Err(event.Err)
	}
	entry.Msg(event.Message)
}

func parseEventLevel(level string) zerolog.Level {
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "trace":
		return zerolog.TraceLevel
	case "debug":
		return zerolog.DebugLevel
	case "warn", "warning":
		return zerolog.WarnLevel
	case "error":
		return zerolog.ErrorLevel
	case "fatal":
		return zerolog.FatalLevel
	case "panic":
		return zerolog.PanicLevel
	default:
		return zerolog.InfoLevel
	}
}
