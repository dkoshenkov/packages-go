package configx

import (
	"context"
	"errors"
	"fmt"
	"os"
	"reflect"
	"strings"

	"github.com/spf13/pflag"
	"github.com/spf13/viper"
)

type runtime struct {
	cfg               config
	flags             map[string]string
	yaml              *viper.Viper
	profile           string
	profileEnvPrefix  string
	profileYAMLPrefix string
}

type resolvedValue struct {
	source string
	key    string
	raw    *string
	any    any
}

// Load fills target using source priority: flag > vault > env > yaml.
func Load(ctx context.Context, target any, opts ...Option) error {
	cfg := config{
		resolveMode: StrictGroup,
	}
	for _, opt := range opts {
		if opt == nil {
			continue
		}
		opt.apply(&cfg)
	}

	targetValue, targetType, err := validateTarget(target)
	if err != nil {
		return err
	}

	rt := runtime{
		cfg: cfg,
	}

	var errs []error

	if parseErr := maybeParseFlags(target, &rt.cfg); parseErr != nil {
		errs = append(errs, parseErr)
	}
	rt.flags = buildFlagMap(rt.cfg.flagSet)

	if cfg.yamlFile != "" {
		parsedYAML, yamlErr := loadYAMLFile(cfg.yamlFile)
		if yamlErr != nil {
			errs = append(errs, fmt.Errorf("yaml: %w", yamlErr))
		} else {
			rt.yaml = parsedYAML
		}
	}

	profile, profileErr := resolveProfile(ctx, &rt)
	if profileErr != nil {
		errs = append(errs, profileErr)
	} else {
		rt.profile = profile
		rt.profileEnvPrefix, rt.profileYAMLPrefix = profilePrefixes(profile)
	}

	fields, collectErrs := collectFields(targetValue, targetType, nil, nil)
	errs = append(errs, collectErrs...)

	if len(fields) == 0 {
		if len(errs) == 0 {
			return nil
		}
		return errors.Join(errs...)
	}

	if profileErr != nil {
		return errors.Join(errs...)
	}

	profileGroupIsActive := false
	if cfg.resolveMode == StrictGroup {
		found, findErr := hasGroupValues(ctx, &rt, fields, true)
		if findErr != nil {
			errs = append(errs, findErr)
		} else {
			profileGroupIsActive = found
		}
	}

	for _, field := range fields {
		value, found, resolveErr := resolveField(ctx, &rt, field, profileGroupIsActive)
		if resolveErr != nil {
			errs = append(errs, resolveErr)
			continue
		}

		if found {
			if assignErr := assignResolvedValue(field, value); assignErr != nil {
				errs = append(errs, assignErr)
			}
			continue
		}

		if shouldUseDefault(cfg.resolveMode, field) {
			if assignDefaultErr := assignDefault(field); assignDefaultErr != nil {
				errs = append(errs, assignDefaultErr)
			}
			continue
		}

		if field.required && !cfg.allowMissing {
			errs = append(errs, fmt.Errorf("%s: required value is missing", field.path))
		}
	}

	if len(errs) == 0 {
		return nil
	}

	return errors.Join(errs...)
}

// MustLoad panics when Load returns an error.
func MustLoad(ctx context.Context, target any, opts ...Option) {
	if err := Load(ctx, target, opts...); err != nil {
		panic(err)
	}
}

func validateTarget(target any) (reflect.Value, reflect.Type, error) {
	value := reflect.ValueOf(target)
	if !value.IsValid() || value.Kind() != reflect.Ptr || value.IsNil() {
		return reflect.Value{}, nil, errTargetMustBePointerToStruct
	}
	if value.Elem().Kind() != reflect.Struct {
		return reflect.Value{}, nil, errTargetMustPointToStruct
	}

	return value.Elem(), value.Elem().Type(), nil
}

func shouldUseDefault(mode ResolveMode, field fieldSpec) bool {
	switch mode {
	case StrictGroup, OverlayDefaultHigh, OverlayDefaultLow:
		return field.hasDefault
	default:
		return false
	}
}

func loadYAMLFile(path string) (*viper.Viper, error) {
	v := viper.New()
	v.SetConfigFile(path)
	if err := v.ReadInConfig(); err != nil {
		return nil, err
	}

	return v, nil
}

func maybeParseFlags(target any, cfg *config) error {
	if !cfg.parseFlags {
		return nil
	}

	flagSet := cfg.flagSet
	if flagSet == nil {
		flagSet = pflag.CommandLine
		cfg.flagSet = flagSet
	}

	if err := BindFlags(flagSet, target); err != nil {
		return fmt.Errorf("bind flags: %w", err)
	}

	args := cfg.parseArgs
	if len(args) == 0 {
		args = os.Args[1:]
	}

	if flagSet.Parsed() {
		if len(args) == 0 {
			return nil
		}
		return errFlagSetAlreadyParsed
	}

	if err := flagSet.Parse(args); err != nil {
		return fmt.Errorf("parse flags: %w", err)
	}

	return nil
}

