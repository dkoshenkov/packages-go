# consterr

`consterr` - минимальный тип строковой ошибки-константы.

Пакет полезен, когда нужна стабильная sentinel error без лишних зависимостей и обвязки.

## API

```go
type Error string
```

`Error` реализует интерфейс `error`:

```go
func (e Error) Error() string
```

## Example

```go
package main

import (
	"errors"
	"fmt"

	"github.com/dkoshenkov/packages-go/consterr"
)

const errInvalidState = consterr.Error("invalid state")

func main() {
	err := errInvalidState
	fmt.Println(err)
	fmt.Println(errors.Is(err, errInvalidState))
}
```
