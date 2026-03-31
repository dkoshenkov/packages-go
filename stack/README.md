# stack

`stack` - generic LIFO stack с двумя реализациями:

- `Stack[T]` - обычный стек
- `Sync[T]` - потокобезопасный стек

Обе реализации следуют общему контракту:

```go
type Interface[T any] interface {
	Push(v T)
	Pop() (v T, ok bool)
	Peek() (v T, ok bool)
	Len() int
}
```

`Pop` и `Peek` total-функции:

- не паникуют на пустом стеке
- возвращают zero value и `ok=false`

## Constructors

Обычный стек:

- `New[T]()`
- `NewCap[T](cap int)`

Потокобезопасный стек:

- `NewSync[T]()`
- `NewSyncStackCap[T](cap int)`

Если capacity отрицательная, constructors с capacity возвращают `ErrNegativeCapacity`.

## Example

```go
package main

import (
	"fmt"

	"github.com/dkoshenkov/packages-go/stack"
)

func main() {
	s := stack.New[int]()
	s.Push(10)
	s.Push(20)

	v, ok := s.Pop()
	fmt.Println(v, ok)

	v, ok = s.Pop()
	fmt.Println(v, ok)

	v, ok = s.Pop()
	fmt.Println(v, ok)
}
```

## Notes

- `Stack[T]` подходит для обычного single-threaded использования
- `Sync[T]` использует `sync.RWMutex` и подходит для concurrent access
