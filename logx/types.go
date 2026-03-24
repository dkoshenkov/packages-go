package logx

import (
	"io"
	"os"
	"time"

	"github.com/rs/zerolog"
)

// Option customizes logger construction.
type Option interface {
	apply(*config)
}

type optionFunc func(*config)

func (f optionFunc) apply(cfg *config) {
	f(cfg)
}

type config struct {
	writer               io.Writer
	serviceName          string
	serviceFieldName     string
	level                zerolog.Level
	levelText            string
	timestamp            bool
	caller               bool
	callerSkipFrameCount int
	pretty               bool
	timeFormat           string
	fields               map[string]any
}

func defaultConfig(serviceName string) config {
	return config{
		writer:           os.Stdout,
		serviceName:      serviceName,
		serviceFieldName: "service",
		level:            zerolog.InfoLevel,
		timestamp:        true,
		caller:           true,
		timeFormat:       time.RFC3339,
	}
}
