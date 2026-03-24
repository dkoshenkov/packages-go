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
