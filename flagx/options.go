package flagx

import (
	"fmt"
	"strings"
)

func Default[T any](value T) Option[T] {
	return optionFunc[T](func(cfg *config[T]) {
		cfg.defaultValue = value
		cfg.hasDefault = true
	})
}

func Format[T any](format Formatter[T]) Option[T] {
	return optionFunc[T](func(cfg *config[T]) {
		cfg.format = format
	})
}

func Type[T any](name string) Option[T] {
	return optionFunc[T](func(cfg *config[T]) {
		cfg.typeName = name
	})
}

func Validate[T any](validator Validator[T]) Option[T] {
	return optionFunc[T](func(cfg *config[T]) {
		cfg.validators = append(cfg.validators, validator)
	})
}

func OneOf[T comparable](allowed ...T) Option[T] {
	values := make(map[T]struct{}, len(allowed))
	names := make([]string, 0, len(allowed))
	for _, value := range allowed {
		values[value] = struct{}{}
		names = append(names, fmt.Sprint(value))
	}

	return Validate(func(value T) error {
		if _, ok := values[value]; ok {
			return nil
		}

		return fmt.Errorf("must be one of %s", strings.Join(names, ", "))
	})
}
