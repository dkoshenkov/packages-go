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

// WithVaultCredentials configures built-in Vault reader.
func WithVaultCredentials(credentials VaultCredentials) Option {
	return optionFunc(func(cfg *config) {
		copy := credentials
		cfg.vaultCredentials = &copy
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

// SeedDefaults enables writing defaults/placeholders to selected targets.
func SeedDefaults(targets ...string) Option {
	return optionFunc(func(cfg *config) {
		cfg.seedDefaults = true
		ensureSeedTargets(cfg)
		if len(targets) == 0 {
			cfg.seedTargets[seedTargetVault] = struct{}{}
			cfg.seedTargets[seedTargetYAML] = struct{}{}
			cfg.seedTargets[seedTargetENV] = struct{}{}
			return
		}

		for _, target := range targets {
			cfg.seedTargets[target] = struct{}{}
		}
	})
}

// SeedForce enables overwrite for seeding.
func SeedForce() Option {
	return optionFunc(func(cfg *config) {
		cfg.seedForce = true
	})
}

// SeedYAMLFile overrides YAML target file path for seeding.
func SeedYAMLFile(path string) Option {
	return optionFunc(func(cfg *config) {
		cfg.seedYAMLFile = path
	})
}

// SeedENVFile overrides ENV target file path for seeding.
func SeedENVFile(path string) Option {
	return optionFunc(func(cfg *config) {
		cfg.seedENVFile = path
	})
}

// SeedOnly writes defaults/placeholders and skips value resolution.
func SeedOnly() Option {
	return optionFunc(func(cfg *config) {
		cfg.seedOnly = true
		cfg.seedDefaults = true
	})
}

func ensureSeedTargets(cfg *config) {
	if cfg.seedTargets != nil {
		return
	}
	cfg.seedTargets = make(map[string]struct{})
}
