package logx

import (
	"io"
	"strings"

	"github.com/rs/zerolog"
)

// WithWriter sets log output writer.
func WithWriter(writer io.Writer) Option {
	return optionFunc(func(cfg *config) {
		cfg.writer = writer
	})
}

// WithServiceName sets service name field value.
func WithServiceName(serviceName string) Option {
	return optionFunc(func(cfg *config) {
		cfg.serviceName = serviceName
	})
}

// WithServiceFieldName sets field name used for service value.
func WithServiceFieldName(fieldName string) Option {
	return optionFunc(func(cfg *config) {
		cfg.serviceFieldName = strings.TrimSpace(fieldName)
	})
}

// WithLevel sets minimum logger level.
func WithLevel(level zerolog.Level) Option {
	return optionFunc(func(cfg *config) {
		cfg.level = level
		cfg.levelText = ""
	})
}

// WithLevelText sets minimum logger level using text (trace, debug, info, warn, error, fatal, panic).
func WithLevelText(level string) Option {
	return optionFunc(func(cfg *config) {
		cfg.levelText = strings.TrimSpace(level)
	})
}

// WithoutTimestamp disables time field.
func WithoutTimestamp() Option {
	return optionFunc(func(cfg *config) {
		cfg.timestamp = false
	})
}

// WithoutCaller disables caller field.
func WithoutCaller() Option {
	return optionFunc(func(cfg *config) {
		cfg.caller = false
		cfg.callerSkipFrameCount = 0
	})
}

// WithCallerSkipFrameCount configures caller frame skip.
func WithCallerSkipFrameCount(skip int) Option {
	return optionFunc(func(cfg *config) {
		cfg.caller = true
		cfg.callerSkipFrameCount = skip
	})
}

// WithPretty enables human-readable console output.
func WithPretty() Option {
	return optionFunc(func(cfg *config) {
		cfg.pretty = true
	})
}

// WithTimeFormat sets timestamp format for pretty output.
func WithTimeFormat(format string) Option {
	return optionFunc(func(cfg *config) {
		cfg.timeFormat = format
	})
}

// WithField appends a single static field to each log entry.
func WithField(key string, value any) Option {
	return optionFunc(func(cfg *config) {
		key = strings.TrimSpace(key)
		if key == "" {
			return
		}

		if cfg.fields == nil {
			cfg.fields = make(map[string]any)
		}
		cfg.fields[key] = value
	})
}

// WithFields appends static fields to each log entry.
func WithFields(fields map[string]any) Option {
	return optionFunc(func(cfg *config) {
		if len(fields) == 0 {
			return
		}

		if cfg.fields == nil {
			cfg.fields = make(map[string]any, len(fields))
		}

		for key, value := range fields {
			key = strings.TrimSpace(key)
			if key == "" {
				continue
			}
			cfg.fields[key] = value
		}
	})
}
