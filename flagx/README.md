# flagx

`flagx` - typed-обертка над `pflag` для bind'инга флагов через generic `Codec[T]`.

Пакет позволяет:

- регистрировать typed flags
- задавать `Default(...)`
- добавлять `Validate(...)`
- ограничивать значения через `OneOf(...)`
- переопределять `Type(...)` и `Format(...)`

## Core API

Основной entrypoint:

```go
func Any[T any](flagSet *pflag.FlagSet, name string, target *T, usage string, codec Codec[T], opts ...Option[T])
func AnyP[T any](flagSet *pflag.FlagSet, name string, shorthand string, target *T, usage string, codec Codec[T], opts ...Option[T])
```

Базовые типы:

```go
type Parser[T any] func(string) (T, error)
type Formatter[T any] func(T) string
type Validator[T any] func(T) error

type Codec[T any] struct {
	Parse  Parser[T]
	Format Formatter[T]
	Type   string
	IsBool bool
	NoOptDefVal string
}
```

Опции:

- `Default(value)`
- `Format(formatter)`
- `Type(name)`
- `Validate(validator)`
- `OneOf(values...)`

## Example

```go
package main

import (
	"fmt"
	"strconv"

	"github.com/spf13/pflag"

	"github.com/dkoshenkov/packages-go/flagx"
)

func main() {
	flags := pflag.NewFlagSet("demo", pflag.ContinueOnError)

	var port int
	flagx.Any(flags, "port", &port, "HTTP port", flagx.Codec[int]{
		Parse:  strconv.Atoi,
		Format: strconv.Itoa,
		Type:   "int",
	},
		flagx.Default(8080),
	)

	_ = flags.Parse([]string{"--port=9090"})
	fmt.Println(port)
}
```

## Notes

- `Any` и `AnyP` паникуют при `nil target`
- валидация применяется и к initial value, и к parsed value
- bool-style flags поддерживаются через `Codec.IsBool`
