# middlewarex

`middlewarex` - набор generic middleware и HTTP-адаптеров для сервисного кода.

Пакет разделён на два слоя:

- `middlewarex` - transport-agnostic core
- `middlewarex/httpx` - адаптер и HTTP-specific middleware поверх `net/http`

## Core API

Базовый контракт:

```go
type Handler[Req, Resp any] func(ctx context.Context, req Req) (Resp, error)
type Middleware[Req, Resp any] func(Handler[Req, Resp]) Handler[Req, Resp]
func Chain[Req, Resp any](handler Handler[Req, Resp], middleware ...Middleware[Req, Resp]) Handler[Req, Resp]
```

В core доступны:

- `RequireAuth`
- `RequireRoles`
- `RequireScopes`
- `Logging`
- `Recovery`
- `Timeout`
- `WithIdentity` / `IdentityFromContext`
- `WithRequestID` / `RequestIDFromContext`

## Generic Example

```go
package main

import (
	"context"
	"errors"
	"time"

	"github.com/dkoshenkov/packages-go/middlewarex"
)

type request struct {
	Name string
}

type response struct {
	Message string
}

func main() {
	handler := middlewarex.Chain(func(ctx context.Context, req request) (response, error) {
		identity, ok := middlewarex.IdentityFromContext(ctx)
		if !ok {
			return response{}, middlewarex.Unauthorized(errors.New("identity is missing"))
		}

		return response{Message: "hello, " + identity.Subject + ": " + req.Name}, nil
	},
		middlewarex.RequireAuth[request, response](),
		middlewarex.Timeout[request, response](2*time.Second),
	)

	ctx := middlewarex.WithIdentity(context.Background(), middlewarex.Identity{Subject: "user-1"})
	_, _ = handler(ctx, request{Name: "demo"})
}
```

## HTTP Layer

Пакет `middlewarex/httpx` использует `Exchange` как request type:

```go
type Exchange struct {
	Writer  http.ResponseWriter
	Request *http.Request
}
```

HTTP-specific middleware:

- `Auth`
- `RequireHeader`
- `RequireMethod`
- `RequestID`
- `CORS`
- `JSON`
- `Response[T]`
- `Runtime`
- `Adapt`
- `Wrap`
- `WrapFunc`

## HTTP Example

```go
package main

import (
	"context"
	"errors"
	"net/http"

	"github.com/dkoshenkov/packages-go/middlewarex"
	"github.com/dkoshenkov/packages-go/middlewarex/httpx"
)

type verifier struct{}

func (verifier) Verify(_ context.Context, token string) (middlewarex.Identity, error) {
	if token != "secret" {
		return middlewarex.Identity{}, errors.New("invalid token")
	}

	return middlewarex.Identity{
		Subject: "user-1",
		Roles:   []string{"admin"},
		Scopes:  []string{"read"},
	}, nil
}

func main() {
	type request struct {
		Name string `json:"name"`
	}

	type response struct {
		Message string `json:"message"`
	}

	runtime := httpx.NewRuntime()

	handler := httpx.JSON(
		func(ctx context.Context, req request) (httpx.Response[response], error) {
			identity, ok := middlewarex.IdentityFromContext(ctx)
			if !ok {
				return httpx.Response[response]{}, middlewarex.Unauthorized(errors.New("identity is missing"))
			}

			return httpx.OK(response{Message: "hello, " + identity.Subject + ": " + req.Name}), nil
		},
		httpx.WithRuntime[request, response](runtime),
	)

	_ = http.ListenAndServe(":8080", handler)
}
```

Старый `Adapt(...)` остаётся для plain `http.Handler`, а `JSON(...)` закрывает typed request/response, JSON decode/encode и централизованный error handling.
Для runtime-конфига можно использовать `httpx.LoadDefaultRuntime(...)` с `configx`.

## Error Model

`middlewarex` использует классифицированные ошибки.

Конструкторы:

- `Unauthorized(err)`
- `Forbidden(err)`
- `BadRequest(err)`
- `MethodNotAllowed(err)`
- `TimeoutError(err)`
- `Internal(err)`

Проверки:

- `IsUnauthorized(err)`
- `IsForbidden(err)`
- `IsBadRequest(err)`
- `IsMethodNotAllowed(err)`
- `IsTimeout(err)`
- `IsInternal(err)`

Это позволяет безопасно пользоваться `errors.Is`-семантикой без экспортируемых изменяемых sentinel errors.

## Notes

- `Auth` не привязан к JWT/OAuth библиотеке и принимает внешний `Verifier`
- `RequestID` использует `uuid.NewString()`
- `CORS` реализован через `github.com/rs/cors`
- `DefaultStatusMapper` в `httpx` маппит classified errors в HTTP status codes
