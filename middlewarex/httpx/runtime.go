package httpx

import (
	"context"
	"os"
	"strings"
	"time"

	"github.com/dkoshenkov/packages-go/configx"
	"github.com/dkoshenkov/packages-go/logx"
	"github.com/dkoshenkov/packages-go/middlewarex"
	"github.com/rs/zerolog"
)

const (
	defaultRequestIDHeader = "X-Request-ID"
	defaultTimeout         = 30 * time.Second
)

// Config configures default HTTP runtime.
type Config struct {
	RequestIDHeader string        `cfgx:"request_id_header,default=X-Request-ID" env:"HTTP_REQUEST_ID_HEADER"`
	Timeout         time.Duration `cfgx:"timeout,default=30s" env:"HTTP_TIMEOUT"`
	LogRequests     bool          `cfgx:"log_requests,default=true" env:"HTTP_LOG_REQUESTS"`
	PrettyLogs      bool          `cfgx:"pretty_logs,default=false" env:"HTTP_PRETTY_LOGS"`
}

// Runtime carries shared HTTP transport defaults.
type Runtime struct {
	Logger           middlewarex.Logger
	StatusMapper     StatusMapper
	ErrorEncoder     ErrorEncoder
	contextLogger    zerolog.Logger
	contextLoggerSet bool

	timeout           time.Duration
	requestIDHeader   string
	logRequests       bool
	logRequestsWasSet bool
}

type runtimeOptionFunc func(*runtimeConfig)

func (f runtimeOptionFunc) applyRuntime(cfg *runtimeConfig) {
	f(cfg)
}

// WithLogger sets generic runtime logger.
func WithLogger(logger middlewarex.Logger) RuntimeOption {
	return runtimeOptionFunc(func(cfg *runtimeConfig) {
		cfg.logger = logger
	})
}

// WithContextLogger sets zerolog logger stored in request context.
func WithContextLogger(logger zerolog.Logger) RuntimeOption {
	return runtimeOptionFunc(func(cfg *runtimeConfig) {
		cfg.contextLogger = logger
		cfg.contextLoggerSet = true
	})
}

// WithZerolog stores zerolog logger in request context.
func WithZerolog(logger zerolog.Logger) RuntimeOption {
	return runtimeOptionFunc(func(cfg *runtimeConfig) {
		cfg.contextLogger = logger
		cfg.contextLoggerSet = true
	})
}

// NewRuntime builds runtime with sensible defaults.
func NewRuntime(opts ...RuntimeOption) Runtime {
	cfg := runtimeConfig{
		statusMapper:    StatusMapperFunc(DefaultStatusMapper),
		timeout:         defaultTimeout,
		requestIDHeader: defaultRequestIDHeader,
		logRequests:     true,
	}
	for _, opt := range opts {
		if opt == nil {
			continue
		}
		opt.applyRuntime(&cfg)
	}
	if cfg.statusMapper == nil {
		cfg.statusMapper = StatusMapperFunc(DefaultStatusMapper)
	}
	if cfg.errorEncoder == nil {
		cfg.errorEncoder = DefaultErrorEncoder(cfg.statusMapper)
	}
	if strings.TrimSpace(cfg.requestIDHeader) == "" {
		cfg.requestIDHeader = defaultRequestIDHeader
	}
	if cfg.timeout == 0 {
		cfg.timeout = defaultTimeout
	}

	return Runtime{
		Logger:            cfg.logger,
		StatusMapper:      cfg.statusMapper,
		ErrorEncoder:      cfg.errorEncoder,
		contextLogger:     cfg.contextLogger,
		contextLoggerSet:  cfg.contextLoggerSet,
		timeout:           cfg.timeout,
		requestIDHeader:   cfg.requestIDHeader,
		logRequests:       cfg.logRequests,
		logRequestsWasSet: true,
	}
}

// DefaultRuntime builds runtime with default zerolog-backed logger.
func DefaultRuntime(service string, cfg Config) (Runtime, error) {
	cfg = normalizeConfig(cfg)

	var opts []logx.Option
	if cfg.PrettyLogs {
		opts = append(opts, logx.WithPretty())
	}

	logger, err := logx.New(service, opts...)
	if err != nil {
		return Runtime{}, err
	}

	rt := NewRuntime(
		WithZerolog(logger),
		WithStatusMapper(StatusMapperFunc(DefaultStatusMapper)),
	)
	rt.timeout = cfg.Timeout
	rt.requestIDHeader = cfg.RequestIDHeader
	rt.logRequests = cfg.LogRequests
	rt.logRequestsWasSet = true
	rt.ErrorEncoder = DefaultErrorEncoder(rt.StatusMapper)

	return rt, nil
}

// LoadDefaultRuntime loads HTTP runtime config via configx and builds runtime defaults.
func LoadDefaultRuntime(ctx context.Context, service string, target *Config, opts ...configx.Option) (Runtime, error) {
	if target == nil {
		target = &Config{}
	}
	loadOpts := make([]configx.Option, 0, len(opts)+1)
	loadOpts = append(loadOpts, configx.WithProfile(defaultConfigProfile()))
	loadOpts = append(loadOpts, opts...)
	if err := configx.Load(ctx, target, loadOpts...); err != nil {
		return Runtime{}, err
	}

	return DefaultRuntime(service, *target)
}

func normalizeConfig(cfg Config) Config {
	if cfg == (Config{}) {
		return Config{
			RequestIDHeader: defaultRequestIDHeader,
			Timeout:         defaultTimeout,
			LogRequests:     true,
		}
	}
	if strings.TrimSpace(cfg.RequestIDHeader) == "" {
		cfg.RequestIDHeader = defaultRequestIDHeader
	}
	if cfg.Timeout == 0 {
		cfg.Timeout = defaultTimeout
	}
	return cfg
}

func zerologBridge(logger zerolog.Logger) middlewarex.Logger {
	return middlewarex.LoggerFunc(func(_ context.Context, event middlewarex.Event) {
		entry := logger.WithLevel(parseLevel(event.Level)).
			Str("name", event.Name).
			Dur("duration", event.Duration)
		if event.RequestID != "" {
			entry = entry.Str("request_id", event.RequestID)
		}
		if event.Subject != "" {
			entry = entry.Str("subject", event.Subject)
		}
		for key, value := range event.Fields {
			entry = entry.Interface(key, value)
		}
		if event.Err != nil {
			entry = entry.Err(event.Err)
		}
		entry.Msg(event.Message)
	})
}

func parseLevel(level string) zerolog.Level {
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

func defaultConfigProfile() string {
	profile := strings.TrimSpace(os.Getenv("ENV"))
	if profile == "" {
		return "dev"
	}
	return profile
}
