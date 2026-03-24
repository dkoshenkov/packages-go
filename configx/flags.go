package configx

import (
	"errors"
	"fmt"
	"reflect"
	"strings"

	"github.com/spf13/pflag"
)

// BindGlobalFlags registers cfgx-derived flags in pflag.CommandLine.
func BindGlobalFlags(target any) error {
	return BindFlags(pflag.CommandLine, target)
}

// BindFlags registers flags for all exported cfgx-tagged fields.
//
// Name generation uses canonical env key converted to kebab-case:
// SERVER_I -> server-i.
// bool fields are registered as bool flags, all other types as string flags.
func BindFlags(flagSet *pflag.FlagSet, target any) error {
	if flagSet == nil {
		return errFlagSetIsNil
	}

	targetValue, targetType, err := validateTarget(target)
	if err != nil {
		return err
	}

	fields, fieldErrs := collectFields(targetValue, targetType, nil, nil)
	if len(fieldErrs) > 0 {
		return joinErrors(fieldErrs)
	}

	var errs []error
	for _, field := range fields {
		flagName := buildFlagName(field.envKey)
		if flagName == "" {
			errs = append(errs, fmt.Errorf("%s: generated empty flag name", field.path))
			continue
		}

		if flagSet.Lookup(flagName) != nil {
			errs = append(errs, fmt.Errorf("%s: flag %q is already registered", field.path, flagName))
			continue
		}

		if field.typ.Kind() == reflect.Bool {
			defaultValue, defaultErr := boolDefault(field)
			if defaultErr != nil {
				errs = append(errs, defaultErr)
				continue
			}
			flagSet.Bool(flagName, defaultValue, "")
			continue
		}

		flagSet.String(flagName, field.defaultRaw, "")
	}

	return joinErrors(errs)
}

func boolDefault(field fieldSpec) (bool, error) {
	if !field.hasDefault {
		return false, nil
	}

	decoded, err := decodeYAMLBytes([]byte(field.defaultRaw), field.typ)
	if err != nil {
		return false, fmt.Errorf("%s: decode default for bool flag: %w", field.path, err)
	}

	return decoded.Bool(), nil
}

func buildFlagName(envKey string) string {
	parts := strings.Split(envKey, envPartsSeparator)
	for i := range parts {
		parts[i] = strings.ToLower(strings.TrimSpace(parts[i]))
	}

	return strings.Join(parts, flagPartsSeparator)
}

func joinErrors(errs []error) error {
	if len(errs) == 0 {
		return nil
	}

	return errors.Join(errs...)
}
