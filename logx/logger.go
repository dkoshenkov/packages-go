package logx

import (
	"fmt"
	"io"
	"sort"

	"github.com/rs/zerolog"
)

// New builds zerolog logger with sensible defaults for service logs.
func New(serviceName string, opts ...Option) (zerolog.Logger, error) {
	cfg := defaultConfig(serviceName)

	for _, opt := range opts {
		if opt == nil {
			continue
		}
		opt.apply(&cfg)
	}

	if cfg.writer == nil {
		return zerolog.Logger{}, errWriterIsNil
	}
	if cfg.callerSkipFrameCount < 0 {
		return zerolog.Logger{}, errCallerSkipFrameCountNegative
	}
	if cfg.serviceFieldName == "" {
		return zerolog.Logger{}, errServiceFieldNameMustNotBeEmpty
	}

	if cfg.levelText != "" {
		level, err := zerolog.ParseLevel(cfg.levelText)
		if err != nil {
			return zerolog.Logger{}, fmt.Errorf("logx: parse level %q: %w", cfg.levelText, err)
		}
		cfg.level = level
	}

	var writer io.Writer = cfg.writer
	if cfg.pretty {
		writer = zerolog.ConsoleWriter{
			Out:        cfg.writer,
			TimeFormat: cfg.timeFormat,
		}
	}

	logger := zerolog.New(writer).Level(cfg.level)
	context := logger.With()

	if cfg.timestamp {
		context = context.Timestamp()
	}

	if cfg.caller {
		if cfg.callerSkipFrameCount > 0 {
			context = context.CallerWithSkipFrameCount(cfg.callerSkipFrameCount)
		} else {
			context = context.Caller()
		}
	}

	if cfg.serviceName != "" {
		context = context.Str(cfg.serviceFieldName, cfg.serviceName)
	}

	if len(cfg.fields) > 0 {
		keys := make([]string, 0, len(cfg.fields))
		for key := range cfg.fields {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			context = context.Interface(key, cfg.fields[key])
		}
	}

	return context.Logger(), nil
}

// MustNew builds logger or panics when config is invalid.
func MustNew(serviceName string, opts ...Option) zerolog.Logger {
	logger, err := New(serviceName, opts...)
	if err != nil {
		panic(err)
	}

	return logger
}
