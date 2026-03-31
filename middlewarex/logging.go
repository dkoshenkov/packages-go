package middlewarex

import (
	"context"
	"time"
)

// Logger receives middleware events.
type Logger interface {
	Log(ctx context.Context, event Event)
}

// LoggerFunc adapts function to Logger.
type LoggerFunc func(ctx context.Context, event Event)

// Log calls f(ctx, event).
func (f LoggerFunc) Log(ctx context.Context, event Event) {
	if f == nil {
		return
	}

	f(ctx, event)
}

// Event describes middleware log event.
type Event struct {
	Level     string
	Name      string
	Message   string
	Duration  time.Duration
	Err       error
	RequestID string
	Subject   string
	Fields    map[string]any
}

// LogFieldsFunc returns event fields for request execution.
type LogFieldsFunc[Req, Resp any] func(ctx context.Context, req Req, resp Resp, err error) map[string]any

type LoggingOption[Req, Resp any] interface {
	apply(*loggingConfig[Req, Resp])
}

type loggingOptionFunc[Req, Resp any] func(*loggingConfig[Req, Resp])

func (f loggingOptionFunc[Req, Resp]) apply(cfg *loggingConfig[Req, Resp]) {
	f(cfg)
}

type loggingConfig[Req, Resp any] struct {
	name       string
	startMsg   string
	finishMsg  string
	fieldsFunc LogFieldsFunc[Req, Resp]
}

// WithLogName sets event name.
func WithLogName[Req, Resp any](name string) LoggingOption[Req, Resp] {
	return loggingOptionFunc[Req, Resp](func(cfg *loggingConfig[Req, Resp]) {
		cfg.name = name
	})
}

// WithLogMessages sets start and finish messages.
func WithLogMessages[Req, Resp any](start, finish string) LoggingOption[Req, Resp] {
	return loggingOptionFunc[Req, Resp](func(cfg *loggingConfig[Req, Resp]) {
		cfg.startMsg = start
		cfg.finishMsg = finish
	})
}

// WithLogFields sets additional field extractor.
func WithLogFields[Req, Resp any](fieldsFunc LogFieldsFunc[Req, Resp]) LoggingOption[Req, Resp] {
	return loggingOptionFunc[Req, Resp](func(cfg *loggingConfig[Req, Resp]) {
		cfg.fieldsFunc = fieldsFunc
	})
}

// Logging logs handler lifecycle.
func Logging[Req, Resp any](logger Logger, opts ...LoggingOption[Req, Resp]) Middleware[Req, Resp] {
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
			if logger == nil {
				return *new(Resp), Internal(errLoggerIsNil)
			}

			logger.Log(ctx, buildEvent(ctx, cfg, req, *new(Resp), nil, 0, "info", cfg.startMsg))

			startedAt := time.Now()
			resp, err := next(ctx, req)
			duration := time.Since(startedAt)
			level := "info"
			if err != nil {
				level = "error"
			}
			logger.Log(ctx, buildEvent(ctx, cfg, req, resp, err, duration, level, cfg.finishMsg))
			return resp, err
		}
	}
}

func buildEvent[Req, Resp any](ctx context.Context, cfg loggingConfig[Req, Resp], req Req, resp Resp, err error, duration time.Duration, level string, msg string) Event {
	fields := map[string]any{}
	if cfg.fieldsFunc != nil {
		for key, value := range cfg.fieldsFunc(ctx, req, resp, err) {
			fields[key] = value
		}
	}

	event := Event{
		Level:    level,
		Name:     cfg.name,
		Message:  msg,
		Duration: duration,
		Err:      err,
		Fields:   fields,
	}

	if requestID, ok := RequestIDFromContext(ctx); ok {
		event.RequestID = requestID
	}
	if identity, ok := IdentityFromContext(ctx); ok {
		event.Subject = identity.Subject
	}

	return event
}