func resolveProfile(ctx context.Context, rt *runtime) (string, error) {
	if rt.cfg.profile != "" {
		return normalizeProfile(rt.cfg.profile)
	}

	if value, ok := rt.flags[normalizeKey(flagProfileKey)]; ok {
		return normalizeProfile(value)
	}

	if rt.cfg.vault != nil {
		value, ok, err := rt.cfg.vault.Get(ctx, envProfileKey)
		if err != nil {
			return "", fmt.Errorf("profile: vault lookup %s: %w", envProfileKey, err)
		}
		if ok {
			return normalizeProfile(value)
		}
	}

	if value, ok := os.LookupEnv(envProfileKey); ok {
		return normalizeProfile(value)
	}

	if rt.yaml != nil && rt.yaml.IsSet(yamlProfileKey) {
		return normalizeProfile(fmt.Sprint(rt.yaml.Get(yamlProfileKey)))
	}

	return "", errProfileIsNotSet
}

func normalizeProfile(profile string) (string, error) {
	value := strings.ToLower(strings.TrimSpace(profile))
	switch value {
	case profileProd, profileDev:
		return value, nil
	default:
		return "", fmt.Errorf("invalid profile %q: expected prod or dev", profile)
	}
}

func profilePrefixes(profile string) (string, string) {
	switch profile {
	case profileProd:
		return envPrefixProd, yamlPrefixProd
	case profileDev:
		return envPrefixDev, yamlPrefixDev
	default:
		return "", ""
	}
}

func buildFlagMap(flagSet *pflag.FlagSet) map[string]string {
	flags := make(map[string]string)
	if flagSet == nil {
		return flags
	}

	flagSet.Visit(func(flag *pflag.Flag) {
		flags[normalizeKey(flag.Name)] = flag.Value.String()
	})

	return flags
}

func hasGroupValues(ctx context.Context, rt *runtime, fields []fieldSpec, profileGroup bool) (bool, error) {
	for _, field := range fields {
		_, found, err := lookupGroupValue(ctx, rt, field, profileGroup)
		if err != nil {
			return false, err
		}
		if found {
			return true, nil
		}
	}

	return false, nil
}

func resolveField(ctx context.Context, rt *runtime, field fieldSpec, profileGroupIsActive bool) (resolvedValue, bool, error) {
	switch rt.cfg.resolveMode {
	case StrictGroup:
		return lookupGroupValue(ctx, rt, field, profileGroupIsActive)
	case OverlayDefaultHigh:
		value, found, err := lookupGroupValue(ctx, rt, field, true)
		if err != nil || found {
			return value, found, err
		}
		if field.hasDefault {
			return resolvedValue{}, false, nil
		}
		return lookupGroupValue(ctx, rt, field, false)
	case OverlayDefaultLow:
		value, found, err := lookupGroupValue(ctx, rt, field, true)
		if err != nil || found {
			return value, found, err
		}
		value, found, err = lookupGroupValue(ctx, rt, field, false)
		if err != nil || found {
			return value, found, err
		}
		return resolvedValue{}, false, nil
	default:
		return resolvedValue{}, false, fmt.Errorf("%s: unsupported resolve mode %d", field.path, rt.cfg.resolveMode)
	}
}

func lookupGroupValue(ctx context.Context, rt *runtime, field fieldSpec, profileGroup bool) (resolvedValue, bool, error) {
	envKey := field.envKey
	yamlKey := field.yamlKey
	if profileGroup {
		envKey = rt.profileEnvPrefix + envKey
		yamlKey = rt.profileYAMLPrefix + yamlKey
	}

	if value, ok := rt.flags[normalizeKey(envKey)]; ok {
		return resolvedValue{
			source: sourceFlag,
			key:    envKey,
			raw:    &value,
		}, true, nil
	}

	if rt.cfg.vault != nil {
		value, ok, err := rt.cfg.vault.Get(ctx, envKey)
		if err != nil {
			return resolvedValue{}, false, fmt.Errorf("%s: vault lookup %q: %w", field.path, envKey, err)
		}
		if ok {
			return resolvedValue{
				source: sourceVault,
				key:    envKey,
				raw:    &value,
			}, true, nil
		}
	}

	if value, ok := os.LookupEnv(envKey); ok {
		return resolvedValue{
			source: sourceEnv,
			key:    envKey,
			raw:    &value,
		}, true, nil
	}

	if rt.yaml != nil && rt.yaml.IsSet(yamlKey) {
		return resolvedValue{
			source: sourceYAML,
			key:    yamlKey,
			any:    rt.yaml.Get(yamlKey),
		}, true, nil
	}

	return resolvedValue{}, false, nil
}
