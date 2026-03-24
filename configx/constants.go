package configx

const (
	profileProd = "prod"
	profileDev  = "dev"
)

const (
	envProfileKey  = "ENV"
	yamlProfileKey = "env"
	flagProfileKey = "env"
	envPrefixProd  = "PROD_"
	envPrefixDev   = "DEV_"
	yamlPrefixProd = "prod."
	yamlPrefixDev  = "dev."
)

const (
	sourceFlag  = "flag"
	sourceVault = "vault"
	sourceEnv   = "env"
	sourceYAML  = "yaml"
)

const (
	tagCfgx = "cfgx"
	tagEnv  = "env"
	tagYAML = "yaml"
)

const (
	cfgxSkipValue      = "-"
	cfgxOptionOptional = "optional"
	cfgxOptionRequired = "required"
	cfgxOptionDefault  = "default="
)

const (
	envPartsSeparator  = "_"
	yamlPartsSeparator = "."
	flagPartsSeparator = "-"
)

const (
	seedTargetVault = "vault"
	seedTargetYAML  = "yaml"
	seedTargetENV   = "env"
)

const (
	flagSeedDefaults = "cfgx-seed-defaults"
	flagSeedTargets  = "cfgx-seed-targets"
	flagSeedForce    = "cfgx-seed-force"
	flagSeedYAMLFile = "cfgx-seed-yaml-file"
	flagSeedENVFile  = "cfgx-seed-env-file"
	flagSeedOnly     = "cfgx-seed-only"
	flagVaultAddress = "cfgx-vault-address"
	flagVaultToken   = "cfgx-vault-token"
	flagVaultNS      = "cfgx-vault-namespace"
	flagVaultPath    = "cfgx-vault-path"
)

const (
	defaultSeedYAMLFile = "config.yaml"
	defaultSeedENVFile  = ".env"
)
