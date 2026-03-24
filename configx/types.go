package configx

import (
	"context"

	"github.com/spf13/pflag"
)

// ResolveMode defines how profile-group, no-prefix values and defaults are merged.
type ResolveMode uint8

const (
	// StrictGroup chooses one active group for the whole config.
	StrictGroup ResolveMode = iota
	// OverlayDefaultHigh resolves as profile-group -> default -> no-prefix.
	OverlayDefaultHigh
	// OverlayDefaultLow resolves as profile-group -> no-prefix -> default.
	OverlayDefaultLow
)

// VaultReader provides config values by key.
type VaultReader interface {
	Get(ctx context.Context, key string) (value string, ok bool, err error)
}

// Option customizes config loading behavior.
type Option interface {
	apply(*config)
}

type optionFunc func(*config)

func (f optionFunc) apply(cfg *config) {
	f(cfg)
}

type config struct {
	flagSet      *pflag.FlagSet
	vault        VaultReader
	yamlFile     string
	profile      string
	resolveMode  ResolveMode
	allowMissing bool
	parseFlags   bool
	parseArgs    []string
}
