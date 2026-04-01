package logx

import (
	"context"

	"github.com/dkoshenkov/packages-go/middlewarex"
	"github.com/rs/zerolog"
)

// NewLogContext builds logger with New and stores it in context.
func NewLogContext(ctx context.Context, serviceName string, opts ...Option) (context.Context, error) {
	if ctx == nil {
		return nil, nil
	}

	logger, err := New(serviceName, opts...)
	if err != nil {
		return nil, err
	}

	return logger.WithContext(ctx), nil
}

// WithContext stores logger in context.
func WithContext(ctx context.Context, logger zerolog.Logger) context.Context {
	if ctx == nil {
		return nil
	}

	return logger.WithContext(ctx)
}

// MustNewLogContext builds logger with New and stores it in context or panics on error.
func MustNewLogContext(ctx context.Context, serviceName string, opts ...Option) context.Context {
	result, err := NewLogContext(ctx, serviceName, opts...)
	if err != nil {
		panic(err)
	}

	return result
}

// Ctx returns logger from context, enriched with request metadata when available.
func Ctx(ctx context.Context) zerolog.Logger {
	if ctx == nil {
		return zerolog.Logger{}
	}

	logger := zerolog.Ctx(ctx)
	if logger == nil {
		return zerolog.Logger{}
	}
	base := *logger

	if requestID, ok := middlewarex.RequestIDFromContext(ctx); ok && requestID != "" {
		base = base.With().Str("request_id", requestID).Logger()
	}
	if identity, ok := middlewarex.IdentityFromContext(ctx); ok && identity.Subject != "" {
		base = base.With().Str("subject", identity.Subject).Logger()
	}

	return base
}

// FromContext returns logger from context.
func FromContext(ctx context.Context) zerolog.Logger {
	return Ctx(ctx)
}

// Log writes event using the logger stored in context.
func Log(ctx context.Context, level zerolog.Level) *zerolog.Event {
	logger := FromContext(ctx)
	return logger.WithLevel(level)
}

// LogMsg writes a message using the logger stored in context.
func LogMsg(ctx context.Context, level zerolog.Level, msg string) {
	Log(ctx, level).Msg(msg)
}

// Trace writes a trace-level event using the logger stored in context.
func Trace(ctx context.Context) *zerolog.Event {
	logger := FromContext(ctx)
	return logger.Trace()
}

// TraceMsg writes a trace-level message using the logger stored in context.
func TraceMsg(ctx context.Context, msg string) {
	LogMsg(ctx, zerolog.TraceLevel, msg)
}

// Debug writes a debug-level event using the logger stored in context.
func Debug(ctx context.Context) *zerolog.Event {
	logger := FromContext(ctx)
	return logger.Debug()
}

// DebugMsg writes a debug-level message using the logger stored in context.
func DebugMsg(ctx context.Context, msg string) {
	LogMsg(ctx, zerolog.DebugLevel, msg)
}

// Info writes an info-level event using the logger stored in context.
func Info(ctx context.Context) *zerolog.Event {
	logger := FromContext(ctx)
	return logger.Info()
}

// InfoMsg writes an info-level message using the logger stored in context.
func InfoMsg(ctx context.Context, msg string) {
	LogMsg(ctx, zerolog.InfoLevel, msg)
}

// Warn writes a warn-level event using the logger stored in context.
func Warn(ctx context.Context) *zerolog.Event {
	logger := FromContext(ctx)
	return logger.Warn()
}

// WarnMsg writes a warn-level message using the logger stored in context.
func WarnMsg(ctx context.Context, msg string) {
	LogMsg(ctx, zerolog.WarnLevel, msg)
}

// Error writes an error-level event using the logger stored in context.
func Error(ctx context.Context) *zerolog.Event {
	logger := FromContext(ctx)
	return logger.Error()
}

// ErrorMsg writes an error-level message using the logger stored in context.
func ErrorMsg(ctx context.Context, msg string) {
	LogMsg(ctx, zerolog.ErrorLevel, msg)
}

// Fatal writes a fatal-level event using the logger stored in context.
func Fatal(ctx context.Context) *zerolog.Event {
	logger := FromContext(ctx)
	return logger.Fatal()
}

// FatalMsg writes a fatal-level message using the logger stored in context.
func FatalMsg(ctx context.Context, msg string) {
	LogMsg(ctx, zerolog.FatalLevel, msg)
}

// Panic writes a panic-level event using the logger stored in context.
func Panic(ctx context.Context) *zerolog.Event {
	logger := FromContext(ctx)
	return logger.Panic()
}

// PanicMsg writes a panic-level message using the logger stored in context.
func PanicMsg(ctx context.Context, msg string) {
	LogMsg(ctx, zerolog.PanicLevel, msg)
}
