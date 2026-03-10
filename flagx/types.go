package flagx

type Parser[T any] func(string) (T, error)

type Formatter[T any] func(T) string

type Validator[T any] func(T) error

type Codec[T any] struct {
	Parse  Parser[T]
	Format Formatter[T]
	Type   string
	IsBool bool
}

type Option[T any] interface {
	apply(*config[T])
}

type optionFunc[T any] func(*config[T])

func (f optionFunc[T]) apply(cfg *config[T]) {
	f(cfg)
}

type config[T any] struct {
	defaultValue T
	hasDefault   bool
	format       Formatter[T]
	typeName     string
	validators   []Validator[T]
}

type value[T any] struct {
	target     *T
	parse      Parser[T]
	format     Formatter[T]
	typeName   string
	validators []Validator[T]
}
