package configx

import "github.com/spf13/pflag"

// WithFlagSet sets pflag source; only Changed flags are used.
func WithFlagSet(flagSet *pflag.FlagSet) Option {
	return optionFunc(func(cfg *config) {
		cfg.flagSet = flagSet
	})
}

// WithVault sets Vault source.
func WithVault(vault VaultReader) Option {
	return optionFunc(func(cfg *config) {
		cfg.vault = vault
	})
}

// WithYAMLFile sets YAML file source path.
func WithYAMLFile(path string) Option {
	return optionFunc(func(cfg *config) {
		cfg.yamlFile = path
	})
}

// WithProfile sets explicit profile ("prod" or "dev").
func WithProfile(profile string) Option {
	return optionFunc(func(cfg *config) {
		cfg.profile = profile
	})
}

// WithResolveMode sets merge strategy.
func WithResolveMode(mode ResolveMode) Option {
	return optionFunc(func(cfg *config) {
		cfg.resolveMode = mode
	})
}

// WithAllowMissing disables required-field errors globally.
func WithAllowMissing() Option {
	return optionFunc(func(cfg *config) {
		cfg.allowMissing = true
	})
}

// ParseFlags enables automatic BindFlags+Parse inside Load.
//
// If args are provided, Parse uses them.
// If args are omitted, Parse uses os.Args[1:].
// If WithFlagSet is not set, pflag.CommandLine is used.
func ParseFlags(args ...string) Option {
	return optionFunc(func(cfg *config) {
		cfg.parseFlags = true
		cfg.parseArgs = append([]string(nil), args...)
	})
}
