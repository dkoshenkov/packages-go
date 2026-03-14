package flagx

import (
	"fmt"

	"github.com/spf13/pflag"
)

type boolFlagValue struct {
	pflag.Value
}

func (v boolFlagValue) IsBoolFlag() bool {
	return true
}

func Any[T any](flagSet *pflag.FlagSet, name string, target *T, usage string, codec Codec[T], opts ...Option[T]) {
	bind(flagSet, name, "", target, usage, codec, opts...)
}

func AnyP[T any](flagSet *pflag.FlagSet, name string, shorthand string, target *T, usage string, codec Codec[T], opts ...Option[T]) {
	bind(flagSet, name, shorthand, target, usage, codec, opts...)
}

func (v *value[T]) String() string {
	if v == nil || v.target == nil {
		return ""
	}

	if v.format != nil {
		return v.format(*v.target)
	}

	return fmt.Sprint(*v.target)
}

func (v *value[T]) Set(raw string) error {
	if v == nil || v.target == nil {
		return errNilTarget
	}

	if v.parse == nil {
		return errNilParser
	}

	parsed, err := v.parse(raw)
	if err != nil {
		return err
	}

	if err := validateValue(raw, parsed, v.validators); err != nil {
		return err
	}

	*v.target = parsed
	return nil
}

func (v *value[T]) Type() string {
	if v == nil || v.typeName == "" {
		return "value"
	}

	return v.typeName
}

func bind[T any](flagSet *pflag.FlagSet, name string, shorthand string, target *T, usage string, codec Codec[T], opts ...Option[T]) {
	if target == nil {
		panic(errNilTarget)
	}

	cfg := config[T]{
		defaultValue: *target,
		format:       codec.Format,
		typeName:     codec.Type,
	}

	for _, opt := range opts {
		if opt == nil {
			continue
		}
		opt.apply(&cfg)
	}

	if cfg.hasDefault {
		*target = cfg.defaultValue
	}

	if err := validateInitialValue(*target, cfg.format, cfg.validators); err != nil {
		panic(err)
	}

	flagValue := &value[T]{
		target:     target,
		parse:      codec.Parse,
		format:     cfg.format,
		typeName:   cfg.typeName,
		validators: cfg.validators,
	}

	var registeredValue pflag.Value = flagValue
	if codec.IsBool {
		registeredValue = boolFlagValue{Value: flagValue}
	}

	flag := flagSet.VarPF(registeredValue, name, shorthand, usage)
	if codec.IsBool {
		flag.NoOptDefVal = noOptDefVal(codec)
	}
}

func validateValue[T any](raw string, parsed T, validators []Validator[T]) error {
	for _, validator := range validators {
		if validator == nil {
			continue
		}
		if err := validator(parsed); err != nil {
			return fmt.Errorf("invalid value %q: %w", raw, err)
		}
	}

	return nil
}

func validateInitialValue[T any](value T, format Formatter[T], validators []Validator[T]) error {
	rendered := formatValue(value, format)
	for _, validator := range validators {
		if validator == nil {
			continue
		}
		if err := validator(value); err != nil {
			return fmt.Errorf("invalid initial value %q: %w", rendered, err)
		}
	}

	return nil
}

func formatValue[T any](value T, format Formatter[T]) string {
	if format != nil {
		return format(value)
	}

	return fmt.Sprint(value)
}

func noOptDefVal[T any](codec Codec[T]) string {
	if codec.NoOptDefVal != "" {
		return codec.NoOptDefVal
	}

	return "true"
}
